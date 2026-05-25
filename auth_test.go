package main

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/barnes-c/go-garminconnect/garminconnect"
	"github.com/jpillora/backoff"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestAuthManagerLoginFailureSchedulesRetry(t *testing.T) {
	now := time.Unix(1000, 0)
	state := newAuthState()
	var installedClient *garminconnect.Client
	manager := authManager{
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
		state: state,
		backoff: &backoff.Backoff{
			Min:    time.Minute,
			Max:    3 * time.Hour,
			Factor: 2,
		},
		now: func() time.Time { return now },
	}

	delay, ok := manager.attemptLogin()
	if ok {
		t.Fatal("expected login attempt to fail")
	}
	if delay != time.Minute {
		t.Fatalf("expected retry delay %s, got %s", time.Minute, delay)
	}
	if installedClient != nil {
		t.Fatal("expected failed login to clear the Garmin client")
	}

	success, nextRetry := state.snapshot()
	if success != 0 {
		t.Fatalf("expected login success metric 0, got %v", success)
	}
	if nextRetry != float64(now.Add(time.Minute).Unix()) {
		t.Fatalf("expected next retry timestamp %v, got %v", now.Add(time.Minute).Unix(), nextRetry)
	}
}

func TestAuthManagerLoginSuccessInstallsClient(t *testing.T) {
	state := newAuthState()
	client := &garminconnect.Client{}
	var installedClient *garminconnect.Client
	manager := authManager{
		username: "user",
		password: "pass",
		logger:   slog.Default(),
		login: func(username, password string) (*garminconnect.Client, error) {
			return client, nil
		},
		setClient: func(client *garminconnect.Client) {
			installedClient = client
		},
		state: state,
		backoff: &backoff.Backoff{
			Min:    time.Minute,
			Max:    3 * time.Hour,
			Factor: 2,
		},
		now: time.Now,
	}

	delay, ok := manager.attemptLogin()
	if !ok {
		t.Fatal("expected login attempt to succeed")
	}
	if delay != 0 {
		t.Fatalf("expected no retry delay, got %s", delay)
	}
	if installedClient != client {
		t.Fatal("expected successful login to install the Garmin client")
	}

	success, nextRetry := state.snapshot()
	if success != 1 {
		t.Fatalf("expected login success metric 1, got %v", success)
	}
	if nextRetry != 0 {
		t.Fatalf("expected next retry timestamp 0, got %v", nextRetry)
	}
}

func TestAuthManagerBackoffDelayIsCapped(t *testing.T) {
	now := time.Unix(1000, 0)
	state := newAuthState()
	manager := authManager{
		logger: slog.Default(),
		login: func(username, password string) (*garminconnect.Client, error) {
			return nil, errors.New("rate limited")
		},
		setClient: func(client *garminconnect.Client) {},
		state:     state,
		backoff: &backoff.Backoff{
			Min:    time.Hour,
			Max:    3 * time.Hour,
			Factor: 2,
		},
		now: func() time.Time { return now },
	}

	for _, want := range []time.Duration{time.Hour, 2 * time.Hour, 3 * time.Hour, 3 * time.Hour} {
		delay, ok := manager.attemptLogin()
		if ok {
			t.Fatal("expected login attempt to fail")
		}
		if delay != want {
			t.Fatalf("expected retry delay %s, got %s", want, delay)
		}
		_, nextRetry := state.snapshot()
		if nextRetry != float64(now.Add(want).Unix()) {
			t.Fatalf("expected next retry timestamp %v, got %v", now.Add(want).Unix(), nextRetry)
		}
	}
}

func TestAuthManagerRunReauthOnSignal(t *testing.T) {
	reauthCh := make(chan struct{}, 1)
	loginCount := 0
	second := make(chan struct{})
	client := &garminconnect.Client{}

	manager := authManager{
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
		state:     newAuthState(),
		backoff: &backoff.Backoff{
			Min: time.Millisecond, Max: time.Millisecond, Factor: 1,
		},
		reauthCh: reauthCh,
		now:      time.Now,
		sleep:    func(time.Duration) {},
	}

	go manager.run()
	reauthCh <- struct{}{}

	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("re-login not triggered after reauth signal")
	}
	if loginCount != 2 {
		t.Fatalf("expected 2 login calls, got %d", loginCount)
	}
}

func TestAuthStateCollectsMetrics(t *testing.T) {
	state := newAuthState()
	state.setLoginFailure(time.Unix(1234, 0))

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
