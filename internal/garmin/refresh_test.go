package garmin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/barnes-c/go-garminconnect/garminconnect"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewRefresh_ReturnsErrNotLoggedInWhenClientNil(t *testing.T) {
	r := NewRefresh(NewClient(), discardLogger(), RefreshConfig{})
	snap, err := r(context.Background())
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
	if snap != nil {
		t.Errorf("snap = %v, want nil", snap)
	}
}

func TestNewRefresh_NoDataIsNotAFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient()
	c.Set(garminconnect.NewClient("",
		garminconnect.WithBaseURL(srv.URL),
		garminconnect.WithToken("test"),
		garminconnect.WithDisplayName("test"),
	))
	snap, err := NewRefresh(c, discardLogger(), RefreshConfig{})(context.Background())
	if err != nil {
		t.Fatalf("err = %v, want nil when Garmin has no data", err)
	}
	if snap == nil {
		t.Fatal("snap = nil, want an empty snapshot")
	}
}
