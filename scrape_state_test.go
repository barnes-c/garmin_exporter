package main

import "testing"

func TestScrapeStateReadyBeforeFirstScrape(t *testing.T) {
	s := newScrapeState()
	if !s.ready() {
		t.Fatal("expected scrape state to be ready before the first scrape")
	}
}

func TestScrapeStateRecordSuccess(t *testing.T) {
	s := newScrapeState()
	s.record(true)

	if !s.ready() {
		t.Fatal("expected ready after a successful scrape")
	}
	if !s.recorded || !s.succeeded {
		t.Fatalf("unexpected state: recorded=%v succeeded=%v", s.recorded, s.succeeded)
	}
}

func TestScrapeStateRecordFailure(t *testing.T) {
	s := newScrapeState()
	s.record(false)

	if s.ready() {
		t.Fatal("expected not ready after a failed scrape")
	}
}

func TestScrapeStateRecoversAfterFailure(t *testing.T) {
	s := newScrapeState()
	s.record(false)
	s.record(true)
	if !s.ready() {
		t.Fatal("expected ready after a recovery scrape")
	}
}
