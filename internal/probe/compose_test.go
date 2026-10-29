package probe

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/budget"
)

func TestComposeIdenticalVariants(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{results: map[string]fakeResult{
		"echo same":                   {stdout: "same\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	report, conflicts, ran, err := ComposeWithRunner(dir, "demo/T1", "typecheck", "echo same", "echo same", "echo same", time.Now().UTC(), runner)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if !ran {
		t.Fatal("ran must be true when all three runs executed")
	}
	if conflicts {
		t.Fatalf("identical variants must not conflict, got:\n%s", report)
	}
	if !strings.Contains(report, "(no conflicts)") {
		t.Fatalf("report must carry an empty ## Conflicts section, got:\n%s", report)
	}
	if !strings.Contains(report, "## Composed stdout\nsame\n") {
		t.Fatalf("merged stdout must equal base stdout, got:\n%s", report)
	}
	entries := budgetEntries(t, dir)
	if len(entries) != 1 {
		t.Fatalf("want exactly one budget entry, got %v", entries)
	}
	e := entries[0]
	if e.Kind != budget.KindProbe || e.Outcome != budget.OutcomeProbeRun {
		t.Fatalf("entry = %+v, want kind probe outcome probe-run", e)
	}
	if e.Task != "demo/T1" || !strings.Contains(e.Detail, "sha256") || !strings.Contains(e.Detail, "compose") {
		t.Fatalf("entry must carry the task and the composed-report digest, got %+v", e)
	}
	if len(runner.calls) != 4 { // base + A + B + git head
		t.Fatalf("all three variants must run, calls=%v", runner.calls)
	}
}

func TestComposeDisjointStdoutMerges(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{results: map[string]fakeResult{
		"echo base":                   {stdout: "l1\nl2\nl3\n"},
		"echo a":                      {stdout: "A1\nl2\nl3\n"},
		"echo b":                      {stdout: "l1\nl2\nB3\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	report, conflicts, ran, err := ComposeWithRunner(dir, "demo/T1", "typecheck", "echo base", "echo a", "echo b", time.Now().UTC(), runner)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if !ran {
		t.Fatal("ran must be true when all three runs executed")
	}
	if conflicts {
		t.Fatalf("disjoint changes must merge without conflict, got:\n%s", report)
	}
	if !strings.Contains(report, "## Composed stdout\nA1\nl2\nB3\n") {
		t.Fatalf("merged stdout must carry both disjoint changes, got:\n%s", report)
	}
	if len(budgetEntries(t, dir)) != 1 {
		t.Fatal("a clean merge is evidence: want one probe-run entry")
	}
}

func TestComposeSameChangeInBothIsNotAConflict(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{results: map[string]fakeResult{
		"echo base":                   {stdout: "l1\nl2\nl3\n"},
		"echo a":                      {stdout: "X\nl2\nl3\n"},
		"echo b":                      {stdout: "X\nl2\nl3\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	report, conflicts, ran, err := ComposeWithRunner(dir, "demo/T1", "typecheck", "echo base", "echo a", "echo b", time.Now().UTC(), runner)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if !ran {
		t.Fatal("ran must be true when all three runs executed")
	}
	if conflicts {
		t.Fatalf("the same change in both variants is not a conflict, got:\n%s", report)
	}
	if !strings.Contains(report, "## Composed stdout\nX\nl2\nl3\n") {
		t.Fatalf("the shared change must win the merge, got:\n%s", report)
	}
}

func TestComposeConflictingStdoutAndExit(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{results: map[string]fakeResult{
		"echo base":                   {stdout: "out-base\n", code: 0},
		"echo a":                      {stdout: "out-a\n", code: 1},
		"echo b":                      {stdout: "out-b\n", code: 2},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	report, conflicts, ran, err := ComposeWithRunner(dir, "demo/T1", "typecheck", "echo base", "echo a", "echo b", time.Now().UTC(), runner)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if !ran {
		t.Fatal("conflicts are evidence: ran must be true")
	}
	if !conflicts {
		t.Fatalf("differing changes to the same lines must conflict, got:\n%s", report)
	}
	if !strings.Contains(report, "## Composed exit_code\n0\n") {
		t.Fatalf("a conflicted field must keep the base value, got:\n%s", report)
	}
	if !strings.Contains(report, "- exit_code: base 0, A 1, B 2") {
		t.Fatalf("exit_code conflict must name base and both variants, got:\n%s", report)
	}
	for _, want := range []string{"out-base", "out-a", "out-b"} {
		if !strings.Contains(report, want) {
			t.Fatalf("stdout conflict must record base and both variants (%q missing), got:\n%s", want, report)
		}
	}
	if !strings.Contains(report, "## Composed stdout\nout-base\n") {
		t.Fatalf("conflicted stdout must keep the base value, got:\n%s", report)
	}
	entries := budgetEntries(t, dir)
	if len(entries) != 1 || entries[0].Outcome != budget.OutcomeProbeRun {
		t.Fatalf("conflicts are evidence: want one probe-run entry, got %v", entries)
	}
}

func TestComposeUnknownGateRecordsNothing(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{}
	_, _, ran, err := ComposeWithRunner(dir, "demo/T1", "nope", "echo base", "echo a", "echo b", time.Now().UTC(), runner)
	if err == nil || !strings.Contains(err.Error(), `unknown gate "nope"`) {
		t.Fatalf("want unknown-gate error, got %v (ran=%v)", err, ran)
	}
	if ran {
		t.Fatal("ran must be false on refusal")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("no command may run on refusal, calls=%v", runner.calls)
	}
	if len(budgetEntries(t, dir)) != 0 {
		t.Fatal("refusals must record no budget entry")
	}
}

func TestComposeEmptyCommandsRecordNothing(t *testing.T) {
	for _, tc := range [][3]string{
		{"", "echo a", "echo b"},
		{"echo base", "", "echo b"},
		{"echo base", "echo a", ""},
		{"  ", "echo a", "echo b"},
	} {
		dir := writeProject(t, fixture)
		runner := &fakeRunner{}
		_, _, ran, err := ComposeWithRunner(dir, "demo/T1", "typecheck", tc[0], tc[1], tc[2], time.Now().UTC(), runner)
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

func TestComposeBadTaskOrGateRecordsNothing(t *testing.T) {
	dir := writeProject(t, fixture)
	runner := &fakeRunner{}
	if _, _, _, err := ComposeWithRunner(dir, "", "typecheck", "echo base", "echo a", "echo b", time.Now().UTC(), runner); err == nil {
		t.Fatal("empty task must be refused")
	}
	if _, _, _, err := ComposeWithRunner(dir, "../evil", "typecheck", "echo base", "echo a", "echo b", time.Now().UTC(), runner); err == nil {
		t.Fatal("path-traversal task must be refused")
	}
	if _, _, _, err := ComposeWithRunner(dir, "demo/T1", "", "echo base", "echo a", "echo b", time.Now().UTC(), runner); err == nil {
		t.Fatal("empty gate must be refused")
	}
	if len(budgetEntries(t, dir)) != 0 {
		t.Fatal("refusals must record no budget entry")
	}
}

func TestComposeMissingConfigRecordsNothing(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{}
	if _, _, ran, err := ComposeWithRunner(dir, "demo/T1", "typecheck", "echo base", "echo a", "echo b", time.Now().UTC(), runner); err == nil || ran {
		t.Fatalf("missing regulus.json: err=%v ran=%v", err, ran)
	}
	if len(budgetEntries(t, dir)) != 0 {
		t.Fatal("run errors must record no budget entry")
	}
}

func TestComposeEnforcesTaskCap(t *testing.T) {
	cfg := `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}],"evolution":{"max_per_day":10,"max_per_task":1}}`
	dir := writeProject(t, cfg)
	at := time.Now().UTC()
	runner := &fakeRunner{results: map[string]fakeResult{
		"echo base":                   {stdout: "base\n"},
		"echo a":                      {stdout: "a\n"},
		"echo b":                      {stdout: "b\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	if _, _, ran, err := ComposeWithRunner(dir, "demo/T1", "typecheck", "echo base", "echo a", "echo b", at, runner); err != nil || !ran {
		t.Fatalf("first compose: ran=%v err=%v", ran, err)
	}
	calls := len(runner.calls)
	if calls == 0 {
		t.Fatal("first compose must run commands")
	}
	_, _, ran, err := ComposeWithRunner(dir, "demo/T1", "typecheck", "echo base", "echo a", "echo b", at.Add(time.Minute), runner)
	if err == nil || !strings.Contains(err.Error(), "task evolution cap") {
		t.Fatalf("want task-cap refusal, got ran=%v err=%v", ran, err)
	}
	if ran {
		t.Fatal("ran must be false on task-cap refusal")
	}
	if len(runner.calls) != calls {
		t.Fatalf("no gate command may run on task-cap refusal, calls=%v", runner.calls)
	}
	entries := budgetEntries(t, dir)
	last := entries[len(entries)-1]
	if last.Kind != budget.KindProbe || last.Outcome != budget.OutcomeRefusedCap {
		t.Fatalf("last entry must be the probe refusal, got %+v", last)
	}
}

func TestComposeRefusesAtEvolutionCap(t *testing.T) {
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
	_, _, ran, err := ComposeWithRunner(dir, "demo/T1", "typecheck", "echo base", "echo a", "echo b", at, runner)
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
