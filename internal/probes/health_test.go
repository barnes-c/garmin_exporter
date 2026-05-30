package probes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthz(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	Healthz(rr, req)

	if want, have := http.StatusOK, rr.Code; want != have {
		t.Errorf("want status %d, have %d", want, have)
	}
	body, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(body), "ok") {
		t.Errorf("want body to contain %q, have %q", "ok", string(body))
	}
}
