// Package auth manages Garmin Connect authentication and exposes the
// resulting login state as OTel observable metrics.
package auth

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/barnes-c/go-garminconnect/garminconnect"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/garmin_exporter/internal/garmin"
)

const (
	backoffMin    = time.Minute
	backoffMax    = 3 * time.Hour
	backoffFactor = 2
)

// State tracks the most recent Garmin login attempt and exposes it as OTel
// observable gauges via Register.
type State struct {
	mtx                sync.RWMutex
	loginSuccess       int64
	nextRetryTimestamp int64

	registration metric.Registration
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
	s.nextRetryTimestamp = nextRetry.Unix()
}

// Snapshot returns the current loginSuccess (0 or 1) and nextRetry timestamp.
func (s *State) Snapshot() (int64, int64) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.loginSuccess, s.nextRetryTimestamp
}

// Register installs Int64ObservableGauges on meter for the login success and
// next retry timestamp. The metrics are emitted with the legacy Prometheus
// names so existing dashboards and alerts continue to match.
func (s *State) Register(meter metric.Meter) error {
	loginSuccess, err := meter.Int64ObservableGauge(
		"garmin.auth.login_success",
		metric.WithDescription("Whether the last Garmin login attempt succeeded (1) or failed (0)."),
	)
	if err != nil {
		return err
	}
	nextRetry, err := meter.Int64ObservableGauge(
		"garmin.auth.next_retry_timestamp_seconds",
		metric.WithDescription("Unix timestamp of the next scheduled Garmin login attempt, or 0 when no retry is scheduled."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return err
	}

	s.registration, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		success, retry := s.Snapshot()
		o.ObserveInt64(loginSuccess, success)
		o.ObserveInt64(nextRetry, retry)
		return nil
	}, loginSuccess, nextRetry)
	return err
}

// Close unregisters the OTel callback installed by Register. Safe to call
// before Register or after a prior Close.
func (s *State) Close() error {
	if s.registration == nil {
		return nil
	}
	err := s.registration.Unregister()
	s.registration = nil
	return err
}

// Manager runs the Garmin login loop, retrying with exponential backoff and
// installing successful clients into the supplied *garmin.Client wrapper so
// the scraper picks them up on the next refresh.
type Manager struct {
	username  string
	password  string
	logger    *slog.Logger
	login     func(username, password string) (*garminconnect.Client, error)
	newClient func(tokenFile string, opts ...garminconnect.Option) *garminconnect.Client
	client    *garmin.Client
	state     *State
	delay     time.Duration
	reauthCh  chan struct{}

	readyOnce sync.Once
	readyCh   chan struct{}

	// For testing.
	now    func() time.Time
	sleep  func(time.Duration)
	jitter func() time.Duration
}

// SetLogger replaces the logger used by the manager. Must be called before Run.
func (m *Manager) SetLogger(l *slog.Logger) {
	m.logger = l
}

// NewManager constructs a Manager. Logins use the supplied username and
// password; successful clients are installed into the supplied garmin.Client
// wrapper, where the scraper reads them via Get on each refresh.
func NewManager(username, password, tokenFile string, logger *slog.Logger, client *garmin.Client, state *State, mfaPrompt func() (string, error)) *Manager {
	if client == nil {
		// Construct an empty wrapper so callers passing nil don't crash;
		// they just won't be able to observe the installed client.
		client = garmin.NewClient()
	}
	m := &Manager{
		username:  username,
		password:  password,
		logger:    logger,
		newClient: garminconnect.NewClient,
		client:    client,
		state:     state,
		delay:     backoffMin,
		reauthCh:  make(chan struct{}, 1),
		readyCh:   make(chan struct{}),
		now:       time.Now,
		sleep:     time.Sleep,
		jitter:    func() time.Duration { return time.Duration(rand.Int63n(int64(backoffMin))) },
	}
	m.login = func(username, password string) (*garminconnect.Client, error) {
		garminClient := m.newClient(tokenFile, garminconnect.WithMFAPrompt(mfaPrompt))
		if err := garminClient.Login(context.Background(), username, password); err != nil {
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
	ctx := context.Background()
	for {
		delay, ok := m.attemptLogin(ctx)
		if ok {
			break
		}
		m.sleep(delay)
	}
	for range m.reauthCh {
		m.logger.Info("re-authenticating due to stale token")
		m.resetDelay()
		for {
			delay, ok := m.attemptLogin(ctx)
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

// TriggerReauth requests a re-authentication on the next Run loop iteration.
// Non-blocking: if a request is already queued, this is a no-op.
func (m *Manager) TriggerReauth() {
	select {
	case m.reauthCh <- struct{}{}:
	default:
	}
}

// Client returns the wrapper whose Get returns the currently-installed
// *garminconnect.Client (or nil before first login). Pass this to the
// scraper's refresh func.
func (m *Manager) Client() *garmin.Client {
	return m.client
}

func (m *Manager) attemptLogin(ctx context.Context) (time.Duration, bool) {
	tracer := otel.Tracer("garmin_exporter/auth")
	_, span := tracer.Start(ctx, "garmin.auth.login")
	defer span.End()

	garminClient, err := m.login(m.username, m.password)
	if err == nil {
		span.SetAttributes(attribute.Bool("garmin.auth.success", true))
		m.client.Set(garminClient)
		m.state.SetLoginSuccess()
		m.resetDelay()
		m.readyOnce.Do(func() { close(m.readyCh) })
		m.logger.Info("Garmin login succeeded")
		return 0, true
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, "login failed")
	span.SetAttributes(attribute.Bool("garmin.auth.success", false))

	delay := m.nextDelay()
	nextRetry := m.now().Add(delay)
	span.SetAttributes(attribute.String("garmin.auth.next_retry", nextRetry.Format(time.RFC3339)))
	m.client.Set(nil)
	m.state.SetLoginFailure(nextRetry)
	m.logger.Error("Garmin login failed", "err", err, "retry_after", delay, "next_retry", nextRetry.Unix())
	return delay, false
}
