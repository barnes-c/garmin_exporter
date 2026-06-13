package garmin

import (
	"errors"
	"sync/atomic"

	"github.com/barnes-c/go-garminconnect/garminconnect"
)

var ErrNotLoggedIn = errors.New("garmin: not logged in")

type Client struct {
	inner atomic.Pointer[garminconnect.Client]
}

// NewClient returns an empty wrapper. Get returns nil until the first Set.
func NewClient() *Client {
	return &Client{}
}

func (c *Client) Set(inner *garminconnect.Client) {
	c.inner.Store(inner)
}

func (c *Client) Get() *garminconnect.Client {
	return c.inner.Load()
}

func (c *Client) Healthy() error {
	if c.Get() == nil {
		return ErrNotLoggedIn
	}
	return nil
}
