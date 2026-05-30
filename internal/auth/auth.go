// Package auth manages Garmin Connect authentication and exposes the
// resulting login state as Prometheus metrics.
package auth

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/barnes-c/go-garminconnect/garminconnect"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/barnes-c/garmin_exporter/collector"
)

const (
	backoffMin    = time.Minute
	backoffMax    = 3 * time.Hour
	backoffFactor = 2
)

var (
	loginSuccessDesc = prometheus.NewDesc(
		prometheus.BuildFQName("garmin", "auth", "login_success"),
		"garmin_exporter: Whether the last Garmin login attempt succeeded.",
		nil,
		nil,
	)
	nextRetryTimestampDesc = prometheus.NewDesc(
		prometheus.BuildFQName("garmin", "auth", "next_retry_timestamp_seconds"),
		"garmin_exporter: Unix timestamp of the next scheduled Garmin login attempt, or 0 when no retry is scheduled.",
		nil,
		nil,
	)
)

// State tracks the most recent Garmin login attempt and exposes it as
// Prometheus metrics.
type State struct {
	mtx                sync.RWMutex
	loginSuccess       float64
	nextRetryTimestamp float64
}

// NewState returns an empty login state.
func NewState() *State {
	return &State{}
}

// SetLoginSuccess marks the most recent login as successful.
func (s *State) SetLoginSuccess() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.loginSuccess = 1
	s.nextRetryTimestamp = 0
}

// SetLoginFailure marks the most recent login as failed and records when the
// next retry is scheduled.
func (s *State) SetLoginFailure(nextRetry time.Time) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.loginSuccess = 0
	s.nextRetryTimestamp = float64(nextRetry.Unix())
}

// Snapshot returns the current loginSuccess (0 or 1) and nextRetry timestamp.
func (s *State) Snapshot() (float64, float64) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.loginSuccess, s.nextRetryTimestamp
}

// Describe implements prometheus.Collector.
func (s *State) Describe(ch chan<- *prometheus.Desc) {
	ch <- loginSuccessDesc
	ch <- nextRetryTimestampDesc
}

// Collect implements prometheus.Collector.
func (s *State) Collect(ch chan<- prometheus.Metric) {
	loginSuccess, nextRetryTimestamp := s.Snapshot()
	ch <- prometheus.MustNewConstMetric(loginSuccessDesc, prometheus.GaugeValue, loginSuccess)
	ch <- prometheus.MustNewConstMetric(nextRetryTimestampDesc, prometheus.GaugeValue, nextRetryTimestamp)
}

// Manager runs the Garmin login loop, retrying with exponential backoff and
// installing successful clients into the collector package.
type Manager struct {
	username  string
	password  string
	logger    *slog.Logger
	login     func(username, password string) (*garminconnect.Client, error)
	newClient func(tokenFile string, opts ...garminconnect.Option) *garminconnect.Client
	setClient func(*garminconnect.Client)
	state     *State
	delay     time.Duration
	reauthCh  <-chan struct{}

	readyOnce sync.Once
	readyCh   chan struct{}

	// For testing.
	now    func() time.Time
	sleep  func(time.Duration)
	jitter func() time.Duration
}

// NewManager constructs a Manager. Logins use the supplied username and
// password; successful clients are installed into the collector package.
func NewManager(username, password, tokenFile string, logger *slog.Logger, state *State, reauthCh <-chan struct{}, mfaPrompt func() (string, error)) *Manager {
	m := &Manager{
		username:  username,
		password:  password,
		logger:    logger,
		newClient: garminconnect.NewClient,
		setClient: collector.SetClient,
		state:     state,
		delay:     backoffMin,
		reauthCh:  reauthCh,
		readyCh:   make(chan struct{}),
		now:       time.Now,
		sleep:     time.Sleep,
		jitter:    func() time.Duration { return time.Duration(rand.Int63n(int64(backoffMin))) },
	}
	m.login = func(username, password string) (*garminconnect.Client, error) {
		garminClient := m.newClient(tokenFile, garminconnect.WithMFAPrompt(mfaPrompt))
		if err := garminClient.Login(username, password); err != nil {
			return nil, err
		}
		return garminClient, nil
	}
	return m
}

func (m *Manager) nextDelay() time.Duration {
	d := m.delay + m.jitter()
	m.delay = min(m.delay*backoffFactor, backoffMax)
	return d
}

func (m *Manager) resetDelay() {
	m.delay = backoffMin
}

// Run runs the login loop: attempts an initial login (retrying on failure),
// then re-logs in whenever a signal arrives on reauthCh.
func (m *Manager) Run() {
	for {
		delay, ok := m.attemptLogin()
		if ok {
			break
		}
		m.sleep(delay)
	}
	for range m.reauthCh {
		m.logger.Info("re-authenticating due to stale token")
		m.resetDelay()
		for {
			delay, ok := m.attemptLogin()
			if ok {
				break
			}
			m.sleep(delay)
		}
	}
}

// Ready blocks until the first successful login has completed, or returns
// ctx.Err() if ctx is cancelled first. Subsequent calls return immediately.
func (m *Manager) Ready(ctx context.Context) error {
	select {
	case <-m.readyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) attemptLogin() (time.Duration, bool) {
	garminClient, err := m.login(m.username, m.password)
	if err == nil {
		m.setClient(garminClient)
		m.state.SetLoginSuccess()
		m.resetDelay()
		m.readyOnce.Do(func() { close(m.readyCh) })
		m.logger.Info("Garmin login succeeded")
		return 0, true
	}

	delay := m.nextDelay()
	nextRetry := m.now().Add(delay)
	m.setClient(nil)
	m.state.SetLoginFailure(nextRetry)
	m.logger.Error("Garmin login failed", "err", err, "retry_after", delay, "next_retry", nextRetry.Unix())
	return delay, false
}
