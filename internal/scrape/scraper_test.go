package scrape

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeSnap struct {
	gen int
}

func TestNew_ValidatesConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config[fakeSnap]
	}{
		{"missing name", Config[fakeSnap]{Refresh: nullRefresh, Logger: discardLogger()}},
		{"missing refresh", Config[fakeSnap]{Name: "x", Logger: discardLogger()}},
		{"missing logger", Config[fakeSnap]{Name: "x", Refresh: nullRefresh}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func nullRefresh(context.Context) (*fakeSnap, error) {
	return &fakeSnap{}, nil
}

func TestRefresh_StoresSnapshotOnSuccess(t *testing.T) {
	var gen atomic.Int64
	s, err := New(Config[fakeSnap]{
		Name:   "ovs",
		Logger: discardLogger(),
		Refresh: func(context.Context) (*fakeSnap, error) {
			return &fakeSnap{gen: int(gen.Add(1))}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if got := s.Snapshot(); got != nil {
		t.Errorf("Snapshot before first refresh = %+v, want nil", got)
	}

	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got := s.Snapshot(); got == nil || got.gen != 1 {
		t.Errorf("Snapshot after first refresh = %+v, want {gen:1}", got)
	}
	if o := s.Outcome(); !o.Success || o.Err != nil {
		t.Errorf("Outcome = %+v, want success", o)
	}
}

func TestRefresh_PreservesSnapshotOnError(t *testing.T) {
	var fail atomic.Bool
	s, err := New(Config[fakeSnap]{
		Name:   "ovs",
		Logger: discardLogger(),
		Refresh: func(context.Context) (*fakeSnap, error) {
			if fail.Load() {
				return nil, errors.New("boom")
			}
			return &fakeSnap{gen: 42}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	first := s.Snapshot()

	fail.Store(true)
	if err := s.Refresh(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if got := s.Snapshot(); got != first {
		t.Errorf("Snapshot after failed refresh = %+v, want preserved %+v", got, first)
	}
	if o := s.Outcome(); o.Success {
		t.Error("Outcome.Success should be false after failure")
	}
}

func TestRun_RefreshesOnInterval(t *testing.T) {
	var calls atomic.Int64
	s, err := New(Config[fakeSnap]{
		Name:     "ovs",
		Interval: 5 * time.Millisecond,
		Logger:   discardLogger(),
		Refresh: func(context.Context) (*fakeSnap, error) {
			calls.Add(1)
			return &fakeSnap{}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	// Sleep long enough for the initial refresh + at least 2 ticks.
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}

	if n := calls.Load(); n < 3 {
		t.Errorf("calls = %d, want >= 3 (initial + at least 2 ticks)", n)
	}
}

func TestRun_StopsOnContextCancel(t *testing.T) {
	s, err := New(Config[fakeSnap]{
		Name:     "ovs",
		Interval: time.Minute,
		Logger:   discardLogger(),
		Refresh:  nullRefresh,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestStale_NoScrapeYet(t *testing.T) {
	s, _ := New(Config[fakeSnap]{Name: "x", Refresh: nullRefresh, Logger: discardLogger()})
	if err := s.Stale(time.Minute); err == nil {
		t.Error("expected error before any scrape attempt")
	}
}

func TestStale_LastFailed(t *testing.T) {
	s, _ := New(Config[fakeSnap]{
		Name:   "x",
		Logger: discardLogger(),
		Refresh: func(context.Context) (*fakeSnap, error) {
			return nil, errors.New("boom")
		},
	})
	_ = s.Refresh(context.Background())
	if err := s.Stale(time.Minute); err == nil {
		t.Error("expected error after failed scrape")
	}
}

func TestStale_FreshSucceeds(t *testing.T) {
	s, _ := New(Config[fakeSnap]{Name: "x", Refresh: nullRefresh, Logger: discardLogger()})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if err := s.Stale(time.Minute); err != nil {
		t.Errorf("Stale = %v, want nil for fresh successful scrape", err)
	}
}

func TestStale_TooOld(t *testing.T) {
	s, _ := New(Config[fakeSnap]{Name: "x", Refresh: nullRefresh, Logger: discardLogger()})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	// Zero maxAge: any positive age qualifies as stale.
	time.Sleep(2 * time.Millisecond)
	if err := s.Stale(time.Microsecond); err == nil {
		t.Error("expected stale error when age exceeds maxAge")
	}
}

func TestSnapshot_ConcurrentReadsDuringRefresh(t *testing.T) {
	// Refresh is intentionally slow; concurrent Snapshot() calls must not
	// block on the same mutex (atomic.Pointer guarantees lock-free reads).
	gate := make(chan struct{})
	release := make(chan struct{})
	s, err := New(Config[fakeSnap]{
		Name:   "ovs",
		Logger: discardLogger(),
		Refresh: func(context.Context) (*fakeSnap, error) {
			close(gate)
			<-release
			return &fakeSnap{gen: 1}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = s.Refresh(context.Background())
	}()

	<-gate
	// Refresh is parked. Multiple Snapshot() calls must succeed immediately.
	deadline := time.After(50 * time.Millisecond)
	readsDone := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			_ = s.Snapshot()
		}
		close(readsDone)
	}()
	select {
	case <-readsDone:
	case <-deadline:
		t.Fatal("Snapshot() reads blocked during in-flight Refresh")
	}
	close(release)
	wg.Wait()
}
