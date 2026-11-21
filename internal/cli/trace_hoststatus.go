// F11.5 (NOV-91): `regulus trace host-status` declares the verified
// per-host measurement state — which host reports measured tokens
// end to end and which stays unknown with its verified reason.
// The table comes from trace.DeclareHostProvenance (audit
// 2026-11-16; versions live in docs/host-usage.md), so the CLI never
// invents provenance: it prints the declared state.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"

	"regulus-ai/internal/trace"
)

// traceHostStatusCmd implements `regulus trace host-status [--json]`:
// one row per known host (opencode, claude, codex, devin) with its
// source label and verified reason. Human text by default, --json
// for tooling. The command reads no host data and spawns nothing:
// it reports the declared audit state.
func (c CLI) traceHostStatusCmd(args []string) int {
	fs := flag.NewFlagSet("trace host-status", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	asJSON := fs.Bool("json", false, "print the host provenance table as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace host-status [--json]")
		return 2
	}
	rows := make([]trace.HostProvenance, 0, len(trace.KnownHosts))
	for _, h := range trace.KnownHosts {
		rows = append(rows, trace.DeclareHostProvenance(h))
	}
	if *asJSON {
		raw, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "trace host-status: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	for _, r := range rows {
		fmt.Fprintf(c.Stdout, "host %s: %s (%s)\n", r.Host, r.Source, r.Reason)
	}
	return 0
}
