// Mission planner handlers (F13.2/NOV-105): `regulus mission
// plan|replan|explain`. Moved-out-of-cli by construction (the F12.4
// pattern): the logic lives here, internal/cli keeps only thin
// forwarders.
//
// Reuse, never duplication:
//   - plans: internal/planning (versioned store, deterministic
//     builder, quality gates) — this layer only parses flags,
//     loads the mission, persists one revision and prints;
//   - trace: observability.RecordExec sharing ids (F11.1) via
//     emitMissionEvent; dry-run records nothing;
//   - locking: planning.SavePlan is the single writer path (dir
//     lock + sha manifest); handlers never touch plan files.
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

func missionPlanUsage() string {
	return "usage: regulus mission plan --id <id> [--objective <o>] [--constraint <a,b>] [--criteria <a,b>] [--author <a>] [--actor <a>] [--dry-run] [--json] | regulus mission replan --id <id> --change <reason> [--objective <o>] [--constraint <a,b>] [--criteria <a,b>] [--confirm <task=result,a,b>] [--author <a>] [--actor <a>] [--dry-run] [--json] | regulus mission explain --id <id> [--version N] [--json]"
}

// MissionPlanCmd implements `regulus mission plan`.
func (io IO) MissionPlanCmd(args []string) int {
	fs := flag.NewFlagSet("mission plan", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	objective := fs.String("objective", "", "objective override (default: the mission objective)")
	constraint := fs.String("constraint", "", "comma-separated extra restrictions")
	criteria := fs.String("criteria", "", "comma-separated extra acceptance criteria")
	author := fs.String("author", "", "plan author (audit)")
	actor := fs.String("actor", "", "operator ordering this write (audit)")
	dryRun := fs.Bool("dry-run", false, "compute and print without persisting anything")
	asJSON := fs.Bool("json", false, "print the plan as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionPlanUsage())
		return 2
	}
	_, m, err := mission.LoadMission(io.Dir, *id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission plan: %v\n", err)
		return 1
	}
	if mission.IsTerminal(m.State) {
		fmt.Fprintf(io.Stderr, "mission plan: mission %q is %s (terminal: reopen explicitly with a reason before planning)\n", m.ID, m.State)
		return 1
	}
	now := time.Now().UTC()
	p, err := planning.BuildPlan(io.Dir, m, planning.Request{
		Objective: *objective, Constraints: splitList(*constraint),
		Criteria: splitList(*criteria), Author: *author,
	}, now)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission plan: %v\n", err)
		return 1
	}
	if code := checkPlanValid(p, io); code != 0 {
		return code
	}
	if *dryRun {
		return printPlan(io, p, *asJSON, "dry-run: nothing persisted")
	}
	if _, err := planning.SavePlan(io.Dir, p); err != nil {
		fmt.Fprintf(io.Stderr, "mission plan: %v\n", err)
		return 1
	}
	sealed, err := planning.LatestPlan(io.Dir, m.ID)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission plan: %v\n", err)
		return 1
	}
	io.emitMissionEvent(m, "mission plan", map[string]any{"actor": *actor, "plan_version": sealed.PlanVersion, "status": sealed.Status})
	if *asJSON {
		return printPlanJSON(io, sealed)
	}
	fmt.Fprintf(io.Stdout, "mission: %s planned v%d status=%s kind=%s tasks=%d\n", sealed.MissionID, sealed.PlanVersion, sealed.Status, sealed.Kind, len(sealed.Tasks))
	fmt.Fprint(io.Stdout, planning.RenderPlan(sealed))
	return planExitFor(sealed)
}

// MissionReplanCmd implements `regulus mission replan`.
func (io IO) MissionReplanCmd(args []string) int {
	fs := flag.NewFlagSet("mission replan", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	change := fs.String("change", "", "why this revision exists (required, kept per version)")
	objective := fs.String("objective", "", "objective override (default: keep the previous one)")
	constraint := fs.String("constraint", "", "comma-separated extra restrictions")
	criteria := fs.String("criteria", "", "comma-separated extra acceptance criteria")
	confirm := fs.String("confirm", "", "comma-separated task=result outcomes already confirmed (append-only)")
	author := fs.String("author", "", "plan author (audit)")
	actor := fs.String("actor", "", "operator ordering this write (audit)")
	dryRun := fs.Bool("dry-run", false, "compute and print without persisting anything")
	asJSON := fs.Bool("json", false, "print the plan as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionPlanUsage())
		return 2
	}
	if strings.TrimSpace(*change) == "" || strings.HasPrefix(strings.TrimSpace(*change), "-") {
		fmt.Fprintln(io.Stderr, missionPlanUsage())
		return 2
	}
	_, m, err := mission.LoadMission(io.Dir, *id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission replan: %v\n", err)
		return 1
	}
	if mission.IsTerminal(m.State) {
		fmt.Fprintf(io.Stderr, "mission replan: mission %q is %s (terminal: reopen explicitly with a reason before replanning)\n", m.ID, m.State)
		return 1
	}
	prev, err := planning.LatestPlan(io.Dir, *id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission replan: %v\n", err)
		return 1
	}
	confirmed, err := parseConfirmed(*confirm)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission replan: %v\n", err)
		return 2
	}
	now := time.Now().UTC()
	next, err := planning.Replan(io.Dir, prev, m, planning.Request{
		Objective: *objective, Constraints: splitList(*constraint),
		Criteria: splitList(*criteria), Author: *author,
		ChangeReason: *change, Confirmed: confirmed,
	}, now)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission replan: %v\n", err)
		return 1
	}
	if code := checkPlanValid(next, io); code != 0 {
		return code
	}
	if *dryRun {
		if *asJSON {
			return printPlanJSON(io, next)
		}
		fmt.Fprintf(io.Stdout, "dry-run: nothing persisted\n%s%s", planning.RenderDiff(planning.DiffPlans(prev, next)), planning.RenderPlan(next))
		return planExitFor(next)
	}
	if _, err := planning.SavePlan(io.Dir, next); err != nil {
		fmt.Fprintf(io.Stderr, "mission replan: %v\n", err)
		return 1
	}
	io.emitMissionEvent(m, "mission replan", map[string]any{"actor": *actor, "plan_version": next.PlanVersion, "status": next.Status})
	if *asJSON {
		return printPlanJSON(io, next)
	}
	fmt.Fprintf(io.Stdout, "mission: %s replanned v%d status=%s (%s)\n", next.MissionID, next.PlanVersion, next.Status, *change)
	fmt.Fprintf(io.Stdout, "%s", planning.RenderDiff(planning.DiffPlans(prev, next)))
	fmt.Fprint(io.Stdout, planning.RenderPlan(next))
	return planExitFor(next)
}

// MissionExplainCmd implements `regulus mission explain`.
func (io IO) MissionExplainCmd(args []string) int {
	fs := flag.NewFlagSet("mission explain", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	version := fs.Int("version", 0, "plan revision to explain (default: latest)")
	asJSON := fs.Bool("json", false, "print the explanation as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionPlanUsage())
		return 2
	}
	if _, _, err := mission.LoadMission(io.Dir, *id); err != nil {
		fmt.Fprintf(io.Stderr, "mission explain: %v\n", err)
		return 1
	}
	var current *planning.Plan
	var err error
	if *version <= 0 {
		current, err = planning.LatestPlan(io.Dir, *id)
	} else {
		current, err = planning.LoadPlan(io.Dir, *id, *version)
	}
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission explain: %v\n", err)
		return 1
	}
	entries, err := planning.ListPlans(io.Dir, *id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission explain: %v\n", err)
		return 1
	}
	var history []*planning.Plan
	for _, e := range entries {
		h, herr := planning.LoadPlan(io.Dir, *id, e.Version)
		if herr != nil {
			fmt.Fprintf(io.Stderr, "mission explain: %v\n", herr)
			return 1
		}
		history = append(history, h)
	}
	if *asJSON {
		out := map[string]any{"plan": current, "history": entries}
		if len(history) > 0 {
			out["diff_vs_original"] = planning.DiffPlans(history[0], current)
			for i, h := range history {
				if h.PlanVersion == current.PlanVersion && i > 0 {
					out["diff_vs_previous"] = planning.DiffPlans(history[i-1], current)
				}
			}
		}
		raw, merr := json.MarshalIndent(out, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "mission explain: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return planExitFor(current)
	}
	fmt.Fprint(io.Stdout, planning.RenderExplain(current, history))
	return planExitFor(current)
}

// planRegistry rebuilds the F12.1 routing inventory the planner
// graded capabilities against, so validation re-checks the same
// verified evidence the builder assigned from.
func planRegistry() ([]orchestrate.RoutedAgent, map[string]bench.HostManifest) {
	manifests := bench.DefaultHostManifests(bench.EvidenceStamp())
	return orchestrate.RegistryFromManifests(manifests), manifests
}

// checkPlanValid fails closed on structural plan defects before
// anything persists or executes. Quality gates that ask for a
// human (clarification/approval) print as guidance and keep their
// planExitFor code; structural defects return 1.
func checkPlanValid(p *planning.Plan, io IO) int {
	reg, manifests := planRegistry()
	if err := planning.ValidatePlan(p, reg, manifests); err != nil {
		fmt.Fprintf(io.Stderr, "mission plan: invalid plan: %v\n", err)
		return 1
	}
	return 0
}

// planExitFor maps plan readiness to exit codes without laundering:
// ready and approved-gated plans print 0 (the operator acts on the
// status line), refused and clarification-needing plans print 1 —
// never a passing zero that hides a blocked plan.
func planExitFor(p *planning.Plan) int {
	switch p.Status {
	case planning.StatusReady, planning.StatusNeedsApproval:
		return 0
	default:
		return 1
	}
}

// printPlan renders the human plan with its dry-run banner.
func printPlan(io IO, p *planning.Plan, asJSON bool, banner string) int {
	if asJSON {
		return printPlanJSON(io, p)
	}
	fmt.Fprintf(io.Stdout, "%s\n", banner)
	fmt.Fprint(io.Stdout, planning.RenderPlan(p))
	return planExitFor(p)
}

// printPlanJSON prints one plan revision as JSON.
func printPlanJSON(io IO, p *planning.Plan) int {
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission plan: %v\n", err)
		return 1
	}
	fmt.Fprintln(io.Stdout, string(raw))
	return planExitFor(p)
}

// parseConfirmed parses --confirm task=result pairs (append-only
// confirmed outcomes travelling across replans).
func parseConfirmed(s string) ([]planning.ConfirmedResult, error) {
	var out []planning.ConfirmedResult
	for _, part := range splitList(s) {
		task, result, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(task) == "" || strings.TrimSpace(result) == "" {
			return nil, fmt.Errorf("invalid --confirm entry %q (want task=result)", part)
		}
		out = append(out, planning.ConfirmedResult{TaskID: strings.TrimSpace(task), Result: strings.TrimSpace(result), At: time.Now().UTC()})
	}
	return out, nil
}
