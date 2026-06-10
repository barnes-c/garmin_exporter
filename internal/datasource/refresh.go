package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/barnes-c/ovs-exporter/internal/scrape"
	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

// NewOVSRefresh returns a scrape.RefreshFunc that calls every
// ovs-vswitchd appctl method backing a default-on collector and
// assembles the parsed results into an OVSSnapshot.
//
// Failures are best-effort: a per-method transport or parse error logs
// at debug and leaves the corresponding snapshot field nil. The whole
// refresh only fails (marking scrape.Outcome.Success=false) when every
// attempted call failed — that's the signal worth firing readyz on.
func NewOVSRefresh(client *unixctl.Client, log *slog.Logger) scrape.RefreshFunc[unixctl.OVSSnapshot] {
	return func(ctx context.Context) (*unixctl.OVSSnapshot, error) {
		snap := &unixctl.OVSSnapshot{}
		attempts := 0
		failures := 0

		call := func(method string, store func(json.RawMessage) error) {
			attempts++
			raw, err := client.Call(ctx, method)
			if err != nil {
				log.Debug("unixctl call failed", "method", method, "err", err)
				failures++
				return
			}
			if err := store(raw); err != nil {
				log.Debug("unixctl parse failed", "method", method, "err", err)
				failures++
			}
		}

		call("coverage/show", func(raw json.RawMessage) error {
			cov, err := unixctl.ParseCoverage(raw)
			if err == nil {
				snap.Coverage = cov
			}
			return err
		})
		call("memory/show", func(raw json.RawMessage) error {
			mem, err := unixctl.ParseMemory(raw)
			if err == nil {
				snap.Memory = mem
			}
			return err
		})
		call("dpif/show", func(raw json.RawMessage) error {
			dp, err := unixctl.ParseDPIF(raw)
			if err == nil {
				snap.DPIF = dp
			}
			return err
		})
		call("dpctl/show", func(raw json.RawMessage) error {
			dp, err := unixctl.ParseDPCTL(raw)
			if err == nil {
				snap.DPCTL = dp
			}
			return err
		})
		call("upcall/show", func(raw json.RawMessage) error {
			up, err := unixctl.ParseUpcall(raw)
			if err == nil {
				snap.Upcall = up
			}
			return err
		})

		if attempts > 0 && failures == attempts {
			return nil, fmt.Errorf("unixctl: all %d ovs-vswitchd appctl methods failed", failures)
		}
		return snap, nil
	}
}
