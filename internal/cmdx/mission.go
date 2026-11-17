// Mission handlers (F13.1/NOV-104): `regulus mission ...`
// Moved-out-of-cli by construction (the F12.4 pattern): the logic
// lives here, internal/cli keeps only a thin forwarder plus init.
//
// Reuse, never duplication:
//   - durability: internal/mission (model, state machine,
//     atomic store, checksum, migration, generation conflicts) —
//     this layer only parses flags, applies one Update and prints.
//   - trace: observability.RecordExec sharing ids (F11.1); the
//     mission store never duplicates the exec log, it cites
//     mission/run/task/agent ids the handlers link.
//   - locking/concurrency: mission.UpdateMission is the single
//     coordinator path (dir lock + generation check); handlers
//     never touch the store files directly.
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/mission"
	"regulus-ai/internal/observability"
)

func missionUsage() string {
	return "usage: regulus mission create --objective <o> [--id <id>] [--description <d>] [--repo <r>] [--workspace <w>] [--constraint <a,b>] [--criteria <a,b>] [--risk low|medium|high|critical] [--budget-time-ms N] [--budget-tokens N] [--budget-cost N] [--creator <c>] [--actor <a>] [--json] | regulus mission show --id <id> [--json] | regulus mission list [--state <s>] [--json] | regulus mission status --id <id> [--json] | regulus mission advance --id <id> --to <state> [--reason <r>] [--actor <a>] [--idem-key <k>] | regulus mission pause --id <id> [--reason <r>] [--actor <a>] [--idem-key <k>] | regulus mission resume --id <id> [--reason <r>] [--actor <a>] [--idem-key <k>] | regulus mission cancel --id <id> --reason <r> [--actor <a>] [--idem-key <k>] | regulus mission link --id <id> [--run <a,b>] [--task <a,b>] [--agent <a,b>] [--worktree <a,b>] [--commit <a,b>] [--trace <a,b>] [--receipt <a,b>] [--actor <a>] | regulus mission reopen --id <id> --reason <r> [--actor <a>] [--idem-key <k>] | regulus mission plan --id <id> [--objective <o>] [--constraint <a,b>] [--criteria <a,b>] [--author <a>] [--actor <a>] [--dry-run] [--json] | regulus mission replan --id <id> --change <reason> [--objective <o>] [--constraint <a,b>] [--criteria <a,b>] [--confirm <task=result,a,b>] [--author <a>] [--actor <a>] [--dry-run] [--json] | regulus mission explain --id <id> [--version N] [--json] | regulus mission report --id <mid> [--json] | regulus mission verify-report --id <mid>"
}

// MissionCmd routes `regulus mission ...`.
func (io IO) MissionCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, missionUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	switch args[0] {
	case "create":
		return io.MissionCreateCmd(args[1:])
	case "show":
		return io.MissionShowCmd(args[1:])
	case "list":
		return io.MissionListCmd(args[1:])
	case "status":
		return io.MissionStatusCmd(args[1:])
	case "advance":
		return io.MissionAdvanceCmd(args[1:])
	case "pause":
		return io.MissionPauseCmd(args[1:])
	case "resume":
		return io.MissionResumeCmd(args[1:])
	case "cancel":
		return io.MissionCancelCmd(args[1:])
	case "link":
		return io.MissionLinkCmd(args[1:])
	case "reopen":
		return io.MissionReopenCmd(args[1:])
	case "plan":
		return io.MissionPlanCmd(args[1:])
	case "replan":
		return io.MissionReplanCmd(args[1:])
	case "explain":
		return io.MissionExplainCmd(args[1:])
	case "report":
		return io.MissionReportCmd(args[1:])
	case "verify-report":
		return io.MissionVerifyReportCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "mission: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
}

// MissionCreateCmd implements `regulus mission create`.
func (io IO) MissionCreateCmd(args []string) int {
	fs := flag.NewFlagSet("mission create", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (optional, minted when empty)")
	objective := fs.String("objective", "", "mission objective (required)")
	description := fs.String("description", "", "one-line description")
	repo := fs.String("repo", "", "repository/workspace label")
	workspace := fs.String("workspace", "", "workspace path label")
	constraints := fs.String("constraint", "", "comma-separated restrictions")
	criteria := fs.String("criteria", "", "comma-separated acceptance criteria")
	risk := fs.String("risk", "medium", "low|medium|high|critical")
	budgetTime := fs.Int64("budget-time-ms", 0, "wall-clock ceiling in ms (0 = unset)")
	budgetTokens := fs.Int64("budget-tokens", 0, "token ceiling (0 = unset)")
	budgetCost := fs.Float64("budget-cost", 0, "money ceiling (0 = unset)")
	creator := fs.String("creator", "", "mission creator")
	actor := fs.String("actor", "", "operator ordering this write (audit)")
	asJSON := fs.Bool("json", false, "print the sealed record as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *objective == "" {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	m, err := mission.NewMission(mission.NewInput{
		ID: *id, Objective: *objective, Description: *description,
		Repo: *repo, Workspace: *workspace,
		Constraints: splitList(*constraints), Criteria: splitList(*criteria),
		Risk: *risk, Creator: *creator,
		Budget: mission.Budget{TimeMS: *budgetTime, Tokens: *budgetTokens, Cost: *budgetCost},
	}, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission create: %v\n", err)
		return 1
	}
	if _, err := mission.CreateMission(io.Dir, m, time.Now().UTC()); err != nil {
		fmt.Fprintf(io.Stderr, "mission create: %v\n", err)
		return 1
	}
	_, sealed, err := mission.LoadMission(io.Dir, m.ID)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission create: %v\n", err)
		return 1
	}
	io.emitMissionEvent(sealed, "mission create", map[string]any{"actor": *actor})
	if *asJSON {
		return printMissionJSON(io, sealed)
	}
	fmt.Fprintf(io.Stdout, "mission: created %s state=%s generation=%d\n", sealed.ID, sealed.State, sealed.Generation)
	return 0
}

// MissionShowCmd implements `regulus mission show`.
func (io IO) MissionShowCmd(args []string) int {
	fs := flag.NewFlagSet("mission show", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	asJSON := fs.Bool("json", false, "print the record as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	_, m, err := mission.LoadMission(io.Dir, *id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission show: %v\n", err)
		return 1
	}
	if *asJSON {
		return printMissionJSON(io, m)
	}
	fmt.Fprint(io.Stdout, mission.RenderShow(m))
	return 0
}

// MissionListCmd implements `regulus mission list`.
func (io IO) MissionListCmd(args []string) int {
	fs := flag.NewFlagSet("mission list", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	state := fs.String("state", "", "only list missions in this state (optional)")
	asJSON := fs.Bool("json", false, "print the summaries as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	entries, err := mission.ListMissions(io.Dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission list: %v\n", err)
		return 1
	}
	if *state != "" {
		kept := entries[:0]
		for _, e := range entries {
			if e.State == *state || e.Problem != "" {
				kept = append(kept, e)
			}
		}
		entries = kept
	}
	if *asJSON {
		raw, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "mission list: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
	} else {
		fmt.Fprint(io.Stdout, mission.RenderList(entries))
	}
	for _, e := range entries {
		if e.Problem != "" {
			fmt.Fprintf(io.Stderr, "mission list: %s corrupt: %s\n", e.ID, e.Problem)
		}
	}
	if missionListHasCorrupt(entries) {
		return 1
	}
	return 0
}

func missionListHasCorrupt(entries []mission.ListEntry) bool {
	for _, e := range entries {
		if e.Problem != "" {
			return true
		}
	}
	return false
}

// MissionStatusCmd implements `regulus mission status`.
func (io IO) MissionStatusCmd(args []string) int {
	fs := flag.NewFlagSet("mission status", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	asJSON := fs.Bool("json", false, "print the lifecycle as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	_, m, err := mission.LoadMission(io.Dir, *id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission status: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(m.History, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "mission status: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, mission.RenderStatus(m))
	return 0
}

// missionMutationFlags registers the shared --id/--reason/--actor/--idem-key
// flags every state-changing mission subcommand takes.
func missionMutationFlags(fs *flag.FlagSet) (id, reason, actor, idem *string) {
	id = fs.String("id", "", "mission id (required)")
	reason = fs.String("reason", "", "audit reason")
	actor = fs.String("actor", "", "operator ordering this write (audit)")
	idem = fs.String("idem-key", "", "idempotency key: a replay returns the stored event without mutating")
	return id, reason, actor, idem
}

// missionReplayShortCircuit answers an idempotent replay before any
// write: the stored event already covers this key, so the handler
// reports it and exits 0 with the store untouched.
func (io IO) missionReplayShortCircuit(m *mission.Mission, idemKey, command string) (bool, int) {
	if idemKey == "" || !m.SeenIdem(idemKey) {
		return false, 0
	}
	fmt.Fprintf(io.Stdout, "mission: %s %s already applied (idempotent replay, generation=%d)\n", m.ID, command, m.Generation)
	return true, 0
}

// missionLoadForMutation loads the mission and short-circuits
// idempotent replays; ok=false means the handler must stop now.
func (io IO) missionLoadForMutation(id, idemKey, command string) (*mission.Mission, int, bool) {
	_, m, err := mission.LoadMission(io.Dir, id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission %s: %v\n", command, err)
		return nil, 1, false
	}
	if done, code := io.missionReplayShortCircuit(m, idemKey, command); done {
		return nil, code, false
	}
	return m, 0, true
}

// MissionAdvanceCmd implements `regulus mission advance`.
func (io IO) MissionAdvanceCmd(args []string) int {
	fs := flag.NewFlagSet("mission advance", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id, reason, actor, idem := missionMutationFlags(fs)
	to := fs.String("to", "", "target state (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || *to == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	if _, code, ok := io.missionLoadForMutation(*id, *idem, "advance"); !ok {
		return code
	}
	now := time.Now().UTC()
	updated, err := mission.UpdateMission(io.Dir, *id, now, func(m *mission.Mission) error {
		_, err := m.Transition(*to, *actor, *reason, *idem, now)
		return err
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission advance: %v\n", err)
		return 1
	}
	io.emitMissionEvent(updated, "mission advance", map[string]any{"actor": *actor, "to": *to})
	fmt.Fprintf(io.Stdout, "mission: %s advanced to %s generation=%d\n", updated.ID, updated.State, updated.Generation)
	return 0
}

// MissionPauseCmd implements `regulus mission pause`.
func (io IO) MissionPauseCmd(args []string) int {
	fs := flag.NewFlagSet("mission pause", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id, reason, actor, idem := missionMutationFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	if _, code, ok := io.missionLoadForMutation(*id, *idem, "pause"); !ok {
		return code
	}
	now := time.Now().UTC()
	updated, err := mission.UpdateMission(io.Dir, *id, now, func(m *mission.Mission) error {
		_, err := m.Pause(*actor, *reason, *idem, now)
		return err
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission pause: %v\n", err)
		return 1
	}
	io.emitMissionEvent(updated, "mission pause", map[string]any{"actor": *actor})
	fmt.Fprintf(io.Stdout, "mission: %s paused (was %s) generation=%d\n", updated.ID, updated.PausedFrom, updated.Generation)
	return 0
}

// MissionResumeCmd implements `regulus mission resume`.
func (io IO) MissionResumeCmd(args []string) int {
	fs := flag.NewFlagSet("mission resume", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id, reason, actor, idem := missionMutationFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	if _, code, ok := io.missionLoadForMutation(*id, *idem, "resume"); !ok {
		return code
	}
	now := time.Now().UTC()
	updated, err := mission.UpdateMission(io.Dir, *id, now, func(m *mission.Mission) error {
		_, err := m.Resume(*actor, *reason, *idem, now)
		return err
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission resume: %v\n", err)
		return 1
	}
	io.emitMissionEvent(updated, "mission resume", map[string]any{"actor": *actor})
	fmt.Fprintf(io.Stdout, "mission: %s resumed to %s generation=%d\n", updated.ID, updated.State, updated.Generation)
	return 0
}

// MissionCancelCmd implements `regulus mission cancel`.
func (io IO) MissionCancelCmd(args []string) int {
	fs := flag.NewFlagSet("mission cancel", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id, reason, actor, idem := missionMutationFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || *reason == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	if strings.HasPrefix(*reason, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	if _, code, ok := io.missionLoadForMutation(*id, *idem, "cancel"); !ok {
		return code
	}
	now := time.Now().UTC()
	updated, err := mission.UpdateMission(io.Dir, *id, now, func(m *mission.Mission) error {
		_, err := m.Cancel(*actor, *reason, *idem, now)
		return err
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission cancel: %v\n", err)
		return 1
	}
	io.emitMissionEvent(updated, "mission cancel", map[string]any{"actor": *actor})
	fmt.Fprintf(io.Stdout, "mission: %s cancelled generation=%d\n", updated.ID, updated.Generation)
	return 0
}

// MissionLinkCmd implements `regulus mission link`.
func (io IO) MissionLinkCmd(args []string) int {
	fs := flag.NewFlagSet("mission link", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	runs := fs.String("run", "", "comma-separated run ids to correlate")
	tasks := fs.String("task", "", "comma-separated task ids to correlate")
	agents := fs.String("agent", "", "comma-separated agent ids to correlate")
	worktrees := fs.String("worktree", "", "comma-separated worktrees to correlate")
	commits := fs.String("commit", "", "comma-separated commits to correlate")
	traces := fs.String("trace", "", "comma-separated trace ids to correlate")
	receipts := fs.String("receipt", "", "comma-separated receipts to correlate")
	actor := fs.String("actor", "", "operator ordering this write (audit)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	now := time.Now().UTC()
	updated, err := mission.UpdateMission(io.Dir, *id, now, func(m *mission.Mission) error {
		return m.AttachLinks(mission.LinkInput{
			RunIDs: splitList(*runs), TaskIDs: splitList(*tasks),
			AgentIDs: splitList(*agents), Worktrees: splitList(*worktrees),
			Commits: splitList(*commits), Traces: splitList(*traces),
			Receipts: splitList(*receipts),
		})
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission link: %v\n", err)
		return 1
	}
	io.emitMissionEvent(updated, "mission link", map[string]any{"actor": *actor})
	fmt.Fprintf(io.Stdout, "mission: %s linked generation=%d\n", updated.ID, updated.Generation)
	return 0
}

// MissionReopenCmd implements `regulus mission reopen`: the only
// explicit audited way out of a terminal state, back to
// recovering. The reason is required and stays in the history.
func (io IO) MissionReopenCmd(args []string) int {
	fs := flag.NewFlagSet("mission reopen", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id, reason, actor, idem := missionMutationFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || *reason == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	if strings.HasPrefix(*reason, "-") {
		fmt.Fprintln(io.Stderr, missionUsage())
		return 2
	}
	if _, code, ok := io.missionLoadForMutation(*id, *idem, "reopen"); !ok {
		return code
	}
	now := time.Now().UTC()
	updated, err := mission.UpdateMission(io.Dir, *id, now, func(m *mission.Mission) error {
		_, err := m.Reopen(*actor, *reason, *idem, now)
		return err
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission reopen: %v\n", err)
		return 1
	}
	io.emitMissionEvent(updated, "mission reopen", map[string]any{"actor": *actor})
	fmt.Fprintf(io.Stdout, "mission: %s reopened to %s generation=%d (reopen #%d)\n", updated.ID, updated.State, updated.Generation, updated.ReopenCount)
	return 0
}

func printMissionJSON(io IO, m *mission.Mission) int {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		fmt.Fprintf(io.Stderr, "mission: %v\n", err)
		return 1
	}
	fmt.Fprintln(io.Stdout, string(raw))
	return 0
}

// emitMissionEvent links one mission command to the F11.1 exec log
// under a correlated id: the first linked run when the mission
// cites one, else mission/<id>. TaskID always carries the mission
// id, so every mission event is queryable by it. Best-effort (a
// trace failure never fails the mission write).
func (io IO) emitMissionEvent(m *mission.Mission, command string, extra map[string]any) {
	runID := "mission/" + m.ID
	if len(m.Links.RunIDs) > 0 {
		runID = m.Links.RunIDs[0]
	}
	eventID, err := observability.MintEventID()
	if err != nil {
		return
	}
	payload := map[string]any{"command": command, "mission_id": m.ID, "state": m.State}
	for k, v := range extra {
		payload[k] = v
	}
	evType := observability.ExecArtifact
	if command == "mission create" {
		evType = observability.ExecTaskStart
	}
	if command == "mission cancel" {
		evType = observability.ExecTaskEnd
	}
	if _, _, rerr := observability.RecordExec(io.Dir, observability.ExecEvent{
		SchemaVersion: observability.ExecSchemaVersion,
		EventID:       eventID, RunID: runID, TaskID: m.ID,
		Type: evType, Status: missionEventStatus(m.State),
		Payload: payload,
	}, time.Now().UTC()); rerr != nil {
		fmt.Fprintf(io.Stderr, "mission: evento de traza no registrado (%v)\n", rerr)
	}
}

// missionEventStatus maps mission states onto the exec status
// vocabulary: terminal states close the span, live ones stay
// running.
func missionEventStatus(state string) string {
	switch state {
	case mission.StateCompleted, mission.StatePartiallyCompleted:
		return observability.ExecSucceeded
	case mission.StateFailed:
		return observability.ExecFailed
	case mission.StateCancelled:
		return observability.ExecCancelled
	default:
		return observability.ExecRunning
	}
}
