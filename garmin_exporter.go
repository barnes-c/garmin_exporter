package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/user"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	promcollectors "github.com/prometheus/client_golang/prometheus/collectors"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/barnes-c/garmin_exporter/collector"
	"github.com/barnes-c/garmin_exporter/internal/auth"
	"github.com/barnes-c/garmin_exporter/internal/otlp"
	"github.com/barnes-c/garmin_exporter/internal/probes"
	"github.com/barnes-c/garmin_exporter/internal/scrape"
)

// handler serves Prometheus metrics from the scraper's cached snapshot
// alongside the exporter's own meta-metrics (process_*, go_*, garmin_auth_*,
// garmin_last_scrape_timestamp_seconds, etc.).
type handler struct {
	exporterMetricsRegistry *prometheus.Registry
	scraper                 *scrape.Scraper
	knownCollectors         []string
	includeExporterMetrics  bool
	maxRequests             int
	logger                  *slog.Logger
}

func newHandler(includeExporterMetrics bool, maxRequests int, logger *slog.Logger, authState *auth.State, scrapeOutcome *scrape.Outcome, scrp *scrape.Scraper, knownCollectors []string) *handler {
	h := &handler{
		exporterMetricsRegistry: prometheus.NewRegistry(),
		scraper:                 scrp,
		knownCollectors:         knownCollectors,
		includeExporterMetrics:  includeExporterMetrics,
		maxRequests:             maxRequests,
		logger:                  logger,
	}
	h.exporterMetricsRegistry.MustRegister(versioncollector.NewCollector("garmin_exporter"))
	h.exporterMetricsRegistry.MustRegister(authState, scrapeOutcome, scrp)
	if includeExporterMetrics {
		h.exporterMetricsRegistry.MustRegister(
			promcollectors.NewProcessCollector(promcollectors.ProcessCollectorOpts{}),
			promcollectors.NewGoCollector(),
		)
	}
	return h
}

// ServeHTTP implements http.Handler. It always serves from the scraper's
// cached snapshot; filtered scrapes intersect that snapshot in memory and
// never trigger a Garmin call.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	collects := r.URL.Query()["collect[]"]
	excludes := r.URL.Query()["exclude[]"]

	if len(collects) > 0 && len(excludes) > 0 {
		h.logger.Debug("rejecting combined collect and exclude queries")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Combined collect and exclude queries are not allowed."))
		return
	}

	for _, name := range collects {
		if !slices.Contains(h.knownCollectors, name) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "unknown or disabled collector: %s", name)
			return
		}
	}
	for _, name := range excludes {
		if !slices.Contains(h.knownCollectors, name) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "unknown or disabled collector: %s", name)
			return
		}
	}

	dataGatherer := h.scraper.Gatherer()
	switch {
	case len(collects) > 0:
		dataGatherer = h.scraper.FilteredGatherer(collects)
	case len(excludes) > 0:
		wanted := make([]string, 0, len(h.knownCollectors))
		for _, n := range h.knownCollectors {
			if !slices.Contains(excludes, n) {
				wanted = append(wanted, n)
			}
		}
		dataGatherer = h.scraper.FilteredGatherer(wanted)
	}

	var inner http.Handler
	opts := promhttp.HandlerOpts{
		ErrorLog:            slog.NewLogLogger(h.logger.Handler(), slog.LevelError),
		ErrorHandling:       promhttp.ContinueOnError,
		MaxRequestsInFlight: h.maxRequests,
	}
	if h.includeExporterMetrics {
		opts.Registry = h.exporterMetricsRegistry
		inner = promhttp.HandlerFor(prometheus.Gatherers{h.exporterMetricsRegistry, dataGatherer}, opts)
		inner = promhttp.InstrumentMetricHandler(h.exporterMetricsRegistry, inner)
	} else {
		inner = promhttp.HandlerFor(prometheus.Gatherers{h.exporterMetricsRegistry, dataGatherer}, opts)
	}
	inner.ServeHTTP(w, r)
}

// Gatherers returns the union of the exporter meta-metrics registry and the
// scraper's cached data gatherer. Used to feed the OTLP push pipeline.
func (h *handler) Gatherers() prometheus.Gatherers {
	return prometheus.Gatherers{h.exporterMetricsRegistry, h.scraper.Gatherer()}
}

func main() {
	var (
		metricsPath = kingpin.Flag(
			"web.telemetry-path",
			"Path under which to expose metrics.",
		).Default("/metrics").String()
		disableExporterMetrics = kingpin.Flag(
			"web.disable-exporter-metrics",
			"Exclude metrics about the exporter itself (promhttp_*, process_*, go_*).",
		).Bool()
		maxRequests = kingpin.Flag(
			"web.max-requests",
			"Maximum number of parallel scrape requests. Use 0 to disable.",
		).Default("40").Int()
		disableDefaultCollectors = kingpin.Flag(
			"collector.disable-defaults",
			"Set all collectors to disabled by default.",
		).Default("false").Bool()
		maxProcs = kingpin.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()
		toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, ":10045")

		garminUsername  = kingpin.Flag("garmin.username", "Garmin Connect username.").Envar("GARMIN_USERNAME").Required().String()
		garminPassword  = kingpin.Flag("garmin.password", "Garmin Connect password.").Envar("GARMIN_PASSWORD").Required().String()
		garminTokenFile = kingpin.Flag("garmin.token-file", "Path to cached OAuth2 token file.").Default("garmin_token.json").String()
		garminLimit     = kingpin.Flag("garmin.activity-limit", "Number of recent activities to fetch.").Default("30").Int()

		cacheTTL = kingpin.Flag("cache.ttl", "How often to refresh data from Garmin Connect. Controls the Garmin API call rate; independent of Prometheus scrape interval.").Default("1h").Duration()

		otlpEndpoint         = kingpin.Flag("otlp.endpoint", "OTLP collector endpoint (e.g. localhost:4317). Enables OTLP export when set.").Envar("OTEL_EXPORTER_OTLP_ENDPOINT").Default("").String()
		otlpProtocol         = kingpin.Flag("otlp.protocol", "OTLP transport protocol.").Envar("OTEL_EXPORTER_OTLP_PROTOCOL").Default("grpc").String()
		otlpInterval         = kingpin.Flag("otlp.interval", "OTLP push interval. Independent of --cache.ttl; pushes always send the most recent cached values.").Default("15s").Duration()
		otlpMetricsExporter = kingpin.Flag("otlp.metrics-exporter", "OTLP metrics exporter. Set to \"none\" to disable metrics export.").Envar("OTEL_METRICS_EXPORTER").Default("otlp").String()
		otlpTracesExporter  = kingpin.Flag("otlp.traces-exporter", "OTLP traces exporter. Set to \"none\" to disable traces export.").Envar("OTEL_TRACES_EXPORTER").Default("otlp").String()
		otlpLogsExporter    = kingpin.Flag("otlp.logs-exporter", "OTLP logs exporter. Set to \"none\" to disable logs export.").Envar("OTEL_LOGS_EXPORTER").Default("otlp").String()
	)

	promslogConfig := &promslog.Config{}
	flag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.Version(version.Print("garmin_exporter"))
	kingpin.CommandLine.UsageWriter(os.Stdout)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promslog.New(promslogConfig)

	if *disableDefaultCollectors {
		collector.DisableDefaultCollectors()
	}

	collector.SetActivityLimit(*garminLimit)
	scrapeOutcome := scrape.NewOutcome()
	authState := auth.NewState()
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
	authManager := auth.NewManager(*garminUsername, *garminPassword, *garminTokenFile, logger, authState, mfaPrompt)
	go authManager.Run()

	logger.Info("Starting garmin_exporter", "version", version.Info())
	logger.Info("Build context", "build_context", version.BuildContext())
	if u, err := user.Current(); err == nil && u.Uid == "0" {
		logger.Warn("Garmin Exporter is running as root user. This exporter is designed to run as unprivileged user, root is not required.")
	}
	runtime.GOMAXPROCS(*maxProcs)
	logger.Debug("Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// Enumerate the enabled Garmin sub-collectors once for filter validation
	// and landing-page logging.
	bootstrapGC, err := collector.NewGarminCollector(logger)
	if err != nil {
		logger.Error("Couldn't enumerate collectors", "err", err)
		os.Exit(1)
	}
	enabledNames := make([]string, 0, len(bootstrapGC.Collectors))
	for n := range bootstrapGC.Collectors {
		enabledNames = append(enabledNames, n)
	}
	sort.Strings(enabledNames)
	logger.Info("Enabled collectors")
	for _, n := range enabledNames {
		logger.Info(n)
	}

	scrp := scrape.New(scrape.Config{
		TTL:    *cacheTTL,
		Logger: logger,
		BuildCollectors: func() (map[string]prometheus.Collector, error) {
			gc, err := collector.NewGarminCollector(logger)
			if err != nil {
				return nil, err
			}
			return gc.PromCollectors(collector.WithUnauthorizedHandler(authManager.TriggerReauth)), nil
		},
		AuthReady: authManager.Ready,
		OnScrape:  scrapeOutcome.Record,
	})
	scraperCtx, cancelScraper := context.WithCancel(context.Background())
	defer cancelScraper()
	go scrp.Run(scraperCtx)

	h := newHandler(!*disableExporterMetrics, *maxRequests, logger, authState, scrapeOutcome, scrp, enabledNames)
	http.Handle(*metricsPath, h)
	http.HandleFunc("/healthz", probes.Healthz)
	http.Handle("/readyz", probes.Readyz(authState, scrapeOutcome))

	if *otlpEndpoint != "" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		shutdown, otlpLogger, err := otlp.Setup(ctx, h.Gatherers(), logger, otlp.Config{
			Protocol:        *otlpProtocol,
			Interval:        *otlpInterval,
			MetricsExporter: *otlpMetricsExporter,
			TracesExporter:  *otlpTracesExporter,
			LogsExporter:    *otlpLogsExporter,
		})
		if err != nil {
			logger.Error("Failed to setup OTLP", "err", err)
			os.Exit(1)
		}
		if otlpLogger != nil {
			logger = otlpLogger
		}
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := shutdown(shutdownCtx); err != nil {
				logger.Error("OTLP shutdown error", "err", err)
			}
		}()
	}

	if *metricsPath != "/" {
		landingConfig := web.LandingConfig{
			Name:        "Garmin Exporter",
			Description: "Prometheus Garmin Exporter",
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{
					Address: *metricsPath,
					Text:    "Metrics",
				},
				{
					Address: "/healthz",
					Text:    "Health",
				},
				{
					Address: "/readyz",
					Text:    "Readiness",
				},
			},
		}
		landingPage, err := web.NewLandingPage(landingConfig)
		if err != nil {
			logger.Error("Couldn't create landing page", "err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}

	server := &http.Server{}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		logger.Error("ListenAndServe failed", "err", err)
		os.Exit(1)
	}
}
