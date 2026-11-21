// Package mission implements F13.1/NOV-104: the Mission Engine,
// the durable central entity governing one engineering mission
// from creation to terminal result. It is the source of truth of
// the global state; it never replaces the specialized F12.2
// (orchestrate worktrees) or F12.5 (recovery checkpoints) states,
// it only cites them by id.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - durability discipline (atomic temp+rename+manifest sha,
//     fail-closed reads, deterministic budgets) follows the
//     F12.5 recovery store pattern; the shape is mission-owned.
//   - traceability rides the F11.1 exec log by sharing ids (the
//     cmdx layer emits under the linked run_id); this package
//     never owns a trace store, it only correlates mission_id
//     with run/task/agent/worktree/commit/trace/receipt ids.
//   - inter-process locking reuses internal/fsx (LockDir), the
//     same primitive the observability log serializes on.
//
// Crash-safe means: a transition mutates memory first and only
// becomes truth when Save/Update persists it; a crash between
// mutating and saving simply never happened (the next load sees
// the previous confirmed state).
package mission

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// MissionVersion pins the mission document shape: readers migrate
// what they know (v0 -> v1) and refuse what they do not.
const MissionVersion = "mission-v1"

// LegacyVersion is the pre-F13 shape the migration accepts: same
// ids and states, without risk tiers, budgets, links, effects or
// generations. Anything else fails closed, never guessed.
const LegacyVersion = "mission-v0"

// Mission lifecycle states (13). Terminal states close the
// mission for good: completed, partially_completed, failed and
// cancelled. blocked is the parked state pause/resume use;
// recovering is the re-entry after an interruption; every other
// state is the forward path created -> analyzing -> planning ->
// awaiting_approval -> executing -> integrating -> validating.
const (
	StateCreated            = "created"
	StateAnalyzing          = "analyzing"
	StatePlanning           = "planning"
	StateAwaitingApproval   = "awaiting_approval"
	StateExecuting          = "executing"
	StateIntegrating        = "integrating"
	StateValidating         = "validating"
	StateRecovering         = "recovering"
	StateCompleted          = "completed"
	StatePartiallyCompleted = "partially_completed"
	StateBlocked            = "blocked"
	StateFailed             = "failed"
	StateCancelled          = "cancelled"
)

// AllStates lists the closed state vocabulary in lifecycle order.
var AllStates = []string{
	StateCreated, StateAnalyzing, StatePlanning,
	StateAwaitingApproval, StateExecuting, StateIntegrating,
	StateValidating, StateRecovering, StateCompleted,
	StatePartiallyCompleted, StateBlocked, StateFailed,
	StateCancelled,
}

// IsTerminal reports whether state closes the mission: no normal
// transition may leave it afterwards. Only Reopen (explicit,
// reasoned, audited) may move a terminal mission back to
// recovering, and it counts every reopening.
func IsTerminal(state string) bool {
	switch state {
	case StateCompleted, StatePartiallyCompleted, StateFailed, StateCancelled:
		return true
	}
	return false
}

// legalTransitions is the allowed forward edge set. Terminal
// states have no out-edges here on purpose: Reopen is the only
// audited way back, and it is not a normal transition.
var legalTransitions = map[string]map[string]bool{
	StateCreated:          {StateAnalyzing: true, StateCancelled: true},
	StateAnalyzing:        {StatePlanning: true, StateBlocked: true, StateFailed: true, StateCancelled: true},
	StatePlanning:         {StateAwaitingApproval: true, StateBlocked: true, StateFailed: true, StateCancelled: true},
	StateAwaitingApproval: {StateExecuting: true, StatePlanning: true, StateBlocked: true, StateFailed: true, StateCancelled: true},
	StateExecuting:        {StateIntegrating: true, StateValidating: true, StateRecovering: true, StateBlocked: true, StateFailed: true, StateCancelled: true},
	StateIntegrating:      {StateValidating: true, StateExecuting: true, StateRecovering: true, StateBlocked: true, StateFailed: true, StateCancelled: true},
	StateValidating:       {StateCompleted: true, StatePartiallyCompleted: true, StateExecuting: true, StateRecovering: true, StateBlocked: true, StateFailed: true, StateCancelled: true},
	StateRecovering:       {StateExecuting: true, StateIntegrating: true, StateValidating: true, StateBlocked: true, StateFailed: true, StateCancelled: true},
	StateBlocked: {
		StateAnalyzing: true, StatePlanning: true, StateAwaitingApproval: true,
		StateExecuting: true, StateIntegrating: true, StateValidating: true,
		StateRecovering: true, StateFailed: true, StateCancelled: true,
	},
}

// CheckTransition refuses an illegal edge before anything is
// recorded, so a crash can never be "recovered" into a state the
// live path could not reach. Same-state is an idempotent no-op
// (success, no history appended). Terminal sources refuse: use
// Reopen for the explicit audited way back.
func CheckTransition(from, to string) error {
	if from == to {
		return nil
	}
	if !knownState(from) {
		return fmt.Errorf("mission: unknown state %q", from)
	}
	if !knownState(to) {
		return fmt.Errorf("mission: unknown state %q", to)
	}
	if IsTerminal(from) {
		return fmt.Errorf("mission: state %q is terminal (reopen explicitly with a reason, never transition)", from)
	}
	if nexts, ok := legalTransitions[from]; ok && nexts[to] {
		return nil
	}
	return fmt.Errorf("mission: illegal transition %q -> %q", from, to)
}

func knownState(s string) bool {
	for _, st := range AllStates {
		if st == s {
			return true
		}
	}
	return false
}

// Risk tiers (closed vocabulary, same scale orchestrate routes on).
const (
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

func validRisk(r string) bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		return true
	}
	return false
}

// Budget bounds the mission: 0 means unset for every field.
type Budget struct {
	TimeMS int64   `json:"time_ms,omitempty"`
	Tokens int64   `json:"tokens,omitempty"`
	Cost   float64 `json:"cost,omitempty"`
}

func (b Budget) validate() error {
	if b.TimeMS < 0 || b.Tokens < 0 || b.Cost < 0 {
		return fmt.Errorf("mission: budget must not be negative (time=%d tokens=%d cost=%v)", b.TimeMS, b.Tokens, b.Cost)
	}
	return nil
}

// Correlation cites every specialized artifact by id without
// owning any specialized store: run/task/agent ids (F12.5, F11.1),
// worktrees (F12.2), commits, traces (F11.1) and receipts. Sets,
// never lists: attaching twice is a no-op, never a duplicate.
type Correlation struct {
	RunIDs    []string `json:"run_ids,omitempty"`
	TaskIDs   []string `json:"task_ids,omitempty"`
	AgentIDs  []string `json:"agent_ids,omitempty"`
	Worktrees []string `json:"worktrees,omitempty"`
	Commits   []string `json:"commits,omitempty"`
	Traces    []string `json:"traces,omitempty"`
	Receipts  []string `json:"receipts,omitempty"`
}

// Event is one auditable lifecycle line: what moved, who ordered
// it and why. IdemKey replays safely: a transition carrying a key
// already seen returns the stored event without mutating.
type Event struct {
	EventID string    `json:"event_id"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	Actor   string    `json:"actor,omitempty"`
	Reason  string    `json:"reason,omitempty"`
	IdemKey string    `json:"idem_key,omitempty"`
	At      time.Time `json:"at"`
}

// EffectResult is one exactly-once mission effect: the output
// recorded under its key. Re-applying the key returns the stored
// output without re-executing, so resume never duplicates side
// effects (same guarantee as the recovery ledger, mission-owned).
type EffectResult struct {
	Key    string    `json:"key"`
	Output string    `json:"output"`
	At     time.Time `json:"at"`
}

// Mission is the durable central entity: identity, intent,
// bounds, global state, optimistic-concurrency generation, the
// correlation set, the exactly-once effect ledger, the pause
// bookmark and the audit history.
type Mission struct {
	Version      string                  `json:"version"`
	ID           string                  `json:"mission_id"`
	Objective    string                  `json:"objective"`
	Description  string                  `json:"description,omitempty"`
	Repo         string                  `json:"repo,omitempty"`
	Workspace    string                  `json:"workspace,omitempty"`
	Constraints  []string                `json:"constraints,omitempty"`
	Criteria     []string                `json:"acceptance_criteria,omitempty"`
	Risk         string                  `json:"risk"`
	Budget       Budget                  `json:"budget,omitempty"`
	Creator      string                  `json:"creator,omitempty"`
	CreatedAt    time.Time               `json:"created_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
	State        string                  `json:"state"`
	Generation   int64                   `json:"generation"`
	Links        Correlation             `json:"links,omitempty"`
	Effects      map[string]EffectResult `json:"effects,omitempty"`
	PausedFrom   string                  `json:"paused_from,omitempty"`
	History      []Event                 `json:"history,omitempty"`
	CancelReason string                  `json:"cancel_reason,omitempty"`
	ReopenCount  int                     `json:"reopen_count,omitempty"`
}

// NewInput carries one mission creation request.
type NewInput struct {
	ID          string
	Objective   string
	Description string
	Repo        string
	Workspace   string
	Constraints []string
	Criteria    []string
	Risk        string
	Budget      Budget
	Creator     string
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)

// validID accepts store ids usable as directory names without
// traversal: alphanumeric start, conservative charset, no "..",
// bounded. Same rule recovery enforces, so mission ids compose
// with run/task/agent ids without a second vocabulary.
func validID(s string) bool {
	return idRe.MatchString(s) && !strings.Contains(s, "..")
}

// ValidateID refuses hostile ids before they touch the store.
func ValidateID(id string) error {
	if !validID(id) {
		return fmt.Errorf("mission: invalid mission id %q (alphanumeric start, [A-Za-z0-9._/-], no .., max 128)", id)
	}
	return nil
}

// validRef accepts correlation refs that are not strict store ids
// (worktrees, commits, receipts): single line, bounded, no ".."
// segments, never empty.
func validRef(s string) error {
	if strings.TrimSpace(s) == "" || len(s) > 256 {
		return fmt.Errorf("mission: invalid ref %q (non-empty single line, max 256)", s)
	}
	if strings.ContainsAny(s, "\n\r") {
		return fmt.Errorf("mission: invalid ref %q (must be a single line)", s)
	}
	for _, seg := range strings.Split(strings.ReplaceAll(s, "\\", "/"), "/") {
		if seg == ".." {
			return fmt.Errorf("mission: invalid ref %q (no .. segments)", s)
		}
	}
	return nil
}

func singleLine(s string) bool {
	return !strings.ContainsAny(s, "\n\r")
}

// MintID mints a stable unique mission id: msn_ + 16 hex chars
// (64 bits, crypto/rand), following the decisions MintID pattern.
func MintID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("mission: cannot mint mission id: %w", err)
	}
	return "msn_" + hex.EncodeToString(b[:]), nil
}

func mintEventID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("mission: cannot mint event id: %w", err)
	}
	return "evt_" + hex.EncodeToString(b[:]), nil
}

// NewMission validates one creation request and returns the
// confirmed initial record: state created, generation 0, one
// audit event. The caller persists with SaveMission; a crash
// before saving simply never created anything.
func NewMission(in NewInput, now time.Time) (*Mission, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		minted, err := MintID()
		if err != nil {
			return nil, err
		}
		id = minted
	}
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	objective := strings.TrimSpace(in.Objective)
	if objective == "" {
		return nil, fmt.Errorf("mission: objective is required")
	}
	if !singleLine(objective) || len(objective) > 512 {
		return nil, fmt.Errorf("mission: objective must be a single line of at most 512 chars")
	}
	if !singleLine(in.Description) || len(in.Description) > 4096 {
		return nil, fmt.Errorf("mission: description must be a single line of at most 4096 chars")
	}
	for _, field := range []struct {
		name, value string
	}{{"repo", in.Repo}, {"workspace", in.Workspace}, {"creator", in.Creator}} {
		if field.value != "" {
			if !singleLine(field.value) || len(field.value) > 512 {
				return nil, fmt.Errorf("mission: %s must be a single line of at most 512 chars", field.name)
			}
		}
	}
	risk := strings.TrimSpace(in.Risk)
	if risk == "" {
		risk = RiskMedium
	}
	if !validRisk(risk) {
		return nil, fmt.Errorf("mission: unknown risk %q (want low|medium|high|critical)", in.Risk)
	}
	if err := in.Budget.validate(); err != nil {
		return nil, err
	}
	for _, c := range in.Constraints {
		if !singleLine(c) || strings.TrimSpace(c) == "" || len(c) > 256 {
			return nil, fmt.Errorf("mission: invalid constraint %q (non-empty single line, max 256)", c)
		}
	}
	for _, c := range in.Criteria {
		if !singleLine(c) || strings.TrimSpace(c) == "" || len(c) > 256 {
			return nil, fmt.Errorf("mission: invalid acceptance criterion %q (non-empty single line, max 256)", c)
		}
	}
	eid, err := mintEventID()
	if err != nil {
		return nil, err
	}
	return &Mission{
		Version: MissionVersion, ID: id, Objective: objective,
		Description: in.Description, Repo: in.Repo, Workspace: in.Workspace,
		Constraints: append([]string(nil), in.Constraints...),
		Criteria:    append([]string(nil), in.Criteria...),
		Risk:        risk, Budget: in.Budget, Creator: in.Creator,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
		State: StateCreated, Generation: 0,
		History: []Event{{EventID: eid, From: "", To: StateCreated, Reason: "mission created", At: now.UTC()}},
	}, nil
}

// Validate refuses empty ids, unknown states, unknown risks,
// negative budgets and hostile refs before recording.
func (m *Mission) Validate() error {
	if err := ValidateID(m.ID); err != nil {
		return err
	}
	if strings.TrimSpace(m.Objective) == "" {
		return fmt.Errorf("mission: objective is required")
	}
	if !knownState(m.State) {
		return fmt.Errorf("mission: unknown state %q", m.State)
	}
	if !validRisk(m.Risk) {
		return fmt.Errorf("mission: unknown risk %q", m.Risk)
	}
	if err := m.Budget.validate(); err != nil {
		return err
	}
	if m.Generation < 0 {
		return fmt.Errorf("mission: generation must not be negative, got %d", m.Generation)
	}
	if m.PausedFrom != "" && !knownState(m.PausedFrom) {
		return fmt.Errorf("mission: unknown paused_from %q", m.PausedFrom)
	}
	return nil
}

// SeenIdem reports whether idemKey already anchors a stored audit
// event: cmdx short-circuits the save entirely on replays instead
// of bumping the generation for a duplicate delivery.
func (m *Mission) SeenIdem(idemKey string) bool {
	return m.findEvent(idemKey) != nil
}

// findEvent returns the stored event carrying idemKey, or nil.
func (m *Mission) findEvent(idemKey string) *Event {
	if idemKey == "" {
		return nil
	}
	for i := range m.History {
		if m.History[i].IdemKey == idemKey {
			return &m.History[i]
		}
	}
	return nil
}

// record appends one audit event; same-state moves append nothing
// (idempotent no-op). A repeated idemKey replays the stored event
// without mutating: resume-safe by construction.
func (m *Mission) record(to, actor, reason, idemKey string, now time.Time) (*Event, error) {
	if prev := m.findEvent(idemKey); prev != nil {
		return prev, nil
	}
	eid, err := mintEventID()
	if err != nil {
		return nil, err
	}
	ev := Event{EventID: eid, From: m.State, To: to, Actor: actor, Reason: reason, IdemKey: idemKey, At: now.UTC()}
	m.History = append(m.History, ev)
	return &m.History[len(m.History)-1], nil
}

// Transition moves the mission along one legal edge with its audit
// event. Illegal edges and terminal sources refuse before anything
// is recorded; same-state is a no-op success. Pure memory
// mutation: the caller persists with Save/Update, so a crash
// between mutating and saving recovers to the previous confirmed
// state on the next load.
func (m *Mission) Transition(to, actor, reason, idemKey string, now time.Time) (*Event, error) {
	if err := CheckTransition(m.State, to); err != nil {
		return nil, err
	}
	if prev := m.findEvent(idemKey); prev != nil {
		return prev, nil
	}
	if m.State == to {
		return &Event{From: m.State, To: to, Actor: actor, Reason: reason, IdemKey: idemKey, At: now.UTC()}, nil
	}
	if m.PausedFrom != "" && to != StateBlocked {
		m.PausedFrom = ""
	}
	m.State = to
	return m.record(to, actor, reason, idemKey, now)
}

// Pause parks a live mission on blocked, bookmarking where it
// paused so resume returns to the same state. Already-blocked,
// created and terminal missions refuse with the reason.
func (m *Mission) Pause(actor, reason, idemKey string, now time.Time) (*Event, error) {
	switch m.State {
	case StateAnalyzing, StatePlanning, StateAwaitingApproval,
		StateExecuting, StateIntegrating, StateValidating, StateRecovering:
	default:
		return nil, fmt.Errorf("mission: state %q cannot pause (only live missions park on blocked)", m.State)
	}
	if prev := m.findEvent(idemKey); prev != nil {
		return prev, nil
	}
	m.PausedFrom = m.State
	m.State = StateBlocked
	return m.record(StateBlocked, actor, pauseReason(reason), idemKey, now)
}

func pauseReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "pause requested"
	}
	return "pause: " + reason
}

// Resume returns a paused mission to the state it paused from.
// Blocked missions without a pause bookmark refuse (advance
// explicitly instead); non-blocked missions refuse (nothing to
// resume). The bookmark clears on success, so a second resume
// without a new pause refuses instead of duplicating.
func (m *Mission) Resume(actor, reason, idemKey string, now time.Time) (*Event, error) {
	if m.State != StateBlocked {
		return nil, fmt.Errorf("mission: state %q cannot resume (only blocked missions resume)", m.State)
	}
	if m.PausedFrom == "" {
		return nil, fmt.Errorf("mission: mission %q is blocked without a pause bookmark (advance explicitly to a live state)", m.ID)
	}
	if prev := m.findEvent(idemKey); prev != nil {
		return prev, nil
	}
	to := m.PausedFrom
	m.PausedFrom = ""
	m.State = to
	return m.record(to, actor, resumeReason(reason), idemKey, now)
}

func resumeReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "resume to paused state"
	}
	return "resume: " + reason
}

// Cancel closes the mission as cancelled with its reason.
// Terminal missions refuse: cancellation is not a reopening.
func (m *Mission) Cancel(actor, reason, idemKey string, now time.Time) (*Event, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("mission: cancel needs a reason (audit)")
	}
	if IsTerminal(m.State) {
		return nil, fmt.Errorf("mission: state %q is terminal (cancel refuses a closed mission)", m.State)
	}
	if prev := m.findEvent(idemKey); prev != nil {
		return prev, nil
	}
	m.State = StateCancelled
	m.CancelReason = reason
	m.PausedFrom = ""
	return m.record(StateCancelled, actor, reason, idemKey, now)
}

// Reopen is the only explicit audited way out of a terminal
// state, and it only leads back to recovering. It requires a
// reason, counts every reopening and refuses non-terminal
// sources (use Transition for the live path).
func (m *Mission) Reopen(actor, reason, idemKey string, now time.Time) (*Event, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("mission: reopen needs a reason (audit)")
	}
	if !IsTerminal(m.State) {
		return nil, fmt.Errorf("mission: state %q is not terminal (reopen is only for closed missions)", m.State)
	}
	if prev := m.findEvent(idemKey); prev != nil {
		return prev, nil
	}
	m.State = StateRecovering
	m.ReopenCount++
	m.CancelReason = ""
	m.PausedFrom = ""
	return m.record(StateRecovering, actor, "reopen: "+reason, idemKey, now)
}

// LinkInput carries one correlation write: ids from the
// specialized systems this mission governs.
type LinkInput struct {
	RunIDs    []string
	TaskIDs   []string
	AgentIDs  []string
	Worktrees []string
	Commits   []string
	Traces    []string
	Receipts  []string
}

// AttachLinks merges correlation ids as sets: attaching twice is
// a no-op, never a duplicate, so resume and retries cite the same
// artifacts without growing the record. Strict store ids for
// run/task/agent/trace; lenient single-line refs for
// worktree/commit/receipt paths and hashes.
func (m *Mission) AttachLinks(in LinkInput) error {
	for _, id := range in.RunIDs {
		if err := ValidateID(id); err != nil {
			return fmt.Errorf("mission: invalid run id: %w", err)
		}
	}
	for _, id := range in.TaskIDs {
		if err := ValidateID(id); err != nil {
			return fmt.Errorf("mission: invalid task id: %w", err)
		}
	}
	for _, id := range in.AgentIDs {
		if err := ValidateID(id); err != nil {
			return fmt.Errorf("mission: invalid agent id: %w", err)
		}
	}
	for _, id := range in.Traces {
		if err := ValidateID(id); err != nil {
			return fmt.Errorf("mission: invalid trace id: %w", err)
		}
	}
	for _, ref := range in.Worktrees {
		if err := validRef(ref); err != nil {
			return err
		}
	}
	for _, ref := range in.Commits {
		if err := validRef(ref); err != nil {
			return err
		}
	}
	for _, ref := range in.Receipts {
		if err := validRef(ref); err != nil {
			return err
		}
	}
	m.Links.RunIDs = unionSorted(m.Links.RunIDs, in.RunIDs)
	m.Links.TaskIDs = unionSorted(m.Links.TaskIDs, in.TaskIDs)
	m.Links.AgentIDs = unionSorted(m.Links.AgentIDs, in.AgentIDs)
	m.Links.Worktrees = unionSorted(m.Links.Worktrees, in.Worktrees)
	m.Links.Commits = unionSorted(m.Links.Commits, in.Commits)
	m.Links.Traces = unionSorted(m.Links.Traces, in.Traces)
	m.Links.Receipts = unionSorted(m.Links.Receipts, in.Receipts)
	return nil
}

func unionSorted(have, add []string) []string {
	set := map[string]bool{}
	for _, s := range have {
		set[s] = true
	}
	for _, s := range add {
		set[s] = true
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// ApplyEffect runs fn for key once per mission: a stored result
// returns without calling fn (the no-duplicate guarantee resume
// relies on); a fresh key executes, stores the output on success
// and leaves no record on failure so the next resume retries the
// same key. Terminal missions refuse new effects. Memory-only:
// the caller persists afterwards, so a crash before saving
// replays the key instead of duplicating it.
func (m *Mission) ApplyEffect(key, output string, now time.Time, fn func() (string, error)) (EffectResult, error) {
	if strings.TrimSpace(key) == "" || len(key) > 256 || !singleLine(key) {
		return EffectResult{}, fmt.Errorf("mission: invalid effect key %q (non-empty single line, max 256)", key)
	}
	if IsTerminal(m.State) {
		return EffectResult{}, fmt.Errorf("mission: mission %q is %s (terminal, effect refused)", m.ID, m.State)
	}
	if prev, done := m.Effects[key]; done {
		return prev, nil
	}
	out, err := fn()
	if err != nil {
		return EffectResult{}, err
	}
	if output != "" {
		out = output
	}
	res := EffectResult{Key: key, Output: out, At: now.UTC()}
	if m.Effects == nil {
		m.Effects = map[string]EffectResult{}
	}
	m.Effects[key] = res
	return res, nil
}
