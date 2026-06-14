package main

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"strings"
	"syscall"
	"time"

	"log/slog"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/barnes-c/garmin_exporter/collector"
	"github.com/barnes-c/garmin_exporter/internal/auth"
	"github.com/barnes-c/garmin_exporter/internal/garmin"
	"github.com/barnes-c/garmin_exporter/internal/otel"
	"github.com/barnes-c/garmin_exporter/internal/probes"
	"github.com/barnes-c/garmin_exporter/internal/scrape"
)

var (
	metricsPath = kingpin.Flag(
		"web.telemetry-path",
		"Path under which to expose metrics.",
	).Default("/metrics").String()

	maxProcs = kingpin.Flag(
		"runtime.gomaxprocs",
		"The target number of CPUs Go will run on (GOMAXPROCS).",
	).Envar("GOMAXPROCS").Default("1").Int()

	garminUsername = kingpin.Flag(
		"garmin.username", "Garmin Connect username.",
	).Envar("GARMIN_USERNAME").Required().String()
	garminPassword = kingpin.Flag(
		"garmin.password", "Garmin Connect password.",
	).Envar("GARMIN_PASSWORD").Required().String()
	garminTokenFile = kingpin.Flag(
		"garmin.token-file", "Path to cached OAuth2 token file.",
	).Default("garmin_token.json").String()
	garminLimit = kingpin.Flag(
		"garmin.activity-limit", "Number of recent activities to fetch.",
	).Default("30").Int()

	cacheTTL = kingpin.Flag(
		"cache.ttl",
		"How often to refresh data from Garmin Connect. Controls the Garmin API call rate; independent of Prometheus scrape interval.",
	).Default("1h").Duration()

	webPrometheus = kingpin.Flag(
		"web.prometheus",
		"Serve the Prometheus scrape endpoint at --web.telemetry-path. Disable for OTLP-push-only deployments.",
	).Default("true").Bool()

	toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, ":10045")

	logLevel = kingpin.Flag("log.level", "Log level (debug, info, warn, error).").
			Default("info").String()
)

const otelHelp = `OTel pipeline configuration is environment-driven; see the spec for the
full list of OTEL_* variables:

  https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/

Note: OTEL_{METRICS,TRACES,LOGS}_EXPORTER default to "none" here (the
spec default is "otlp"), so the exporter is silent until OTLP is opted in.`

// buildHandler wires the HTTP routes served by the exporter: the OTel
// Prometheus handler at metricsPath, healthz/readyz probes, and the
// exporter-toolkit landing page at "/" (unless metricsPath itself is "/").
func buildHandler(res *otel.Result, metricsPath string, readyChecks map[string]probes.Checker) (http.Handler, error) {
	mux := http.NewServeMux()
	if res.PromHandler != nil {
		mux.Handle(metricsPath, res.PromHandler)
	}
	mux.Handle("/healthz", probes.Health())
	mux.Handle("/readyz", probes.Ready(readyChecks))

	if metricsPath != "/" {
		links := []web.LandingLinks{}
		if res.PromHandler != nil {
			links = append(links, web.LandingLinks{Address: metricsPath, Text: "Metrics"})
		}
		links = append(links,
			web.LandingLinks{Address: "/healthz", Text: "Health"},
			web.LandingLinks{Address: "/readyz", Text: "Readiness"},
		)
		landing, err := web.NewLandingPage(web.LandingConfig{
			Name:        "Garmin Exporter",
			Description: "OTel-native Prometheus exporter for Garmin Connect",
			Version:     version.Info(),
			Links:       links,
		})
		if err != nil {
			return nil, fmt.Errorf("creating landing page: %w", err)
		}
		mux.Handle("/", landing)
	}
	return mux, nil
}

func main() {
	kingpin.Version(version.Print("garmin_exporter"))
	kingpin.CommandLine.UsageWriter(os.Stdout)
	kingpin.HelpFlag.Short('h')
	kingpin.CommandLine.Help = otelHelp
	kingpin.Parse()

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	runtime.GOMAXPROCS(*maxProcs)

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	otelResult, err := otel.Setup(rootCtx, logger, otel.Config{
		ServiceName:       cmp.Or(os.Getenv("OTEL_SERVICE_NAME"), "garmin_exporter"),
		ServiceVersion:    version.Version,
		MetricsExporter:   cmp.Or(os.Getenv("OTEL_METRICS_EXPORTER"), "none"),
		TracesExporter:    cmp.Or(os.Getenv("OTEL_TRACES_EXPORTER"), "none"),
		LogsExporter:      cmp.Or(os.Getenv("OTEL_LOGS_EXPORTER"), "none"),
		PrometheusEnabled: *webPrometheus,
		ConfigFile:        os.Getenv("OTEL_CONFIG_FILE"),
	})
	if err != nil {
		logger.Error("Failed to set up OTel pipeline", "err", err)
		os.Exit(1)
	}
	if otelResult.Logger != nil {
		logger = otelResult.Logger
	}

	// Auth manager: drives the Garmin login loop and exposes the resulting
	// client via a *garmin.Client wrapper that the scraper reads on every
	// refresh.
	authState := auth.NewState()
	if err := authState.Register(otelResult.Meter); err != nil {
		logger.Error("Failed to register auth metrics", "err", err)
		os.Exit(1)
	}
	garminClient := garmin.NewClient()
	mfaPrompt := func() (string, error) {
		fmt.Fprint(os.Stderr, "MFA code (check your email): ")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			return "", fmt.Errorf("no MFA code provided")
		}
		return strings.TrimSpace(scanner.Text()), nil
	}
	authManager := auth.NewManager(*garminUsername, *garminPassword, *garminTokenFile,
		logger, garminClient, authState, mfaPrompt)
	authManager.SetLogger(logger)

	// Scraper: one Scraper[garmin.Snapshot] driven by --cache.ttl. The
	// refresh func fans out across every Garmin endpoint best-effort and
	// triggers a re-auth when an Unauthorized error surfaces.
	garminScraper, err := scrape.New(scrape.Config[garmin.Snapshot]{
		Name:     "garmin",
		Interval: *cacheTTL,
		Logger:   logger.With("component", "scrape-garmin"),
		Tracer:   otelResult.Tracer,
		Refresh: garmin.NewRefresh(garminClient, logger.With("component", "garmin"), garmin.RefreshConfig{
			ActivityLimit:  *garminLimit,
			OnUnauthorized: authManager.TriggerReauth,
			Tracer:         otelResult.Tracer,
		}),
	})
	if err != nil {
		logger.Error("Failed to create Garmin scraper", "err", err)
		os.Exit(1)
	}

	// Collectors: instantiate every enabled collector, register their
	// observable instruments on the OTel Meter, and feed them the scraper
	// (which directly satisfies garmin.Source).
	group, err := collector.NewGroup(logger)
	if err != nil {
		logger.Error("Failed to instantiate collectors", "err", err)
		os.Exit(1)
	}
	if err := group.RegisterAll(otelResult.Meter, garminScraper); err != nil {
		logger.Error("Failed to register collectors", "err", err)
		os.Exit(1)
	}
	logger.Info("Collectors registered", "names", group.Names())

	scrapeCtx, scrapeCancel := context.WithCancel(rootCtx)
	go authManager.Run()
	go func() {
		if err := authManager.Ready(rootCtx); err != nil {
			return
		}
		garminScraper.Run(scrapeCtx)
	}()

	readyChecks := buildReadyChecks(garminClient, garminScraper, *cacheTTL)

	mux, err := buildHandler(otelResult, *metricsPath, readyChecks)
	if err != nil {
		logger.Error("Failed to build HTTP handler", "err", err)
		os.Exit(1)
	}

	logger.Info("Starting garmin_exporter", "version", version.Info())
	logger.Info("Build context", "build_context", version.BuildContext())
	if u, err := user.Current(); err == nil && u.Uid == "0" {
		logger.Warn("Garmin Exporter is running as root user. This exporter is designed to run as unprivileged user, root is not required.")
	}
	logger.Debug("Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	server := &http.Server{Handler: otelhttp.NewHandler(mux, "garmin_exporter")}
	serveErrCh := make(chan error, 1)
	go func() {
		err := web.ListenAndServe(server, toolkitFlags, logger)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
			return
		}
		close(serveErrCh)
	}()

	exitCode := 0
	select {
	case err := <-serveErrCh:
		if err != nil {
			logger.Error("ListenAndServe failed", "err", err)
			exitCode = 1
		}
	case <-rootCtx.Done():
		logger.Info("Shutdown signal received")
	}

	scrapeCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "HTTP shutdown error: %v\n", err)
	}
	if err := group.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Collector close error: %v\n", err)
	}
	if err := authState.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Auth state close error: %v\n", err)
	}
	if err := otelResult.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "OTel shutdown error: %v\n", err)
	}
	os.Exit(exitCode)
}

// buildReadyChecks wires the readyz dependency checks. Each subsystem owns
// its own health verdict; this function just decides which checks to expose
// under what name and what staleness threshold counts as not-ready.
func buildReadyChecks(client *garmin.Client, scraper *scrape.Scraper[garmin.Snapshot], ttl time.Duration) map[string]probes.Checker {
	return map[string]probes.Checker{
		"auth": probes.CheckerFunc(func(context.Context) error {
			return client.Healthy()
		}),
		"scrape": probes.CheckerFunc(func(context.Context) error {
			return scraper.Stale(3 * ttl)
		}),
	}
}
