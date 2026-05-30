// Package probes provides HTTP handlers for Kubernetes-style /healthz and
// /readyz endpoints.
package probes

import (
	"net/http"

	"github.com/barnes-c/garmin_exporter/internal/auth"
	"github.com/barnes-c/garmin_exporter/internal/scrape"
)

// Healthz always responds 200 OK; the process is alive.
func Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// Readyz returns 200 OK once authentication has succeeded and the most recent
// scrape produced data, otherwise 503 Service Unavailable.
func Readyz(authState *auth.State, scrapeState *scrape.State) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		loginSuccess, _ := authState.Snapshot()
		if loginSuccess != 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not authenticated\n"))
			return
		}
		if !scrapeState.Ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("last scrape failed\n"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}
}
