// `regulus bench host-capabilities`: the F11.7 capability matrix.
// Probe every surface (no model cost) and grade the 12 capability
// dimensions per host from the declared manifests plus today's run;
// with --run, execute the fixture journey where the host is usable
// and fold green journeys into the matrix. Strategy hosts (Devin)
// never execute: there is no CLI to run, and the command says so.
//
// The execution class travels with the run and prints on its own
// banner line ("clase de ejecución probe|journey"), never inferred
// from capability states, so a probe is never reported as a journey.
// Every probada cell cites dated evidence; secrets never reach the
// evidence pack (redaction choke point in bench.NewHostEvidence);
// the F11.1 trace link prints for every run.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/hostcfg"
	"regulus-ai/internal/observability"
)

func (c CLI) benchHostCapabilitiesCmd(args []string) int {
	fs := flag.NewFlagSet("bench host-capabilities", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	run := fs.Bool("run", false, "execute fixture journeys where the host is usable (default: probe only, no model cost)")
	model := fs.String("model", "", "model for headless hosts (host default when empty)")
	out := fs.String("out", "", "write redacted per-host evidence JSON into this directory")
	only := fs.String("host", "all", "opencode | claude | codex | devin | all")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus bench host-capabilities [--run] [--model M] [--out DIR] [--host all|opencode|claude|codex|devin]")
		return 2
	}
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	execClass := bench.ClassProbe
	if *run {
		execClass = bench.ClassJourney
	}

	manifests := bench.DefaultHostManifests(now)
	outcomes := probeHostJourneys(ctx, c, false, "", *only, now)
	if *run {
		for _, p := range bench.DefaultHostProbes() {
			if *only != "all" && p.ID != *only {
				continue
			}
			m := manifests[p.ID]
			if m.Class == bench.ClassStrategy {
				continue // no CLI to run: strategy hosts never execute
			}
			ver, err := p.Version(ctx)
			if err != nil {
				continue // probe already recorded the reason
			}
			outcomes[p.ID] = c.executeHostJourney(ctx, p.ID, ver, *model, now)
		}
	}
	for host, m := range manifests {
		if *only != "all" && host != *only {
			delete(manifests, host)
			continue
		}
		if o, ok := outcomes[host]; ok {
			manifests[host] = bench.ApplyJourneyOutcome(m, o, now)
		} else {
			manifests[host] = m
		}
	}
	if err := bench.ValidateCapabilitiesAttributable(manifests); err != nil {
		fmt.Fprintf(c.Stderr, "host-capabilities: %v\n", err)
		return 1
	}
	if err := bench.ValidateNoProbeAsJourney(manifests); err != nil {
		fmt.Fprintf(c.Stderr, "host-capabilities: %v\n", err)
		return 1
	}

	runID, traceHint := c.recordHostCapabilitiesTrace(manifests, outcomes, execClass)
	for host, m := range manifests {
		m.TraceHint = traceHint
		manifests[host] = m
	}

	if execClass == bench.ClassProbe {
		fmt.Fprintln(c.Stdout, "host-capabilities: clase de ejecución probe (sin costo de modelo; esto no es un journey)")
	} else {
		fmt.Fprintln(c.Stdout, "host-capabilities: clase de ejecución journey (fixture E2E; el agent-leg con modelo sigue siendo agent-journey credencial-gated)")
	}
	hosts := orderedHostIDs(*only)
	fmt.Fprint(c.Stdout, bench.RenderCapabilityMatrix(hosts, manifests))
	for _, h := range hosts {
		m := manifests[h]
		fmt.Fprint(c.Stdout, bench.RenderManifestSummary(m))
		if o, ok := outcomes[h]; ok {
			fmt.Fprintf(c.Stdout, "hoy: %s (%s)\n", o.Status, o.Reason)
		}
	}
	if traceHint != "" {
		fmt.Fprintf(c.Stdout, "traza: %s\n", traceHint)
	} else {
		fmt.Fprintf(c.Stdout, "traza: sin vínculo F11.1 (run id no emitido: %s)\n", runID)
	}
	if *out != "" {
		if err := saveHostCapabilities(*out, manifests, execClass, traceHint, now); err != nil {
			fmt.Fprintf(c.Stderr, "host-capabilities: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(c.Stdout, "host-capabilities OK: %d host(s) con capabilities atribuidas (clase %s)\n", len(manifests), execClass)
	return 0
}

// recordHostCapabilitiesTrace links the run to its F11.1 timeline:
// one run id, run.start, one task.end per host, run.end — all through
// observability.RecordExec, which redacts secrets before the store.
// Best effort: a store failure warns on stderr and leaves the trace
// hint empty rather than failing the matrix. Returns the run id (or
// the reason it is missing) and the hint.
func (c CLI) recordHostCapabilitiesTrace(manifests map[string]bench.HostManifest, outcomes map[string]bench.HostJourneyOutcome, execClass string) (string, string) {
	runID, err := observability.MintRunID()
	if err != nil {
		fmt.Fprintf(c.Stderr, "host-capabilities: sin traza F11.1 (%v)\n", err)
		return "mint run id: " + err.Error(), ""
	}
	root := ResolveRoot("")
	if root == "" {
		if wd, werr := os.Getwd(); werr == nil {
			root = wd
		} else {
			fmt.Fprintln(c.Stderr, "host-capabilities: sin traza F11.1 (sin raíz de proyecto)")
			return "sin raíz de proyecto", ""
		}
	}
	emit := func(e observability.ExecEvent) {
		if _, _, rerr := observability.RecordExec(root, e, time.Now().UTC()); rerr != nil {
			fmt.Fprintf(c.Stderr, "host-capabilities: evento de traza no registrado (%v)\n", rerr)
		}
	}
	emit(observability.ExecEvent{
		RunID: runID, Type: observability.ExecRunStart, Status: observability.ExecStarted,
		Payload: map[string]any{"command": "bench host-capabilities", "execution_class": execClass},
	})
	for _, h := range orderedHostCapabilitiesHosts(manifests) {
		m := manifests[h]
		status := observability.ExecSucceeded
		detail := "capabilities atribuidas (clase " + m.Class + ")"
		if o, ok := outcomes[h]; ok {
			switch {
			case o.Status == bench.AgentFail:
				status = observability.ExecFailed
				detail = o.Reason
			case o.Status == bench.AgentNotRun && m.Class != bench.ClassStrategy:
				status = observability.ExecCancelled
				detail = o.Reason
			case o.Status == bench.AgentNotRun:
				detail = o.Reason
			}
		}
		emit(observability.ExecEvent{
			RunID: runID, TaskID: "host-cap/" + h,
			Type: observability.ExecTaskEnd, Status: status,
			Host: h, HostVersion: m.Version,
			Payload:      map[string]any{"capability_class": m.Class, "execution_class": execClass},
			ErrorDetails: detail,
		})
	}
	emit(observability.ExecEvent{
		RunID: runID, Type: observability.ExecRunEnd, Status: observability.ExecSucceeded,
		Payload: map[string]any{"command": "bench host-capabilities", "execution_class": execClass},
	})
	return runID, bench.TraceHintFor(runID)
}

// orderedHostCapabilitiesHosts lists manifests in stable order.
func orderedHostCapabilitiesHosts(manifests map[string]bench.HostManifest) []string {
	hosts := make([]string, 0, len(manifests))
	for h := range manifests {
		hosts = append(hosts, h)
	}
	for i := 0; i < len(hosts); i++ {
		for j := i + 1; j < len(hosts); j++ {
			if hosts[j] < hosts[i] {
				hosts[i], hosts[j] = hosts[j], hosts[i]
			}
		}
	}
	return hosts
}

// saveHostCapabilities writes the redacted evidence pack: versions,
// capabilities, reproduction commands, non-sensitive config refs and
// redacted artifacts, plus the execution class and trace link.
// Secrets never reach disk: the choke point is NewHostEvidence.
func saveHostCapabilities(out string, manifests map[string]bench.HostManifest, execClass, traceHint, now string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	pack := map[string]any{
		"execution_class": execClass,
		"trace_hint":      traceHint,
		"recorded_at":     now,
	}
	for host, m := range manifests {
		artifacts := []string{
			"host " + host + " version " + m.Version,
			"class " + m.Class,
		}
		if m.ProbeNote != "" {
			artifacts = append(artifacts, m.ProbeNote)
		}
		if m.JourneyEvidence != "" {
			artifacts = append(artifacts, m.JourneyEvidence+" ("+m.JourneyDate+")")
		}
		pack[host] = bench.NewHostEvidence(m, hostcfg.ConfigRefs(host, home), artifacts)
	}
	raw, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return err
	}
	name := fmt.Sprintf("host-capabilities-%s.json", time.Now().UTC().Format("20060102T150405Z"))
	return os.WriteFile(filepath.Join(out, name), raw, 0o644)
}
