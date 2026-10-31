// Package observability is the provider-agnostic structured event log
// (Phase 8 T1, specs S1+S6): one append-only JSONL line per event, scoped
// by task. Field names stay OTel-compatible so any backend can ingest
// without coupling the core to a vendor SDK. Telemetry lives under
// .regulus/observability/ (gitignored, never committed).
//
// S1 fields per event: timestamp, severity, component, execution ID,
// task ID, optional agent ID, correlation ID, event type, error details.
//
// S6: every write path passes through RedactEvent (see redact.go); there
// is no unredacted write helper.
package observability

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Severity is the event severity (S1).
type Severity string

// Severities.
const (
	SeverityDebug Severity = "debug"
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Event is one structured observability event carrying exactly the S1
// fields. AgentID and ErrorDetails are optional; Timestamp is filled
// with the write time when zero.
type Event struct {
	Timestamp     time.Time `json:"timestamp"`
	Severity      Severity  `json:"severity"`
	Component     string    `json:"component"`
	ExecutionID   string    `json:"execution_id"`
	TaskID        string    `json:"task_id"`
	AgentID       string    `json:"agent_id,omitempty"`
	CorrelationID string    `json:"correlation_id"`
	EventType     string    `json:"event_type"`
	ErrorDetails  string    `json:"error_details,omitempty"`
}

// File returns the task-scoped event log path under a project root. The
// sanitization mirrors the trace package pattern: anything outside
// [A-Za-z0-9-_] becomes '_'. The canonical default log is the "events"
// scope (see EventsFile), which yields
// .regulus/observability/events.jsonl.
func File(root, taskID string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, taskID)
	return filepath.Join(root, ".regulus", "observability", safe+".jsonl")
}

// EventsFile returns the default global event log path under a project
// root: .regulus/observability/events.jsonl.
func EventsFile(root string) string {
	return File(root, "events")
}

func validate(e Event) error {
	switch e.Severity {
	case SeverityDebug, SeverityInfo, SeverityWarn, SeverityError:
	default:
		return fmt.Errorf("severity must be debug, info, warn or error, got %q", e.Severity)
	}
	for name, v := range map[string]string{
		"component":      e.Component,
		"execution_id":   e.ExecutionID,
		"task_id":        e.TaskID,
		"correlation_id": e.CorrelationID,
		"event_type":     e.EventType,
	} {
		if strings.TrimSpace(v) == "" {
			return errors.New(name + " must not be empty")
		}
		if strings.ContainsAny(v, "\n\r") {
			return fmt.Errorf("%s must be a single line", name)
		}
	}
	if strings.ContainsAny(e.AgentID, "\n\r") {
		return errors.New("agent_id must be a single line")
	}
	return nil
}

// Record validates an event, passes it through redaction, and appends it
// as one JSONL line to the task-scoped log. The redacted event is
// returned. There is no unredacted write path.
func Record(root string, e Event, now time.Time) (Event, error) {
	if err := validate(e); err != nil {
		return Event{}, err
	}
	saved := RedactEvent(e)
	if saved.Timestamp.IsZero() {
		saved.Timestamp = now.UTC()
	}
	if err := os.MkdirAll(filepath.Dir(File(root, saved.TaskID)), 0o755); err != nil {
		return Event{}, err
	}
	f, err := os.OpenFile(File(root, saved.TaskID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Event{}, err
	}
	line, err := json.Marshal(saved)
	if err != nil {
		f.Close()
		return Event{}, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return Event{}, err
	}
	if err := f.Close(); err != nil {
		return Event{}, err
	}
	return saved, nil
}

// List returns every event of a task scope in write order. A missing log
// is empty.
func List(root, taskID string) ([]Event, error) {
	f, err := os.Open(File(root, taskID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ln := 0
	for sc.Scan() {
		ln++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("malformed observability event at line %d: %v", ln, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}
