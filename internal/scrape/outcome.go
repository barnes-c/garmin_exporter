package scrape

import "time"

type Outcome struct {
	Time     time.Time
	Duration time.Duration
	Success  bool
	Err      error
}
