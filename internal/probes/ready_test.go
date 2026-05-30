package probes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/barnes-c/garmin_exporter/internal/auth"
	"github.com/barnes-c/garmin_exporter/internal/scrape"
)

func TestReadyz(t *testing.T) {
	tests := []struct {
		name        string
		setupAuth   func(*auth.State)
		setupScrape func(*scrape.Outcome)
		wantStatus  int
		wantBody    string
	}{
		{
			name:        "not authenticated",
			setupAuth:   func(*auth.State) {},
			setupScrape: func(*scrape.Outcome) {},
			wantStatus:  http.StatusServiceUnavailable,
			wantBody:    "not authenticated",
		},
		{
			name:        "login failed",
			setupAuth:   func(s *auth.State) { s.SetLoginFailure(time.Now().Add(time.Minute)) },
			setupScrape: func(*scrape.Outcome) {},
			wantStatus:  http.StatusServiceUnavailable,
			wantBody:    "not authenticated",
		},
		{
			name:        "authenticated, no scrape yet",
			setupAuth:   func(s *auth.State) { s.SetLoginSuccess() },
			setupScrape: func(*scrape.Outcome) {},
			wantStatus:  http.StatusOK,
			wantBody:    "ok",
		},
		{
			name:        "authenticated, last scrape succeeded",
			setupAuth:   func(s *auth.State) { s.SetLoginSuccess() },
			setupScrape: func(s *scrape.Outcome) { s.Record(true) },
			wantStatus:  http.StatusOK,
			wantBody:    "ok",
		},
		{
			name:        "authenticated, last scrape failed",
			setupAuth:   func(s *auth.State) { s.SetLoginSuccess() },
			setupScrape: func(s *scrape.Outcome) { s.Record(false) },
			wantStatus:  http.StatusServiceUnavailable,
			wantBody:    "last scrape failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authState := auth.NewState()
			scrapeOutcome := scrape.NewOutcome()
			tc.setupAuth(authState)
			tc.setupScrape(scrapeOutcome)

			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			Readyz(authState, scrapeOutcome).ServeHTTP(rr, req)

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
