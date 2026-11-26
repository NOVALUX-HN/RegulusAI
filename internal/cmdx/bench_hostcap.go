package cmdx

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/hostcfg"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/pxexec"
)

// BenchHostCapabilitiesCmd implements `regulus bench
// host-capabilities`. Moved from internal/cli (bench_hostcap.go)
// without behavior change.
func (io IO) BenchHostCapabilitiesCmd(args []string) int {
	fs := flag.NewFlagSet("bench host-capabilities", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	run := fs.Bool("run", false, "execute fixture journeys where the host is usable (default: probe only, no model cost)")
	model := fs.String("model", "", "model for headless hosts (host default when empty)")
	out := fs.String("out", "", "write redacted per-host evidence JSON into this directory")
	only := fs.String("host", "all", "opencode | claude | codex | devin | all")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, "usage: regulus bench host-capabilities [--run] [--model M] [--out DIR] [--host all|opencode|claude|codex|devin]")
		return 2
	}
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	execClass := hostCapExecClass(*run)

	manifests := bench.DefaultHostManifests(now)
	outcomes := probeHostJourneys(ctx, io, false, "", *only, now)
	if *run {
		io.runHostJourneyOutcomes(ctx, manifests, outcomes, *only, *model, now)
	}
	applyHostJourneyOutcomes(manifests, outcomes, *only, now)
	if err := bench.ValidateCapabilitiesAttributable(manifests); err != nil {
		fmt.Fprintf(io.Stderr, "host-capabilities: %v\n", err)
		return 1
	}
	if err := bench.ValidateNoProbeAsJourney(manifests); err != nil {
		fmt.Fprintf(io.Stderr, "host-capabilities: %v\n", err)
		return 1
	}
	if err := bench.ValidateEvidenceCoverage(manifests); err != nil {
		fmt.Fprintf(io.Stderr, "host-capabilities: %v\n", err)
		return 1
	}

	runID, traceHint := io.recordHostCapabilitiesTrace(manifests, outcomes, execClass)
	stampHostTraceHints(manifests, traceHint)
	io.printHostCapabilities(manifests, outcomes, *only, execClass, traceHint, runID)
	if *out != "" {
		if err := saveHostCapabilities(*out, manifests, execClass, traceHint, now); err != nil {
			fmt.Fprintf(io.Stderr, "host-capabilities: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(io.Stdout, "host-capabilities OK: %d host(s) con capabilities atribuidas (clase %s)\n", len(manifests), execClass)
	return 0
}

// hostCapExecClass names the execution class: journeys only run with
// --run, the default probe costs no model. Pure.
func hostCapExecClass(run bool) string {
	if run {
		return bench.ClassJourney
	}
	return bench.ClassProbe
}

// runHostJourneyOutcomes executes the fixture journey for every usable
// host under --run, skipping strategy hosts (no CLI to run) and hosts
// whose probe already failed. Outcomes land in the shared map.
func (io IO) runHostJourneyOutcomes(ctx context.Context, manifests map[string]bench.HostManifest, outcomes map[string]bench.HostJourneyOutcome, only, model, now string) {
	for _, p := range bench.DefaultHostProbes() {
		if only != "all" && p.ID != only {
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
		outcomes[p.ID] = io.executeHostJourney(ctx, p.ID, ver, model, now)
	}
}

// applyHostJourneyOutcomes filters manifests to --host and folds fresh
// outcomes in, so dated journey evidence survives probes that never
// promote or demote. Deleting while ranging is safe in Go.
func applyHostJourneyOutcomes(manifests map[string]bench.HostManifest, outcomes map[string]bench.HostJourneyOutcome, only, now string) {
	for host, m := range manifests {
		if only != "all" && host != only {
			delete(manifests, host)
			continue
		}
		if o, ok := outcomes[host]; ok {
			manifests[host] = bench.ApplyJourneyOutcome(m, o, now)
		} else {
			manifests[host] = m
		}
	}
}

// stampHostTraceHints links every manifest to its F11.1 timeline hint.
func stampHostTraceHints(manifests map[string]bench.HostManifest, traceHint string) {
	for host, m := range manifests {
		m.TraceHint = traceHint
		manifests[host] = m
	}
}

// printHostCapabilities renders the execution class banner, the
// capability matrix, one summary per host and the trace line. The
// final OK line stays with the caller: it prints only after --out
// seals, so a failed save never reads success.
func (io IO) printHostCapabilities(manifests map[string]bench.HostManifest, outcomes map[string]bench.HostJourneyOutcome, only, execClass, traceHint, runID string) {
	if execClass == bench.ClassProbe {
		fmt.Fprintln(io.Stdout, "host-capabilities: clase de ejecución probe (sin costo de modelo; esto no es un journey)")
	} else {
		fmt.Fprintln(io.Stdout, "host-capabilities: clase de ejecución journey (fixture E2E; el agent-leg con modelo sigue siendo agent-journey credencial-gated)")
	}
	hosts := orderedHostIDs(only)
	fmt.Fprint(io.Stdout, bench.RenderCapabilityMatrix(hosts, manifests))
	for _, h := range hosts {
		m := manifests[h]
		fmt.Fprint(io.Stdout, bench.RenderManifestSummary(m))
		if o, ok := outcomes[h]; ok {
			fmt.Fprintf(io.Stdout, "hoy: %s (%s)\n", o.Status, o.Reason)
		}
	}
	if traceHint != "" {
		fmt.Fprintf(io.Stdout, "traza: %s\n", traceHint)
	} else {
		fmt.Fprintf(io.Stdout, "traza: sin vínculo F11.1 (run id no emitido: %s)\n", runID)
	}
}

// recordHostCapabilitiesTrace links the run to its F11.1 timeline.
func (io IO) recordHostCapabilitiesTrace(manifests map[string]bench.HostManifest, outcomes map[string]bench.HostJourneyOutcome, execClass string) (string, string) {
	runID, err := observability.MintRunID()
	if err != nil {
		fmt.Fprintf(io.Stderr, "host-capabilities: sin traza F11.1 (%v)\n", err)
		return "mint run id: " + err.Error(), ""
	}
	root := io.Dir
	if root == "" {
		if wd, werr := os.Getwd(); werr == nil {
			root = wd
		} else {
			fmt.Fprintln(io.Stderr, "host-capabilities: sin traza F11.1 (sin raíz de proyecto)")
			return "sin raíz de proyecto", ""
		}
	}
	emit := func(e observability.ExecEvent) {
		if _, _, rerr := observability.RecordExec(root, e, time.Now().UTC()); rerr != nil {
			fmt.Fprintf(io.Stderr, "host-capabilities: evento de traza no registrado (%v)\n", rerr)
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

// saveHostCapabilities writes the redacted evidence pack.
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

// Journey helpers duplicated trivially from internal/cli
// (bench_host.go) so host-capabilities keeps its probe/journey
// behavior without importing cli (which would be a cycle).

func probeHostJourneys(ctx context.Context, io IO, run bool, model, only, now string) map[string]bench.HostJourneyOutcome {
	results := map[string]bench.HostJourneyOutcome{}
	for _, p := range bench.DefaultHostProbes() {
		if only != "all" && p.ID != only {
			continue
		}
		results[p.ID] = probeOneHostJourney(ctx, io, p, run, model, now)
	}
	return results
}

func probeOneHostJourney(ctx context.Context, io IO, p bench.HostProbe, run bool, model, now string) bench.HostJourneyOutcome {
	ver, err := p.Version(ctx)
	r := bench.HostJourneyOutcome{Host: p.ID, Version: ver, RecordedAt: now, Windows: isWindows()}
	if err != nil {
		r.Level, r.Status, r.Reason = bench.LevelNotRun, bench.AgentNotRun, err.Error()
		return r
	}
	if !run {
		r.Level, r.Status, r.Reason = bench.LevelContract, bench.AgentNotRun, "probe only: binary present, version "+ver+" (journey not executed, no model cost)"
		return r
	}
	return io.executeHostJourney(ctx, p.ID, ver, model, now)
}

// orderedHostIDs lists the matrix rows in stable order, honoring --host.
// Duplicated from internal/cli (bench_host.go).
func orderedHostIDs(only string) []string {
	ordered := []string{}
	for _, h := range []string{"opencode", "claude", "codex", "devin"} {
		if only != "all" && h != only {
			continue
		}
		ordered = append(ordered, h)
	}
	return ordered
}

// setupJourneyWork materializes the isolated workdir a conformity
// journey runs against: a temp dir plus the offline fixture chart.
// mkTemp and fixture are injectable so tests drive both infra
// refusals without touching the real layout.
func setupJourneyWork(mkTemp func(string, string) (string, error), fixture func(string) error) (string, error) {
	work, err := mkTemp("", "host-journey-")
	if err != nil {
		return "", err
	}
	if err := fixture(work); err != nil {
		os.RemoveAll(work)
		return "", err
	}
	return work, nil
}

// runJourneySteps runs every journey step with a per-step timeout,
// stopping at the first failure: it reports the failing step name,
// its detail and the error.
func runJourneySteps(ctx context.Context, work string, steps []bench.JourneyStep) (string, string, error) {
	for _, s := range steps {
		tctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		detail, err := s.Run(tctx, work)
		cancel()
		if err != nil {
			return s.Name, detail, err
		}
	}
	return "", "", nil
}

// applyJourneyFailure folds a failed step into the outcome: the
// classifier decides fail vs not-run, the evidence stays attached,
// and a not-run downgrades the level to contract-probed (a host that
// never ran never reads host-tested).
func applyJourneyFailure(r *bench.HostJourneyOutcome, step, detail string, err error) {
	status, reason := classifyJourneyErr(step, err)
	r.Status, r.Reason = status, reason
	r.Evidence = detail
	if status == bench.AgentNotRun {
		r.Level = bench.LevelContract
	}
}

// executeHostJourney runs the conformity journey for one usable host.
// Duplicated from internal/cli (bench_host.go).
func (io IO) executeHostJourney(ctx context.Context, host, version, model, now string) bench.HostJourneyOutcome {
	r := bench.HostJourneyOutcome{Host: host, Version: version, Level: bench.LevelHost, RecordedAt: now, Windows: isWindows()}
	work, err := setupJourneyWork(os.MkdirTemp, bench.JourneyFixture)
	if err != nil {
		r.Status, r.Reason = bench.AgentInfra, err.Error()
		return r
	}
	defer os.RemoveAll(work)
	steps := hostJourneySteps(journeyBin(), work)
	if step, detail, err := runJourneySteps(ctx, work, steps); err != nil {
		applyJourneyFailure(&r, step, detail, err)
		return r
	}
	r.Status, r.Reason = bench.AgentPass, "install, doctor, context, spec, gate, receipt and close green"
	return r
}

func hostJourneySteps(selfBin, workdir string) []bench.JourneyStep {
	sub := func(args ...string) func(context.Context, string) (string, error) {
		return func(ctx context.Context, _ string) (string, error) {
			res := pxexec.Run(ctx, pxexec.Spec{Name: selfBin, Args: args, Dir: workdir})
			if res.Outcome == pxexec.OutcomePass {
				return res.Stdout, nil
			}
			if res.ErrClass == pxexec.ErrFail {
				return res.Stdout, fmt.Errorf("%s: exit status %d (%s)", strings.Join(args, " "), res.Exit, strings.TrimSpace(res.Stderr))
			}
			return res.Stdout, fmt.Errorf("%s: %s (%s)", strings.Join(args, " "), res.Detail, strings.TrimSpace(res.Stderr))
		}
	}
	return []bench.JourneyStep{
		{Name: "install", Run: sub("init", "--stack", "smoke", "--force")},
		{Name: "doctor", Run: sub("doctor")},
		{Name: "context", Run: sub("memory", "context")},
		{Name: "spec", Run: sub("spec", "check", "charts/demo.md")},
		{Name: "gate", Run: sub("gate", "run", "--yes", "demo/T1")},
		{Name: "close", Run: sub("task", "close", "charts/demo.md", "T1")},
	}
}

func journeyBin() string {
	if b := os.Getenv("REGULUS_BIN"); b != "" {
		return b
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	return "regulus"
}

func isQuotaErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "quota") || strings.Contains(msg, "insufficient balance") ||
		strings.Contains(msg, "not logged in") || strings.Contains(msg, "402")
}

func classifyJourneyErr(step string, err error) (string, string) {
	if isQuotaErr(err) {
		return bench.AgentNotRun, "host unusable at run time: " + shortErr(err)
	}
	if step == "doctor" {
		return bench.AgentNotRun, "doctor refused (runtime probe; advisory, not a journey verdict)"
	}
	return bench.AgentFail, step + ": " + shortErr(err)
}

func shortErr(err error) string {
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 300 {
		return msg[:300]
	}
	return msg
}

func isWindows() bool { return runtime.GOOS == "windows" }
