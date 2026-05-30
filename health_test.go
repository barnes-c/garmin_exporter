package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthzHandler(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	healthzHandler(rr, req)

	if want, have := http.StatusOK, rr.Code; want != have {
		t.Errorf("want status %d, have %d", want, have)
	}
	body, _ := io.ReadAll(rr.Body)
	if !strings.Contains(string(body), "ok") {
		t.Errorf("want body to contain %q, have %q", "ok", string(body))
	}
}

func TestReadyzHandler(t *testing.T) {
	tests := []struct {
		name        string
		setupAuth   func(*authState)
		setupScrape func(*scrapeState)
		wantStatus  int
		wantBody    string
	}{
		{
			name:        "not authenticated",
			setupAuth:   func(*authState) {},
			setupScrape: func(*scrapeState) {},
			wantStatus:  http.StatusServiceUnavailable,
			wantBody:    "not authenticated",
		},
		{
			name:        "login failed",
			setupAuth:   func(s *authState) { s.setLoginFailure(time.Now().Add(time.Minute)) },
			setupScrape: func(*scrapeState) {},
			wantStatus:  http.StatusServiceUnavailable,
			wantBody:    "not authenticated",
		},
		{
			name:        "authenticated, no scrape yet",
			setupAuth:   func(s *authState) { s.setLoginSuccess() },
			setupScrape: func(*scrapeState) {},
			wantStatus:  http.StatusOK,
			wantBody:    "ok",
		},
		{
			name:        "authenticated, last scrape succeeded",
			setupAuth:   func(s *authState) { s.setLoginSuccess() },
			setupScrape: func(s *scrapeState) { s.record(true) },
			wantStatus:  http.StatusOK,
			wantBody:    "ok",
		},
		{
			name:        "authenticated, last scrape failed",
			setupAuth:   func(s *authState) { s.setLoginSuccess() },
			setupScrape: func(s *scrapeState) { s.record(false) },
			wantStatus:  http.StatusServiceUnavailable,
			wantBody:    "last scrape failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			auth := newAuthState()
			scrape := newScrapeState()
			tc.setupAuth(auth)
			tc.setupScrape(scrape)

			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			readyzHandler(auth, scrape).ServeHTTP(rr, req)

			if want, have := tc.wantStatus, rr.Code; want != have {
				t.Errorf("want status %d, have %d", want, have)
			}
			body, _ := io.ReadAll(rr.Body)
			if !strings.Contains(string(body), tc.wantBody) {
				t.Errorf("want body to contain %q, have %q", tc.wantBody, string(body))
			}
		})
	}
}
