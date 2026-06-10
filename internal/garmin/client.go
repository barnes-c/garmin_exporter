// Package garmin wraps the github.com/barnes-c/go-garminconnect/garminconnect
// client with the supporting types the exporter needs: an atomic handle that
// can be swapped on re-auth, a typed Snapshot consumed by collectors, and a
// best-effort Refresh function that drives one Garmin API fan-out per scrape
// tick.
package garmin

import (
	"errors"
	"sync/atomic"

	"github.com/barnes-c/go-garminconnect/garminconnect"
)

// ErrNotLoggedIn is returned by Refresh when the auth manager has not yet
// installed a client. Distinct sentinel so callers (probes, logs) can tell
// "no login yet" from "Garmin is failing".
var ErrNotLoggedIn = errors.New("garmin: not logged in")

// Client is a thin atomic.Pointer wrapper around *garminconnect.Client. The
// auth manager calls Set after a successful login; the refresh func reads
// the current pointer via Get on every tick so a re-auth-driven client swap
// is picked up without restarting the scraper.
type Client struct {
	inner atomic.Pointer[garminconnect.Client]
}

// NewClient returns an empty wrapper. Get returns nil until the first Set.
func NewClient() *Client {
	return &Client{}
}

// Set installs c as the current client. Safe to call concurrently with Get.
// A nil c clears the handle (used by auth on failed re-login).
func (c *Client) Set(inner *garminconnect.Client) {
	c.inner.Store(inner)
}

// Get returns the current client, or nil before the first successful login.
func (c *Client) Get() *garminconnect.Client {
	return c.inner.Load()
}

// Healthy returns nil when a client is installed and ErrNotLoggedIn
// otherwise. Plugs into the probes.Checker interface so /readyz can use it
// directly without a wrapper closure.
func (c *Client) Healthy() error {
	if c.Get() == nil {
		return ErrNotLoggedIn
	}
	return nil
}
