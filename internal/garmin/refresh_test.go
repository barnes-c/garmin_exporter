package garmin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
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
