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
	"sort"
	"strings"
	"time"
)

// Statuses.
const (
	StatusCompleted = "completed"
	StatusError     = "error"
)

// Entry is one recorded tool execution. Files lists touched paths
// (summaries only — never values); it is a hint for relevance, and
// Fingerprint deliberately ignores it so diffs stay stable.
type Entry struct {
	At      time.Time `json:"at"`
	Session string    `json:"session"`
	Message string    `json:"message"`
	Call    string    `json:"call"`
	Tool    string    `json:"tool"`
	Status  string    `json:"status"` // completed | error
	Files   []string  `json:"files,omitempty"`
}

// maxFiles caps the touched-path hints per entry: relevance needs a
// few paths, not an exhaustive listing.
const maxFiles = 20

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
	if len(e.Files) > maxFiles {
		return fmt.Errorf("too many files: %d (max %d)", len(e.Files), maxFiles)
	}
	for _, f := range e.Files {
		if strings.TrimSpace(f) == "" {
			return errors.New("files must not contain empty entries")
		}
		if strings.ContainsAny(f, "\n\r") {
			return fmt.Errorf("file path must be a single line, got %q", f)
		}
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

// Sessions lists the session ids with trace logs on disk, sorted.
func Sessions(root string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".regulus", "traces", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range paths {
		out = append(out, strings.TrimSuffix(filepath.Base(p), ".jsonl"))
	}
	sort.Strings(out)
	return out, nil
}

// Prune rewrites a session trace keeping only the last keep entries
// (rotation for unbounded sessions). keep <= 0 keeps everything;
// unknown sessions are a no-op. It returns the kept entry count.
func Prune(root, session string, keep int) (int, error) {
	entries, err := List(root, session)
	if err != nil {
		return 0, err
	}
	if keep <= 0 || len(entries) <= keep {
		return len(entries), nil
	}
	entries = entries[len(entries)-keep:]
	var lines []string
	for _, e := range entries {
		raw, err := json.Marshal(e)
		if err != nil {
			return 0, err
		}
		lines = append(lines, string(raw))
	}
	if err := os.MkdirAll(filepath.Dir(File(root, session)), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(File(root, session)), "trace-*.jsonl")
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
	if err := os.Rename(tmpName, File(root, session)); err != nil {
		return 0, err
	}
	return len(entries), nil
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
