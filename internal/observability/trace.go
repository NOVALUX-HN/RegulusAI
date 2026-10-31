// Span primitives (Phase 8 T2, spec S2): OTel-compatible distributed
// tracing without an OTel SDK (evaluated, not adopted — zero-dep core
// policy; see odd/tasks/phase8-observability.md L2). Field names use
// snake_case (trace_id/span_id/...) so any backend can ingest the JSONL
// export without coupling the core to a vendor.
//
// Correlation (S2): every span carries the correlation_id that links it
// to the structured events of the same task (see log.go). ChildSpan
// inherits trace_id + correlation_id and sets parent_span_id.
//
// Security (S6): every constructor passes free-text fields through
// redaction (see redact.go); there is no unredacted span constructor.
// Use RedactSpan after mutating Attributes post-construction.
package observability

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Span statuses mirror the OTel status vocabulary.
const (
	StatusUnset = "unset"
	StatusOK    = "ok"
	StatusError = "error"
)

// Span is one unit of traced work: task lifecycle, agent execution,
// tool call, guardian verdict detail, gauntlet gate, memory retrieval,
// evolution experiment or external integration (S2). ServiceName maps
// the S1 component to the OTel resource attribute service.name.
type Span struct {
	TraceID       string         `json:"trace_id"`
	SpanID        string         `json:"span_id"`
	ParentSpanID  string         `json:"parent_span_id"`
	ServiceName   string         `json:"service.name"`
	Name          string         `json:"name"`
	TaskID        string         `json:"task_id"`
	CorrelationID string         `json:"correlation_id"`
	StartTime     time.Time      `json:"start_time"`
	EndTime       time.Time      `json:"end_time,omitempty"`
	Status        string         `json:"status"`
	Attributes    map[string]any `json:"attributes"`
}

// NewTraceID mints a random 128-bit trace id as 32 hex chars (OTel
// trace-id shape), following the decisions MintID crypto/rand pattern.
func NewTraceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("observability: cannot mint trace id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// NewSpanID mints a random 64-bit span id as 16 hex chars (OTel
// span-id shape), following the decisions MintID crypto/rand pattern.
func NewSpanID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("observability: cannot mint span id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// newCorrelationID mints a random correlation id for one projected
// timeline: every span of the timeline shares it, linking spans to
// each other and to the task's structured events.
func newCorrelationID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("observability: cannot mint correlation id: %w", err)
	}
	return "corr_" + hex.EncodeToString(b[:]), nil
}

func validateSpanFields(component, name, correlationID string) error {
	for field, v := range map[string]string{
		"component":      component,
		"name":           name,
		"correlation_id": correlationID,
	} {
		if strings.TrimSpace(v) == "" {
			return errors.New("observability: span " + field + " must not be empty")
		}
		if strings.ContainsAny(v, "\n\r") {
			return fmt.Errorf("observability: span %s must be a single line", field)
		}
	}
	return nil
}

// StartSpan opens a root span: fresh trace and span ids, status unset.
// Free-text fields pass through redaction.
func StartSpan(component, name, taskID, correlationID string, start time.Time) (Span, error) {
	if err := validateSpanFields(component, name, correlationID); err != nil {
		return Span{}, err
	}
	if strings.ContainsAny(taskID, "\n\r") {
		return Span{}, errors.New("observability: span task_id must be a single line")
	}
	traceID, err := NewTraceID()
	if err != nil {
		return Span{}, err
	}
	spanID, err := NewSpanID()
	if err != nil {
		return Span{}, err
	}
	s := Span{
		TraceID:       traceID,
		SpanID:        spanID,
		ServiceName:   component,
		Name:          name,
		TaskID:        taskID,
		CorrelationID: correlationID,
		StartTime:     start.UTC(),
		Status:        StatusUnset,
		Attributes:    map[string]any{},
	}
	return RedactSpan(s), nil
}

// ChildSpan opens a nested span: it inherits the parent's trace,
// correlation, task and component, and records the parent's span id.
// Free-text fields pass through redaction.
func (s Span) ChildSpan(name string, start time.Time) (Span, error) {
	if err := validateSpanFields(s.ServiceName, name, s.CorrelationID); err != nil {
		return Span{}, err
	}
	spanID, err := NewSpanID()
	if err != nil {
		return Span{}, err
	}
	c := Span{
		TraceID:       s.TraceID,
		SpanID:        spanID,
		ParentSpanID:  s.SpanID,
		ServiceName:   s.ServiceName,
		Name:          name,
		TaskID:        s.TaskID,
		CorrelationID: s.CorrelationID,
		StartTime:     start.UTC(),
		Status:        StatusUnset,
		Attributes:    map[string]any{},
	}
	return RedactSpan(c), nil
}

// EndSpan closes a span. An unknown or empty status becomes unset.
func (s *Span) EndSpan(end time.Time, status string) {
	s.EndTime = end.UTC()
	switch status {
	case StatusOK, StatusError:
		s.Status = status
	default:
		s.Status = StatusUnset
	}
}

// RedactSpan returns a copy of s with every free-text field scrubbed
// for embedded credential patterns and deny-listed attribute keys
// replaced (S6). Call it after mutating Attributes post-construction;
// constructors already apply it.
func RedactSpan(s Span) Span {
	s.ServiceName = redactText(s.ServiceName)
	s.Name = redactText(s.Name)
	s.TaskID = redactText(s.TaskID)
	s.CorrelationID = redactText(s.CorrelationID)
	s.Attributes = RedactMap(s.Attributes)
	return s
}
