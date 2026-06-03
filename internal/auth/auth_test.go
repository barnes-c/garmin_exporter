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
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type failTransport struct{}

func (failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("no network in tests")
}

func TestManagerLoginFailureSchedulesRetry(t *testing.T) {
	now := time.Unix(1000, 0)
	state := NewState()
	var installedClient *garminconnect.Client
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
		setClient: func(client *garminconnect.Client) {
			installedClient = client
		},
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
	if installedClient != nil {
		t.Fatal("expected failed login to clear the Garmin client")
	}

	success, nextRetry := state.Snapshot()
	if success != 0 {
		t.Fatalf("expected login success metric 0, got %v", success)
	}
	if nextRetry != float64(now.Add(time.Minute).Unix()) {
		t.Fatalf("expected next retry timestamp %v, got %v", now.Add(time.Minute).Unix(), nextRetry)
	}
}

func TestManagerLoginSuccessInstallsClient(t *testing.T) {
	state := NewState()
	client := &garminconnect.Client{}
	var installedClient *garminconnect.Client
	manager := Manager{
		username: "user",
		password: "pass",
		logger:   slog.Default(),
		login: func(username, password string) (*garminconnect.Client, error) {
			return client, nil
		},
		setClient: func(client *garminconnect.Client) {
			installedClient = client
		},
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
	if installedClient != client {
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
		setClient: func(client *garminconnect.Client) {},
		state:     state,
		delay:     time.Hour,
		now:       func() time.Time { return now },
		jitter:    func() time.Duration { return 0 },
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
		if nextRetry != float64(now.Add(want).Unix()) {
			t.Fatalf("expected next retry timestamp %v, got %v", now.Add(want).Unix(), nextRetry)
		}
	}
}

func TestManagerRunReauthOnSignal(t *testing.T) {
	loginCount := 0
	second := make(chan struct{})
	client := &garminconnect.Client{}

	manager := Manager{
		username: "user",
		password: "pass",
		logger:   slog.Default(),
		login: func(username, password string) (*garminconnect.Client, error) {
			loginCount++
			if loginCount == 2 {
				close(second)
			}
			return client, nil
		},
		setClient: func(*garminconnect.Client) {},
		state:     NewState(),
		delay:     time.Millisecond,
		reauthCh:  make(chan struct{}, 1),
		readyCh:   make(chan struct{}),
		now:       time.Now,
		sleep:     func(time.Duration) {},
		jitter:    func() time.Duration { return 0 },
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
		setClient: func(*garminconnect.Client) {},
		state:     NewState(),
		delay:     backoffMin,
		readyCh:   make(chan struct{}),
		now:       time.Now,
		jitter:    func() time.Duration { return 0 },
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
	m := NewManager("user", "pass", "", slog.Default(), NewState(), mfaPrompt)
	m.newClient = func(tokenFile string, opts ...garminconnect.Option) *garminconnect.Client {
		capturedOpts = opts
		return garminconnect.NewClient(tokenFile, garminconnect.WithHTTPClient(&http.Client{Transport: failTransport{}}))
	}
	m.login("user", "pass") //login failure expected
	if len(capturedOpts) != 1 {
		t.Fatalf("expected 1 opt (WithMFAPrompt), got %d", len(capturedOpts))
	}
}

func TestStateCollectsMetrics(t *testing.T) {
	state := NewState()
	state.SetLoginFailure(time.Unix(1234, 0))

	registry := prometheus.NewRegistry()
	if err := registry.Register(state); err != nil {
		t.Fatalf("register auth state collector: %s", err)
	}
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather auth metrics: %s", err)
	}

	assertGaugeValue(t, metricFamilies, "garmin_auth_login_success", 0)
	assertGaugeValue(t, metricFamilies, "garmin_auth_next_retry_timestamp_seconds", 1234)
}

func assertGaugeValue(t *testing.T, metricFamilies []*dto.MetricFamily, name string, want float64) {
	t.Helper()

	for _, metricFamily := range metricFamilies {
		if metricFamily.GetName() != name {
			continue
		}
		metrics := metricFamily.GetMetric()
		if len(metrics) != 1 {
			t.Fatalf("expected metric family %q to have 1 metric, got %d", name, len(metrics))
		}
		if got := metrics[0].GetGauge().GetValue(); got != want {
			t.Fatalf("expected metric %q value %v, got %v", name, want, got)
		}
		return
	}

	t.Fatalf("missing metric family %q", name)
}
