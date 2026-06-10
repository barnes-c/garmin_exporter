package scrape

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var noopTracer = noop.NewTracerProvider().Tracer("scrape-noop")

// RefreshFunc produces a fresh snapshot on every tick. It is invoked with
// the Scraper's context (cancellation propagates) and should respect any
// deadline. On error the previous snapshot is preserved.
type RefreshFunc[T any] func(ctx context.Context) (*T, error)

// Config configures a Scraper.
type Config[T any] struct {
	// Name identifies this scraper in logs and span attributes
	// (e.g. "ovs", "northd"). Required.
	Name string
	// Interval is the TTL between refreshes. Defaults to 15s.
	Interval time.Duration
	// Refresh produces the next snapshot. Required.
	Refresh RefreshFunc[T]
	// Logger receives one debug line per successful tick and one warn line
	// per failure. Required.
	Logger *slog.Logger
	// Tracer wraps each refresh in an `ovsx.scrape.unixctl` span.
	// Optional; when nil, no spans are emitted.
	Tracer trace.Tracer
}

// Scraper runs Refresh on a ticker and stores the most recent successful
// result in an atomic pointer for lock-free reads.
type Scraper[T any] struct {
	cfg     Config[T]
	snap    atomic.Pointer[T]
	outcome atomic.Pointer[Outcome]
}

// New validates cfg and returns an idle Scraper. Call Run to start the
// ticker, or Refresh to drive one tick manually.
func New[T any](cfg Config[T]) (*Scraper[T], error) {
	if cfg.Name == "" {
		return nil, errors.New("scrape: Name is required")
	}
	if cfg.Refresh == nil {
		return nil, errors.New("scrape: Refresh is required")
	}
	if cfg.Logger == nil {
		return nil, errors.New("scrape: Logger is required")
	}
	if cfg.Interval == 0 {
		cfg.Interval = 15 * time.Second
	}
	return &Scraper[T]{cfg: cfg}, nil
}

// Snapshot returns the most recent successful refresh. Returns nil before
// the first successful tick — callers must handle that case.
func (s *Scraper[T]) Snapshot() *T {
	return s.snap.Load()
}

// Outcome returns the result of the most recent tick (success or failure).
// Returns zero value before the first tick.
func (s *Scraper[T]) Outcome() Outcome {
	if o := s.outcome.Load(); o != nil {
		return *o
	}
	return Outcome{}
}

// Stale returns nil when the last scrape succeeded and is no older than
// maxAge, otherwise a descriptive error. Plugs into the probes.Checker
// interface via a CheckerFunc wrapper so the probes package stays free
// of an import on internal/scrape. The caller owns the policy (what age
// counts as stale).
func (s *Scraper[T]) Stale(maxAge time.Duration) error {
	o := s.Outcome()
	if o.Time.IsZero() {
		return errors.New("scrape: no attempt yet")
	}
	if !o.Success {
		return fmt.Errorf("scrape: last attempt failed: %v", o.Err)
	}
	if age := time.Since(o.Time); age > maxAge {
		return fmt.Errorf("scrape: stale (%v > %v)", age, maxAge)
	}
	return nil
}

// Refresh runs a single refresh synchronously. Exposed so tests and
// readyz probes can drive a tick without waiting for the ticker.
func (s *Scraper[T]) Refresh(ctx context.Context) error {
	start := time.Now()

	ctx, span := s.startSpan(ctx)
	snap, err := s.cfg.Refresh(ctx)
	endSpan(span, err)

	out := Outcome{
		Time:     start,
		Duration: time.Since(start),
		Success:  err == nil,
		Err:      err,
	}
	s.outcome.Store(&out)

	if err != nil {
		s.cfg.Logger.Warn("scrape failed",
			"scraper", s.cfg.Name,
			"duration", out.Duration,
			"err", err)
		return err
	}
	s.snap.Store(snap)
	s.cfg.Logger.Debug("scrape ok",
		"scraper", s.cfg.Name,
		"duration", out.Duration)
	return nil
}

// Run drives Refresh on Config.Interval until ctx is cancelled. The first
// refresh runs immediately so collectors don't see a nil snapshot for a
// full interval after startup.
func (s *Scraper[T]) Run(ctx context.Context) {
	_ = s.Refresh(ctx)

	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.Refresh(ctx)
		}
	}
}

func (s *Scraper[T]) startSpan(ctx context.Context) (context.Context, trace.Span) {
	tracer := s.cfg.Tracer
	if tracer == nil {
		tracer = noopTracer
	}
	return tracer.Start(ctx, "ovsx.scrape.unixctl",
		trace.WithAttributes(attribute.String("scraper.name", s.cfg.Name)))
}

func endSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
