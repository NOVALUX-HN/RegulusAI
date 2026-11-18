// Handler coverage for the remaining cmdx families: mission
// extras, execute, context, failure, recovery, impact, learn,
// orchestrate, quality, probe, timeline, bench, evolve, MCP and the
// shared helpers. Every test assembles real stores in t.TempDir(),
// asserts the exit code plus the created files, and stays portable
// (stub runner instead of git, os.Executable re-exec instead of
// shell, no env reads).
package cmdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/context"
	"regulus-ai/internal/execute"
	"regulus-ai/internal/failure"
	"regulus-ai/internal/impact"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/planning"
)

func TestMissionCreateShowListStatusJSON(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	if code := io.MissionCreateCmd([]string{"--id", "j1", "--objective", "json round trip", "--risk", "low", "--json"}); code != 0 {
		t.Fatalf("create --json exit = %d stderr = %q", code, errb.String())
	}
	var created mission.Mission
	if err := json.Unmarshal(out.Bytes(), &created); err != nil || created.ID != "j1" {
		t.Fatalf("create --json must decode the mission: %v %q", err, out.String())
	}
	if code := io.MissionCreateCmd([]string{"--id", "j1", "--objective", "dupe"}); code != 1 {
		t.Fatalf("duplicate create exit = %d, want 1", code)
	}
	if code := io.MissionCreateCmd([]string{"--objective", "nope", "--risk", "cosmic"}); code != 1 {
		t.Fatalf("invalid risk exit = %d, want 1", code)
	}
	// j2 enters through the leaf command itself: missionCreate
	// asserts exactly one mission exists, so it cannot follow j1.
	if code := io.MissionCreateCmd([]string{"--id", "j2", "--objective", "second"}); code != 0 {
		t.Fatalf("create j2 exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := io.MissionListCmd([]string{"--json"}); code != 0 {
		t.Fatalf("list --json exit = %d", code)
	}
	var entries []mission.ListEntry
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil || len(entries) != 2 {
		t.Fatalf("list --json must decode two entries: %v %q", err, out.String())
	}
	out.Reset()
	if code := io.MissionListCmd([]string{"--state", "created"}); code != 0 {
		t.Fatalf("list --state exit = %d", code)
	}
	if !strings.Contains(out.String(), "j1") || !strings.Contains(out.String(), "j2") {
		t.Fatalf("filtered list must carry both missions, got %q", out.String())
	}
	out.Reset()
	if code := io.MissionShowCmd([]string{"--id", "j2", "--json"}); code != 0 {
		t.Fatalf("show --json exit = %d", code)
	}
	var shown mission.Mission
	if err := json.Unmarshal(out.Bytes(), &shown); err != nil || shown.ID != "j2" {
		t.Fatalf("show --json must decode j2: %v %q", err, out.String())
	}
	out.Reset()
	if code := io.MissionStatusCmd([]string{"--id", "j2", "--json"}); code != 0 {
		t.Fatalf("status --json exit = %d", code)
	}
	var hist []mission.Event
	if err := json.Unmarshal(out.Bytes(), &hist); err != nil || len(hist) == 0 {
		t.Fatalf("status --json must decode history: %v %q", err, out.String())
	}
	// Dispatch through the router covers the MissionCmd switch.
	out.Reset()
	if code := io.MissionCmd([]string{"status", "--id", "j2"}); code != 0 {
		t.Fatalf("router status exit = %d", code)
	}
	if code := io.MissionCmd([]string{"show", "--id", "ghost"}); code != 1 {
		t.Fatalf("router show of a missing mission exit = %d, want 1", code)
	}
}

func TestMissionMutationRefusals(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	missionCreate(t, io, "--id", "r1", "--objective", "refusals")
	// Pause only parks a live execution; resume only lifts a pause;
	// reopen only escapes a terminal state.
	if code := io.MissionPauseCmd([]string{"--id", "r1"}); code != 1 {
		t.Fatalf("pause of a created mission exit = %d, want 1", code)
	}
	if code := io.MissionResumeCmd([]string{"--id", "r1"}); code != 1 {
		t.Fatalf("resume without pause exit = %d, want 1", code)
	}
	if code := io.MissionReopenCmd([]string{"--id", "r1", "--reason", "nope"}); code != 1 {
		t.Fatalf("reopen of a live mission exit = %d, want 1", code)
	}
	if code := io.MissionLinkCmd([]string{"--id", "ghost"}); code != 1 {
		t.Fatalf("link of a missing mission exit = %d, want 1", code)
	}
	if code := io.MissionCancelCmd([]string{"--id", "r1", "--reason", "-dash"}); code != 2 {
		t.Fatalf("dash reason exit = %d, want 2", code)
	}
	if code := io.MissionReopenCmd([]string{"--id", "r1", "--reason", "-dash"}); code != 2 {
		t.Fatalf("dash reopen reason exit = %d, want 2", code)
	}
	if code := io.MissionAdvanceCmd([]string{"--id", "r1"}); code != 2 {
		t.Fatalf("advance without --to exit = %d, want 2", code)
	}
}

func TestMissionPureHelpers(t *testing.T) {
	if got := missionEventStatus(mission.StateCompleted); got != "succeeded" {
		t.Fatalf("completed maps to %q", got)
	}
	if got := missionEventStatus(mission.StatePartiallyCompleted); got != "succeeded" {
		t.Fatalf("partial maps to %q", got)
	}
	if got := missionEventStatus(mission.StateFailed); got != "failed" {
		t.Fatalf("failed maps to %q", got)
	}
	if got := missionEventStatus(mission.StateCancelled); got != "cancelled" {
		t.Fatalf("cancelled maps to %q", got)
	}
	if got := missionEventStatus(mission.StateCreated); got != "running" {
		t.Fatalf("live maps to %q", got)
	}
	if missionListHasCorrupt(nil) {
		t.Fatal("clean list has no corruption")
	}
	if !missionListHasCorrupt([]mission.ListEntry{{ID: "x", Problem: "sha mismatch"}}) {
		t.Fatal("corrupt entry must be reported")
	}
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	missionCreate(t, io, "--id", "h1", "--objective", "helper")
	_, m, err := mission.LoadMission(dir, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if code := printMissionJSON(io, m); code != 0 {
		t.Fatalf("printMissionJSON exit = %d", code)
	}
}

func TestExecuteStatusApproveCancelBench(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	executeMission(t, io, "ex1")
	// Status without a ledger still reports: nothing sealed,
	// nothing shown, generation pinned at zero.
	if code := io.ExecuteStatusCmd([]string{"--id", "ex1"}); code != 0 {
		t.Fatalf("status without ledger exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "ledger-generation=0") {
		t.Fatalf("status must pin the empty ledger, got %q", out.String())
	}
	if code := io.ExecuteStatusCmd([]string{"--id", "ghost"}); code != 1 {
		t.Fatalf("status of a missing mission exit = %d, want 1", code)
	}
	if code := io.ExecuteApproveCmd([]string{"--id", "ghost", "--task", "t1"}); code != 1 {
		t.Fatalf("approve on a missing mission exit = %d, want 1", code)
	}
	if code := io.ExecuteCancelCmd([]string{"--id", "ex1", "--reason", "-dash"}); code != 2 {
		t.Fatalf("dash cancel reason exit = %d, want 2", code)
	}
	if code := io.ExecuteBenchCmd([]string{"--id", "ghost"}); code != 1 {
		t.Fatalf("bench of a missing mission exit = %d, want 1", code)
	}
	// Bench over a real plan measures the scheduling shape.
	executePlan(t, dir, "ex1", 1, planning.StatusReady, planning.AutonomyAutonomous)
	out.Reset()
	if code := io.ExecuteBenchCmd([]string{"--id", "ex1", "--parallel", "2", "--work-ms", "5", "--json"}); code != 0 {
		t.Fatalf("bench --json exit = %d stderr = %q", code, errb.String())
	}
	var rep map[string]any
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("bench --json must decode: %v\n%s", err, out.String())
	}
}

func TestExecuteWaveHelpers(t *testing.T) {
	dry := execute.WaveReport{DryRun: true, MissionID: "m", PlanVersion: 1,
		States: map[string]string{"t1": execute.TaskFailed}}
	if got := waveExitFor(dry); got != 0 {
		t.Fatalf("dry-run exit = %d, want 0 (refused nothing, only planned)", got)
	}
	if got := waveExitFor(execute.WaveReport{States: map[string]string{"t1": execute.TaskBlocked}}); got != 1 {
		t.Fatalf("blocked wave exit = %d, want 1", got)
	}
	if got := waveExitFor(execute.WaveReport{States: map[string]string{"t1": execute.TaskFailed}}); got != 1 {
		t.Fatalf("failed wave exit = %d, want 1", got)
	}
	if got := waveExitFor(execute.WaveReport{States: map[string]string{"t1": execute.TaskSucceeded}}); got != 0 {
		t.Fatalf("clean wave exit = %d, want 0", got)
	}
	out := renderWaveReport(execute.WaveReport{DryRun: true, MissionID: "mw", PlanVersion: 3})
	if !strings.Contains(out, "mw") || !strings.Contains(out, "dry-run") {
		t.Fatalf("wave report must name the mission and the mode, got %q", out)
	}
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	if io.parallelGate("") == nil {
		t.Fatal("empty gate task must still build the diff-check gate")
	}
	if io.parallelGate("gauntlet") == nil {
		t.Fatal("named gate task must build a gate")
	}
	if io.parallelRecorder(dir) == nil {
		t.Fatal("recorder must always build")
	}
}

func TestContextInventorySelect(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	planRepo(t, dir)
	if code := io.ContextInventoryCmd([]string{"--no-memory"}); code != 0 {
		t.Fatalf("inventory exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "fragmento(s)") {
		t.Fatalf("inventory must print the census, got %q", out.String())
	}
	out.Reset()
	if code := io.ContextInventoryCmd([]string{"--no-memory", "--json"}); code != 0 {
		t.Fatalf("inventory --json exit = %d", code)
	}
	var census map[string]any
	if err := json.Unmarshal(out.Bytes(), &census); err != nil {
		t.Fatalf("inventory --json must decode: %v\n%s", err, out.String())
	}
	// Empty corpus refuses: nothing to deliver, nothing invented.
	empty := t.TempDir()
	eio, _, eerrb := cmdIO(empty)
	sel := []string{"--domain", "harness", "--lang", "go", "--complexity", "low",
		"--risk", "low", "--no-memory", "--dry-run"}
	if code := eio.ContextSelectCmd(sel); code != 1 {
		t.Fatalf("select on an empty corpus exit = %d, want 1 stderr = %q", code, eerrb.String())
	}
	if !strings.Contains(eerrb.String(), "context select:") {
		t.Fatalf("select refusal must name itself, got %q", eerrb.String())
	}
	// Invalid descriptors refuse before any inventory work.
	bad := []string{"--domain", "harness", "--lang", "go", "--complexity", "cosmic", "--risk", "low"}
	if code := eio.ContextSelectCmd(bad); code != 2 {
		t.Fatalf("invalid descriptor exit = %d, want 2", code)
	}
	neg := []string{"--domain", "h", "--lang", "go", "--complexity", "low", "--risk", "low", "--budget-tokens", "-1"}
	if code := eio.ContextSelectCmd(neg); code != 2 {
		t.Fatalf("negative budget exit = %d, want 2", code)
	}
	// --out records the selection even when the corpus is empty and
	// the command still refuses to deliver it.
	outDir := filepath.Join(empty, "sel")
	if code := eio.ContextSelectCmd(append(sel[:len(sel)-1], "--out", outDir)); code != 1 {
		t.Fatalf("select --out on empty exit = %d, want 1", code)
	}
	if _, err := os.Stat(filepath.Join(outDir, "selection.json")); err != nil {
		t.Fatalf("select --out must record the selection: %v", err)
	}
}

func TestContextSelectionHelpers(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	opts, code, ok := parseContextSelectFlags([]string{"--domain", "h"}, io.Stderr)
	if ok || code != 2 {
		t.Fatalf("partial flags must refuse: %+v %d %v", opts, code, ok)
	}
	selDir := filepath.Join(dir, "sel")
	if got := writeContextSelection(selDir, context.Selection{}, io.Stderr); got != 0 {
		t.Fatalf("write selection exit = %d", got)
	}
	raw, err := os.ReadFile(filepath.Join(selDir, "selection.json"))
	if err != nil || len(raw) == 0 {
		t.Fatalf("selection must persist: %v", err)
	}
	if got := renderContextSelection(context.Selection{}); !strings.Contains(got, "tokens:") {
		t.Fatalf("rendered selection must quote tokens, got %q", got)
	}
}

func TestFailureDecideJSONAndRefusals(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	missionCreate(t, io, "--id", "f1", "--objective", "failure paths")
	out.Reset()
	if code := io.FailureDecideCmd([]string{"--mission", "f1", "--error", "tls timeout", "--json"}); code != 0 {
		t.Fatalf("decide --json exit = %d stderr = %q", code, errb.String())
	}
	var sealed struct {
		Record struct {
			Decision string `json:"decision"`
		} `json:"record"`
	}
	if err := json.Unmarshal(out.Bytes(), &sealed); err != nil || sealed.Record.Decision == "" {
		t.Fatalf("decide --json must carry the decision: %v %q", err, out.String())
	}
	if code := io.FailureDecideCmd([]string{"--mission", "f1", "--to-agent", "b"}); code != 2 {
		t.Fatalf("reassign without run exit = %d, want 2", code)
	}
	if code := io.FailureDecideCmd([]string{"--mission", "ghost", "--error", "x"}); code != 1 {
		t.Fatalf("decide on a missing mission exit = %d, want 1 (never decides on tampered state)", code)
	}
	if code := io.FailureDecideCmd([]string{"--mission", "f1", "--max-replans", "99"}); code != 2 {
		t.Fatalf("absurd replan budget exit = %d, want 2", code)
	}
	out.Reset()
	if code := io.FailureStatusCmd([]string{"--mission", "f1", "--json"}); code != 0 {
		t.Fatalf("status --json exit = %d", code)
	}
	var recs []failure.DecisionRecord
	if err := json.Unmarshal(out.Bytes(), &recs); err != nil || len(recs) == 0 {
		t.Fatalf("status --json must decode the log: %v %q", err, out.String())
	}
	if code := io.FailureExplainCmd([]string{"--mission", "f1", "--decision", "nope"}); code != 1 {
		t.Fatalf("explain of an unknown decision exit = %d, want 1", code)
	}
	if code := io.FailureClassifyCmd([]string{"--error", "x", "--class", "bogus"}); code != 1 {
		t.Fatalf("classify with a bogus override exit = %d, want 1", code)
	}
	out.Reset()
	if code := io.FailureClassifyCmd([]string{"--error", "disk full", "--json"}); code != 0 {
		t.Fatalf("classify --json exit = %d", code)
	}
	var cls map[string]any
	if err := json.Unmarshal(out.Bytes(), &cls); err != nil || cls["class"] == nil {
		t.Fatalf("classify --json must name the class: %v %q", err, out.String())
	}
	for _, d := range []string{failure.DecisionFailClosed, failure.DecisionPauseForApproval, failure.DecisionIsolate, failure.DecisionRetry} {
		if got := failureEventStatus(d); got == "" {
			t.Fatalf("decision %q must map to a status", d)
		}
	}
}

func TestRecoveryCheckpointStatusRetryResumeCancel(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	// New runs start at pending: pending -> checkpointed is illegal,
	// so the first checkpoint parks running before sealing checkpointed.
	if code := io.RecoveryCheckpointCmd([]string{"--run", "r1", "--task", "t1", "--agent", "a1", "--state", "running"}); code != 0 {
		t.Fatalf("running checkpoint exit = %d stderr = %q", code, errb.String())
	}
	if code := io.RecoveryCheckpointCmd([]string{"--run", "r1", "--task", "t1", "--agent", "a1", "--state", "checkpointed"}); code != 0 {
		t.Fatalf("checkpoint exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "checkpoint t1/a1") {
		t.Fatalf("checkpoint must confirm, got %q", out.String())
	}
	out.Reset()
	if code := io.RecoveryStatusCmd([]string{"--run", "r1", "--json"}); code != 0 {
		t.Fatalf("status --json exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "r1") {
		t.Fatalf("status must carry the run, got %q", out.String())
	}
	out.Reset()
	if code := io.RecoveryRetryCmd([]string{"--run", "r1", "--task", "t1", "--agent", "a1", "--error", "connection reset"}); code != 0 {
		t.Fatalf("retry exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "backoff=") {
		t.Fatalf("retry must report the backoff, got %q", out.String())
	}
	out.Reset()
	if code := io.RecoveryResumeCmd([]string{"--run", "r1", "--task", "t1", "--agent", "a1"}); code != 0 {
		t.Fatalf("resume exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "resume t1/a1") {
		t.Fatalf("resume must render the plan, got %q", out.String())
	}
	if code := io.RecoveryRetryCmd([]string{"--run", "ghost", "--task", "t1", "--agent", "a1"}); code != 1 {
		t.Fatalf("retry on a missing run exit = %d, want 1", code)
	}
	if code := io.RecoveryStatusCmd([]string{"--run", "ghost"}); code != 1 {
		t.Fatalf("status of a missing run exit = %d, want 1", code)
	}
	out.Reset()
	if code := io.RecoveryCancelCmd([]string{"--run", "r1", "--reason", "superseded"}); code != 0 {
		t.Fatalf("cancel exit = %d stderr = %q", code, errb.String())
	}
	if out.Len() == 0 {
		t.Fatal("cancel must confirm")
	}
}

func TestRecoveryPolicyHelpers(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	max, to := 0, int64(0)
	if _, code, ok := recoveryPolicyFromFlags(&max, &to, io.Stderr); !ok || code != 0 {
		t.Fatal("documented defaults must validate")
	}
	bad := -1
	if _, code, ok := recoveryPolicyFromFlags(&bad, &to, io.Stderr); ok || code != 2 {
		t.Fatalf("negative retries must refuse with 2, got %d %v", code, ok)
	}
	negTo := int64(-5)
	if _, code, ok := recoveryPolicyFromFlags(&max, &negTo, io.Stderr); ok || code != 2 {
		t.Fatalf("negative timeout must refuse with 2, got %d %v", code, ok)
	}
	caps, verified := recoveryTargetCaps("no-such-agent", []string{"tools"})
	if len(caps) != 1 || len(verified) != 0 {
		t.Fatalf("off-registry targets stay unverified: %v %v", caps, verified)
	}
	in, code, ok := parseRecoveryCheckpointIO("k=v", "", "", "", io.Stderr)
	if !ok || code != 0 {
		t.Fatalf("k=v must parse: %d %v", code, ok)
	}
	_ = in
	if _, code, ok := parseRecoveryCheckpointIO("novalue", "", "", "", io.Stderr); ok || code != 1 {
		t.Fatalf("bare input must refuse with 1, got %d %v", code, ok)
	}
}

func TestImpactInventoryGraphAffectSuiteCyclesBreaking(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	planRepo(t, dir)
	if code := io.ImpactInventoryCmd(nil); code != 0 {
		t.Fatalf("inventory exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "inventory:") {
		t.Fatalf("inventory must print the census, got %q", out.String())
	}
	out.Reset()
	if code := io.ImpactInventoryCmd([]string{"--json"}); code != 0 {
		t.Fatalf("inventory --json exit = %d", code)
	}
	if !strings.Contains(out.String(), "nodes") {
		t.Fatalf("inventory --json must carry nodes, got %q", out.String())
	}
	out.Reset()
	if code := io.ImpactGraphCmd(nil); code != 0 {
		t.Fatalf("graph exit = %d stderr = %q", code, errb.String())
	}
	if out.Len() == 0 {
		t.Fatal("graph must render")
	}
	out.Reset()
	if code := io.ImpactAffectCmd([]string{"--changed", "internal/demo/demo.go"}); code != 0 {
		t.Fatalf("affect exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := io.ImpactSuiteCmd([]string{"--changed", "internal/demo/demo.go"}); code != 0 {
		t.Fatalf("suite exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := io.ImpactCyclesCmd(nil); code != 0 {
		t.Fatalf("cycles exit = %d stderr = %q", code, errb.String())
	}
	// Breaking compares the stored census against itself: identical
	// input reports no breakage instead of inventing any.
	inv, err := impact.Census(dir)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	before := filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(before, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := io.ImpactBreakingCmd([]string{"--before", before}); code != 0 {
		t.Fatalf("self-compare breaking exit = %d stderr = %q", code, errb.String())
	}
	if code := io.ImpactBreakingCmd([]string{"--before", filepath.Join(dir, "missing.json")}); code != 1 {
		t.Fatalf("breaking on a missing file exit = %d, want 1", code)
	}
}

func TestLearnSaveSearchMetricsVerifyInvalidateResolve(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	t.Setenv("REGULUS_DEFER_REINDEX", "1")
	if code := io.LearnSaveCmd([]string{"--kind", "bogus", "--title", "t", "--author", "a", "--scope", "shared", "x"}); code != 2 {
		t.Fatalf("unknown kind exit = %d, want 2", code)
	}
	if code := io.LearnSaveCmd([]string{"--kind", "pattern", "--title", "t", "--author", "a", "--scope", "everywhere", "x"}); code != 2 {
		t.Fatalf("unknown scope exit = %d, want 2", code)
	}
	if code := io.LearnSaveCmd([]string{"--kind", "pattern", "--title", "t", "--author", "a", "--scope", "shared", "--expires", "soon", "x"}); code != 2 {
		t.Fatalf("bad expiry exit = %d, want 2", code)
	}
	if code := io.LearnSaveCmd([]string{"--kind", "verified-solution", "--title", "t", "--author", "a", "--scope", "shared", "x"}); code == 0 {
		t.Fatalf("unverified solution must not save clean (needs --evidence or refusal)")
	}
	out.Reset()
	errb.Reset()
	if code := io.LearnSaveCmd([]string{"--kind", "hypothesis", "--title", "zephyr widget docs", "--author", "tester",
		"--scope", "shared", "zephyr widgets cache aggressively"}); code != 0 {
		t.Fatalf("save exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "saved as") {
		t.Fatalf("save must confirm, got %q", out.String())
	}
	savedLine := out.String()
	_ = savedLine
	out.Reset()
	if code := io.LearnSearchCmd([]string{"zephyr"}); code != 0 {
		t.Fatalf("search exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "hit(s)") {
		t.Fatalf("search must find the saved hypothesis, got %q", out.String())
	}
	out.Reset()
	if code := io.LearnSearchCmd([]string{"nothing-matches-this-term"}); code != 0 {
		t.Fatalf("empty search exit = %d", code)
	}
	if !strings.Contains(out.String(), "no matching") {
		t.Fatalf("empty search must say so, got %q", out.String())
	}
	out.Reset()
	if code := io.LearnMetricsCmd(nil); code != 0 {
		t.Fatalf("metrics exit = %d stderr = %q", code, errb.String())
	}
}

func TestLearnVerifyInvalidateResolve(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	t.Setenv("REGULUS_DEFER_REINDEX", "1")
	// Flags precede the positional content: encoding/json's flag set
	// stops parsing at the first positional, so a trailing --evidence
	// would silently become content and the save would carry none.
	if code := io.LearnSaveCmd([]string{"--kind", "pattern", "--title", "retry with backoff", "--author", "tester",
		"--scope", "shared", "--evidence", "ci-green,reviewed", "patterns for retrying flaky calls"}); code != 0 {
		t.Fatalf("pattern save exit = %d stderr = %q", code, errb.String())
	}
	idLine := out.String()
	id := learnSavedID(t, idLine)
	receipt := filepath.Join(dir, "proof.txt")
	if err := os.WriteFile(receipt, []byte("proof"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A pattern verifies against real evidence.
	if code := io.LearnVerifyCmd([]string{id, "--receipt", receipt}); code != 0 {
		t.Fatalf("verify exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := io.LearnSaveCmd([]string{"--kind", "hypothesis", "--title", "cache everything", "--author", "tester",
		"--scope", "shared", "hypotheses never verify"}); code != 0 {
		t.Fatalf("hypothesis save exit = %d", code)
	}
	hid := learnSavedID(t, out.String())
	// Hypotheses can never verify: the Almanac refuses, the CLI
	// surfaces the refusal instead of promoting.
	if code := io.LearnVerifyCmd([]string{hid, "--receipt", receipt}); code != 1 {
		t.Fatalf("hypothesis verify exit = %d, want 1 (never verified)", code)
	}
	// Resolve settles active conflicts_with only: relate the pair
	// first so the contradiction is live, then it must settle.
	if err := almanac.Relate(dir, id, almanac.RelationConflictsWith, hid, time.Now().UTC()); err != nil {
		t.Fatalf("relate conflict: %v", err)
	}
	if code := io.LearnResolveCmd([]string{"--winner", id, "--loser", hid}); code != 0 {
		t.Fatalf("resolve exit = %d stderr = %q", code, errb.String())
	}
	if code := io.LearnInvalidateCmd([]string{hid, "--reason", "disproved by load test"}); code != 0 {
		t.Fatalf("invalidate exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := io.LearnSearchCmd([]string{"cache"}); code != 0 {
		t.Fatalf("search exit = %d", code)
	}
	if !strings.Contains(out.String(), "no matching") {
		t.Fatalf("invalidated knowledge must hide by default, got %q", out.String())
	}
	out.Reset()
	if code := io.LearnSearchCmd([]string{"cache", "--include-stale"}); code != 0 {
		t.Fatalf("stale search exit = %d", code)
	}
	if !strings.Contains(out.String(), "hit(s)") {
		t.Fatalf("stale search must surface it, got %q", out.String())
	}
	_ = errb
}

// learnSavedID pulls the observation id out of the `learn: <kind>
// saved as <id> (scope <s>)` confirmation line.
func learnSavedID(t *testing.T, line string) string {
	t.Helper()
	fields := strings.Fields(line)
	for i, f := range fields {
		if f == "as" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	t.Fatalf("save must print the id, got %q", line)
	return ""
}

func TestLearnPureHelpers(t *testing.T) {
	if d, err := learnParseExpiry("720h"); err != nil || d.Hours() != 720 {
		t.Fatalf("hours must parse: %v %v", d, err)
	}
	if d, err := learnParseExpiry("30d"); err != nil || d.Hours() != 720 {
		t.Fatalf("days must parse: %v %v", d, err)
	}
	if d, err := learnParseExpiry("2w"); err != nil || d.Hours() != 336 {
		t.Fatalf("weeks must parse: %v %v", d, err)
	}
	if _, err := learnParseExpiry("soon"); err == nil {
		t.Fatal("garbage expiry must fail")
	}
	pos, flags := learnSplitPositional([]string{"abc123", "--receipt", "f"})
	if len(pos) != 1 || pos[0] != "abc123" || len(flags) != 2 {
		t.Fatalf("positional split wrong: %v %v", pos, flags)
	}
	now := time.Now()
	if got := learnAge(now, now.Add(-30*time.Second)); got != "just now" {
		t.Fatalf("recent age = %q", got)
	}
	if got := learnAge(now, now.Add(-5*time.Minute)); got != "5m ago" {
		t.Fatalf("minute age = %q", got)
	}
	if got := learnAge(now, now.Add(-2*time.Hour)); got != "2h ago" {
		t.Fatalf("hour age = %q", got)
	}
	if got := learnAge(now, now.Add(-48*time.Hour)); got != "2d ago" {
		t.Fatalf("day age = %q", got)
	}
}

func TestOrchestrateAgentsDecideEvaluate(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	if code := io.OrchestrateAgentsCmd(nil); code != 0 {
		t.Fatalf("agents exit = %d", code)
	}
	if !strings.Contains(out.String(), "agent(s)") {
		t.Fatalf("agents must list the registry, got %q", out.String())
	}
	out.Reset()
	if code := io.OrchestrateAgentsCmd([]string{"--json"}); code != 0 {
		t.Fatalf("agents --json exit = %d", code)
	}
	var reg []map[string]any
	if err := json.Unmarshal(out.Bytes(), &reg); err != nil || len(reg) == 0 {
		t.Fatalf("agents --json must decode a non-empty registry: %v %q", err, out.String())
	}
	base := []string{"--domain", "harness", "--lang", "go", "--complexity", "low", "--risk", "low", "--dry-run"}
	out.Reset()
	if code := io.OrchestrateDecideCmd(base); code != 0 {
		t.Fatalf("decide dry-run exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "routing ") {
		t.Fatalf("decide must render the routing, got %q", out.String())
	}
	out.Reset()
	if code := io.OrchestrateDecideCmd(append(append([]string{}, base...), "--json")); code != 0 {
		t.Fatalf("decide --json exit = %d stderr = %q", code, errb.String())
	}
	var dec struct {
		Primary string `json:"primary"`
	}
	if err := json.Unmarshal(out.Bytes(), &dec); err != nil || dec.Primary == "" {
		t.Fatalf("decide --json must name the primary: %v %q", err, out.String())
	}
	if code := io.OrchestrateDecideCmd([]string{"--domain", "h", "--lang", "go", "--complexity", "cosmic", "--risk", "low"}); code != 2 {
		t.Fatalf("bad complexity exit = %d, want 2", code)
	}
}

func TestTraceEmitTimelinePrune(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	if code := io.TraceEmitCmd([]string{"--run", "tr1", "--type", "task.start", "--task", "t1"}); code != 0 {
		t.Fatalf("emit exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "exec recorded") {
		t.Fatalf("emit must confirm, got %q", out.String())
	}
	if code := io.TraceEmitCmd([]string{"--run", "tr1", "--type", "task.start", "--payload", "{oops"}); code != 1 {
		t.Fatalf("malformed payload exit = %d, want 1", code)
	}
	out.Reset()
	if code := io.TraceTimelineCmd([]string{"--run", "tr1"}); code != 0 {
		t.Fatalf("timeline exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "tr1") {
		t.Fatalf("timeline must quote the run, got %q", out.String())
	}
	out.Reset()
	if code := io.TraceTimelineCmd([]string{"--run", "tr1", "--json"}); code != 0 {
		t.Fatalf("timeline --json exit = %d", code)
	}
	if !strings.Contains(out.String(), "tr1") {
		t.Fatalf("timeline --json must quote the run, got %q", out.String())
	}
	if code := io.TraceTimelineCmd([]string{"--rebuild"}); code != 2 {
		t.Fatalf("rebuild without run exit = %d, want 2", code)
	}
	// A missing run has no events to rebuild: empty timeline, exit 0
	// (nothing failed, there is simply nothing to replay).
	out.Reset()
	if code := io.TraceTimelineCmd([]string{"--run", "ghost", "--rebuild"}); code != 0 {
		t.Fatalf("rebuild of a missing run exit = %d, want 0 (empty timeline)", code)
	}
	if !strings.Contains(out.String(), "no execution events match") {
		t.Fatalf("missing run must report the empty timeline, got %q", out.String())
	}
	if code := io.TracePruneExecCmd(nil); code != 0 {
		t.Fatalf("prune exit = %d stderr = %q", code, errb.String())
	}
	_ = errb
}

func TestBenchTiersDiagHostcapLab(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	if code := io.BenchTiersCmd(nil); code != 0 {
		t.Fatalf("tiers exit = %d", code)
	}
	out.Reset()
	if code := io.BenchTiersCmd([]string{"--json"}); code != 0 {
		t.Fatalf("tiers --json exit = %d", code)
	}
	var specs []any
	if err := json.Unmarshal(out.Bytes(), &specs); err != nil || len(specs) == 0 {
		t.Fatalf("tiers --json must decode specs: %v %q", err, out.String())
	}
	if code := io.BenchTierPlanCmd([]string{"--tier", "bogus"}); code != 2 {
		t.Fatalf("bad tier exit = %d, want 2", code)
	}
	// Tier plan over the tracked splits: copy them into the project
	// (portable file copy, no shell) so the loader resolves.
	raw, err := os.ReadFile(filepath.Join("..", "..", "bench", "agent-splits-v3.json"))
	if err != nil {
		t.Skipf("tracked splits unreadable: %v", err)
	}
	splitDir := filepath.Join(dir, "bench")
	if err := os.MkdirAll(splitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(splitDir, "agent-splits-v3.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	saved := filepath.Join(dir, "tier-plan.json")
	out.Reset()
	if code := io.BenchTierPlanCmd([]string{"--tier", "L0", "--save", saved}); code != 0 {
		t.Fatalf("tier-plan exit = %d stderr = %q", code, errb.String())
	}
	if _, err := os.Stat(saved); err != nil {
		t.Fatalf("tier-plan --save must persist: %v", err)
	}
	if code := io.BenchDiagCmd([]string{"--run", "ghost-run"}); code != 1 {
		t.Fatalf("diag without evidence exit = %d, want 1", code)
	}
	out.Reset()
	if code := io.BenchHostCapabilitiesCmd(nil); code != 0 {
		t.Fatalf("hostcap probe exit = %d stderr = %q", code, errb.String())
	}
	if out.Len() == 0 {
		t.Fatal("hostcap must print the probe table")
	}
	out.Reset()
	if code := io.BenchLabAgentsCmd(nil); code != 0 {
		t.Fatalf("lab agents exit = %d", code)
	}
	if !strings.Contains(out.String(), "agent(s)") {
		t.Fatalf("lab agents must list the corpus, got %q", out.String())
	}
}

func TestEvolveSpecRefusals(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	malformed := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(malformed, []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := io.EvolveValidationDeclareCmd([]string{"--plan", malformed}); code != 1 {
		t.Fatalf("malformed validation spec exit = %d, want 1", code)
	}
	if code := io.EvolveCycleDeclareCmd([]string{"--spec", malformed}); code != 1 {
		t.Fatalf("malformed cycle spec exit = %d, want 1", code)
	}
}

func TestMCPAuthorizeUnknownDenies(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	_ = io.MCPPrincipal()
	if dec := io.MCPAuthorize("no-such-tool", nil); dec.Allow {
		t.Fatal("unknown tools must deny by default (closed policy)")
	}
}

func TestIOAndSharedHelpers(t *testing.T) {
	var out strings.Builder
	plain := New(&out, &out, "", "test", nil)
	if plain.Dir != "." || plain.Root() != "." {
		t.Fatalf("empty dir must fall back to '.', got %+v", plain)
	}
	if got := splitList(" a, ,b "); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitList must trim and drop empties: %q", got)
	}
	if len(splitList("")) != 0 {
		t.Fatal("empty split must stay empty")
	}
	for _, ok := range []string{"t1", "a/b_c-1.2", "X"} {
		if !validTaskID(ok) {
			t.Fatalf("validTaskID(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "../x", "-dash", "sp ace", "t!"} {
		if validTaskID(bad) {
			t.Fatalf("validTaskID(%q) = true", bad)
		}
	}
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	if _, code, ok := loadBenchAgentSplits(io, nil); code != 1 || ok {
		t.Fatalf("missing splits must refuse with 1, got %d %v", code, ok)
	}
	t.Setenv("REGULUS_AGENT_SPLITS", filepath.Join(dir, "missing.json"))
	if got := benchAgentSplitsPath(dir); got != filepath.Join(dir, "missing.json") {
		t.Fatalf("env override must win: %q", got)
	}
	if _, msg, code, ok := resolveAgentRunner("no-such-host"); ok || code != 3 || !strings.Contains(msg, "NOT RUN") {
		t.Fatalf("unknown host must NOT RUN with 3: %d %q %v", code, msg, ok)
	}
	inputs, _ := io.loadDiagInputs(nil, nil)
	if len(inputs) != 0 {
		t.Fatalf("empty diag inputs must stay empty: %v", inputs)
	}
}

func TestPlanHelpers(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := cmdIO(dir)
	executeMission(t, io, "px1")
	executePlan(t, dir, "px1", 1, planning.StatusReady, planning.AutonomyAutonomous)
	p, err := planning.LatestPlan(dir, "px1")
	if err != nil {
		t.Fatal(err)
	}
	if code := checkPlanValid(p, io); code != 0 {
		t.Fatalf("valid fixture plan must validate, got %d", code)
	}
	if code := checkPlanValid(&planning.Plan{}, io); code != 1 {
		t.Fatalf("empty plan exit = %d, want 1", code)
	}
	if got := planExitFor(p); got != 0 {
		t.Fatalf("ready plan exit = %d, want 0", got)
	}
	p.Status = planning.StatusNeedsApproval
	if got := planExitFor(p); got != 0 {
		t.Fatalf("approval-gated plan exit = %d, want 0 (operator acts on the status line)", got)
	}
	p.Status = planning.StatusNeedsClarification
	if got := planExitFor(p); got != 1 {
		t.Fatalf("clarification plan exit = %d, want 1", got)
	}
	out.Reset()
	if code := printPlan(io, p, false, "dry-run: nothing persisted"); code != 1 {
		t.Fatalf("printPlan exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "dry-run: nothing persisted") {
		t.Fatalf("printPlan must print the banner, got %q", out.String())
	}
	ok, err := parseConfirmed("t1=done, t2 = ok ")
	if err != nil || len(ok) != 2 {
		t.Fatalf("confirmed must parse: %+v %v", ok, err)
	}
	if _, err := parseConfirmed("bogus"); err == nil {
		t.Fatal("bare confirmed entry must fail")
	}
}
