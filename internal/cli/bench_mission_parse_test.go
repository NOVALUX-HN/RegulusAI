// Parse paydown for benchMissionCmd: focused coverage for the extracted
// parse/print helpers plus the sealed-holdout and dry-run contracts.
// Messages and exit codes stay byte-identical; these tests pin the
// helper contracts so the orchestrator stays under the standard CRAP
// ceiling. Portable: no shell, only t.TempDir/filepath.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/mbench"
)

const benchMissionTestSplits = `{
  "version": "mission-splits-v1",
  "seed": 20261117,
  "exploration": ["MB1", "MB2", "MB3", "MB4", "MB5", "MB6", "MB7", "MB8", "MB9", "MB10"],
  "holdout": ["MH1", "MH2", "MH3"]
}`

// benchMissionTestCLI returns a CLI scoped to a fresh temp project
// root carrying a valid committed-style splits file.
func benchMissionTestCLI(t *testing.T) (CLI, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bench"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bench", "mission-splits-v1.json"), []byte(benchMissionTestSplits), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	return CLI{Version: "test", Stdout: &out, Stderr: &errb, Dir: root}, &out, &errb
}

func TestParseBenchMissionArgsDefaults(t *testing.T) {
	var errb bytes.Buffer
	opts, code, ok := parseBenchMissionArgs(nil, &errb)
	if !ok || code != 0 {
		t.Fatalf("parse = (%d, %v), want (0, true)", code, ok)
	}
	if opts.split != "exploration" {
		t.Fatalf("split = %q, want exploration", opts.split)
	}
	if opts.holdout || opts.dryRun || opts.asJSON || opts.save != "" {
		t.Fatalf("defaults = %+v, want zero flags", opts)
	}
}

func TestParseBenchMissionArgsRejects(t *testing.T) {
	var errb bytes.Buffer
	if _, code, ok := parseBenchMissionArgs([]string{"--nope"}, &errb); ok || code != 2 {
		t.Fatalf("bad flag = (%d, %v), want (2, false)", code, ok)
	}
	var errb2 bytes.Buffer
	if _, code, ok := parseBenchMissionArgs([]string{"extra"}, &errb2); ok || code != 2 {
		t.Fatalf("positional = (%d, %v), want (2, false)", code, ok)
	}
	if !strings.Contains(errb2.String(), "usage: regulus bench mission") {
		t.Fatalf("positional must print usage, got %q", errb2.String())
	}
}

func TestParseBenchMissionArgsFlags(t *testing.T) {
	var errb bytes.Buffer
	opts, code, ok := parseBenchMissionArgs([]string{"--split", "all", "--holdout", "--dry-run"}, &errb)
	if !ok || code != 0 {
		t.Fatalf("parse = (%d, %v), want (0, true)", code, ok)
	}
	if opts.split != "all" || !opts.holdout || !opts.dryRun {
		t.Fatalf("opts = %+v, want split=all holdout dry-run", opts)
	}
}

func TestBenchMissionCmdSealedHoldout(t *testing.T) {
	for _, split := range []string{"holdout", "all"} {
		c, _, errb := benchMissionTestCLI(t)
		if code := c.benchMissionCmd([]string{"--split", split}); code != 2 {
			t.Fatalf("split=%s without --holdout = %d, want 2", split, code)
		} else if !strings.Contains(errb.String(), "includes sealed holdout") {
			t.Fatalf("split=%s must name the seal, got %q", split, errb.String())
		}
	}
}

func TestBenchMissionCmdDryRunListsWithoutExecuting(t *testing.T) {
	c, out, errb := benchMissionTestCLI(t)
	if code := c.benchMissionCmd([]string{"--dry-run"}); code != 0 {
		t.Fatalf("dry-run = %d, want 0 (stderr %q)", code, errb.String())
	}
	got := out.String()
	if !strings.Contains(got, "dry-run") || !strings.Contains(got, "no execution") {
		t.Fatalf("dry-run must list without executing, got %q", got)
	}
	if !strings.Contains(got, "MB1") {
		t.Fatalf("dry-run must list exploration cases, got %q", got)
	}
	if strings.Contains(got, "MH1") {
		t.Fatalf("exploration dry-run must not list holdout, got %q", got)
	}
}

func TestBenchMissionCmdDryRunHoldoutWithFlag(t *testing.T) {
	c, out, errb := benchMissionTestCLI(t)
	if code := c.benchMissionCmd([]string{"--split", "holdout", "--holdout", "--dry-run"}); code != 0 {
		t.Fatalf("holdout dry-run = %d, want 0 (stderr %q)", code, errb.String())
	}
	if !strings.Contains(out.String(), "MH1") {
		t.Fatalf("holdout dry-run must list sealed cases, got %q", out.String())
	}
}

func TestPrintBenchMissionText(t *testing.T) {
	var out, errb bytes.Buffer
	corpus := mbench.MissionCorpus()
	v := mbench.Aggregate(nil, corpus)
	saved := benchMissionSaved{Verdict: v}
	if rc := printBenchMission(&out, &errb, saved, nil, v, "", false); rc != 0 {
		t.Fatalf("text rc = %d, want 0", rc)
	}
	if !strings.Contains(out.String(), "mission gate") {
		t.Fatalf("text must quote the gate, got %q", out.String())
	}
}

func TestPrintBenchMissionJSON(t *testing.T) {
	var out, errb bytes.Buffer
	corpus := mbench.MissionCorpus()
	v := mbench.Aggregate(nil, corpus)
	saved := benchMissionSaved{Verdict: v}
	if rc := printBenchMission(&out, &errb, saved, nil, v, "", true); rc != 0 {
		t.Fatalf("json rc = %d, want 0", rc)
	}
	if !strings.Contains(out.String(), `"Decision"`) {
		t.Fatalf("json must carry the verdict, got %q", out.String())
	}
}

func TestPrintBenchMissionSave(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "saved.json")
	var out, errb bytes.Buffer
	corpus := mbench.MissionCorpus()
	v := mbench.Aggregate(nil, corpus)
	saved := benchMissionSaved{Verdict: v}
	if rc := printBenchMission(&out, &errb, saved, nil, v, save, false); rc != 0 {
		t.Fatalf("save rc = %d, want 0 (stderr %q)", rc, errb.String())
	}
	raw, err := os.ReadFile(save)
	if err != nil {
		t.Fatalf("save must write the file: %v", err)
	}
	if !strings.Contains(string(raw), `"Decision"`) {
		t.Fatalf("saved payload must carry the verdict, got %q", raw)
	}
	if !strings.Contains(out.String(), "evidence: verdict+results saved to") {
		t.Fatalf("save must print the evidence line, got %q", out.String())
	}
}
