// Mission evidence report handlers (F13.7/NOV-110): `regulus
// mission report|verify-report` over the mission expediente
// (internal/mission report.go).
//
// Reuse, never duplication:
//   - truth: mission store (model/states), planning store (plan
//     revisions + DAG), execute ledger (task states/agents/
//     commits), govern store (approvals), failure store
//     (decisions). This layer only assembles citations, it never
//     re-derives plans, replays tasks or re-decides recovery.
//   - redaction/integrity: mission.BuildReport/VerifyReport (which
//     apply observability.RedactText and seal the hash).
//   - unavailable data (hosts, measured costs, mission-scoped gate
//     rollups) is recorded as unknown with its reason, never
//     invented (S7: no se inventan datos).
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/execute"
	"regulus-ai/internal/failure"
	"regulus-ai/internal/govern"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/planning"
)

// ReportFileName is the derived expediente next to the mission
// record. Derived, not sealed input: report rewrites it, the hash
// inside proves which bytes were exported.
const ReportFileName = "report-v1.json"

func missionReportPath(root, missionID string) (string, error) {
	dir, err := mission.StoreDir(root, missionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ReportFileName), nil
}

// MissionReportCmd implements `regulus mission report --id <mid>`:
// assemble the expediente from every store that cites the mission,
// seal it, persist it next to the mission record and quote it.
func (io IO) MissionReportCmd(args []string) int {
	fs := flag.NewFlagSet("mission report", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	asJSON := fs.Bool("json", false, "print the sealed report as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*id) == "" {
		fmt.Fprintln(io.Stderr, "mission report: --id is required")
		return 2
	}
	rep, err := AssembleReport(io.Dir, strings.TrimSpace(*id))
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission report: %v\n", err)
		return 1
	}
	raw, err := rep.Bytes()
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission report: %v\n", err)
		return 1
	}
	path, err := missionReportPath(io.Dir, strings.TrimSpace(*id))
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission report: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(io.Stderr, "mission report: %v\n", err)
		return 1
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		fmt.Fprintf(io.Stderr, "mission report: %v\n", err)
		return 1
	}
	if eventID, merr := observability.MintEventID(); merr == nil {
		_, _, _ = observability.RecordExec(io.Dir, observability.ExecEvent{
			SchemaVersion: observability.ExecSchemaVersion,
			EventID:       eventID, TaskID: strings.TrimSpace(*id),
			Type: observability.ExecTaskEnd, Status: "pass",
			Payload: map[string]any{"action": "mission report"},
		}, time.Now().UTC())
	}
	if *asJSON {
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, mission.RenderReport(rep))
	fmt.Fprintf(io.Stdout, "  file: %s\n", path)
	return 0
}

// MissionVerifyReportCmd implements `regulus mission verify-report
// --id <mid>`: reload the persisted expediente and revalidate
// version, outcome vocabulary and the integrity hash.
func (io IO) MissionVerifyReportCmd(args []string) int {
	fs := flag.NewFlagSet("mission verify-report", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*id) == "" {
		fmt.Fprintln(io.Stderr, "mission verify-report: --id is required")
		return 2
	}
	path, err := missionReportPath(io.Dir, strings.TrimSpace(*id))
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission verify-report: %v\n", err)
		return 1
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission verify-report: %v\n", err)
		return 1
	}
	rep, err := mission.VerifyReport(raw)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission verify-report: INVALID: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "mission verify-report: VALID %s outcome=%s hash=%s\n", rep.MissionID, rep.Outcome, rep.Hash)
	return 0
}

// AssembleReport gathers every citation for one terminal mission.
// Stores stay authoritative; gaps arrive as unknown metrics or
// limitation lines, never as invented data.
func AssembleReport(root, missionID string) (mission.Report, error) {
	_, m, err := mission.LoadMission(root, missionID)
	if err != nil {
		return mission.Report{}, err
	}
	in := mission.ReportInput{Mission: m}
	limitations := []string{}

	// Plans: every revision with its digest and change note; the
	// DAG edges derive from the latest plan levels (each level runs
	// after the previous one).
	if entries, err := planning.ListPlans(root, missionID); err == nil {
		for _, e := range entries {
			in.Plans = append(in.Plans, mission.PlanRef{Version: e.Version, Digest: e.SHA, ChangeNote: e.ChangeReason})
		}
		if len(entries) > 0 {
			latest := entries[len(entries)-1]
			if p, err := planning.LoadPlan(root, missionID, latest.Version); err == nil {
				in.DAG = dagEdges(p.DAG.Levels)
				in.Routing = append(in.Routing, fmt.Sprintf("plan v%d (%s): %d tasks, %d levels",
					p.PlanVersion, shortSHA(p.RepoHead), len(p.Tasks), len(p.DAG.Levels)))
			} else {
				limitations = append(limitations, "latest plan unloadable ("+err.Error()+"): DAG unknown, not defaulted")
			}
		}
	} else {
		limitations = append(limitations, "plan store unreadable ("+err.Error()+"): plans unknown, not defaulted")
	}

	// Ledger: task states, agents, commits and run/opkey evidence.
	// The ledger records no hosts: hosts stay unknown with the
	// reason, per S7 (registrar no-ejecutado/unknown).
	agents := map[string]bool{}
	commits := map[string]bool{}
	for _, c := range m.Links.Commits {
		commits[c] = true
	}
	if led, err := execute.LoadLedger(root, missionID); err == nil {
		for _, id := range execute.SortedTaskIDs(led) {
			rec := led.Tasks[id]
			ev := []string{}
			if rec.Result.RunID != "" {
				ev = append(ev, "run:"+rec.Result.RunID)
			}
			if rec.OpKey != "" {
				ev = append(ev, "opkey:"+rec.OpKey)
			}
			if rec.Result.Commit != "" {
				commits[rec.Result.Commit] = true
			}
			if rec.Agent != "" {
				agents[rec.Agent] = true
			}
			if rec.Result.Agent != "" {
				agents[rec.Result.Agent] = true
			}
			in.Tasks = append(in.Tasks, mission.TaskReport{
				ID: id, State: rec.State, Agent: rec.Agent,
				Host: "", Commit: rec.Result.Commit, Evidence: ev,
			})
		}
	} else {
		limitations = append(limitations, "execution ledger unreadable ("+err.Error()+"): tasks unknown, not defaulted")
	}
	limitations = append(limitations, "hosts not recorded by the execution ledger: unknown, not defaulted (multi-host runs cite hosts when available)")
	for a := range agents {
		in.Agents = append(in.Agents, a)
	}
	sort.Strings(in.Agents)
	for c := range commits {
		in.Commits = append(in.Commits, c)
	}
	sort.Strings(in.Commits)
	in.Diffs = append([]string(nil), m.Links.Receipts...)

	// Governance: approvals citing this mission, decision states
	// quoted, never re-decided.
	if reqs, _, err := govern.ListApprovals(root); err == nil {
		for _, r := range reqs {
			if r.MissionID != "" && r.MissionID != missionID {
				continue
			}
			in.Approvals = append(in.Approvals, mission.ApprovalRef{
				ID: r.ID, State: r.State, Artifact: r.BoundArtifact,
				Policy: r.PolicyName + "@" + r.PolicyVersion,
			})
		}
	} else {
		limitations = append(limitations, "governance store unreadable: approvals unknown, not defaulted")
	}

	// Failures + recovery: the coordinator decisions are the
	// mission-level record; task-local retries stay in the F12.5
	// ledger, cited via task evidence.
	if recs, err := failure.LoadDecisions(root, missionID); err == nil {
		for _, d := range recs {
			in.Failures = append(in.Failures, mission.FailureEntry{
				Class: d.Class, Detail: d.Cause + " — " + d.ClassDetail, Recovered: recoveredDecision(d.Decision),
			})
			in.Recovery = append(in.Recovery, mission.RecoveryEntry{
				Action: d.Decision, Reason: d.Reason, Outcome: d.Result,
			})
		}
	} else {
		limitations = append(limitations, "failure decisions unreadable: failures unknown, not defaulted")
	}

	// Costs: the mission budget is an estimate by construction;
	// measured spend needs a metered host the ledger does not cite.
	if m.Budget.Tokens > 0 || m.Budget.Cost > 0 || m.Budget.TimeMS > 0 {
		in.Costs = append(in.Costs, mission.Metric{Label: "budget-tokens", Quality: mission.QualityEstimated,
			Value: fmt.Sprintf("%d", m.Budget.Tokens)})
		in.Costs = append(in.Costs, mission.Metric{Label: "budget-cost", Quality: mission.QualityEstimated,
			Value: fmt.Sprintf("%v", m.Budget.Cost)})
		in.Costs = append(in.Costs, mission.Metric{Label: "budget-time-ms", Quality: mission.QualityEstimated,
			Value: fmt.Sprintf("%d", m.Budget.TimeMS)})
	}
	in.Costs = append(in.Costs, mission.Metric{Label: "spend-measured", Quality: mission.QualityUnknown,
		Value: "unavailable: no metered host cited by the ledger"})
	in.Latency = mission.Metric{Label: "wall-clock", Quality: mission.QualityUnknown,
		Value: "unavailable: the mission store records transitions, not wall-clock spans"}

	// Tests: no mission-scoped verify index exists; per-task gate
	// verdicts live behind task evidence links. Recorded unknown
	// with the path, never a fabricated rollup.
	limitations = append(limitations, "gate rollup unknown: per-task verdicts live behind tasks[].evidence run links (no mission-scoped verify index); full-suite contrast not run")
	in.Limitations = limitations

	return mission.BuildReport(in)
}

// dagEdges derives citation edges from plan levels: level N+1 runs
// after level N. The plan stays authoritative; these edges are the
// quote, ordered deterministically.
func dagEdges(levels [][]string) [][2]string {
	var out [][2]string
	for i := 1; i < len(levels); i++ {
		prev := append([]string(nil), levels[i-1]...)
		cur := append([]string(nil), levels[i]...)
		sort.Strings(prev)
		sort.Strings(cur)
		for _, a := range prev {
			for _, b := range cur {
				out = append(out, [2]string{a, b})
			}
		}
	}
	return out
}

func recoveredDecision(decision string) bool {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "retry", "reassign", "replan", "rollback", "isolate":
		return true
	}
	return false
}

func shortSHA(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// missionReportJSON renders one assembled report for --json paths.
func missionReportJSON(rep mission.Report) string {
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(raw)
}
