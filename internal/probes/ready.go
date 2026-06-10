package probes

import (
	"context"
	"fmt"
	"net/http"
	"sort"
)

type Checker interface {
	Check(ctx context.Context) error
}

type CheckerFunc func(ctx context.Context) error

func (f CheckerFunc) Check(ctx context.Context) error { return f(ctx) }

func Ready(checks map[string]Checker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		names := make([]string, 0, len(checks))
		for name := range checks {
			names = append(names, name)
		}
		sort.Strings(names)

		results := make(map[string]string, len(checks))
		allOK := true
		for _, name := range names {
			if err := checks[name].Check(r.Context()); err != nil {
				results[name] = err.Error()
				allOK = false
				continue
			}
			results[name] = "ok"
		}

		status := http.StatusOK
		if !allOK {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		for _, name := range names {
			_, _ = fmt.Fprintf(w, "%s: %s\n", name, results[name])
		}
	})
}
