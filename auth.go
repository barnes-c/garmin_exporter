package main

import (
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/barnes-c/go-garminconnect/garminconnect"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/barnes-c/garmin_exporter/collector"
)

const (
	authBackoffMin    = time.Minute
	authBackoffMax    = 3 * time.Hour
	authBackoffFactor = 2
)

var (
	authLoginSuccessDesc = prometheus.NewDesc(
		prometheus.BuildFQName("garmin", "auth", "login_success"),
		"garmin_exporter: Whether the last Garmin login attempt succeeded.",
		nil,
		nil,
	)
	authNextRetryTimestampDesc = prometheus.NewDesc(
		prometheus.BuildFQName("garmin", "auth", "next_retry_timestamp_seconds"),
		"garmin_exporter: Unix timestamp of the next scheduled Garmin login attempt, or 0 when no retry is scheduled.",
		nil,
		nil,
	)
)

type authState struct {
	mtx                sync.RWMutex
	loginSuccess       float64
	nextRetryTimestamp float64
}

func newAuthState() *authState {
	return &authState{}
}

func (s *authState) setLoginSuccess() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.loginSuccess = 1
	s.nextRetryTimestamp = 0
}

func (s *authState) setLoginFailure(nextRetry time.Time) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.loginSuccess = 0
	s.nextRetryTimestamp = float64(nextRetry.Unix())
}

func (s *authState) snapshot() (float64, float64) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.loginSuccess, s.nextRetryTimestamp
}

// Describe implements prometheus.Collector.
func (s *authState) Describe(ch chan<- *prometheus.Desc) {
	ch <- authLoginSuccessDesc
	ch <- authNextRetryTimestampDesc
}

// Collect implements prometheus.Collector.
func (s *authState) Collect(ch chan<- prometheus.Metric) {
	loginSuccess, nextRetryTimestamp := s.snapshot()
	ch <- prometheus.MustNewConstMetric(authLoginSuccessDesc, prometheus.GaugeValue, loginSuccess)
	ch <- prometheus.MustNewConstMetric(authNextRetryTimestampDesc, prometheus.GaugeValue, nextRetryTimestamp)
}

type authManager struct {
	username  string
	password  string
	logger    *slog.Logger
	login     func(username, password string) (*garminconnect.Client, error)
	setClient func(*garminconnect.Client)
	state     *authState
	delay     time.Duration
	reauthCh  <-chan struct{}

	// For testing.
	now    func() time.Time
	sleep  func(time.Duration)
	jitter func() time.Duration
}

func newAuthManager(username, password, tokenFile string, logger *slog.Logger, state *authState, reauthCh <-chan struct{}) *authManager {
	return &authManager{
		username: username,
		password: password,
		logger:   logger,
		login: func(username, password string) (*garminconnect.Client, error) {
			garminClient := garminconnect.NewClient(tokenFile)
			if err := garminClient.Login(username, password); err != nil {
				return nil, err
			}
			return garminClient, nil
		},
		setClient: collector.SetClient,
		state:     state,
		delay:     authBackoffMin,
		reauthCh:  reauthCh,
		now:       time.Now,
		sleep:     time.Sleep,
		jitter:    func() time.Duration { return time.Duration(rand.Int63n(int64(authBackoffMin))) },
	}
}

func (m *authManager) nextDelay() time.Duration {
	d := m.delay + m.jitter()
	m.delay = min(m.delay*authBackoffFactor, authBackoffMax)
	return d
}

func (m *authManager) resetDelay() {
	m.delay = authBackoffMin
}

func (m *authManager) run() {
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

func (m *authManager) attemptLogin() (time.Duration, bool) {
	garminClient, err := m.login(m.username, m.password)
	if err == nil {
		m.setClient(garminClient)
		m.state.setLoginSuccess()
		m.resetDelay()
		m.logger.Info("Garmin login succeeded")
		return 0, true
	}

	delay := m.nextDelay()
	nextRetry := m.now().Add(delay)
	m.setClient(nil)
	m.state.setLoginFailure(nextRetry)
	m.logger.Error("Garmin login failed", "err", err, "retry_after", delay, "next_retry", nextRetry.Unix())
	return delay, false
}
