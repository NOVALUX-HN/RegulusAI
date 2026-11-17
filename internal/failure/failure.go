// Package failure implements F13.5/NOV-108: Mission Failure
// Intelligence and mission-level recovery. The Failure Coordinator
// consumes mission states, errors, gate verdicts, traces and
// receipts, classifies the failure, selects one recovery decision
// (retry, reassign, replan, isolate, rollback, pause_for_approval
// or fail_closed) and records cause, evidence, decision, discarded
// alternatives and the proposed result.
//
// The coordinator proposes, never executes (precedents
// `orchestrate decide`, `quality check`): retries run through
// `recovery retry`/`resume`, reassignments through
// `recovery resume --new-agent`, replans through `mission replan`,
// compensations through `recovery rollback`, and lessons enter
// F12.6 as hypotheses, never facts.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - task recovery: internal/recovery (checkpoints, effect
//     ledger, idempotency keys, RetryPolicy budgets/backoff/
//     circuit, ClassifyError base, CheckScopesSubset, RollbackAll)
//     — this package owns no checkpoint, no ledger, no retry
//     budget of its own; mission-level budgets embed the F12.5
//     RetryPolicy verbatim.
//   - mission truth: internal/mission (state vocabulary,
//     CheckTransition for the suggested mission edge,
//     ValidateID for decision-log paths) — the coordinator never
//     mutates a mission, it only suggests the audited edge the
//     operator runs with `mission advance|pause|cancel`.
//   - learning: internal/almanac (LearnSave) — lessons are always
//     kind hypothesis with a low/medium self-assessed confidence;
//     promotion to verified-solution stays with `learn verify`.
//   - durability: internal/fsx (AppendLine) for the append-only
//     decision log; traces ride the F11.1 exec log by citing
//     run/task/mission ids, never duplicating it.
package failure

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"regulus-ai/internal/recovery"
)

// Failure classes (closed mission-level vocabulary, 10). The first
// eight come from the F13.5 spec; agent_offline and host_offline
// extend the F12.5 classification (recovery.ClassHostOff and the
// worker-missing signals F12.5 never named) instead of replacing
// it — Classify maps the F12.5 base onto this vocabulary first.
const (
	ClassTransient        = "transient"
	ClassPermanent        = "permanent"
	ClassPermissionDenied = "permission_denied"
	ClassBudgetExceeded   = "budget_exceeded"
	ClassMergeConflict    = "merge_conflict"
	ClassValidationFailed = "validation_failed"
	ClassCorruptedState   = "corrupted_state"
	ClassUnknown          = "unknown"
	ClassAgentOffline     = "agent_offline"
	ClassHostOffline      = "host_offline"
)

// AllClasses lists the closed vocabulary in a stable order.
var AllClasses = []string{
	ClassTransient, ClassPermanent, ClassPermissionDenied,
	ClassBudgetExceeded, ClassMergeConflict, ClassValidationFailed,
	ClassCorruptedState, ClassUnknown, ClassAgentOffline,
	ClassHostOffline,
}

// validClass reports whether c belongs to the closed vocabulary.
func validClass(c string) bool {
	for _, k := range AllClasses {
		if c == k {
			return true
		}
	}
	return false
}

// Recovery decisions (closed vocabulary, 7): exactly one is
// selected per failure, the rest are recorded as discarded with
// the reason each lost.
const (
	DecisionRetry            = "retry"
	DecisionReassign         = "reassign"
	DecisionReplan           = "replan"
	DecisionIsolate          = "isolate"
	DecisionRollback         = "rollback"
	DecisionPauseForApproval = "pause_for_approval"
	DecisionFailClosed       = "fail_closed"
)

// AllDecisions lists the closed vocabulary in a stable order.
var AllDecisions = []string{
	DecisionRetry, DecisionReassign, DecisionReplan,
	DecisionIsolate, DecisionRollback, DecisionPauseForApproval,
	DecisionFailClosed,
}

// Signal is one mission-level failure intake: what failed, what the
// mission looked like, what the gates said and what recovery
// resources remain. Receipts, traces and run/task/agent ids ride
// as cited evidence, never as owned state.
type Signal struct {
	MissionID    string
	MissionState string
	ErrorText    string
	// ClassOverride forces classification (validated against the
	// closed vocabulary); empty means classify the evidence.
	ClassOverride string
	// GateVerdict carries the F13.4 gate outcome ("red", "green",
	// "blocked", ...): a red gate classifies as validation_failed
	// unless sharper evidence (corruption, permission, budget)
	// wins by precedence.
	GateVerdict string
	// BudgetExhausted flags an externally confirmed exhausted
	// budget (time/tokens/cost ceiling hit): retries refuse even
	// when the error text looks transient.
	BudgetExhausted bool
	// RetriesUsed/ReplansUsed count prior mission-level recovery
	// attempts against the policy budgets.
	RetriesUsed int
	ReplansUsed int
	// EffectsApplied counts mission effects already confirmed
	// (never re-executed: forward recovery resumes through the
	// F12.5 ledger, it never replays them).
	EffectsApplied int
	// CompensationsPending counts explicit undo steps the
	// mission owns and may roll back.
	CompensationsPending int
	// WorktreeState attests the integration area ("clean",
	// "dirty", "unknown"): reassignment requires "clean".
	WorktreeState string
	// Confirmed lists task ids with confirmed results the replan
	// must preserve; PlanTasks the proposed plan; ModifiedPlan
	// the confirmed tasks the replan would change.
	Confirmed    []string
	PlanTasks    []string
	ModifiedPlan []string
	// CompensationKeys are the undo steps a rollback would run;
	// OwnedPrefixes are the mission-owned key prefixes (mission
	// id, run ids) every key must sit under.
	CompensationKeys []string
	OwnedPrefixes    []string
	// EvidenceRefs cites receipts/traces/runs backing the cause.
	EvidenceRefs []string
	Actor        string
	// IdemKey makes repeated intakes idempotent: the decision
	// log replays the stored record instead of appending twice.
	IdemKey string
}

// ReassignBid carries one reassignment candidate into Decide:
// Present means the operator offers a target; Verified means
// VerifyReassign already attested capabilities, context, worktree
// state, scopes and circuit. An unverified bid never wins — the
// decision falls back to isolate with the reason recorded.
type ReassignBid struct {
	Present  bool
	Verified bool
	ToAgent  string
	ToHost   string
	Detail   string
}

// DecisionRecord is one sealed coordinator outcome: the cause, the
// class, the cited evidence, the selected decision, why it won,
// why the alternatives lost, and the concrete next command the
// operator runs (Result is always a proposal — execution stays
// with recovery/mission/learn commands).
type DecisionRecord struct {
	ID                 string    `json:"id"`
	At                 time.Time `json:"at"`
	MissionID          string    `json:"mission_id"`
	MissionState       string    `json:"mission_state,omitempty"`
	Cause              string    `json:"cause"`
	Class              string    `json:"class"`
	ClassDetail        string    `json:"class_detail,omitempty"`
	Evidence           []string  `json:"evidence,omitempty"`
	Decision           string    `json:"decision"`
	Reason             string    `json:"reason"`
	Discarded          []string  `json:"discarded,omitempty"`
	BackoffMS          int64     `json:"backoff_ms,omitempty"`
	SuggestedMissionTo string    `json:"suggested_mission_to,omitempty"`
	ReassignDetail     string    `json:"reassign_detail,omitempty"`
	Actor              string    `json:"actor,omitempty"`
	IdemKey            string    `json:"idem_key,omitempty"`
	Result             string    `json:"result"`
}

// Policy bounds mission-level recovery. Retry embeds the F12.5
// budget verbatim (max retries, deterministic backoff, timeout,
// circuit threshold/cooldown) so task and mission retries share
// one discipline; MaxReplans caps mission replan attempts.
// Escalation is structural: exhausted budgets, permanent,
// permission, corrupted and unknown outcomes all resolve to a
// human-owned state (pause_for_approval or fail_closed), never to
// silent continuation.
type Policy struct {
	Retry      recovery.RetryPolicy `json:"retry"`
	MaxReplans int                  `json:"max_replans"`
}

// DefaultPolicy is the documented budget: F12.5 defaults (3
// retries, 1s base doubling to a 30s cap, 60s attempt timeout,
// host opens after 3 consecutive failures for 5 minutes) plus at
// most 2 mission replans before human approval is required.
func DefaultPolicy() Policy {
	return Policy{Retry: recovery.DefaultRetryPolicy(), MaxReplans: 2}
}

// Validate refuses unbounded or hostile budgets before anything
// is decided: the F12.5 retry discipline plus MaxReplans in
// 0..5 (0 means replan is never selected — validation failures
// escalate directly to pause_for_approval).
func (p Policy) Validate() error {
	if err := p.Retry.Validate(); err != nil {
		return err
	}
	if p.MaxReplans < 0 || p.MaxReplans > 5 {
		return fmt.Errorf("failure: --max-replans %d outside 0..5", p.MaxReplans)
	}
	return nil
}

// MintDecisionID mints a stable unique decision id: dec_ + 16 hex
// chars (64 bits, crypto/rand), following the mission MintID
// pattern.
func MintDecisionID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("failure: cannot mint decision id: %w", err)
	}
	return "dec_" + hex.EncodeToString(b[:]), nil
}
