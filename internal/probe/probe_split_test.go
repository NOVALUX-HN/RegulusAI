package probe

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/budget"
)

// D2 SPLIT: the shared cap-check (checkProbeCaps) serves Contrast and
// Compose with identical budget semantics; only the op prefix in the
// error message differs. Refusals are recorded but free: no command
// may run on refusal.

func TestSplitCheckCapsDailyRefusal(t *testing.T) {
	cfg := `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}],"evolution":{"max_per_day":1}}`
	dir := writeProject(t, cfg)
	at := time.Now().UTC()
	if err := budget.Record(dir, budget.Entry{
		Kind: budget.KindProbe, Timestamp: at, Task: "other/T9",
		Outcome: budget.OutcomeProbeRun, Detail: "fills the cap",
	}); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"probe contrast", "probe compose"} {
		err := checkProbeCaps(dir, "demo/T1", op, at)
		if err == nil || !strings.Contains(err.Error(), "evolution cap reached") {
			t.Fatalf("op %s: want daily-cap refusal, got %v", op, err)
		}
		if !strings.Contains(err.Error(), op) {
			t.Fatalf("op %s: refusal must carry the op prefix, got %v", op, err)
		}
	}
	entries := budgetEntries(t, dir)
	if len(entries) != 3 {
		t.Fatalf("want the probe plus two refusals, got %+v", entries)
	}
	for _, e := range entries[1:] {
		if e.Kind != budget.KindProbe || e.Outcome != budget.OutcomeRefusedCap {
			t.Fatalf("refusal entry = %+v, want probe/refused-cap", e)
		}
		if !strings.Contains(e.Detail, "cap 1 reached") {
			t.Fatalf("refusal detail must name the cap, got %+v", e)
		}
	}
}

func TestSplitCheckCapsTaskRefusal(t *testing.T) {
	cfg := `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}],"evolution":{"max_per_day":10,"max_per_task":1}}`
	dir := writeProject(t, cfg)
	at := time.Now().UTC()
	runner := &fakeRunner{results: map[string]fakeResult{
		"echo a":                      {stdout: "a\n"},
		"echo b":                      {stdout: "b\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	if _, _, err := ContrastWithRunner(dir, "demo/T1", "typecheck", "echo a", "echo b", at, runner); err != nil {
		t.Fatalf("first contrast: %v", err)
	}
	err := checkProbeCaps(dir, "demo/T1", "probe compose", at.Add(time.Minute))
	if err == nil || !strings.Contains(err.Error(), "task evolution cap reached") {
		t.Fatalf("want task-cap refusal across ops, got %v", err)
	}
	entries := budgetEntries(t, dir)
	last := entries[len(entries)-1]
	if last.Kind != budget.KindProbe || last.Outcome != budget.OutcomeRefusedCap {
		t.Fatalf("last entry must be the probe refusal, got %+v", last)
	}
	if !strings.Contains(last.Detail, "task cap 1 reached") {
		t.Fatalf("refusal detail must name the task cap, got %+v", last)
	}
}

func TestSplitSharedRefusalRunsNothing(t *testing.T) {
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
	if _, _, err := ContrastWithRunner(dir, "demo/T1", "typecheck", "echo a", "echo b", at, runner); err == nil {
		t.Fatal("contrast at cap must refuse")
	}
	if _, _, _, err := ComposeWithRunner(dir, "demo/T1", "typecheck", "echo base", "echo a", "echo b", at, runner); err == nil {
		t.Fatal("compose at cap must refuse")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("no command may run on cap refusal, calls=%v", runner.calls)
	}
	entries := budgetEntries(t, dir)
	if len(entries) != 3 {
		t.Fatalf("want the probe plus both refusals recorded, got %+v", entries)
	}
}

func TestSplitValidateInputs(t *testing.T) {
	if err := validateContrastInputs("", "g", "a", "b"); err == nil || !strings.Contains(err.Error(), "probe contrast") {
		t.Fatalf("contrast empty task must refuse with op prefix, got %v", err)
	}
	if err := validateComposeInputs("demo/T1", "", "base", "a", "b"); err == nil || !strings.Contains(err.Error(), "probe compose") {
		t.Fatalf("compose empty gate must refuse with op prefix, got %v", err)
	}
	if err := validateContrastInputs("demo/T1", "g", "", "b"); err == nil || !strings.Contains(err.Error(), "--a and --b") {
		t.Fatalf("contrast empty command must refuse, got %v", err)
	}
	if err := validateComposeInputs("demo/T1", "g", "", "a", "b"); err == nil || !strings.Contains(err.Error(), "--base, --a and --b") {
		t.Fatalf("compose empty base must refuse, got %v", err)
	}
	if err := validateComposeInputs("demo/T1", "typecheck", "base", "a", "b"); err != nil {
		t.Fatalf("valid compose inputs must pass, got %v", err)
	}
}
