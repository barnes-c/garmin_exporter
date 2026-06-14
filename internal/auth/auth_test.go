package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/barnes-c/go-garminconnect/garminconnect"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

type failTransport struct{}

func (failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("no network in tests")
}

func TestManagerLoginFailureSchedulesRetry(t *testing.T) {
	now := time.Unix(1000, 0)
	state := NewState()
	client := garmin.NewClient()
	client.Set(&garminconnect.Client{}) // start non-nil; failed login should clear it
	manager := Manager{
		username: "user",
		password: "pass",
		logger:   slog.Default(),
		login: func(username, password string) (*garminconnect.Client, error) {
			if username != "user" || password != "pass" {
				t.Fatalf("unexpected credentials: username=%q password=%q", username, password)
			}
			return nil, errors.New("rate limited")
		},
		client: client,
		state:  state,
		delay:  backoffMin,
		now:    func() time.Time { return now },
		jitter: func() time.Duration { return 0 },
	}

	delay, ok := manager.attemptLogin(context.Background())
	if ok {
		t.Fatal("expected login attempt to fail")
	}
	if delay != time.Minute {
		t.Fatalf("expected retry delay %s, got %s", time.Minute, delay)
	}
	if client.Get() != nil {
		t.Fatal("expected failed login to clear the Garmin client")
	}

	success, nextRetry := state.Snapshot()
	if success != 0 {
		t.Fatalf("expected login success metric 0, got %v", success)
	}
	if nextRetry != now.Add(time.Minute).Unix() {
		t.Fatalf("expected next retry timestamp %v, got %v", now.Add(time.Minute).Unix(), nextRetry)
	}
}

func TestManagerLoginSuccessInstallsClient(t *testing.T) {
	state := NewState()
	want := &garminconnect.Client{}
	client := garmin.NewClient()
	manager := Manager{
		username: "user",
		password: "pass",
		logger:   slog.Default(),
		login: func(username, password string) (*garminconnect.Client, error) {
			return want, nil
		},
		client:  client,
		state:   state,
		delay:   backoffMin,
		readyCh: make(chan struct{}),
		now:     time.Now,
		jitter:  func() time.Duration { return 0 },
	}

	delay, ok := manager.attemptLogin(context.Background())
	if !ok {
		t.Fatal("expected login attempt to succeed")
	}
	if delay != 0 {
		t.Fatalf("expected no retry delay, got %s", delay)
	}
	if got := client.Get(); got != want {
		t.Fatal("expected successful login to install the Garmin client")
	}

	success, nextRetry := state.Snapshot()
	if success != 1 {
		t.Fatalf("expected login success metric 1, got %v", success)
	}
	if nextRetry != 0 {
		t.Fatalf("expected next retry timestamp 0, got %v", nextRetry)
	}
}

func TestManagerBackoffDelayIsCapped(t *testing.T) {
	now := time.Unix(1000, 0)
	state := NewState()
	manager := Manager{
		logger: slog.Default(),
		login: func(username, password string) (*garminconnect.Client, error) {
			return nil, errors.New("rate limited")
		},
		client: garmin.NewClient(),
		state:  state,
		delay:  time.Hour,
		now:    func() time.Time { return now },
		jitter: func() time.Duration { return 0 },
	}

	for _, want := range []time.Duration{time.Hour, 2 * time.Hour, 3 * time.Hour, 3 * time.Hour} {
		delay, ok := manager.attemptLogin(context.Background())
		if ok {
			t.Fatal("expected login attempt to fail")
		}
		if delay != want {
			t.Fatalf("expected retry delay %s, got %s", want, delay)
		}
		_, nextRetry := state.Snapshot()
		if nextRetry != now.Add(want).Unix() {
			t.Fatalf("expected next retry timestamp %v, got %v", now.Add(want).Unix(), nextRetry)
		}
	}
}

func TestManagerRunReauthOnSignal(t *testing.T) {
	loginCount := 0
	second := make(chan struct{})
	want := &garminconnect.Client{}

	manager := Manager{
		username: "user",
		password: "pass",
		logger:   slog.Default(),
		login: func(username, password string) (*garminconnect.Client, error) {
			loginCount++
			if loginCount == 2 {
				close(second)
			}
			return want, nil
		},
		client:   garmin.NewClient(),
		state:    NewState(),
		delay:    time.Millisecond,
		reauthCh: make(chan struct{}, 1),
		readyCh:  make(chan struct{}),
		now:      time.Now,
		sleep:    func(time.Duration) {},
		jitter:   func() time.Duration { return 0 },
	}

	go manager.Run()
	manager.TriggerReauth()

	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("re-login not triggered after reauth signal")
	}
	if loginCount != 2 {
		t.Fatalf("expected 2 login calls, got %d", loginCount)
	}
}

func TestManagerTriggerReauthNonBlocking(t *testing.T) {
	m := Manager{reauthCh: make(chan struct{}, 1)}
	// First call queues the request.
	m.TriggerReauth()
	// Second call must not block even though the buffer is full.
	done := make(chan struct{})
	go func() { m.TriggerReauth(); close(done) }()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("TriggerReauth blocked when buffer was full")
	}
}

func TestManagerReadySignalsOnFirstSuccess(t *testing.T) {
	loginErr := errors.New("rate limited")
	loginCount := 0
	manager := Manager{
		logger: slog.Default(),
		login: func(string, string) (*garminconnect.Client, error) {
			loginCount++
			if loginCount == 1 {
				return nil, loginErr
			}
			return &garminconnect.Client{}, nil
		},
		client:  garmin.NewClient(),
		state:   NewState(),
		delay:   backoffMin,
		readyCh: make(chan struct{}),
		now:     time.Now,
		jitter:  func() time.Duration { return 0 },
	}

	// First attempt fails; Ready must not return yet.
	if _, ok := manager.attemptLogin(context.Background()); ok {
		t.Fatal("expected first login to fail")
	}
	earlyCtx, earlyCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer earlyCancel()
	if err := manager.Ready(earlyCtx); err == nil {
		t.Fatal("expected Ready to block before first successful login")
	}

	// Second attempt succeeds; Ready must return nil.
	if _, ok := manager.attemptLogin(context.Background()); !ok {
		t.Fatal("expected second login to succeed")
	}
	if err := manager.Ready(context.Background()); err != nil {
		t.Fatalf("expected Ready to return nil after success, got %v", err)
	}

	// Subsequent successes must not panic (sync.Once guards the close).
	if _, ok := manager.attemptLogin(context.Background()); !ok {
		t.Fatal("expected third login to succeed")
	}
	if err := manager.Ready(context.Background()); err != nil {
		t.Fatalf("expected Ready to keep returning nil, got %v", err)
	}
}

func TestNewManagerLoginPassesMFAPrompt(t *testing.T) {
	var capturedOpts []garminconnect.Option
	mfaPrompt := func() (string, error) { return "123456", nil }
	m := NewManager("user", "pass", "", slog.Default(), garmin.NewClient(), NewState(), mfaPrompt)
	m.newClient = func(tokenFile string, opts ...garminconnect.Option) *garminconnect.Client {
		capturedOpts = opts
		return garminconnect.NewClient(tokenFile, garminconnect.WithHTTPClient(&http.Client{Transport: failTransport{}}))
	}
	if _, err := m.login("user", "pass"); err == nil {
		t.Fatal("expected login to fail")
	}
	if len(capturedOpts) != 1 {
		t.Fatalf("expected 1 opt (WithMFAPrompt), got %d", len(capturedOpts))
	}
}

func TestStateRegisterEmitsOTelGauges(t *testing.T) {
	state := NewState()
	state.SetLoginFailure(time.Unix(1234, 0))

	reader := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	meter := mp.Meter("test")

	if err := state.Register(meter); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() })

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	assertOTelGaugeInt(t, rm, "garmin.auth.login_success", 0)
	assertOTelGaugeInt(t, rm, "garmin.auth.next_retry_timestamp_seconds", 1234)
}

func assertOTelGaugeInt(t *testing.T, rm metricdata.ResourceMetrics, name string, want int64) {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok {
				t.Fatalf("metric %q has data type %T, want Gauge[int64]", name, m.Data)
			}
			if len(g.DataPoints) != 1 {
				t.Fatalf("metric %q has %d data points, want 1", name, len(g.DataPoints))
			}
			if got := g.DataPoints[0].Value; got != want {
				t.Fatalf("metric %q value = %v, want %v", name, got, want)
			}
			return
		}
	}
	t.Fatalf("missing metric %q", name)
}
