// Extended mission-report tests (coverage-80/T3): dagEdges,
// recoveredDecision and shortSHA branches, AssembleReport against
// real stores (bare mission, full stores, unloadable plan,
// budget on/off) and every CLI error path of
// MissionReportCmd/MissionVerifyReportCmd.
package cmdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/execute"
	"regulus-ai/internal/failure"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/planning"
)

func terminalMission(t *testing.T, io IO, id string, extraCreate ...string) string {
	t.Helper()
	args := append([]string{"--id", id, "--creator", "tester"}, extraCreate...)
	mid := missionCreate(t, io, args...)
	for _, st := range []string{"analyzing", "planning", "awaiting_approval", "executing", "integrating", "validating", "completed"} {
		advance(t, io, mid, st)
	}
	return mid
}

func TestDagEdgesCases(t *testing.T) {
	if got := dagEdges(nil); len(got) != 0 {
		t.Fatalf("nil levels must yield no edges, got %v", got)
	}
	if got := dagEdges([][]string{}); len(got) != 0 {
		t.Fatalf("empty levels must yield no edges, got %v", got)
	}
	if got := dagEdges([][]string{{"a", "b"}}); len(got) != 0 {
		t.Fatalf("one level runs in parallel: no edges, got %v", got)
	}
	got := dagEdges([][]string{{"b", "a"}, {"d", "c"}})
	want := [][2]string{{"a", "c"}, {"a", "d"}, {"b", "c"}, {"b", "d"}}
	if len(got) != len(want) {
		t.Fatalf("two levels must cross in sorted order, got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("edge %d = %v, want %v (full: %v)", i, got[i], want[i], got)
		}
	}
	// Three levels cite consecutive steps only, never the transitive skip.
	got = dagEdges([][]string{{"a"}, {"b"}, {"c"}})
	if len(got) != 2 || got[0] != [2]string{"a", "b"} || got[1] != [2]string{"b", "c"} {
		t.Fatalf("three levels must chain consecutively, got %v", got)
	}
	// Determinism: shuffled input levels quote the same edges.
	again := dagEdges([][]string{{"a", "b"}, {"c", "d"}})
	for i := range want {
		if again[i] != want[i] {
			t.Fatalf("shuffled levels must quote the same edges, got %v want %v", again, want)
		}
	}
}

func TestRecoveredDecisionBranches(t *testing.T) {
	for _, d := range []string{"retry", "Retry", " RETRY ", "reassign", "Replan", "rollback", "ISOLATE", " reassign "} {
		if !recoveredDecision(d) {
			t.Fatalf("recoveredDecision(%q) = false, want true (recovery continues the mission)", d)
		}
	}
	for _, d := range []string{"", "  ", "escalate", "escalate_to_human", "fail_closed", "pause_for_approval", "ignore", "retry-later", "unknown"} {
		if recoveredDecision(d) {
			t.Fatalf("recoveredDecision(%q) = true, want false (the mission stays failed)", d)
		}
	}
}

func TestShortSHA(t *testing.T) {
	if got := shortSHA(""); got != "" {
		t.Fatalf("shortSHA(\"\") = %q, want empty", got)
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Fatalf("short digests pass through, got %q", got)
	}
	if got := shortSHA("0123456789ab"); got != "0123456789ab" {
		t.Fatalf("12-char digests pass through, got %q", got)
	}
	if got := shortSHA("0123456789abcdef"); got != "0123456789ab" {
		t.Fatalf("long digests truncate to 12, got %q", got)
	}
}

func TestMissionReportCmdFlagErrors(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	if code := io.MissionReportCmd([]string{"--bogus"}); code != 2 {
		t.Fatalf("unknown flag exit = %d, want 2", code)
	}
	if code := io.MissionReportCmd(nil); code != 2 {
		t.Fatalf("missing --id exit = %d, want 2", code)
	}
	errb.Reset()
	if code := io.MissionReportCmd([]string{"--id", "   "}); code != 2 {
		t.Fatalf("blank --id exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--id is required") {
		t.Fatalf("blank --id must say so, got %q", errb.String())
	}
}

func TestMissionReportCmdMissingMission(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	if code := io.MissionReportCmd([]string{"--id", "ghost"}); code != 1 {
		t.Fatalf("report of a missing mission exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "mission report:") {
		t.Fatalf("must name the failed report, got %q", errb.String())
	}
}

func TestMissionReportCmdJSONHappyPath(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	id := terminalMission(t, io, "mjson",
		"--criteria", "engine persists state",
		"--budget-tokens", "100", "--budget-cost", "1.5", "--budget-time-ms", "5000")
	out.Reset()
	if code := io.MissionReportCmd([]string{"--id", id, "--json"}); code != 0 {
		t.Fatalf("report --json exit = %d", code)
	}
	var decoded mission.Report
	if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
		t.Fatalf("--json must print the sealed report: %v\n%s", err, out.String())
	}
	if decoded.MissionID != id || decoded.Outcome != "completed" || decoded.Hash == "" {
		t.Fatalf("--json decoded = %+v", decoded)
	}
	path := filepath.Join(dir, ".regulus", "missions", id, ReportFileName)
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		t.Fatalf("report file must persist: %v", err)
	}
	if _, err := mission.VerifyReport(raw); err != nil {
		t.Fatalf("persisted file must verify: %v", err)
	}
}

func TestMissionVerifyReportCmdErrors(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	if code := io.MissionVerifyReportCmd([]string{"--bogus"}); code != 2 {
		t.Fatalf("unknown flag exit = %d, want 2", code)
	}
	if code := io.MissionVerifyReportCmd(nil); code != 2 {
		t.Fatalf("missing --id exit = %d, want 2", code)
	}
	if code := io.MissionVerifyReportCmd([]string{"--id", "   "}); code != 2 {
		t.Fatalf("blank --id exit = %d, want 2", code)
	}
	// A terminal mission with no expediente yet: nothing to verify.
	id := terminalMission(t, io, "mnover")
	if code := io.MissionVerifyReportCmd([]string{"--id", id}); code != 1 {
		t.Fatalf("verify without a report exit = %d, want 1", code)
	}
	if code := io.MissionVerifyReportCmd([]string{"--id", "ghost"}); code != 1 {
		t.Fatalf("verify of a missing mission exit = %d, want 1", code)
	}
}

func TestAssembleReportBareMission(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	id := terminalMission(t, io, "mbare", "--criteria", "engine persists state")

	rep, err := AssembleReport(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Plans) != 0 || len(rep.Tasks) != 0 || len(rep.Approvals) != 0 {
		t.Fatalf("a bare mission cites nothing: plans=%v tasks=%v approvals=%v", rep.Plans, rep.Tasks, rep.Approvals)
	}
	if len(rep.Failures) != 0 || len(rep.Recovery) != 0 {
		t.Fatalf("no decisions recorded, none cited: %+v / %+v", rep.Failures, rep.Recovery)
	}
	quoted := strings.Join(rep.Limitations, "\n")
	for _, want := range []string{"hosts not recorded", "gate rollup unknown"} {
		if !strings.Contains(quoted, want) {
			t.Fatalf("bare report must record %q as unknown, got %q", want, quoted)
		}
	}
	for _, no := range []string{"plan store unreadable", "failure decisions unreadable", "governance store unreadable"} {
		if strings.Contains(quoted, no) {
			t.Fatalf("present-but-empty stores are not unreadable: %q", quoted)
		}
	}
	if len(rep.Costs) != 1 || rep.Costs[0].Label != "spend-measured" || rep.Costs[0].Quality != mission.QualityUnknown {
		t.Fatalf("zero budget cites only unknown spend: %+v", rep.Costs)
	}
	if rep.Latency.Quality != mission.QualityUnknown {
		t.Fatalf("wall-clock stays unknown: %+v", rep.Latency)
	}
	raw, err := rep.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mission.VerifyReport(raw); err != nil {
		t.Fatalf("bare report must verify: %v", err)
	}
}

func TestAssembleReportFullStores(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	id := missionCreate(t, io, "--id", "mfull", "--creator", "tester",
		"--criteria", "engine persists state",
		"--budget-tokens", "100", "--budget-cost", "1.5", "--budget-time-ms", "5000")
	for _, st := range []string{"analyzing", "planning", "awaiting_approval", "executing"} {
		advance(t, io, id, st)
	}
	if code := io.MissionLinkCmd([]string{"--id", id,
		"--run", "run_1,run_1", "--task", "t1", "--agent", "a1",
		"--worktree", "wt/mfull", "--commit", "abc123",
		"--trace", "run_1", "--receipt", "rc1"}); code != 0 {
		t.Fatalf("link exit = %d", code)
	}
	for _, st := range []string{"integrating", "validating", "completed"} {
		advance(t, io, id, st)
	}

	if _, err := planning.SavePlan(dir, &planning.Plan{
		Version: planning.PlanVersion, PlanVersion: 1, MissionID: id,
		Objective: "ship durable missions", Kind: planning.KindSimple, Status: planning.StatusReady,
		Tasks:    []planning.PlanTask{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
		DAG:      planning.DAG{Levels: [][]string{{"b", "a"}, {"d", "c"}}},
		RepoHead: "0123456789abcdef0123456789abcdef",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := execute.UpdateLedger(dir, id, time.Now(), func(l *execute.Ledger) error {
		if l.Tasks == nil {
			l.Tasks = map[string]execute.TaskRecord{}
		}
		l.Tasks["t2"] = execute.TaskRecord{TaskID: "t2", PlanVersion: 1, State: execute.TaskSucceeded, Agent: "agent-a"}
		l.Tasks["t1"] = execute.TaskRecord{
			TaskID: "t1", PlanVersion: 1, State: execute.TaskSucceeded,
			Agent: "agent-b", OpKey: "exec/mfull/v1/t1/a0",
			Result: execute.TaskResult{RunID: "run-9", Commit: "cafef00d", Agent: "agent-a"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []failure.DecisionRecord{
		{ID: "dec_retry_1", MissionID: id, At: time.Now(), Cause: "agent offline", Class: "agent_unavailable", ClassDetail: "no heartbeat for 5m", Decision: "retry", Reason: "transient", Result: "regulus recovery retry t1"},
		{ID: "dec_esc_1", MissionID: id, At: time.Now(), Cause: "quota hit", Class: "budget_exhausted", ClassDetail: "tokens spent", Decision: "escalate", Reason: "needs a human", Result: "regulus mission pause_for_approval"},
	} {
		if _, _, err := failure.AppendDecision(dir, d); err != nil {
			t.Fatal(err)
		}
	}

	rep, err := AssembleReport(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Plans) != 1 || rep.Plans[0].Version != 1 || rep.Plans[0].Digest == "" {
		t.Fatalf("one plan revision must be cited with its digest: %+v", rep.Plans)
	}
	wantDAG := [][2]string{{"a", "c"}, {"a", "d"}, {"b", "c"}, {"b", "d"}}
	if len(rep.DAG) != len(wantDAG) {
		t.Fatalf("DAG must derive from plan levels, got %v", rep.DAG)
	}
	for i := range wantDAG {
		if rep.DAG[i] != wantDAG[i] {
			t.Fatalf("DAG edge %d = %v, want %v", i, rep.DAG[i], wantDAG[i])
		}
	}
	if len(rep.Routing) != 1 || !strings.Contains(rep.Routing[0], "plan v1 (0123456789ab") {
		t.Fatalf("routing must quote the plan with its short SHA: %v", rep.Routing)
	}
	if len(rep.Tasks) != 2 || rep.Tasks[0].ID != "t1" || rep.Tasks[1].ID != "t2" {
		t.Fatalf("ledger tasks quote in deterministic order: %+v", rep.Tasks)
	}
	for _, ev := range []string{"run:run-9", "opkey:exec/mfull/v1/t1/a0"} {
		found := false
		for _, e := range rep.Tasks[0].Evidence {
			if e == ev {
				found = true
			}
		}
		if !found {
			t.Fatalf("task evidence must cite %q, got %v", ev, rep.Tasks[0].Evidence)
		}
	}
	if len(rep.Agents) != 2 || rep.Agents[0] != "agent-a" || rep.Agents[1] != "agent-b" {
		t.Fatalf("agents stay sorted: %v", rep.Agents)
	}
	for _, want := range []string{"abc123", "cafef00d"} {
		found := false
		for _, c := range rep.Commits {
			if c == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("commits must cite %q (links + ledger): %v", want, rep.Commits)
		}
	}
	if len(rep.Diffs) != 1 || rep.Diffs[0] != "rc1" {
		t.Fatalf("receipts ride as diffs: %v", rep.Diffs)
	}
	if len(rep.Failures) != 2 || !rep.Failures[0].Recovered || rep.Failures[1].Recovered {
		t.Fatalf("retry recovers, escalate does not: %+v", rep.Failures)
	}
	if !strings.Contains(rep.Failures[0].Detail, "agent offline") {
		t.Fatalf("failure detail cites the cause: %+v", rep.Failures[0])
	}
	if len(rep.Recovery) != 2 || rep.Recovery[0].Action != "retry" || rep.Recovery[1].Action != "escalate" {
		t.Fatalf("recovery quotes the decisions: %+v", rep.Recovery)
	}
	qualities := map[string]string{}
	for _, c := range rep.Costs {
		qualities[c.Label] = c.Quality
	}
	if qualities["budget-tokens"] != mission.QualityEstimated || qualities["budget-cost"] != mission.QualityEstimated || qualities["budget-time-ms"] != mission.QualityEstimated {
		t.Fatalf("budget figures are estimates by construction: %+v", rep.Costs)
	}
	if qualities["spend-measured"] != mission.QualityUnknown {
		t.Fatalf("measured spend stays unknown: %+v", rep.Costs)
	}
	raw, err := rep.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mission.VerifyReport(raw); err != nil {
		t.Fatalf("full report must verify: %v", err)
	}
}

func TestAssembleReportLatestPlanUnloadable(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	id := terminalMission(t, io, "mplanbad", "--criteria", "engine persists state")
	if _, err := planning.SavePlan(dir, &planning.Plan{
		Version: planning.PlanVersion, PlanVersion: 1, MissionID: id,
		Objective: "ship durable missions", Kind: planning.KindSimple, Status: planning.StatusReady,
		DAG: planning.DAG{Levels: [][]string{{"a"}, {"b"}}},
	}); err != nil {
		t.Fatal(err)
	}
	planFile := filepath.Join(dir, ".regulus", "missions", id, "plans", "plan-0001.json")
	if err := os.WriteFile(planFile, []byte("edited-by-hand"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := AssembleReport(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Plans) != 1 {
		t.Fatalf("the revision stays cited even when its bytes fail: %+v", rep.Plans)
	}
	if len(rep.DAG) != 0 {
		t.Fatalf("an unloadable plan yields no DAG edges: %v", rep.DAG)
	}
	if !strings.Contains(strings.Join(rep.Limitations, "\n"), "latest plan unloadable") {
		t.Fatalf("must record the unloadable plan as unknown: %q", rep.Limitations)
	}
	if _, err := mission.VerifyReport(mustReportBytes(t, rep)); err != nil {
		t.Fatalf("limited report still verifies: %v", err)
	}
}

func mustReportBytes(t *testing.T, rep mission.Report) []byte {
	t.Helper()
	raw, err := rep.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMissionReportPathAndJSONHelpers(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	id := terminalMission(t, io, "mpath", "--criteria", "engine persists state")
	path, err := missionReportPath(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, ReportFileName) || !strings.Contains(path, id) {
		t.Fatalf("report path must cite the mission and file name, got %q", path)
	}
	if _, err := missionReportPath(dir, "../escape"); err == nil {
		t.Fatal("a traversing mission id must not resolve a report path")
	}
	rep, err := AssembleReport(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	var decoded mission.Report
	if err := json.Unmarshal([]byte(missionReportJSON(rep)), &decoded); err != nil {
		t.Fatalf("missionReportJSON must print the sealed report: %v", err)
	}
	if decoded.MissionID != id || decoded.Hash != rep.Hash {
		t.Fatalf("JSON helper drift: %+v", decoded)
	}
}
