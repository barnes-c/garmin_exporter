package probes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealth_AlwaysOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	Health().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ok\n" {
		t.Errorf("body = %q, want %q", body, "ok\n")
	}
}

func TestReady_NoChecks_ReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	Ready(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}

func TestReady_AllChecksPass(t *testing.T) {
	checks := map[string]Checker{
		"ovsdb":   ok(),
		"unixctl": ok(),
	}
	rec := httptest.NewRecorder()
	Ready(checks).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	want := "ovsdb: ok\nunixctl: ok\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestReady_OutputIsSortedByName(t *testing.T) {
	checks := map[string]Checker{
		"zulu":    ok(),
		"alpha":   ok(),
		"mike":    ok(),
		"bravo":   ok(),
		"yankee":  ok(),
		"charlie": ok(),
	}
	rec := httptest.NewRecorder()
	Ready(checks).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	want := "alpha: ok\nbravo: ok\ncharlie: ok\nmike: ok\nyankee: ok\nzulu: ok\n"
	if got := rec.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestReady_PassesRequestContextToChecks(t *testing.T) {
	type ctxKey struct{}
	seen := make(chan any, 1)
	checks := map[string]Checker{
		"ctx": CheckerFunc(func(ctx context.Context) error {
			seen <- ctx.Value(ctxKey{})
			return nil
		}),
	}

	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
	Ready(checks).ServeHTTP(httptest.NewRecorder(), req)

	select {
	case v := <-seen:
		if v != "marker" {
			t.Errorf("ctx value seen by check = %v, want marker", v)
		}
	case <-time.After(time.Second):
		t.Fatal("check did not observe a value from the request context")
	}
}

func ok() Checker {
	return CheckerFunc(func(context.Context) error { return nil })
}

func fail(msg string) Checker {
	return CheckerFunc(func(context.Context) error { return errors.New(msg) })
}
