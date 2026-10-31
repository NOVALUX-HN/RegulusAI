// Package decisions is Decision Intelligence (Phase 4): Regulus evaluates
// technical decisions, never makes them and never auto-approves.
//
// A decision is a structured contract (problem, proposal, alternatives,
// trade-offs, constraints, risks, consequences, evidence, confidence,
// reversibility, artifacts) stored as one JSON file under
// .regulus/decisions/: immutable contract plus an appended transition
// history. State transitions are validated; integrity is re-checkable;
// assessments are advisory and live in assess.go.
package decisions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Decision states.
const (
	StateProposed     = "PROPOSED"
	StateUnderReview  = "UNDER_REVIEW"
	StateAccepted     = "ACCEPTED"
	StateRejected     = "REJECTED"
	StateInconclusive = "INCONCLUSIVE"
	StateSuperseded   = "SUPERSEDED"
)

// SchemaVersion versions the decision document.
const SchemaVersion = 1

// Alternative is one considered option with its price.
type Alternative struct {
	Name     string `json:"name"`
	Tradeoff string `json:"tradeoff"`
}

// Contract is the structured decision record (Phase 4, S1).
type Contract struct {
	ID            string        `json:"id,omitempty"`
	Problem       string        `json:"problem"`
	Scope         string        `json:"scope"`
	Context       string        `json:"context"`
	Proposal      string        `json:"proposal"`
	Alternatives  []Alternative `json:"alternatives"`
	Tradeoffs     []string      `json:"tradeoffs"`
	Constraints   []string      `json:"constraints"`
	Risks         []string      `json:"risks"`
	Consequences  string        `json:"consequences"`
	Evidence      []string      `json:"evidence"`
	Confidence    string        `json:"confidence"`
	Reversibility string        `json:"reversibility"`
	Rollback      string        `json:"rollback,omitempty"`
	Artifacts     []string      `json:"artifacts"`
	Commits       []string      `json:"commits"`
	Task          string        `json:"task,omitempty"`
	Related       []string      `json:"related"`
	Critical      bool          `json:"critical"`
}

// HistoryEvent is one appended transition or assessment record.
type HistoryEvent struct {
	At   string `json:"at"`
	Kind string `json:"kind"`
	From string `json:"from"`
	To   string `json:"to"`
	By   string `json:"by"`
	Note string `json:"note"`
}

// Decision is a stored contract with its state and history.
type Decision struct {
	Contract
	Schema  int            `json:"schema"`
	State   string         `json:"state"`
	History []HistoryEvent `json:"history"`
}

var (
	idRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	commitRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	relRe    = regexp.MustCompile(`^dec:[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// ParseContract validates a decision contract document. Structure and
// formats are checked here; substance (alternatives present, mitigations
// named, evidence resolving) is the assessment's job.
func ParseContract(data []byte) (Contract, error) {
	var c Contract
	if err := json.Unmarshal(data, &c); err != nil {
		return Contract{}, fmt.Errorf("decision: malformed JSON: %v", err)
	}
	if strings.TrimSpace(c.ID) != "" && !idRe.MatchString(c.ID) {
		return Contract{}, fmt.Errorf("decision: bad id %q (want [A-Za-z0-9_.-]+)", c.ID)
	}
	for _, field := range []struct {
		name, value string
	}{
		{"problem", c.Problem}, {"proposal", c.Proposal}, {"scope", c.Scope},
	} {
		if strings.TrimSpace(field.value) == "" {
			return Contract{}, fmt.Errorf("decision: %s is required", field.name)
		}
	}
	switch c.Confidence {
	case "low", "medium", "high":
	default:
		return Contract{}, fmt.Errorf("decision: confidence %q must be low, medium or high", c.Confidence)
	}
	switch c.Reversibility {
	case "easy", "moderate", "hard":
	default:
		return Contract{}, fmt.Errorf("decision: reversibility %q must be easy, moderate or hard", c.Reversibility)
	}
	for i, a := range c.Alternatives {
		if strings.TrimSpace(a.Name) == "" {
			return Contract{}, fmt.Errorf("decision: alternatives[%d] needs a name", i)
		}
	}
	for _, h := range c.Commits {
		if !commitRe.MatchString(strings.TrimSpace(h)) {
			return Contract{}, fmt.Errorf("decision: commit %q must be 7-40 hex chars", h)
		}
	}
	for _, r := range c.Related {
		if !relRe.MatchString(strings.TrimSpace(r)) {
			return Contract{}, fmt.Errorf("decision: related %q must look like dec:<id>", r)
		}
	}
	return c, nil
}

// MintID mints a unique decision id: dec_ + 8 hex chars.
func MintID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("decision: cannot mint id: %w", err)
	}
	return "dec_" + hex.EncodeToString(b[:]), nil
}

// Dir returns the decision store under a project root.
func Dir(root string) string {
	return filepath.Join(root, ".regulus", "decisions")
}

// Propose validates a contract and stores it as a PROPOSED decision.
// Writes are exclusive: a reused id is refused, never overwritten.
func Propose(root string, c Contract) (Decision, error) {
	id := strings.TrimSpace(c.ID)
	if id == "" {
		var err error
		if id, err = MintID(); err != nil {
			return Decision{}, err
		}
	}
	c.ID = id
	d := Decision{
		Contract: c,
		Schema:   SchemaVersion,
		State:    StateProposed,
		History: []HistoryEvent{{
			At: time.Now().UTC().Format(time.RFC3339), Kind: "proposed",
			From: "", To: StateProposed, By: "proposer", Note: "decision proposed",
		}},
	}
	if err := os.MkdirAll(Dir(root), 0o755); err != nil {
		return Decision{}, err
	}
	path := filepath.Join(Dir(root), id+".json")
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return Decision{}, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return Decision{}, fmt.Errorf("decision: %s already exists", id)
		}
		return Decision{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return Decision{}, err
	}
	return d, f.Close()
}

// Load reads one stored decision.
func Load(root, id string) (Decision, error) {
	data, err := os.ReadFile(filepath.Join(Dir(root), id+".json"))
	if err != nil {
		return Decision{}, fmt.Errorf("decision: %v", err)
	}
	var d Decision
	if err := json.Unmarshal(data, &d); err != nil {
		return Decision{}, fmt.Errorf("decision: malformed %s: %v", id, err)
	}
	return d, nil
}

// Save writes a decision back (states advance only through ApplyTransition).
func (d Decision) Save(root string) error {
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(Dir(root), d.ID+".json"), append(raw, '\n'), 0o644)
}

// allowed maps each state to its legal successors.
var allowed = map[string][]string{
	StateProposed:     {StateUnderReview, StateRejected, StateInconclusive},
	StateUnderReview:  {StateAccepted, StateRejected, StateInconclusive},
	StateAccepted:     {StateSuperseded},
	StateRejected:     {},
	StateInconclusive: {},
	StateSuperseded:   {},
}

// ApplyTransition validates an edge and appends it to the history.
func (d *Decision) ApplyTransition(to, by, note, kind string) error {
	next, ok := allowed[d.State]
	if !ok {
		return fmt.Errorf("decision: unknown state %q", d.State)
	}
	for _, n := range next {
		if n == to {
			d.History = append(d.History, HistoryEvent{
				At: time.Now().UTC().Format(time.RFC3339), Kind: kind,
				From: d.State, To: to, By: by, Note: note,
			})
			d.State = to
			return nil
		}
	}
	return fmt.Errorf("decision: transition %s -> %s is not allowed", d.State, to)
}

// CheckIntegrity revalidates a stored decision: state/history agreement,
// chain continuity, and schema. Hand edits without transitions are flagged.
func CheckIntegrity(d Decision) []string {
	var warns []string
	if d.Schema != SchemaVersion {
		warns = append(warns, fmt.Sprintf("integrity: unknown schema %d", d.Schema))
	}
	if len(d.History) == 0 {
		return append(warns, "integrity: empty history")
	}
	prev := ""
	for i, h := range d.History {
		if i == 0 {
			if h.Kind != "proposed" || h.To != StateProposed {
				warns = append(warns, "integrity: history must open with proposed")
			}
		} else if h.From != prev {
			warns = append(warns, fmt.Sprintf("integrity: broken chain at event %d (%s -> %s)", i, prev, h.From))
		}
		prev = h.To
	}
	if d.State != prev {
		warns = append(warns, fmt.Sprintf("integrity: state/history mismatch (state %s, history ends %s)", d.State, prev))
	}
	return warns
}
