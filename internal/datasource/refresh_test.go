package datasource

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

func TestNewOVSRefresh_BestEffort_PartialResults(t *testing.T) {
	// Fake server replies success to memory/show and coverage/show,
	// errors to dpif/show and upcall/show. The refresh should return a
	// non-nil snapshot with Memory and Coverage populated and DPIF/Upcall
	// nil. The error return must be nil (not all-failed).
	srv := startTestServer(t, map[string]string{
		"memory/show":   `"handlers:5 ofconns:1 ports:3 revalidators:1 rules:10"`,
		"coverage/show": `"Event coverage, avg rate over last: 5 seconds, last minute, last hour,  hash=abc\nfoo 0.0/sec 0.0/sec 0.0/sec   total: 7\n0 events never hit\n"`,
	})

	client, err := unixctl.New(unixctl.Config{SocketPath: srv, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("unixctl.New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	refresh := NewOVSRefresh(client, discardLogger())
	snap, err := refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if snap.Memory == nil || snap.Memory.Usage["handlers"] != 5 {
		t.Errorf("Memory not populated: %+v", snap.Memory)
	}
	if snap.Coverage == nil || snap.Coverage.Events["foo"] != 7 {
		t.Errorf("Coverage not populated: %+v", snap.Coverage)
	}
	if snap.DPIF != nil {
		t.Errorf("DPIF should be nil when call failed: %+v", snap.DPIF)
	}
	if snap.Upcall != nil {
		t.Errorf("Upcall should be nil when call failed: %+v", snap.Upcall)
	}
}

func TestNewOVSRefresh_AllFailed_ReturnsError(t *testing.T) {
	// Server replies with errors to every method.
	srv := startTestServer(t, nil)
	client, err := unixctl.New(unixctl.Config{SocketPath: srv, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("unixctl.New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	refresh := NewOVSRefresh(client, discardLogger())
	snap, err := refresh(context.Background())
	if err == nil {
		t.Fatalf("expected all-failed error; snap=%+v", snap)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startTestServer accepts JSON-RPC 1.0 requests and responds with the
// supplied per-method success result strings. Methods not in the map
// receive an error response.
func startTestServer(t *testing.T, responses map[string]string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ds-")
	if err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "f.ctl")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleConn(conn, responses)
		}
	}()
	return path
}

func handleConn(conn net.Conn, responses map[string]string) {
	defer conn.Close()
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		req := string(buf[:n])
		method := extractMethod(req)
		id := extractID(req)

		var resp string
		if result, ok := responses[method]; ok {
			resp = `{"id":` + id + `,"result":` + result + `,"error":null}`
		} else {
			resp = `{"id":` + id + `,"result":null,"error":"unknown method"}`
		}
		if _, err := conn.Write([]byte(resp)); err != nil {
			return
		}
	}
}

func extractMethod(req string) string {
	const tag = `"method":"`
	i := indexOf(req, tag)
	if i < 0 {
		return ""
	}
	rest := req[i+len(tag):]
	j := indexOf(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func extractID(req string) string {
	const tag = `"id":`
	i := indexOf(req, tag)
	if i < 0 {
		return "0"
	}
	rest := req[i+len(tag):]
	end := 0
	for end < len(rest) && (rest[end] == '-' || (rest[end] >= '0' && rest[end] <= '9')) {
		end++
	}
	if end == 0 {
		return "0"
	}
	return rest[:end]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
