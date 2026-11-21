// Package execute implements F13.3/NOV-106: the Mission Executor.
//
// It consumes an approved F13.2 plan and dispatches its tasks through
// the F12.2 scheduler (orchestrate.RunParallel) with F12.1 routing
// (orchestrate.Decide), keeping a durable per-task ledger. It never
// implements a second scheduler: ordering reuses
// orchestrate.PlanParallelLevels, isolation reuses
// orchestrate.IsolateParallelWorker, merging and gates reuse
// orchestrate.RunParallel. Successive waves are separate scheduler
// invocations driven by ledger readiness (resume, approvals), not a
// parallel DAG.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - mission: the durable entity (state machine, generation
//     conflicts, exactly-once effects) this executor advances, never
//     redefines; integration side effects seal through
//     mission.ApplyEffect so resume never re-integrates;
//   - planning: the versioned approved plan (tasks, scopes, agents,
//     budgets, autonomy); the executor maps PlanTask onto
//     orchestrate.ParallelTask, never re-plans;
//   - orchestrate: Decide (routing re-validation, offline agents
//     excluded), PlanParallelLevels (DAG order, collision
//     serialisation, cycle refusal), RunParallel (concurrency,
//     backpressure, timeouts, cancellation, merge, gate, cleanup);
//   - bench/mcp: capability evidence and scope vocabulary flow
//     through the reused Decide call, never re-graded here;
//   - trace: observability.RecordExec shares run/task ids (F11.1);
//     dry-run records nothing;
//   - locking: fsx.LockDir serialises ledger writes, the same
//     primitive the mission and plan stores use.
//
// Crash-safe means: a wave mutates memory first and only becomes
// truth when the ledger seals it; a crash between running and
// sealing reloads the previous confirmed ledger on the next run,
// and tasks already integrated are skipped (same no-duplicate
// guarantee the mission ledger relies on).
package execute

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/fsx"
	"regulus-ai/internal/mission"
)

// ExecutionVersion pins the ledger document shape.
const ExecutionVersion = "execution-v1"

// Ledger file names inside one mission directory. execution-manifest
// avoids the mission store's manifest.json (same directory, must
// never share a shape).
const (
	ExecutionFile = "execution.json"
	ExecutionMeta = "execution-manifest.json"
	maxLedgerSize = 2 * 1024 * 1024
)

// Task ledger states (9, closed vocabulary). pending waits for its
// dependencies; ready is admitted for dispatch; dispatched is handed
// to the scheduler; running executes in its worktree; succeeded and
// failed close the attempt; blocked waits on a human (approval,
// conflict review, offline worker); cancelled dies by cancellation;
// integrated seals verification (terminal per attempt chain).
const (
	TaskPending    = "pending"
	TaskReady      = "ready"
	TaskDispatched = "dispatched"
	TaskRunning    = "running"
	TaskSucceeded  = "succeeded"
	TaskFailed     = "failed"
	TaskBlocked    = "blocked"
	TaskCancelled  = "cancelled"
	TaskIntegrated = "integrated"
)

// AllTaskStates lists the closed ledger vocabulary in lifecycle order.
var AllTaskStates = []string{
	TaskPending, TaskReady, TaskDispatched, TaskRunning,
	TaskSucceeded, TaskFailed, TaskBlocked, TaskCancelled, TaskIntegrated,
}

// legalTaskTransitions is the allowed edge set. Terminal-per-wave
// states (succeeded, failed, cancelled) leave only via explicit
// operator moves (retry reopens to ready, approve reopens blocked);
// integrated never leaves: re-integration returns the stored result
// without re-merging.
var legalTaskTransitions = map[string]map[string]bool{
	TaskPending:    {TaskReady: true, TaskBlocked: true, TaskCancelled: true},
	TaskReady:      {TaskDispatched: true, TaskBlocked: true, TaskCancelled: true},
	TaskDispatched: {TaskRunning: true, TaskBlocked: true, TaskCancelled: true},
	TaskRunning:    {TaskSucceeded: true, TaskFailed: true, TaskBlocked: true, TaskCancelled: true},
	TaskSucceeded:  {TaskIntegrated: true, TaskReady: true, TaskBlocked: true},
	TaskFailed:     {TaskReady: true, TaskBlocked: true, TaskCancelled: true},
	TaskBlocked:    {TaskReady: true, TaskCancelled: true},
	TaskCancelled:  {TaskReady: true},
}

// TerminalTask reports whether the ledger state closes the attempt:
// integrated is forever, the rest reopen only through retry/approve.
func TerminalTask(state string) bool {
	return state == TaskIntegrated
}

// CheckTaskTransition refuses an illegal ledger edge before anything
// is recorded. Same-state is an idempotent no-op.
func CheckTaskTransition(from, to string) error {
	if from == to {
		return nil
	}
	if !knownTaskState(from) {
		return fmt.Errorf("execute: unknown task state %q", from)
	}
	if !knownTaskState(to) {
		return fmt.Errorf("execute: unknown task state %q", to)
	}
	if TerminalTask(from) {
		return fmt.Errorf("execute: task state %q is terminal (resume skips it, never re-opens it)", from)
	}
	if nexts, ok := legalTaskTransitions[from]; ok && nexts[to] {
		return nil
	}
	return fmt.Errorf("execute: illegal task transition %q -> %q", from, to)
}

func knownTaskState(s string) bool {
	for _, st := range AllTaskStates {
		if st == s {
			return true
		}
	}
	return false
}

// TaskResult is the structured outcome one worker produced: verdict,
// measured duration, files touched, commit sealing the work, agent
// attribution, permission scopes held and the naming reason.
type TaskResult struct {
	Outcome    string   `json:"outcome"`
	DurationMS int64    `json:"duration_ms"`
	Files      []string `json:"files,omitempty"`
	Commit     string   `json:"commit,omitempty"`
	Agent      string   `json:"agent,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	RunID      string   `json:"run_id,omitempty"`
}

// TaskRecord is one ledger line: identity, plan revision, agent,
// ledger state, attempt count, idempotency key, structured result
// and audit trail.
type TaskRecord struct {
	TaskID      string     `json:"task_id"`
	PlanVersion int        `json:"plan_version"`
	State       string     `json:"state"`
	Agent       string     `json:"agent,omitempty"`
	Attempt     int        `json:"attempt"`
	OpKey       string     `json:"op_key,omitempty"`
	Result      TaskResult `json:"result,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	ApprovedBy  string     `json:"approved_by,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
	History     []string   `json:"history,omitempty"`
}

// Ledger is the durable per-mission task ledger: every plan task in
// a deterministic order, sealed with a generation so two
// coordinators racing one mission fail loudly instead of
// double-dispatching.
type Ledger struct {
	Version    string                `json:"version"`
	MissionID  string                `json:"mission_id"`
	Generation int64                 `json:"generation"`
	Tasks      map[string]TaskRecord `json:"tasks"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

// ExecutionManifest cites the ledger bytes: sha256 of execution.json
// plus the version and generation that reproduce the write.
type ExecutionManifest struct {
	Version    string    `json:"version"`
	MissionID  string    `json:"mission_id"`
	LedgerFile string    `json:"ledger_file"`
	LedgerSHA  string    `json:"ledger_sha256"`
	Generation int64     `json:"generation"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// OpKeyFor names the idempotency key of one dispatch: one mission,
// one plan revision, one task, one attempt. The ledger refuses a
// second dispatch carrying a key it already sealed; the mission
// effect ledger seals integrations under integrate/<opkey>.
func OpKeyFor(missionID string, planVersion int, taskID string, attempt int) string {
	return fmt.Sprintf("exec/%s/v%d/%s/a%d", missionID, planVersion, taskID, attempt)
}

// IntegrateKeyFor names the exactly-once integration key: one per
// (mission, plan revision, task), attempt-independent, so retries
// never integrate twice.
func IntegrateKeyFor(missionID string, planVersion int, taskID string) string {
	return fmt.Sprintf("integrate/%s/v%d/%s", missionID, planVersion, taskID)
}

// execDir resolves the mission directory holding the ledger. Mission
// ids are validated so no traversal escapes .regulus/missions.
func execDir(root, missionID string) (string, error) {
	return mission.StoreDir(root, missionID)
}

// moveTask validates one ledger edge and stamps the audit line.
// Pure memory mutation: the caller seals with UpdateLedger, so a
// crash before sealing replays from the previous confirmed ledger.
func moveTask(rec *TaskRecord, to, reason string, now time.Time) error {
	if err := CheckTaskTransition(rec.State, to); err != nil {
		return err
	}
	if rec.State != to {
		rec.History = append(rec.History, fmt.Sprintf("%s -> %s: %s", rec.State, to, reason))
		rec.State = to
	}
	rec.Reason = reason
	rec.UpdatedAt = now.UTC()
	return nil
}

// saveLedgerLocked persists one ledger with generation expected+1
// under the caller's dir lock, atomically with its manifest.
func saveLedgerLocked(dir string, l *Ledger, expected int64, now time.Time) error {
	if l.Version == "" {
		l.Version = ExecutionVersion
	}
	if l.Version != ExecutionVersion {
		return fmt.Errorf("execute: ledger version %q unsupported (want %q)", l.Version, ExecutionVersion)
	}
	if err := mission.ValidateID(l.MissionID); err != nil {
		return err
	}
	for id, rec := range l.Tasks {
		if !knownTaskState(rec.State) {
			return fmt.Errorf("execute: task %q holds unknown state %q", id, rec.State)
		}
	}
	l.Generation = expected + 1
	l.UpdatedAt = now.UTC()
	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := atomicWrite(filepath.Join(dir, ExecutionFile), raw); err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	man := ExecutionManifest{
		Version: ExecutionVersion, MissionID: l.MissionID, LedgerFile: ExecutionFile,
		LedgerSHA: hex.EncodeToString(sum[:]), Generation: l.Generation, UpdatedAt: l.UpdatedAt,
	}
	mraw, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, ExecutionMeta), append(mraw, '\n'))
}

// loadLedgerLocked reads the ledger inside the caller's dir lock,
// verifying its sha before use and failing closed. A missing ledger
// reports os.IsNotExist so the first run starts from an empty one.
func loadLedgerLocked(dir, missionID string) (ExecutionManifest, *Ledger, error) {
	mraw, err := os.ReadFile(filepath.Join(dir, ExecutionMeta))
	if err != nil {
		if os.IsNotExist(err) {
			return ExecutionManifest{}, nil, err
		}
		return ExecutionManifest{}, nil, fmt.Errorf("execute: cannot read ledger of mission %q: %w", missionID, err)
	}
	var man ExecutionManifest
	dec := json.NewDecoder(strings.NewReader(string(mraw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&man); err != nil {
		return ExecutionManifest{}, nil, fmt.Errorf("execute: malformed ledger manifest of mission %q: %w", missionID, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ExecutionFile))
	if err != nil {
		return ExecutionManifest{}, nil, fmt.Errorf("execute: cannot read ledger of mission %q: %w", missionID, err)
	}
	if len(raw) > maxLedgerSize {
		return ExecutionManifest{}, nil, fmt.Errorf("execute: ledger of mission %q exceeds %d bytes", missionID, maxLedgerSize)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != man.LedgerSHA {
		return ExecutionManifest{}, nil, fmt.Errorf("execute: ledger of mission %q sha mismatch (edited store is not evidence)", missionID)
	}
	if man.Version != ExecutionVersion {
		return ExecutionManifest{}, nil, fmt.Errorf("execute: ledger of mission %q version %q unsupported (want %q)", missionID, man.Version, ExecutionVersion)
	}
	var l Ledger
	ldec := json.NewDecoder(strings.NewReader(string(raw)))
	ldec.DisallowUnknownFields()
	if err := ldec.Decode(&l); err != nil {
		return ExecutionManifest{}, nil, fmt.Errorf("execute: malformed ledger of mission %q: %w", missionID, err)
	}
	if l.MissionID != missionID || l.MissionID != man.MissionID {
		return ExecutionManifest{}, nil, fmt.Errorf("execute: ledger id drift (%q, want %q)", l.MissionID, missionID)
	}
	if l.Generation != man.Generation {
		return ExecutionManifest{}, nil, fmt.Errorf("execute: ledger generation drift (file %d manifest %d)", l.Generation, man.Generation)
	}
	if l.Tasks == nil {
		l.Tasks = map[string]TaskRecord{}
	}
	return man, &l, nil
}

// LoadLedger reads the confirmed ledger. A missing ledger loads as
// empty (first run seeds it); a corrupt one fails closed.
func LoadLedger(root, missionID string) (*Ledger, error) {
	dir, err := execDir(root, missionID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, ExecutionMeta)); err != nil {
		if os.IsNotExist(err) {
			return &Ledger{Version: ExecutionVersion, MissionID: missionID, Tasks: map[string]TaskRecord{}}, nil
		}
		return nil, fmt.Errorf("execute: cannot read ledger of mission %q: %w", missionID, err)
	}
	release, err := fsx.LockDir(dir)
	if err != nil {
		return nil, err
	}
	defer release()
	_, l, err := loadLedgerLocked(dir, missionID)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// UpdateLedger is the single-coordinator mutation path: lock, load
// the confirmed ledger, apply fn in memory, seal with the loaded
// generation as the expected one. A concurrent coordinator that
// saved first makes this update refuse with a generation conflict
// instead of double-dispatching. fn errors abort before anything
// is written.
func UpdateLedger(root, missionID string, now time.Time, fn func(*Ledger) error) (*Ledger, error) {
	dir, err := execDir(root, missionID)
	if err != nil {
		return nil, err
	}
	release, err := fsx.LockDir(dir)
	if err != nil {
		return nil, err
	}
	defer release()
	var l *Ledger
	var expected int64
	if _, stored, lerr := loadLedgerLocked(dir, missionID); lerr == nil {
		l, expected = stored, stored.Generation
	} else if os.IsNotExist(lerr) {
		l = &Ledger{Version: ExecutionVersion, MissionID: missionID, Tasks: map[string]TaskRecord{}}
		expected = 0
	} else {
		return nil, lerr
	}
	if err := fn(l); err != nil {
		return nil, err
	}
	if err := saveLedgerLocked(dir, l, expected, now); err != nil {
		return nil, err
	}
	return l, nil
}

// SeedLedger registers every plan task as pending without touching
// tasks the ledger already tracks: re-seeding after a crash keeps
// confirmed states (resume never resets progress). New tasks start
// pending; unknown ids already sealed stay untouched.
func SeedLedger(l *Ledger, taskIDs []string, planVersion int, agents map[string]string, now time.Time) {
	if l.Tasks == nil {
		l.Tasks = map[string]TaskRecord{}
	}
	for _, id := range taskIDs {
		if rec, ok := l.Tasks[id]; ok {
			// A task sealed under an older plan revision stays sealed;
			// a live task keeps its confirmed state and attempt but
			// tracks the newest revision driving it.
			if !TerminalTask(rec.State) && rec.PlanVersion != planVersion {
				rec.PlanVersion = planVersion
				rec.History = append(rec.History, fmt.Sprintf("replan v%d: live task keeps state %s", planVersion, rec.State))
				l.Tasks[id] = rec
			}
			continue
		}
		l.Tasks[id] = TaskRecord{
			TaskID: id, PlanVersion: planVersion, State: TaskPending,
			Agent: agents[id], Attempt: 0, UpdatedAt: now.UTC(),
			History: []string{fmt.Sprintf("created -> %s: seeded from plan v%d", TaskPending, planVersion)},
		}
	}
}

// SortedTaskIDs returns the deterministic ledger order.
func SortedTaskIDs(l *Ledger) []string {
	ids := make([]string, 0, len(l.Tasks))
	for id := range l.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// atomicWrite stores data via temp-plus-rename so a crash mid-write
// leaves either the previous bytes or the new ones, never a half
// file the next load would misread.
func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
