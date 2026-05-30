package main

import "sync"

type scrapeState struct {
	mtx       sync.RWMutex
	recorded  bool
	succeeded bool
}

func newScrapeState() *scrapeState {
	return &scrapeState{}
}

func (s *scrapeState) record(success bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.recorded = true
	s.succeeded = success
}

// ready reports whether the most recent scrape succeeded. Before the first
// scrape it returns true so the readiness probe doesn't deadlock the very
// first scrape that would update this state.
func (s *scrapeState) ready() bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	if !s.recorded {
		return true
	}
	return s.succeeded
}
