// Execution events (F11.1/NOV-87, spec S1): versioned structured
// traceability of every agent execution — run, task and delegation —
// from start to terminal result.
//
// IDs (correlacionables, minted once and propagated by Core, plugin
// and adapters via trace emit flags): run_id scopes one integrated
// run; task_id the Sextant task served; agent_execution_id one agent
// invocation; delegation_id one delegation edge (request + result
// share it); session_id the host session; event_id the unique line.
// Every event carries schema_version, host/model/version, a UTC wall
// timestamp (canonical) plus a writer-process monotonic mark (ordering
// aid within one process only), a duration, a terminal-unequivocal
// status and an optional parent_event_id for parent-child links.
//
// Store: one append-only global JSONL log
// (.regulus/observability/exec.jsonl, gitignored like every
// observability scope). Writes are incremental (one locked append per
// event), idempotent on event_id (duplicates are a no-op returning
// dup=true), tolerant on read (malformed lines are counted and
// skipped, never a silent partial read). Payloads are truncated, never
// full transcripts; every write path redacts before disk (see
// redact.go); retention is an explicit prune keeping the last N lines.
package observability

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/fsx"
)

// ExecSchemaVersion versions the execution-event document. Readers
// must refuse versions they do not know; migrations are additive
// (see docs/trace-schema.md).
const ExecSchemaVersion = 1

// Execution event types (S1): start/end pairs for run, task and
// agent; context metadata; tool call/result; delegation request/result
// (attributable origin/destination via agent_execution_id + payload
// "to"); retry, timeout, gate, artifact, decision and error.
const (
	ExecRunStart          = "run.start"
	ExecRunEnd            = "run.end"
	ExecTaskStart         = "task.start"
	ExecTaskEnd           = "task.end"
	ExecAgentStart        = "agent.start"
	ExecAgentEnd          = "agent.end"
	ExecContext           = "context"
	ExecToolCall          = "tool.call"
	ExecToolResult        = "tool.result"
	ExecDelegationRequest = "delegation.request"
	ExecDelegationResult  = "delegation.result"
	ExecRetry             = "retry"
	ExecTimeout           = "timeout"
	ExecGate              = "gate"
	ExecArtifact          = "artifact"
	ExecDecision          = "decision"
	ExecError             = "error"
)

// execTypes is the closed vocabulary RecordExec accepts.
var execTypes = map[string]bool{
	ExecRunStart: true, ExecRunEnd: true,
	ExecTaskStart: true, ExecTaskEnd: true,
	ExecAgentStart: true, ExecAgentEnd: true,
	ExecContext:  true,
	ExecToolCall: true, ExecToolResult: true,
	ExecDelegationRequest: true, ExecDelegationResult: true,
	ExecRetry: true, ExecTimeout: true,
	ExecGate: true, ExecArtifact: true, ExecDecision: true,
	ExecError: true,
}

// Execution statuses. Terminal states are inequívocos: once an
// execution reaches one, no further event may advance it (a later
// terminal run.end is a malformed producer, kept on disk but flagged
// by ReconstructRun counting ends).
const (
	ExecStarted   = "started"
	ExecRunning   = "running"
	ExecSucceeded = "succeeded"
	ExecFailed    = "failed"
	ExecTimedOut  = "timed_out"
	ExecCancelled = "cancelled"
)

// execStatuses is the closed status vocabulary.
var execStatuses = map[string]bool{
	ExecStarted: true, ExecRunning: true,
	ExecSucceeded: true, ExecFailed: true,
	ExecTimedOut: true, ExecCancelled: true,
}

// IsTerminal reports whether status closes an execution. Crashed and
// incomplete are reconstruction verdicts (see RunTimeline), never
// stored statuses.
func IsTerminal(status string) bool {
	switch status {
	case ExecSucceeded, ExecFailed, ExecTimedOut, ExecCancelled:
		return true
	}
	return false
}

// Reconstruction verdicts: never stored, only derived by
// ReconstructRun when no terminal run.end closes the run.
const (
	ExecCrashed    = "crashed"
	ExecIncomplete = "incomplete"
)

// Payload limits (S1: no full transcripts by default). Strings beyond
// MaxExecStringValue are truncated with truncExecMarker and the event
// keeps payload["truncated"]=true; payloads beyond MaxExecPayloadKeys
// keys or MaxExecPayloadBytes JSON bytes are refused.
const (
	MaxExecStringValue  = 2048
	MaxExecPayloadKeys  = 64
	MaxExecPayloadBytes = 8192
	// DefaultExecKeep is the retention cap (lines) for the global
	// execution log; PruneExec keeps the last N lines.
	DefaultExecKeep = 5000
)

const truncExecMarker = "…[truncated; full value not stored]"

// ExecEvent is one versioned structured execution event.
type ExecEvent struct {
	SchemaVersion    int            `json:"schema_version"`
	EventID          string         `json:"event_id"`
	RunID            string         `json:"run_id"`
	TaskID           string         `json:"task_id,omitempty"`
	AgentExecutionID string         `json:"agent_execution_id,omitempty"`
	DelegationID     string         `json:"delegation_id,omitempty"`
	SessionID        string         `json:"session_id,omitempty"`
	ParentEventID    string         `json:"parent_event_id,omitempty"`
	Type             string         `json:"event_type"`
	Status           string         `json:"status"`
	Host             string         `json:"host,omitempty"`
	Model            string         `json:"model,omitempty"`
	HostVersion      string         `json:"host_version,omitempty"`
	At               time.Time      `json:"at"`
	MonoMS           int64          `json:"monotonic_ms,omitempty"`
	DurationMS       int64          `json:"duration_ms,omitempty"`
	Payload          map[string]any `json:"payload,omitempty"`
	ErrorDetails     string         `json:"error_details,omitempty"`
}

// execProcessStart anchors MonoMS: milliseconds from time.Since at
// Record time. Monotonic marks are comparable only within one writer
// process; the UTC wall timestamp At is the canonical order key.
var execProcessStart = time.Now()

// MonoNow returns the current writer-process monotonic mark.
func MonoNow() int64 {
	return time.Since(execProcessStart).Milliseconds()
}

// mintExecID mints a process-safe random id: prefix + 16 hex chars
// (64 bits, crypto/rand), following the decisions MintID pattern.
func mintExecID(prefix string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("observability: cannot mint %s id: %w", prefix, err)
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

// MintEventID mints a unique event_id.
func MintEventID() (string, error) { return mintExecID("evt_") }

// MintRunID mints a unique run_id.
func MintRunID() (string, error) { return mintExecID("run_") }

// MintAgentExecutionID mints a unique agent_execution_id.
func MintAgentExecutionID() (string, error) { return mintExecID("agent_") }

// MintDelegationID mints a unique delegation_id.
func MintDelegationID() (string, error) { return mintExecID("dlg_") }

// ExecFile returns the global execution-event log path under a
// project root: .regulus/observability/exec.jsonl.
func ExecFile(root string) string {
	return filepath.Join(root, ".regulus", "observability", "exec.jsonl")
}

func singleLine(name, v string, required bool) error {
	if strings.TrimSpace(v) == "" {
		if required {
			return errors.New("observability: exec " + name + " must not be empty")
		}
		return nil
	}
	if strings.ContainsAny(v, "\n\r") {
		return fmt.Errorf("observability: exec %s must be a single line", name)
	}
	return nil
}

func validateExec(e ExecEvent) error {
	if e.SchemaVersion != ExecSchemaVersion {
		return fmt.Errorf("observability: exec schema_version must be %d, got %d", ExecSchemaVersion, e.SchemaVersion)
	}
	for name, v := range map[string]string{
		"event_id": e.EventID, "run_id": e.RunID,
		"event_type": e.Type, "status": e.Status,
	} {
		if err := singleLine(name, v, true); err != nil {
			return err
		}
	}
	for name, v := range map[string]string{
		"task_id": e.TaskID, "agent_execution_id": e.AgentExecutionID,
		"delegation_id": e.DelegationID, "session_id": e.SessionID,
		"parent_event_id": e.ParentEventID,
		"host":            e.Host, "model": e.Model, "host_version": e.HostVersion,
	} {
		if err := singleLine(name, v, false); err != nil {
			return err
		}
	}
	if !execTypes[e.Type] {
		return fmt.Errorf("observability: exec unknown event_type %q", e.Type)
	}
	if !execStatuses[e.Status] {
		return fmt.Errorf("observability: exec unknown status %q", e.Status)
	}
	if e.DurationMS < 0 {
		return errors.New("observability: exec duration_ms must not be negative")
	}
	if len(e.Payload) > MaxExecPayloadKeys {
		return fmt.Errorf("observability: exec payload has %d keys (max %d)", len(e.Payload), MaxExecPayloadKeys)
	}
	return nil
}

// truncateExecStrings caps every string inside v at MaxExecStringValue
// runes, marking truncation. Nested maps and slices recurse; key
// names are preserved so truncated transcripts stay attributable
// without storing content.
func truncateExecStrings(v any, truncated *bool) any {
	switch t := v.(type) {
	case string:
		r := []rune(t)
		if len(r) > MaxExecStringValue {
			*truncated = true
			return string(r[:MaxExecStringValue]) + truncExecMarker
		}
		return t
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = truncateExecStrings(e, truncated)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = truncateExecStrings(e, truncated)
		}
		return out
	default:
		return v
	}
}

// RedactExecEvent returns a copy of e with every free-text field
// scrubbed for embedded credential patterns and the payload passed
// through the shared deny-list redactor (see redact.go). There is no
// unredacted execution-event writer.
func RedactExecEvent(e ExecEvent) ExecEvent {
	e.EventID = redactText(e.EventID)
	e.RunID = redactText(e.RunID)
	e.TaskID = redactText(e.TaskID)
	e.AgentExecutionID = redactText(e.AgentExecutionID)
	e.DelegationID = redactText(e.DelegationID)
	e.SessionID = redactText(e.SessionID)
	e.ParentEventID = redactText(e.ParentEventID)
	e.Host = redactText(e.Host)
	e.Model = redactText(e.Model)
	e.HostVersion = redactText(e.HostVersion)
	e.ErrorDetails = redactText(e.ErrorDetails)
	e.Payload = RedactMap(e.Payload)
	return e
}

// RecordExec validates one execution event, truncates oversized
// payload strings (no full transcripts), redacts secrets, and appends
// it as one JSONL line to the global execution log. Empty EventID is
// minted; zero At/MonoMS are filled with the write time. The write is
// idempotent on event_id: a duplicate returns dup=true and the stored
// line is left untouched. It returns the stored (redacted, truncated)
// event.
func RecordExec(root string, e ExecEvent, now time.Time) (saved ExecEvent, dup bool, err error) {
	if e.SchemaVersion == 0 {
		e.SchemaVersion = ExecSchemaVersion
	}
	if e.EventID == "" {
		id, merr := MintEventID()
		if merr != nil {
			return ExecEvent{}, false, merr
		}
		e.EventID = id
	}
	if err := validateExec(e); err != nil {
		return ExecEvent{}, false, err
	}
	if e.Payload != nil {
		var truncated bool
		if m, ok := truncateExecStrings(e.Payload, &truncated).(map[string]any); ok {
			e.Payload = m
			if truncated {
				e.Payload["truncated"] = true
			}
		}
		raw, merr := json.Marshal(e.Payload)
		if merr != nil {
			return ExecEvent{}, false, merr
		}
		if len(raw) > MaxExecPayloadBytes {
			return ExecEvent{}, false, fmt.Errorf("observability: exec payload is %d bytes (max %d)", len(raw), MaxExecPayloadBytes)
		}
	}
	saved = RedactExecEvent(e)
	if saved.At.IsZero() {
		saved.At = now.UTC()
	} else {
		saved.At = saved.At.UTC()
	}
	if saved.MonoMS == 0 {
		saved.MonoMS = MonoNow()
	}
	line, err := json.Marshal(saved)
	if err != nil {
		return ExecEvent{}, false, err
	}
	line = append(line, '\n')
	path := ExecFile(root)
	release, err := fsx.LockDir(filepath.Dir(path))
	if err != nil {
		return ExecEvent{}, false, err
	}
	defer release()
	if have, rerr := execHasID(path, saved.EventID); rerr != nil {
		return ExecEvent{}, false, rerr
	} else if have {
		return saved, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return ExecEvent{}, false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return ExecEvent{}, false, err
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return ExecEvent{}, false, err
	}
	if err := f.Close(); err != nil {
		return ExecEvent{}, false, err
	}
	return saved, false, nil
}

// execHasID reports whether the log at path already holds event_id.
// Callers hold the fsx dir lock.
func execHasID(path, id string) (bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var probe struct {
			EventID string `json:"event_id"`
		}
		if jerr := json.Unmarshal([]byte(line), &probe); jerr != nil {
			continue
		}
		if probe.EventID == id {
			return true, nil
		}
	}
	return false, sc.Err()
}

// LoadExec returns every execution event in write order. It is the
// tolerant reader: blank lines are skipped, malformed lines are
// counted (malformed) and skipped, duplicate event_ids keep their
// first occurrence (counted in dups). A missing log reads as empty.
func LoadExec(root string) (events []ExecEvent, malformed, dups int, err error) {
	f, err := os.Open(ExecFile(root))
	if os.IsNotExist(err) {
		return nil, 0, 0, nil
	}
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e ExecEvent
		if jerr := json.Unmarshal([]byte(line), &e); jerr != nil {
			malformed++
			continue
		}
		if e.EventID == "" || e.SchemaVersion != ExecSchemaVersion {
			malformed++
			continue
		}
		if seen[e.EventID] {
			dups++
			continue
		}
		seen[e.EventID] = true
		events = append(events, e)
	}
	if err := sc.Err(); err != nil {
		return nil, malformed, dups, err
	}
	return events, malformed, dups, nil
}

// ExecFilter scopes an execution query: every non-empty field must
// match (AND). Empty filters match everything.
type ExecFilter struct {
	RunID            string
	TaskID           string
	AgentExecutionID string
	DelegationID     string
	SessionID        string
	Type             string
	Status           string
}

func (f ExecFilter) matches(e ExecEvent) bool {
	if f.RunID != "" && e.RunID != f.RunID {
		return false
	}
	if f.TaskID != "" && e.TaskID != f.TaskID {
		return false
	}
	if f.AgentExecutionID != "" && e.AgentExecutionID != f.AgentExecutionID {
		return false
	}
	if f.DelegationID != "" && e.DelegationID != f.DelegationID {
		return false
	}
	if f.SessionID != "" && e.SessionID != f.SessionID {
		return false
	}
	if f.Type != "" && e.Type != f.Type {
		return false
	}
	if f.Status != "" && e.Status != f.Status {
		return false
	}
	return true
}

// QueryExec loads the execution log (tolerating malformed and
// duplicate lines) and returns the matching events ordered by wall
// timestamp, ties broken by event_id — so out-of-order or concurrent
// writes still read as one coherent timeline.
func QueryExec(root string, f ExecFilter) (events []ExecEvent, malformed, dups int, err error) {
	all, malformed, dups, err := LoadExec(root)
	if err != nil {
		return nil, malformed, dups, err
	}
	for _, e := range all {
		if f.matches(e) {
			events = append(events, e)
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].At.Equal(events[j].At) {
			return events[i].EventID < events[j].EventID
		}
		return events[i].At.Before(events[j].At)
	})
	return events, malformed, dups, nil
}

// DelegationEdge attributes one delegation to origin and destination:
// the request's agent_execution_id is the origin, payload "to" names
// the destination agent, and the result (same delegation_id) closes
// it. A request without a result reads as pending.
type DelegationEdge struct {
	DelegationID string
	FromAgent    string
	ToAgent      string
	Request      *ExecEvent
	Result       *ExecEvent
	Status       string
}

// RunTimeline reconstructs one integrated run from persisted events:
// ordered events, terminal-unequivocal status, delegation edges, and
// crash evidence. A run whose log holds no terminal run.end is
// reported as crashed (a start-type event without its close) or
// incomplete (events but no start): the execution is rebuilt after a
// crash instead of failing the query.
type RunTimeline struct {
	RunID       string
	Events      []ExecEvent
	Status      string
	Crashed     bool
	Malformed   int
	Duplicates  int
	StartedAt   time.Time
	EndedAt     time.Time
	DurationMS  int64
	Delegations []DelegationEdge
}

// ReconstructRun rebuilds one run from persisted events. Terminal
// run.end wins (the last one when producers misbehave); without it
// the run reads as crashed when any start-type event exists, else
// incomplete. Delegations group by delegation_id with origin from
// the request and destination from its payload "to".
func ReconstructRun(root, runID string) (RunTimeline, error) {
	events, malformed, dups, err := QueryExec(root, ExecFilter{RunID: runID})
	if err != nil {
		return RunTimeline{}, err
	}
	tl := RunTimeline{
		RunID:      runID,
		Events:     events,
		Malformed:  malformed,
		Duplicates: dups,
		Status:     ExecIncomplete,
	}
	if len(events) == 0 {
		return tl, nil
	}
	tl.StartedAt = events[0].At
	tl.EndedAt = events[len(events)-1].At
	if tl.EndedAt.After(tl.StartedAt) {
		tl.DurationMS = tl.EndedAt.Sub(tl.StartedAt).Milliseconds()
	}
	var lastEnd *ExecEvent
	sawStart := false
	for i := range events {
		switch events[i].Type {
		case ExecRunStart, ExecTaskStart, ExecAgentStart:
			sawStart = true
		case ExecRunEnd:
			if IsTerminal(events[i].Status) {
				lastEnd = &events[i]
			}
		}
	}
	switch {
	case lastEnd != nil:
		tl.Status = lastEnd.Status
	case sawStart:
		tl.Status = ExecCrashed
		tl.Crashed = true
	default:
		tl.Status = ExecIncomplete
	}
	byDlg := map[string]*DelegationEdge{}
	var order []string
	for i := range events {
		e := events[i]
		if e.DelegationID == "" {
			continue
		}
		edge, ok := byDlg[e.DelegationID]
		if !ok {
			edge = &DelegationEdge{DelegationID: e.DelegationID, Status: "pending"}
			byDlg[e.DelegationID] = edge
			order = append(order, e.DelegationID)
		}
		switch e.Type {
		case ExecDelegationRequest:
			cp := e
			edge.Request = &cp
			edge.FromAgent = e.AgentExecutionID
			if to, _ := e.Payload["to"].(string); to != "" {
				edge.ToAgent = to
			}
		case ExecDelegationResult:
			cp := e
			edge.Result = &cp
			edge.Status = e.Status
		}
	}
	for _, id := range order {
		tl.Delegations = append(tl.Delegations, *byDlg[id])
	}
	return tl, nil
}

// PruneExec rewrites the global execution log keeping only the last
// keep events (rotation for the unbounded log). keep <= 0 keeps
// everything; a missing log is empty. It returns the kept count.
func PruneExec(root string, keep int) (int, error) {
	events, _, _, err := LoadExec(root)
	if err != nil {
		return 0, err
	}
	if keep <= 0 || len(events) <= keep {
		return len(events), nil
	}
	events = events[len(events)-keep:]
	var lines []string
	for _, e := range events {
		raw, err := json.Marshal(e)
		if err != nil {
			return 0, err
		}
		lines = append(lines, string(raw))
	}
	path := ExecFile(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "exec-*.jsonl")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	body := strings.Join(lines, "\n") + "\n"
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return 0, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return 0, err
	}
	return len(events), nil
}
