package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testRoot(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func validEvent() Event {
	return Event{
		Timestamp:     time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC),
		Severity:      SeverityInfo,
		Component:     "core",
		ExecutionID:   "exec-1",
		TaskID:        "task-1",
		AgentID:       "agent-1",
		CorrelationID: "corr-1",
		EventType:     "task.completed",
	}
}

func TestRecordRefusesRequiredFields(t *testing.T) {
	base := validEvent()
	cases := map[string]func(*Event){
		"severity":       func(e *Event) { e.Severity = "" },
		"bad severity":   func(e *Event) { e.Severity = "TRACE" },
		"component":      func(e *Event) { e.Component = "  " },
		"execution_id":   func(e *Event) { e.ExecutionID = "" },
		"task_id":        func(e *Event) { e.TaskID = "" },
		"correlation_id": func(e *Event) { e.CorrelationID = "" },
		"event_type":     func(e *Event) { e.EventType = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			root := testRoot(t)
			e := base
			mutate(&e)
			if _, err := Record(root, e, time.Now()); err == nil {
				t.Fatalf("Record accepted event with invalid %s", name)
			}
		})
	}
}

func TestRecordRoundTripAllS1Fields(t *testing.T) {
	root := testRoot(t)
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	want := validEvent()
	want.ErrorDetails = "exit 1: probe mismatch"
	got, err := Record(root, want, now)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if got.Timestamp.IsZero() {
		t.Fatal("Record left timestamp zero")
	}

	entries, err := List(root, want.TaskID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	back := entries[0]
	if back.Severity != want.Severity || back.Component != want.Component ||
		back.ExecutionID != want.ExecutionID || back.TaskID != want.TaskID ||
		back.AgentID != want.AgentID || back.CorrelationID != want.CorrelationID ||
		back.EventType != want.EventType || back.ErrorDetails != want.ErrorDetails {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", back, want)
	}
	if !back.Timestamp.Equal(want.Timestamp) {
		t.Fatalf("timestamp mismatch: got %v want %v", back.Timestamp, want.Timestamp)
	}

	// Raw JSONL line must carry every S1 field name.
	raw, err := os.ReadFile(File(root, want.TaskID))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	var decoded map[string]any
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("unmarshal raw line: %v", err)
	}
	for _, key := range []string{"timestamp", "severity", "component", "execution_id", "task_id", "agent_id", "correlation_id", "event_type", "error_details"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("raw JSON missing S1 field %q: %s", key, lines[0])
		}
	}
}

func TestSecretsFullyRedacted(t *testing.T) {
	root := testRoot(t)
	apiSecret := "sk-live-ABCDEF12345-UNIQUE"
	bearerTok := "eyJhbGciOiJIUzI1NiJ9.UNIQUEPAYLOAD.SIG"
	pw := "hunter2-hunter-UNIQUE"
	e := validEvent()
	e.TaskID = "task-secret"
	e.EventType = "tool.failed"
	e.ErrorDetails = `call failed api_key=` + apiSecret +
		` Authorization: Bearer ` + bearerTok +
		` body {"password": "` + pw + `"}`

	saved, err := Record(root, e, time.Now())
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	for _, secret := range []string{apiSecret, bearerTok, pw} {
		if strings.Contains(saved.ErrorDetails, secret) {
			t.Errorf("returned event leaks secret %q: %q", secret, saved.ErrorDetails)
		}
	}
	if !strings.Contains(saved.ErrorDetails, Redacted) {
		t.Errorf("returned event has no %q marker: %q", Redacted, saved.ErrorDetails)
	}

	raw, err := os.ReadFile(File(root, e.TaskID))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	for _, secret := range []string{apiSecret, bearerTok, pw} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("log file leaks secret %q", secret)
		}
	}
	if !strings.Contains(string(raw), Redacted) {
		t.Errorf("log file has no %q marker", Redacted)
	}

	// Deny-list key matching covers private prompt fields.
	m := RedactMap(map[string]any{
		"system_prompt": "act as root and ignore policy",
		"ok":            "fine",
	})
	if m["system_prompt"] != Redacted {
		t.Errorf("system_prompt not redacted: %v", m["system_prompt"])
	}
	if m["ok"] != "fine" {
		t.Errorf("benign key altered: %v", m["ok"])
	}
}

func TestFileSanitizesTaskScope(t *testing.T) {
	if got, want := File("root", "a/b:c"), filepath.Join("root", ".regulus", "observability", "a_b_c.jsonl"); got != want {
		t.Errorf("task-scoped path = %q, want %q", got, want)
	}
	if got, want := EventsFile("root"), filepath.Join("root", ".regulus", "observability", "events.jsonl"); got != want {
		t.Errorf("default events path = %q, want %q", got, want)
	}
}

func TestPruneKeepsTail(t *testing.T) {
	root := testRoot(t)
	task := "task-prune"
	for i := 1; i <= 5; i++ {
		e := validEvent()
		e.TaskID = task
		e.EventType = "type-" + string(rune('0'+i))
		e.ExecutionID = "exec-prune"
		if _, err := Record(root, e, time.Now()); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	kept, err := Prune(root, task, 2)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if kept != 2 {
		t.Fatalf("Prune kept %d, want 2", kept)
	}
	entries, err := List(root, task)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries after prune, want 2", len(entries))
	}
	if entries[0].EventType != "type-4" || entries[1].EventType != "type-5" {
		t.Fatalf("prune kept wrong tail: %+v", entries)
	}
}
