package garmin

import (
	"errors"
	"testing"

	"github.com/barnes-c/go-garminconnect/garminconnect"
)

func TestClient_GetReturnsNilBeforeSet(t *testing.T) {
	c := NewClient()
	if got := c.Get(); got != nil {
		t.Errorf("Get before Set = %v, want nil", got)
	}
}

func TestClient_SetGetRoundtrip(t *testing.T) {
	c := NewClient()
	inner := &garminconnect.Client{}
	c.Set(inner)
	if got := c.Get(); got != inner {
		t.Errorf("Get after Set = %v, want %v", got, inner)
	}
}

func TestClient_SetNilClears(t *testing.T) {
	c := NewClient()
	c.Set(&garminconnect.Client{})
	c.Set(nil)
	if got := c.Get(); got != nil {
		t.Errorf("Get after Set(nil) = %v, want nil", got)
	}
}

func TestClient_Healthy(t *testing.T) {
	c := NewClient()
	if err := c.Healthy(); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("Healthy before Set = %v, want ErrNotLoggedIn", err)
	}
	c.Set(&garminconnect.Client{})
	if err := c.Healthy(); err != nil {
		t.Errorf("Healthy after Set = %v, want nil", err)
	}
}
