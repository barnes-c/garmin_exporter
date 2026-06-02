package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"

	"github.com/barnes-c/garmin_exporter/internal/auth"
	"github.com/barnes-c/garmin_exporter/internal/scrape"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

var (
	binary = filepath.Join(os.Getenv("GOPATH"), "bin/garmin_exporter")
)

const (
	address = "localhost:110043"
)

func newTestHandler(buildCollectors func() (map[string]prometheus.Collector, error)) *handler {
	scrp := scrape.New(scrape.Config{
		Logger:          discardLogger(),
		BuildCollectors: buildCollectors,
		OnScrape:        func(bool) {},
	})
	authState := auth.NewState()
	outcome := scrape.NewOutcome()
	return newHandler(false, 0, discardLogger(), authState, outcome, scrp, nil)
}

func TestRefreshHandlerMethodNotAllowed(t *testing.T) {
	h := newTestHandler(func() (map[string]prometheus.Collector, error) {
		return map[string]prometheus.Collector{}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "/refresh", nil)
	w := httptest.NewRecorder()
	h.refreshHandler(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestRefreshHandlerSuccess(t *testing.T) {
	h := newTestHandler(func() (map[string]prometheus.Collector, error) {
		return map[string]prometheus.Collector{}, nil
	})
	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	w := httptest.NewRecorder()
	h.refreshHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRefreshHandlerConflict(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	h := newTestHandler(func() (map[string]prometheus.Collector, error) {
		close(started)
		<-release
		return map[string]prometheus.Collector{}, nil
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
		h.refreshHandler(httptest.NewRecorder(), req)
	}()
	<-started

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	w := httptest.NewRecorder()
	h.refreshHandler(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}

	close(release)
	wg.Wait()
}

func TestFileDescriptorLeak(t *testing.T) {
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("garmin_exporter binary not available, try to run `make build` first: %s", err)
	}
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		t.Skipf("proc filesystem is not available, but currently required to read number of open file descriptors: %s", err)
	}
	if _, err := fs.Stat(); err != nil {
		t.Errorf("unable to read process stats: %s", err)
	}

	exporter := exec.Command(binary, "--web.listen-address", address, "--cache.ttl=24h")
	test := func(pid int) error {
		if err := queryExporter(address); err != nil {
			return err
		}
		proc, err := procfs.NewProc(pid)
		if err != nil {
			return err
		}
		fdsBefore, err := proc.FileDescriptors()
		if err != nil {
			return err
		}
		for range 5 {
			if err := queryExporter(address); err != nil {
				return err
			}
		}
		fdsAfter, err := proc.FileDescriptors()
		if err != nil {
			return err
		}
		if want, have := len(fdsBefore), len(fdsAfter); want != have {
			return fmt.Errorf("want %d open file descriptors after metrics scrape, have %d", want, have)
		}
		return nil
	}

	if err := runCommandAndTests(exporter, address, test); err != nil {
		t.Error(err)
	}
}

func queryExporter(address string) error {
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", address))
	if err != nil {
		return err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d. Body:\n%s", want, have, b)
	}
	return nil
}

func runCommandAndTests(cmd *exec.Cmd, address string, fn func(pid int) error) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %s", err)
	}
	time.Sleep(50 * time.Millisecond)
	for i := range 10 {
		if err := queryExporter(address); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if cmd.Process == nil || i == 9 {
			return fmt.Errorf("can't start command")
		}
	}

	errc := make(chan error)
	go func(pid int) {
		errc <- fn(pid)
	}(cmd.Process.Pid)

	err := <-errc
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
	return err
}
