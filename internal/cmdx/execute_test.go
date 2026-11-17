// Handler tests for F13.3/NOV-106: the execute command family
// through the cmdx API — dry-run sealing nothing, status, approve
// and cancel round-trips, refusals and exit codes. Wet dispatch
// needs real worktrees, so it lives in internal/execute with the
// in-memory backend; here the CLI proves it wires the executor and
// never seals on dry-run.
package cmdx

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/mission"
	"regulus-ai/internal/planning"
)

func executeMission(t *testing.T, io IO, id string) {
	t.Helper()
	if code := io.MissionCreateCmd([]string{"--id", id, "--objective", "Given x When y Then z 1", "--risk", "medium"}); code != 0 {
		t.Fatalf("create exit = %d", code)
	}
	now := time.Now().UTC()
	for _, to := range []string{mission.StateAnalyzing, mission.StatePlanning, mission.StateAwaitingApproval} {
		if code := io.MissionAdvanceCmd([]string{"--id", id, "--to", to}); code != 0 {
			t.Fatalf("advance to %s exit = %d", to, code)
		}
	}
	_ = now
}

func executePlan(t *testing.T, dir, missionID string, version int, status string, autonomy string) {
	t.Helper()
	tasks := []planning.PlanTask{{
		ID: "t1", Title: "task t1", Objective: "do t1", Deliverable: "t1 done",
		Scope: planning.Scope{Files: []string{"a.go"}, Components: []string{"pkg"}},
		Agent: "anchored", RequiredCapabilities: []string{"tools", "filesystem"},
		MinScopes:    []string{"spec:read", "memory:read"},
		Verification: planning.Verification{Strategy: "tests", Criteria: []string{"Given x When y Then z 1"}},
		Estimate:     planning.Estimate{Complexity: "low", Risk: "medium", TimeMS: 100, Tokens: 10, Note: planning.EstimateNote},
		Autonomy:     autonomy,
	}}
	p := &planning.Plan{
		Version: planning.PlanVersion, PlanVersion: version, MissionID: missionID,
		Objective: "Given x When y Then z 1", Kind: planning.KindSimple,
		Status: status, CreatedAt: time.Now().UTC(), Tasks: tasks,
	}
	if _, err := planning.SavePlan(dir, p); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteRunDryRunSealsNothing(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	executeMission(t, io, "mx1")
	executePlan(t, dir, "mx1", 1, planning.StatusReady, planning.AutonomyAutonomous)
	if code := io.ExecuteRunCmd([]string{"--id", "mx1", "--dry-run"}); code != 0 {
		t.Fatalf("dry-run exit = %d", code)
	}
	if !strings.Contains(out.String(), "dry-run") {
		t.Fatalf("dry-run must print the receipt, got %q", out.String())
	}
	out.Reset()
	if code := io.ExecuteStatusCmd([]string{"--id", "mx1"}); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	if strings.Contains(out.String(), "t1") {
		t.Fatalf("dry-run must seal no ledger task, got:\n%s", out.String())
	}
}

func TestExecuteRunRefusesUnexecutable(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	executeMission(t, io, "mx2")
	executePlan(t, dir, "mx2", 1, planning.StatusNeedsClarification, planning.AutonomyAutonomous)
	if code := io.ExecuteRunCmd([]string{"--id", "mx2", "--dry-run"}); code != 1 {
		t.Fatalf("clarification plan exit = %d, want 1", code)
	}
}

func TestExecuteApproveCancelRoundTrip(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	executeMission(t, io, "mx3")
	executePlan(t, dir, "mx3", 1, planning.StatusReady, planning.AutonomyNeedsApproval)
	// Supervised wet run holds the task (wave empty, nothing
	// dispatched) and exits 1 naming the blocked task.
	if code := io.ExecuteRunCmd([]string{"--id", "mx3", "--supervised"}); code != 1 {
		t.Fatalf("supervised held run exit = %d, want 1", code)
	}
	out.Reset()
	if code := io.ExecuteApproveCmd([]string{"--id", "mx3", "--task", "t1", "--actor", "op"}); code != 0 {
		t.Fatalf("approve exit = %d", code)
	}
	if !strings.Contains(out.String(), "approved") {
		t.Fatalf("approve must print the approval, got %q", out.String())
	}
	if code := io.ExecuteApproveCmd([]string{"--id", "mx3", "--task", "nope"}); code != 1 {
		t.Fatalf("unknown task approve exit = %d, want 1", code)
	}
	if code := io.ExecuteCancelCmd([]string{"--id", "mx3"}); code != 2 {
		t.Fatalf("cancel without reason exit = %d, want 2", code)
	}
	if code := io.ExecuteCancelCmd([]string{"--id", "mx3", "--reason", "halt", "--actor", "op"}); code != 0 {
		t.Fatalf("cancel exit = %d", code)
	}
	_, m, err := mission.LoadMission(dir, "mx3")
	if err != nil || m.State != mission.StateCancelled {
		t.Fatalf("mission must cancel, state=%v err=%v", m.State, err)
	}
}

func TestExecuteBenchEquivalent(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	executeMission(t, io, "mx4")
	executePlan(t, dir, "mx4", 1, planning.StatusReady, planning.AutonomyAutonomous)
	if code := io.ExecuteBenchCmd([]string{"--id", "mx4", "--parallel", "2", "--work-ms", "20"}); code != 0 {
		t.Fatalf("bench exit = %d", code)
	}
	if !strings.Contains(out.String(), "equivalent") {
		t.Fatalf("bench must report quality equivalence, got:\n%s", out.String())
	}
}

func TestExecuteUnknownSubcommand(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	if code := io.ExecuteCmd([]string{"frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", code)
	}
}
