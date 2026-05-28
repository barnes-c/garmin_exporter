package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/barnes-c/garmin_exporter/collector"

	"github.com/prometheus/client_golang/prometheus"
	prombridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func doBackfill(authManager *authManager, logger *slog.Logger, otlpProtocol string, startStr, endStr string, days int, delay time.Duration, collectorsStr string) {
	start, end, err := backfillDateRange(startStr, endStr, days)
	if err != nil {
		logger.Error("Invalid backfill date range", "err", err)
		os.Exit(1)
	}

	var collectors []string
	if collectorsStr != "" {
		for _, c := range strings.Split(collectorsStr, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				collectors = append(collectors, c)
			}
		}
	}

	for {
		d, ok := authManager.attemptLogin()
		if ok {
			break
		}
		time.Sleep(d)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runBackfill(ctx, backfillConfig{
		start:      start,
		end:        end,
		delay:      delay,
		collectors: collectors,
		otlpCfg:    otlpConfig{protocol: otlpProtocol},
	}, logger)
	if err != nil {
		logger.Error("Backfill failed", "err", err)
		os.Exit(1)
	}
}

const (
	backfillBackoffMin    = 30 * time.Second
	backfillBackoffMax    = 10 * time.Minute
	backfillBackoffFactor = 2
)

type backfillConfig struct {
	start      time.Time
	end        time.Time
	delay      time.Duration
	collectors []string
	otlpCfg    otlpConfig
}

func runBackfill(ctx context.Context, cfg backfillConfig, logger *slog.Logger) error {
	nc, err := collector.NewGarminCollector(logger)
	if err != nil {
		return fmt.Errorf("creating collector: %w", err)
	}

	backfillable := make(map[string]collector.Collector)
	for name, c := range nc.Collectors {
		if !collector.BackfillableCollectors[name] {
			continue
		}
		if len(cfg.collectors) > 0 {
			found := false
			for _, want := range cfg.collectors {
				if want == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		backfillable[name] = c
	}

	if len(backfillable) == 0 {
		return fmt.Errorf("no backfillable collectors enabled")
	}

	names := make([]string, 0, len(backfillable))
	for n := range backfillable {
		names = append(names, n)
	}
	sort.Strings(names)
	logger.Info("Backfill collectors", "collectors", strings.Join(names, ", "))

	exporter, err := newOTLPExporter(ctx, cfg.otlpCfg.protocol)
	if err != nil {
		return fmt.Errorf("creating OTLP exporter: %w", err)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exporter.Shutdown(shutCtx)
	}()

	totalDays := int(cfg.end.Sub(cfg.start).Hours()/24) + 1
	backoff := backfillBackoffMin

	for day := cfg.start; !day.After(cfg.end); day = day.AddDate(0, 0, 1) {
		dayNum := int(day.Sub(cfg.start).Hours()/24) + 1
		logger.Info("Backfilling", "date", day.Format("2006-01-02"), "day", fmt.Sprintf("%d/%d", dayNum, totalDays))

		metrics, err := collectDay(ctx, backfillable, names, day, cfg.delay, logger)
		if err != nil {
			if isRateLimited(err) {
				logger.Warn("Rate limited, backing off", "delay", backoff, "last_success", day.AddDate(0, 0, -1).Format("2006-01-02"))
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
				backoff = min(backoff*backfillBackoffFactor, backfillBackoffMax)
				day = day.AddDate(0, 0, -1) // retry same day
				continue
			}
			logger.Error("Backfill failed for day", "date", day.Format("2006-01-02"), "err", err,
				"resume_from", day.Format("2006-01-02"))
			return fmt.Errorf("backfill failed at %s: %w", day.Format("2006-01-02"), err)
		}
		backoff = backfillBackoffMin

		if len(metrics) == 0 {
			logger.Debug("No data for day", "date", day.Format("2006-01-02"))
			continue
		}

		if err := pushMetrics(ctx, exporter, metrics, day); err != nil {
			return fmt.Errorf("OTLP push failed for %s: %w", day.Format("2006-01-02"), err)
		}
	}

	logger.Info("Backfill complete", "start", cfg.start.Format("2006-01-02"), "end", cfg.end.Format("2006-01-02"), "days", totalDays)
	return nil
}

func collectDay(ctx context.Context, collectors map[string]collector.Collector, order []string, date time.Time, delay time.Duration, logger *slog.Logger) ([]prometheus.Metric, error) {
	var allMetrics []prometheus.Metric

	for _, name := range order {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		c := collectors[name]
		ch := make(chan prometheus.Metric, 256)
		errCh := make(chan error, 1)
		go func() {
			errCh <- c.Update(ch, date)
			close(ch)
		}()

		for m := range ch {
			allMetrics = append(allMetrics, prometheus.NewMetricWithTimestamp(date, m))
		}

		if err := <-errCh; err != nil {
			if collector.IsNoDataError(err) {
				logger.Debug("No data", "collector", name, "date", date.Format("2006-01-02"))
				continue
			}
			return nil, fmt.Errorf("collector %s: %w", name, err)
		}

		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return allMetrics, nil
}

func pushMetrics(ctx context.Context, exporter sdkmetric.Exporter, metrics []prometheus.Metric, date time.Time) error {
	reg := prometheus.NewRegistry()
	reg.MustRegister(&staticMetricCollector{metrics: metrics})

	bridge := prombridge.NewMetricProducer(prombridge.WithGatherer(reg))
	reader := sdkmetric.NewManualReader(sdkmetric.WithProducer(bridge))
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer func() { _ = provider.Shutdown(ctx) }()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		return fmt.Errorf("collecting metrics: %w", err)
	}

	return exporter.Export(ctx, &rm)
}

// staticMetricCollector implements prometheus.Collector to serve pre-collected metrics.
type staticMetricCollector struct {
	metrics []prometheus.Metric
}

func (c *staticMetricCollector) Describe(chan<- *prometheus.Desc) {}

func (c *staticMetricCollector) Collect(ch chan<- prometheus.Metric) {
	for _, m := range c.metrics {
		ch <- m
	}
}

func newOTLPExporter(ctx context.Context, protocol string) (sdkmetric.Exporter, error) {
	switch protocol {
	case "grpc":
		return otlpmetricgrpc.New(ctx)
	case "http/protobuf", "http":
		return otlpmetrichttp.New(ctx)
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol %q", protocol)
	}
}

func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	var httpErr interface{ StatusCode() int }
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode() == http.StatusTooManyRequests
	}
	return strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "Too Many Requests")
}

// parseBackfillDate parses a YYYY-MM-DD string into a time.Time at midnight UTC.
func parseBackfillDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q (expected YYYY-MM-DD): %w", s, err)
	}
	return t, nil
}

// backfillDateRange computes start/end from the flag values.
func backfillDateRange(startStr, endStr string, days int) (time.Time, time.Time, error) {
	end, err := parseBackfillDate(endStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	var start time.Time
	if startStr != "" && days > 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("--backfill.start and --backfill.days are mutually exclusive")
	}
	if startStr != "" {
		start, err = parseBackfillDate(startStr)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	} else if days > 0 {
		start = end.AddDate(0, 0, -days+1)
	} else {
		return time.Time{}, time.Time{}, fmt.Errorf("either --backfill.start or --backfill.days is required")
	}

	if start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("start date %s is after end date %s", start.Format("2006-01-02"), end.Format("2006-01-02"))
	}

	return start, end, nil
}
