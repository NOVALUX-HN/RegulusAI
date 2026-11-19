package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/quality"
)

func qualityUsage() string {
	return "usage: regulus quality baseline --window <file> [--lab-dir <dir>] [--agent <a>] [--model <m>] [--host <h>] [--task <t>] [--version <v>] [--thresholds <file>] --out <dir> [--json] | regulus quality check --baseline <dir> --window <file> [--thresholds <file>] [--out <dir>] [--json] | regulus quality alerts --dir <dir> [--json]"
}

// QualityCmd routes `regulus quality ...`. Moved from internal/cli
// (quality.go) without behavior change.
func (io IO) QualityCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, qualityUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, qualityUsage())
		return 2
	}
	switch args[0] {
	case "baseline":
		return io.QualityBaselineCmd(args[1:])
	case "check":
		return io.QualityCheckCmd(args[1:])
	case "alerts":
		return io.QualityAlertsCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "quality: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, qualityUsage())
		return 2
	}
}

// qualitySegmentFlags carries the shared --agent/--model/--host/
// --task/--version segment flags.
type qualitySegmentFlags struct {
	agent, model, host, task, version string
}

func (s qualitySegmentFlags) segment() quality.Segment {
	return quality.Segment{
		Agent: s.agent, Model: s.model, Host: s.host,
		Task: s.task, Version: s.version,
	}
}

func addQualitySegmentFlags(fs *flag.FlagSet, s *qualitySegmentFlags) {
	fs.StringVar(&s.agent, "agent", "", "segment agent (empty = any)")
	fs.StringVar(&s.model, "model", "", "segment model (empty = any)")
	fs.StringVar(&s.host, "host", "", "segment host (empty = any)")
	fs.StringVar(&s.task, "task", "", "segment task (empty = any)")
	fs.StringVar(&s.version, "version", "", "segment version (empty = any)")
}

// resolveQualityThresholds loads explicit thresholds or the
// documented defaults when no file is given.
func resolveQualityThresholds(thPath string) (quality.Thresholds, error) {
	if thPath == "" {
		return quality.DefaultThresholds(), nil
	}
	return quality.LoadThresholds(thPath)
}

// buildQualityBaseline folds the window (plus optional Lab evidence)
// into a baseline over seg. Pure except for the clock: the same
// inputs always build the same baseline.
func buildQualityBaseline(seg quality.Segment, window, labDir string, th quality.Thresholds, now time.Time) (quality.Baseline, error) {
	obs, _, ev, err := qualityBaselineInputs(window, labDir)
	if err != nil {
		return quality.Baseline{}, err
	}
	matched, _ := quality.Select(seg, obs)
	return quality.BuildBaseline(seg, matched, th, ev, now)
}

// QualityBaselineCmd implements `regulus quality baseline`.
func (io IO) QualityBaselineCmd(args []string) int {
	var seg qualitySegmentFlags
	fs := flag.NewFlagSet("quality baseline", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	window := fs.String("window", "", "JSON window file (required)")
	labDir := fs.String("lab-dir", "", "Lab evidence dir to merge (sha-verified)")
	thPath := fs.String("thresholds", "", "thresholds JSON file (defaults documented)")
	out := fs.String("out", "", "baseline evidence dir (required)")
	asJSON := fs.Bool("json", false, "print the baseline as JSON")
	addQualitySegmentFlags(fs, &seg)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *window == "" || *out == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, qualityUsage())
		return 2
	}
	th, err := resolveQualityThresholds(*thPath)
	if err != nil {
		fmt.Fprintf(io.Stderr, "quality baseline: %v\n", err)
		return 1
	}
	b, err := buildQualityBaseline(seg.segment(), *window, *labDir, th, time.Now())
	if err != nil {
		fmt.Fprintf(io.Stderr, "quality baseline: %v\n", err)
		return 1
	}
	if _, err := quality.WriteBaselineEvidence(*out, b, gitHeadBounded(io.Runner)); err != nil {
		fmt.Fprintf(io.Stderr, "quality baseline: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(b, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "quality baseline: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "quality: baseline %s n=%d success=%.2f median=%dms -> %s\n",
		b.Segment.Key(), b.N, b.Stats.SuccessRate, b.Stats.MedianMS, *out)
	return 0
}

// qualityBaselineInputs loads the window file plus optional Lab
// evidence, tagging every observation with its provenance.
func qualityBaselineInputs(window, labDir string) ([]quality.Observation, string, []quality.EvidenceRef, error) {
	obs, windowSHA, err := quality.LoadWindow(window)
	if err != nil {
		return nil, "", nil, err
	}
	ev := []quality.EvidenceRef{{Kind: "window-file", Path: window, SHA256: windowSHA}}
	if labDir != "" {
		_, m, _, results, err := bench.ReadLabEvidence(labDir)
		if err != nil {
			return nil, "", nil, err
		}
		obs = append(obs, quality.IngestLabResults(results, "lab "+labDir)...)
		ev = append(ev, quality.EvidenceRef{
			Kind: "lab-evidence", Path: labDir,
			SHA256: m.OutcomesSHA, GitHead: m.GitHead,
		})
	}
	return obs, windowSHA, ev, nil
}

// QualityCheckCmd implements `regulus quality check`.
func (io IO) QualityCheckCmd(args []string) int {
	fs := flag.NewFlagSet("quality check", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	baseline := fs.String("baseline", "", "baseline evidence dir (required)")
	window := fs.String("window", "", "JSON window file (required)")
	thPath := fs.String("thresholds", "", "thresholds JSON file (overrides the stored ones)")
	out := fs.String("out", "", "diagnosis evidence dir (optional)")
	asJSON := fs.Bool("json", false, "print the diagnosis as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *baseline == "" || *window == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, qualityUsage())
		return 2
	}
	_, b, err := quality.ReadBaselineEvidence(*baseline)
	if err != nil {
		fmt.Fprintf(io.Stderr, "quality check: %v\n", err)
		return 1
	}
	if *thPath != "" {
		loaded, err := quality.LoadThresholds(*thPath)
		if err != nil {
			fmt.Fprintf(io.Stderr, "quality check: %v\n", err)
			return 1
		}
		b.Thresholds = loaded
	}
	obs, windowSHA, err := quality.LoadWindow(*window)
	if err != nil {
		fmt.Fprintf(io.Stderr, "quality check: %v\n", err)
		return 1
	}
	d := quality.Check(b, obs, *baseline, *window)
	if *out != "" {
		if _, err := quality.WriteDiagnosisEvidence(*out, d, shortQualitySHA(*baseline), windowSHA); err != nil {
			fmt.Fprintf(io.Stderr, "quality check: %v\n", err)
			return 1
		}
	}
	if *asJSON {
		raw, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "quality check: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, quality.RenderDiagnosis(d))
	return 0
}

// QualityAlertsCmd implements `regulus quality alerts`.
func (io IO) QualityAlertsCmd(args []string) int {
	fs := flag.NewFlagSet("quality alerts", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	dir := fs.String("dir", "", "diagnosis evidence dir (required)")
	asJSON := fs.Bool("json", false, "print the stored alerts as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dir == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, qualityUsage())
		return 2
	}
	_, d, err := quality.ReadDiagnosisEvidence(*dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "quality alerts: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(d.Alerts, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "quality alerts: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, quality.RenderAlerts(d))
	return 0
}

// shortQualitySHA cites the baseline input by its manifest sha.
func shortQualitySHA(baselineDir string) string {
	raw, err := os.ReadFile(baselineDir + string(os.PathSeparator) + "manifest.json")
	if err != nil {
		return baselineDir
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return baselineDir
	}
	sha, _ := m["baseline_sha256"].(string)
	if len(sha) > 12 {
		return sha[:12]
	}
	if sha != "" {
		return sha
	}
	return baselineDir
}
