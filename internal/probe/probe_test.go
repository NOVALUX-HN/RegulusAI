package probe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/budget"
)

type fakeResult struct {
	stdout, stderr string
	code           int
	err            error
}

type fakeRunner struct {
	results map[string]fakeResult
	calls   []string
}

func (f *fakeRunner) Run(_ context.Context, command string) (string, string, int, error) {
	f.calls = append(f.calls, command)
	if res, ok := f.results[command]; ok {
		return res.stdout, res.stderr, res.code, res.err
	}
	return "ok", "", 0, nil
}

func writeProject(t *testing.T, cfgJSON string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "regulus.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const fixture = `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}]}`

func budgetEntries(t *testing.T, root string) []budget.Entry {
	t.Helper()
	entries, err := budget.Show(root)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestContrastIdenticalCommandsEmptyDiff(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{results: map[string]fakeResult{
		"echo same":                   {stdout: "same\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	diff, ran, err := ContrastWithRunner(dir, "demo/T1", "typecheck", "echo same", "echo same", time.Now().UTC(), runner)
	if err != nil {
		t.Fatalf("Contrast: %v", err)
	}
	if !ran {
		t.Fatal("ran must be true when both runs executed")
	}
	if diff != "" {
		t.Fatalf("identical commands must produce an empty diff, got:\n%s", diff)
	}
	entries := budgetEntries(t, dir)
	if len(entries) != 1 {
		t.Fatalf("want exactly one budget entry, got %v", entries)
	}
	e := entries[0]
	if e.Kind != budget.KindProbe || e.Outcome != budget.OutcomeProbeRun {
		t.Fatalf("entry = %+v, want kind probe outcome probe-run", e)
	}
	if e.Task != "demo/T1" || !strings.Contains(e.Detail, "sha256") {
		t.Fatalf("entry must carry the task and the diff digest, got %+v", e)
	}
	// both runs wrote real receipts under the task slug dir
	found, _ := filepath.Glob(filepath.Join(dir, ".regulus", "evidence", "demo__T1", "*.json"))
	if len(found) != 2 {
		t.Fatalf("want 2 real receipts, got %v", found)
	}
}

func TestContrastDifferingStdoutDiffContainsBothSides(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{results: map[string]fakeResult{
		"echo hi":                     {stdout: "hi\n"},
		"echo bye":                    {stdout: "bye\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	diff, ran, err := ContrastWithRunner(dir, "demo/T1", "typecheck", "echo hi", "echo bye", time.Now().UTC(), runner)
	if err != nil {
		t.Fatalf("Contrast: %v", err)
	}
	if !ran {
		t.Fatal("ran must be true when both runs executed")
	}
	if diff == "" {
		t.Fatal("differing commands must produce a non-empty diff")
	}
	for _, want := range []string{"echo hi", "echo bye", "hi", "bye", "- A:", "+ B:"} {
		if !strings.Contains(diff, want) {
			t.Fatalf("diff must contain %q, got:\n%s", want, diff)
		}
	}
	entries := budgetEntries(t, dir)
	if len(entries) != 1 || entries[0].Outcome != budget.OutcomeProbeRun {
		t.Fatalf("differing runs are evidence: want one probe-run entry, got %v", entries)
	}
}

func TestContrastExitCodeDiff(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{results: map[string]fakeResult{
		"echo ok":                     {},
		"exit-two":                    {code: 2, stderr: "boom"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	diff, ran, err := ContrastWithRunner(dir, "demo/T1", "typecheck", "echo ok", "exit-two", time.Now().UTC(), runner)
	if err != nil {
		t.Fatalf("Contrast: %v", err)
	}
	if !ran {
		t.Fatal("a non-zero gate exit still executed: ran must be true")
	}
	if !strings.Contains(diff, "@@ exit_code @@") {
		t.Fatalf("exit-code difference must get its own section, got:\n%s", diff)
	}
	if len(budgetEntries(t, dir)) != 1 {
		t.Fatal("a failing gate is still evidence: want one probe-run entry")
	}
}

func TestContrastUnknownGateRecordsNothing(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{}
	_, ran, err := ContrastWithRunner(dir, "demo/T1", "nope", "echo a", "echo b", time.Now().UTC(), runner)
	if err == nil || !strings.Contains(err.Error(), `unknown gate "nope"`) {
		t.Fatalf("want unknown-gate error, got %v (ran=%v)", err, ran)
	}
	if ran {
		t.Fatal("ran must be false on refusal")
	}
	if len(budgetEntries(t, dir)) != 0 {
		t.Fatal("refusals must record no budget entry")
	}
}

func TestContrastEmptyCommandRecordsNothing(t *testing.T) {
	for _, tc := range [][2]string{{"", "echo b"}, {"echo a", ""}, {"", ""}, {"  ", "echo b"}} {
		dir := writeProject(t, fixture)
		runner := &fakeRunner{}
		_, ran, err := ContrastWithRunner(dir, "demo/T1", "typecheck", tc[0], tc[1], time.Now().UTC(), runner)
		if err == nil {
			t.Fatalf("cmds %q: want empty-command error", tc)
		}
		if ran {
			t.Fatalf("cmds %q: ran must be false on refusal", tc)
		}
		if len(budgetEntries(t, dir)) != 0 {
			t.Fatalf("cmds %q: refusals must record no budget entry", tc)
		}
	}
}

func TestContrastEmptyTaskOrGateRecordsNothing(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{}
	if _, _, err := ContrastWithRunner(dir, "", "typecheck", "echo a", "echo b", time.Now().UTC(), runner); err == nil {
		t.Fatal("empty task must be refused")
	}
	if _, _, err := ContrastWithRunner(dir, "demo/T1", "", "echo a", "echo b", time.Now().UTC(), runner); err == nil {
		t.Fatal("empty gate must be refused")
	}
	if _, _, err := ContrastWithRunner(dir, "../evil", "typecheck", "echo a", "echo b", time.Now().UTC(), runner); err == nil {
		t.Fatal("path-traversal task must be refused")
	}
	if len(budgetEntries(t, dir)) != 0 {
		t.Fatal("refusals must record no budget entry")
	}
}

func TestContrastMissingConfigRecordsNothing(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{}
	if _, ran, err := ContrastWithRunner(dir, "demo/T1", "typecheck", "echo a", "echo b", time.Now().UTC(), runner); err == nil || ran {
		t.Fatalf("missing regulus.json: err=%v ran=%v", err, ran)
	}
	if len(budgetEntries(t, dir)) != 0 {
		t.Fatal("run errors must record no budget entry")
	}
}

func TestContrastRefusesAtEvolutionCap(t *testing.T) {
	cfg := `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}],"evolution":{"max_per_day":1}}`
	dir := writeProject(t, cfg)
	at := time.Now().UTC()
	if err := budget.Record(dir, budget.Entry{
		Kind: budget.KindProbe, Timestamp: at, Task: "other/T9",
		Outcome: budget.OutcomeProbeRun, Detail: "fills the cap",
	}); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	_, ran, err := ContrastWithRunner(dir, "demo/T1", "typecheck", "echo a", "echo b", at, runner)
	if err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("want cap refusal, got ran=%v err=%v", ran, err)
	}
	if ran {
		t.Fatal("ran must be false on cap refusal")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("no gate command may run on cap refusal, calls=%v", runner.calls)
	}
	entries := budgetEntries(t, dir)
	if len(entries) != 2 {
		t.Fatalf("want the probe plus the refusal recorded, got %+v", entries)
	}
	last := entries[len(entries)-1]
	if last.Kind != budget.KindProbe || last.Outcome != budget.OutcomeRefusedCap {
		t.Fatalf("last entry must be the probe refusal, got %+v", last)
	}
}
