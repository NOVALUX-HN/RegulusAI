// Timeline projection (Phase 8 T2, specs S2+S4): read-only spans
// projected from EXISTING stores — no producer rewrites. Receipts
// become verification/gauntlet spans, trace entries become tool-call
// spans, budget entries become task-lifecycle markers, evolution
// experiments become experiment spans. Every span of one timeline
// shares the timeline's trace_id + correlation_id, so receipts,
// tool calls, budget markers and experiments of one task correlate
// with each other and with the task's structured events.
//
// Module telemetry (S4) stays provider-agnostic: stdlib plus
// regulus-ai/internal only, OTel-compatible field names over plain
// data any backend can ingest — no SDK, no core coupling.
//
// Honesty rule: span kinds with no backing store today (guardian
// execution detail, memory retrieval log, external integrations) get
// manual constructors for future producers, but their projections
// return empty with no error and never invent data.
package observability

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/evolve"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

// Projected span kinds (S2 categories), carried in Attributes["kind"].
const (
	KindTaskLifecycle   = "task-lifecycle"
	KindToolCall        = "tool-call"
	KindVerification    = "verification"
	KindGuardian        = "guardian"
	KindMemoryRetrieval = "memory-retrieval"
	KindExperiment      = "experiment"
	KindExternal        = "external"
)

// ReceiptSpan projects one gauntlet receipt as a verification span:
// one span per receipt, correlated by task (S2 gauntlet tracking).
func ReceiptSpan(r receipts.Receipt, traceID, correlationID string) (Span, error) {
	s, err := StartSpan("gauntlet", "verification."+r.Gate, r.Task, correlationID, r.StartedAt)
	if err != nil {
		return Span{}, err
	}
	s.TraceID = traceID
	s.EndTime = r.StartedAt.Add(time.Duration(r.DurationMS) * time.Millisecond).UTC()
	switch r.Verdict {
	case receipts.VerdictPass:
		s.Status = StatusOK
	case receipts.VerdictFail:
		s.Status = StatusError
	default:
		s.Status = StatusUnset
	}
	s.Attributes["kind"] = KindVerification
	s.Attributes["gate"] = r.Gate
	s.Attributes["layer"] = r.Layer
	s.Attributes["verdict"] = r.Verdict
	s.Attributes["exit_code"] = r.ExitCode
	s.Attributes["command"] = r.Command
	s.Attributes["duration_ms"] = r.DurationMS
	s.Attributes["receipt_id"] = r.ID
	return RedactSpan(s), nil
}

// ToolCallSpan projects one trace entry as a tool-call span,
// correlated by session into the caller's timeline (S2 tool calls).
// Trace entries deliberately exclude values (see trace package), so
// projected attributes carry names and order only — never content.
func ToolCallSpan(e trace.Entry, taskID, traceID, correlationID string) (Span, error) {
	s, err := StartSpan("agent", "tool."+e.Tool, taskID, correlationID, e.At)
	if err != nil {
		return Span{}, err
	}
	s.TraceID = traceID
	s.EndTime = e.At.UTC()
	switch e.Status {
	case trace.StatusCompleted:
		s.Status = StatusOK
	case trace.StatusError:
		s.Status = StatusError
	default:
		s.Status = StatusUnset
	}
	s.Attributes["kind"] = KindToolCall
	s.Attributes["tool"] = e.Tool
	s.Attributes["call"] = e.Call
	s.Attributes["message"] = e.Message
	s.Attributes["session"] = e.Session
	s.Attributes["files"] = append([]string{}, e.Files...)
	return RedactSpan(s), nil
}

// BudgetSpan projects one budget ledger entry as a task-lifecycle
// marker: the spend side of the task timeline (S2 task lifecycle).
func BudgetSpan(e budget.Entry, traceID, correlationID string) (Span, error) {
	s, err := StartSpan("budget", "lifecycle."+e.Kind, e.Task, correlationID, e.Timestamp)
	if err != nil {
		return Span{}, err
	}
	s.TraceID = traceID
	s.EndTime = e.Timestamp.UTC()
	switch e.Outcome {
	case budget.OutcomeAdmitted, budget.OutcomeSurfaceApplied, budget.OutcomeProbeRun:
		s.Status = StatusOK
	case budget.OutcomeRefusedCap:
		s.Status = StatusError
	default:
		s.Status = StatusUnset
	}
	s.Attributes["kind"] = KindTaskLifecycle
	s.Attributes["budget_kind"] = e.Kind
	s.Attributes["outcome"] = e.Outcome
	s.Attributes["domain"] = e.Domain
	s.Attributes["detail"] = e.Detail
	return RedactSpan(s), nil
}

// ExperimentSpan projects one evolution experiment as an experiment
// span: history edges give start/end, state gives status (S2 evolution
// experiments). The task link is the caller's timeline task —
// experiments are global, so the correlation_id (not the task) is
// what joins them to the timeline.
func ExperimentSpan(exp evolve.Experiment, taskID, traceID, correlationID string) (Span, error) {
	start, end := experimentWindow(exp)
	s, err := StartSpan("evolution", "experiment."+exp.ID, taskID, correlationID, start)
	if err != nil {
		return Span{}, err
	}
	s.TraceID = traceID
	if !end.IsZero() {
		s.EndTime = end.UTC()
	}
	switch exp.State {
	case evolve.StateAdopted:
		s.Status = StatusOK
	case evolve.StateRejected:
		s.Status = StatusError
	default:
		s.Status = StatusUnset
	}
	s.Attributes["kind"] = KindExperiment
	s.Attributes["experiment_id"] = exp.ID
	s.Attributes["state"] = exp.State
	s.Attributes["domain"] = exp.Domain
	s.Attributes["decision"] = exp.Decision
	return RedactSpan(s), nil
}

// experimentWindow reduces an experiment history to its start (first
// edge) and end (last edge). Unparseable or empty histories yield
// zero times; callers keep the span and its state either way.
func experimentWindow(exp evolve.Experiment) (start, end time.Time) {
	for i, h := range exp.History {
		at, err := time.Parse(time.RFC3339, h.At)
		if err != nil {
			continue
		}
		if i == 0 || start.IsZero() {
			start = at
		}
		end = at
	}
	return start.UTC(), end.UTC()
}

// GuardianSpan builds a guardian-execution span manually: there is no
// guardian execution store today, so no projection mints these —
// future producers use this constructor (honest absence, S2).
func GuardianSpan(component, name, taskID, correlationID string, start time.Time) (Span, error) {
	s, err := StartSpan(component, name, taskID, correlationID, start)
	if err != nil {
		return Span{}, err
	}
	s.Attributes["kind"] = KindGuardian
	return RedactSpan(s), nil
}

// MemoryRetrievalSpan builds a memory-retrieval span manually: there
// is no memory retrieval log today, so no projection mints these —
// future producers use this constructor (honest absence, S2).
func MemoryRetrievalSpan(component, name, taskID, correlationID string, start time.Time) (Span, error) {
	s, err := StartSpan(component, name, taskID, correlationID, start)
	if err != nil {
		return Span{}, err
	}
	s.Attributes["kind"] = KindMemoryRetrieval
	return RedactSpan(s), nil
}

// ExternalSpan builds an external-integration span manually: there is
// no external integration store today, so no projection mints these —
// future producers use this constructor (honest absence, S2).
func ExternalSpan(component, name, taskID, correlationID string, start time.Time) (Span, error) {
	s, err := StartSpan(component, name, taskID, correlationID, start)
	if err != nil {
		return Span{}, err
	}
	s.Attributes["kind"] = KindExternal
	return RedactSpan(s), nil
}

// ProjectReceipts projects one verification span per receipt of a
// task, oldest first. A task with no receipts yields no spans.
func ProjectReceipts(root, taskID, traceID, correlationID string) ([]Span, error) {
	entries, err := evidence.List(root, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]Span, 0, len(entries))
	for _, e := range entries {
		s, err := ReceiptSpan(e.Receipt, traceID, correlationID)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ProjectToolCalls projects one tool-call span per trace entry of a
// session ("" lists every session on disk), in call order. Unknown
// sessions yield no spans.
func ProjectToolCalls(root, session, taskID, traceID, correlationID string) ([]Span, error) {
	sessions := []string{session}
	if session == "" {
		var err error
		sessions, err = trace.Sessions(root)
		if err != nil {
			return nil, err
		}
	}
	var out []Span
	for _, sess := range sessions {
		entries, err := trace.List(root, sess)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			s, err := ToolCallSpan(e, taskID, traceID, correlationID)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
	}
	return out, nil
}

// ProjectBudgetMarkers projects one task-lifecycle marker per budget
// ledger entry of a task, oldest first. A task with no entries
// yields no spans.
func ProjectBudgetMarkers(root, taskID, traceID, correlationID string) ([]Span, error) {
	entries, err := budget.Show(root)
	if err != nil {
		return nil, err
	}
	var out []Span
	for _, e := range entries {
		if e.Task != taskID {
			continue
		}
		s, err := BudgetSpan(e, traceID, correlationID)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ProjectExperiments projects one experiment span per stored
// experiment under .regulus/evolution/, sorted by id. Experiments
// are global (no task field); the caller's taskID and the timeline
// correlation_id join them to the timeline. No experiments on disk
// yields no spans.
func ProjectExperiments(root, taskID, traceID, correlationID string) ([]Span, error) {
	paths, err := filepath.Glob(filepath.Join(evolve.EvolveDir(root), "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []Span
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var exp evolve.Experiment
		if err := json.Unmarshal(data, &exp); err != nil {
			return nil, fmt.Errorf("observability: malformed experiment %s: %w", p, err)
		}
		s, err := ExperimentSpan(exp, taskID, traceID, correlationID)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ProjectGuardianExecutions reports the honest absence of a guardian
// execution store: always empty, never an error, never invented
// data. Guardian detail becomes projectable only when a backing
// store lands; until then GuardianSpan is the manual constructor.
func ProjectGuardianExecutions() ([]Span, error) {
	return nil, nil
}

// ProjectMemoryRetrievals reports the honest absence of a memory
// retrieval log: always empty, never an error, never invented data.
// MemoryRetrievalSpan is the manual constructor for future producers.
func ProjectMemoryRetrievals() ([]Span, error) {
	return nil, nil
}

// ProjectExternalIntegrations reports the honest absence of an
// external integration store: always empty, never an error, never
// invented data. ExternalSpan is the manual constructor for future
// producers.
func ProjectExternalIntegrations() ([]Span, error) {
	return nil, nil
}

// ProjectTimeline projects the full correlated timeline of one task:
// receipts (verification), one session's tool calls ("" for all
// sessions), budget markers and experiments, all sharing one fresh
// trace_id + correlation_id, sorted by start time. Read-only: no
// store is written.
func ProjectTimeline(root, taskID, session string) ([]Span, error) {
	traceID, err := NewTraceID()
	if err != nil {
		return nil, err
	}
	correlationID, err := newCorrelationID()
	if err != nil {
		return nil, err
	}
	receipts, err := ProjectReceipts(root, taskID, traceID, correlationID)
	if err != nil {
		return nil, err
	}
	calls, err := ProjectToolCalls(root, session, taskID, traceID, correlationID)
	if err != nil {
		return nil, err
	}
	markers, err := ProjectBudgetMarkers(root, taskID, traceID, correlationID)
	if err != nil {
		return nil, err
	}
	experiments, err := ProjectExperiments(root, taskID, traceID, correlationID)
	if err != nil {
		return nil, err
	}
	out := append(append(append(receipts, calls...), markers...), experiments...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].StartTime.Equal(out[j].StartTime) {
			return out[i].Name < out[j].Name
		}
		return out[i].StartTime.Before(out[j].StartTime)
	})
	return out, nil
}
