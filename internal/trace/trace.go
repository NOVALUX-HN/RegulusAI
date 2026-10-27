// Package trace is the minimal agent-process capture (PRD G2.6): one
// append-only line per tool execution, scoped by OpenCode session.
// Receipts record gate results (what ran); traces record the agent's
// process (which tools it called, in which order, with what outcome).
// Two traces of the same gate under different harness configs diff
// cleanly (sequence + tool + status only) — the raw material for
// probe-with-contrast (G2.7). Inputs and results are deliberately NOT
// stored: names and order are enough for contrast, values would bloat
// the log and leak session content into committed state.
package trace

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

// Statuses.
const (
	StatusCompleted = "completed"
	StatusError     = "error"
)

// Entry is one recorded tool execution.
type Entry struct {
	At      time.Time `json:"at"`
	Session string    `json:"session"`
	Message string    `json:"message"`
	Call    string    `json:"call"`
	Tool    string    `json:"tool"`
	Status  string    `json:"status"` // completed | error
}

// File returns the session trace log path under a project root.
func File(root, session string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, session)
	return filepath.Join(root, ".regulus", "traces", safe+".jsonl")
}

func validate(e Entry) error {
	if strings.TrimSpace(e.Session) == "" {
		return errors.New("session must not be empty")
	}
	if strings.TrimSpace(e.Message) == "" {
		return errors.New("message must not be empty")
	}
	if strings.TrimSpace(e.Call) == "" {
		return errors.New("call must not be empty")
	}
	if strings.TrimSpace(e.Tool) == "" {
		return errors.New("tool must not be empty")
	}
	switch e.Status {
	case StatusCompleted, StatusError:
	default:
		return fmt.Errorf("status must be completed or error, got %q", e.Status)
	}
	return nil
}

// Record appends one tool execution to the session trace, in call order.
func Record(root string, e Entry, now time.Time) (Entry, error) {
	if err := validate(e); err != nil {
		return Entry{}, err
	}
	saved := e
	if saved.At.IsZero() {
		saved.At = now.UTC()
	}
	if err := os.MkdirAll(filepath.Dir(File(root, e.Session)), 0o755); err != nil {
		return Entry{}, err
	}
	f, err := os.OpenFile(File(root, e.Session), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Entry{}, err
	}
	line, err := json.Marshal(saved)
	if err != nil {
		f.Close()
		return Entry{}, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return Entry{}, err
	}
	if err := f.Close(); err != nil {
		return Entry{}, err
	}
	return saved, nil
}

// List returns every entry of a session trace in call order. A missing
// trace is empty.
func List(root, session string) ([]Entry, error) {
	f, err := os.Open(File(root, session))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ln := 0
	for sc.Scan() {
		ln++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("malformed trace at line %d: %v", ln, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// Fingerprint reduces a session trace to its diffable shape: sequence
// number, tool name, status. Message and call ids are session-unique,
// so they would make every cross-run diff noise; order is the signal.
func Fingerprint(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for i, e := range entries {
		out = append(out, fmt.Sprintf("%d %s %s", i+1, e.Tool, e.Status))
	}
	return out
}
