// Decision log (fixes-hardening F2, spec S2): every consolidated
// guardian evaluation is recorded as one agree|disagree decision in
// .regulus/guardians/decisions.jsonl, which the observability metrics
// aggregate. Without the log, the guardian rate stays an honest zero
// with a coverage note (see internal/observability Summarize).
package guardians

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/receipts"
)

// Consolidated decision verdicts: the evaluation either agreed with the
// core verdict or disputed it.
const (
	DecisionAgree    = "agree"
	DecisionDisagree = "disagree"
)

// Decision is one consolidated guardian decision: the task scope, the
// guardian, whether its assessment agreed with the consolidated
// verdict, and the detail behind the call.
type Decision struct {
	Task     string `json:"task"`
	Guardian string `json:"guardian"`
	Verdict  string `json:"verdict"`
	Detail   string `json:"detail,omitempty"`
	At       string `json:"at"`
}

// DecisionsFile is the append-only JSONL log of consolidated guardian
// decisions under a project root.
func DecisionsFile(root string) string {
	return filepath.Join(root, ".regulus", "guardians", "decisions.jsonl")
}

// RecordDecision appends one consolidated decision to the log with an
// atomic rewrite (tmp + rename, single writer: the core — same pattern
// as budget.Record). The task scope is sanitized to its slug, matching
// evidence.TaskDir. Validation refuses empty guardians and verdicts
// outside agree|disagree; refused input leaves the log untouched.
func RecordDecision(root, task, guardian, verdict, detail string) (Decision, error) {
	g := strings.TrimSpace(guardian)
	if g == "" {
		return Decision{}, fmt.Errorf("guardian decision: guardian is required")
	}
	v := strings.TrimSpace(verdict)
	if v != DecisionAgree && v != DecisionDisagree {
		return Decision{}, fmt.Errorf("guardian decision: invalid verdict %q (want agree or disagree)", verdict)
	}
	d := Decision{
		Task:     receipts.Slug(strings.TrimSpace(task)),
		Guardian: g,
		Verdict:  v,
		Detail:   strings.TrimSpace(detail),
		At:       time.Now().UTC().Format(time.RFC3339),
	}
	path := DecisionsFile(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Decision{}, err
	}
	prev, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return Decision{}, err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return Decision{}, err
	}
	next := append(append([]byte(nil), prev...), append(raw, '\n')...)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, next, 0o644); err != nil {
		return Decision{}, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return Decision{}, err
	}
	return d, nil
}

// ListDecisions reads a task's recorded decisions, oldest first. A task
// of "" lists every task's decisions. A missing log is empty, never an
// error.
func ListDecisions(root, task string) ([]Decision, error) {
	data, err := os.ReadFile(DecisionsFile(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	want := receipts.Slug(strings.TrimSpace(task))
	scoped := strings.TrimSpace(task) != ""
	var out []Decision
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var d Decision
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			return nil, fmt.Errorf("guardian decisions log: malformed entry: %v", err)
		}
		if scoped && d.Task != want {
			continue
		}
		out = append(out, d)
	}
	return out, sc.Err()
}
