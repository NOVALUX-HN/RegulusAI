package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/observability"
)

func diagUsage() string {
	return "usage: regulus bench diag --run <id[,id...]> [--baseline <id>] [--evidence-dir <dir>] [--thresholds <file>] [--json]"
}

// BenchDiagCmd implements `regulus bench diag`. Moved from
// internal/cli (bench_diag.go) without behavior change.
func (io IO) BenchDiagCmd(args []string) int {
	opts, code, ok := parseBenchDiagFlags(args, io.Stderr)
	if !ok {
		return code
	}
	th := bench.DefaultDiagThresholds()
	if opts.thresholds != "" {
		loaded, err := bench.LoadDiagThresholds(opts.thresholds)
		if err != nil {
			fmt.Fprintf(io.Stderr, "bench diag: %v\n", err)
			return 1
		}
		th = loaded
	}
	var results []bench.LabResult
	if opts.evidenceDir != "" {
		_, _, _, rep, err := bench.ReadLabEvidence(opts.evidenceDir)
		if err != nil {
			fmt.Fprintf(io.Stderr, "bench diag: %v\n", err)
			return 1
		}
		results = rep
	}
	ids := opts.runs
	if len(ids) == 0 {
		ids = diagResultRuns(results)
	}
	if len(ids) == 0 {
		fmt.Fprintln(io.Stderr, diagUsage())
		fmt.Fprintln(io.Stderr, "bench diag: --run or --evidence-dir with Lab results is required")
		return 2
	}
	inputs, notes := io.loadDiagInputs(ids, results)
	if len(inputs) == 0 {
		fmt.Fprintln(io.Stderr, "bench diag: nothing to diagnose (no trace events, no Lab results)")
		return 1
	}
	rep := bench.Diagnose(bench.DiagInput{Runs: inputs, BaselineRunID: opts.baseline, Notes: notes}, th)
	if opts.asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "bench diag: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, bench.RenderDiagReport(rep))
	return 0
}

// benchDiagOptions carries the parsed `bench diag` flags.
type benchDiagOptions struct {
	runs        []string
	baseline    string
	evidenceDir string
	thresholds  string
	asJSON      bool
}

// parseBenchDiagFlags parses the diag flags. Failures print the
// byte-identical usage to stderr and report exit 2 with ok=false.
func parseBenchDiagFlags(args []string, stderr io.Writer) (benchDiagOptions, int, bool) {
	var opts benchDiagOptions
	fs := flag.NewFlagSet("bench diag", flag.ContinueOnError)
	fs.SetOutput(stderr)
	runs := fs.String("run", "", "comma-separated execution run id(s) from the F11.1 log (required unless --evidence-dir names runs)")
	baseline := fs.String("baseline", "", "run id of the control for the delta (must be in scope)")
	evidenceDir := fs.String("evidence-dir", "", "Lab evidence directory with config.json + outcomes.json + reports.json (durations, tokens, verdicts)")
	thresholds := fs.String("thresholds", "", "JSON thresholds file (strict; omitted keys keep defaults)")
	asJSON := fs.Bool("json", false, "print the diagnosis as JSON")
	if err := fs.Parse(args); err != nil {
		return benchDiagOptions{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, diagUsage())
		return benchDiagOptions{}, 2, false
	}
	opts = benchDiagOptions{
		runs: splitList(*runs), baseline: *baseline,
		evidenceDir: *evidenceDir, thresholds: *thresholds, asJSON: *asJSON,
	}
	return opts, 0, true
}

// diagResultRuns derives run ids from Lab results (sorted, distinct)
// when --run was not given.
func diagResultRuns(results []bench.LabResult) []string {
	seen := map[string]bool{}
	var ids []string
	for _, r := range results {
		if r.RunID == "" || seen[r.RunID] {
			continue
		}
		seen[r.RunID] = true
		ids = append(ids, r.RunID)
	}
	sort.Strings(ids)
	return ids
}

// loadDiagInputs queries the F11.1 store per run and attaches the
// Lab results with matching run ids.
func (io IO) loadDiagInputs(ids []string, results []bench.LabResult) ([]bench.DiagRunInput, []string) {
	byRun := map[string][]bench.LabResult{}
	var orphan []bench.LabResult
	for _, r := range results {
		if r.RunID == "" {
			orphan = append(orphan, r)
			continue
		}
		byRun[r.RunID] = append(byRun[r.RunID], r)
	}
	var inputs []bench.DiagRunInput
	var notes []string
	known := map[string]bool{}
	for _, id := range ids {
		known[id] = true
		events, malformed, dups, err := observability.QueryExec(io.Dir, observability.ExecFilter{RunID: id})
		if err != nil {
			notes = append(notes, fmt.Sprintf("run %s: trace load failed (%v); Lab results only", id, err))
			events = nil
		}
		inputs = append(inputs, bench.DiagRunInput{
			RunID: id, Events: events, Results: byRun[id],
			Malformed: malformed, Duplicates: dups,
		})
	}
	var extra []string
	for runID := range byRun {
		if !known[runID] {
			extra = append(extra, runID)
		}
	}
	sort.Strings(extra)
	for _, runID := range extra {
		inputs = append(inputs, bench.DiagRunInput{RunID: runID, Results: byRun[runID]})
		notes = append(notes, fmt.Sprintf("run %s: from Lab evidence only (no --run match, no trace events queried)", runID))
	}
	if len(orphan) > 0 && len(inputs) > 0 {
		inputs[0].Results = append(inputs[0].Results, orphan...)
		notes = append(notes, fmt.Sprintf("%d Lab result(s) without run id attached to run %s", len(orphan), inputs[0].RunID))
	} else if len(orphan) > 0 {
		inputs = append(inputs, bench.DiagRunInput{RunID: "(unlinked)", Results: orphan})
		notes = append(notes, "Lab result(s) without run id reported under (unlinked)")
	}
	var nonEmpty []bench.DiagRunInput
	for _, in := range inputs {
		if len(in.Events) > 0 || len(in.Results) > 0 {
			nonEmpty = append(nonEmpty, in)
		} else {
			notes = append(notes, fmt.Sprintf("run %s: no trace events and no Lab results; skipped", in.RunID))
		}
	}
	return nonEmpty, notes
}
