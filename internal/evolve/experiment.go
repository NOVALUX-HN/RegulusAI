// Experiment records (Phase 6, S4) mirror the decisions-package pattern:
// a validated record stored as one JSON file under .regulus/evolution/
// with an appended transition history, exclusive ids and re-checkable
// integrity. States walk PROPOSED -> RUNNING -> EVALUATED ->
// ADOPTED | REJECTED; adoption itself additionally requires the T5
// human-approval gate, never this transition alone.
package evolve

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Experiment states.
const (
	StateProposed  = "PROPOSED"
	StateRunning   = "RUNNING"
	StateEvaluated = "EVALUATED"
	StateAdopted   = "ADOPTED"
	StateRejected  = "REJECTED"
)

// Evolution domains (S9; enforcement lives in T5).
const (
	DomainINV = "INV"
	DomainDOM = "DOM"
)

// ExperimentSchema versions the experiment document.
const ExperimentSchema = 1

// Budget bounds one experiment (S4).
type Budget struct {
	MaxRuns int `json:"max_runs"`
}

// ExperimentSpec is the proposal input: everything S4 names except the
// outcomes, which are recorded by evaluation.
type ExperimentSpec struct {
	ID         string
	Hypothesis Hypothesis
	Baseline   string
	Candidate  string
	Dataset    string
	Model      string
	Runtime    string
	Domain     string
	Parameters map[string]string
	Budget     Budget
}

// ExpHistoryEvent is one appended transition or outcome record.
type ExpHistoryEvent struct {
	At   string `json:"at"`
	Kind string `json:"kind"`
	From string `json:"from"`
	To   string `json:"to"`
	By   string `json:"by"`
	Note string `json:"note"`
}

// Experiment is a stored proposal with its state, outcomes and history.
type Experiment struct {
	ID                 string            `json:"id"`
	Schema             int               `json:"schema"`
	State              string            `json:"state"`
	Hypothesis         Hypothesis        `json:"hypothesis"`
	Baseline           string            `json:"baseline"`
	Candidate          string            `json:"candidate"`
	Dataset            string            `json:"dataset"`
	Model              string            `json:"model"`
	Runtime            string            `json:"runtime"`
	Domain             string            `json:"domain"`
	Parameters         map[string]string `json:"parameters"`
	Budget             Budget            `json:"budget"`
	Results            string            `json:"results,omitempty"`
	RegressionAnalysis string            `json:"regression_analysis,omitempty"`
	Decision           string            `json:"decision,omitempty"`
	History            []ExpHistoryEvent `json:"history"`
}

// EvolveDir returns the experiment store under a project root.
func EvolveDir(root string) string {
	return filepath.Join(root, ".regulus", "evolution")
}

// mintExpID mints a unique experiment id: exp_ + 8 hex chars.
func mintExpID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("evolve: cannot mint experiment id: %w", err)
	}
	return "exp_" + hex.EncodeToString(b[:]), nil
}

// ProposeExperiment validates a spec and stores it as a PROPOSED
// experiment. Writes are exclusive: a reused id is refused.
func ProposeExperiment(root string, spec ExperimentSpec) (Experiment, error) {
	if err := spec.Hypothesis.Validate(); err != nil {
		return Experiment{}, err
	}
	for _, field := range []struct {
		name, value string
	}{
		{"baseline", spec.Baseline}, {"candidate", spec.Candidate}, {"dataset", spec.Dataset},
	} {
		if strings.TrimSpace(field.value) == "" {
			return Experiment{}, errors.New("evolve: experiment needs a " + field.name)
		}
	}
	switch spec.Domain {
	case DomainINV, DomainDOM:
	default:
		return Experiment{}, fmt.Errorf("evolve: domain %q must be INV or DOM", spec.Domain)
	}
	if spec.Budget.MaxRuns < 1 {
		return Experiment{}, errors.New("evolve: experiment budget needs MaxRuns >= 1")
	}
	id := strings.TrimSpace(spec.ID)
	if id == "" {
		var err error
		if id, err = mintExpID(); err != nil {
			return Experiment{}, err
		}
	}
	exp := Experiment{
		ID: id, Schema: ExperimentSchema, State: StateProposed,
		Hypothesis: spec.Hypothesis, Baseline: spec.Baseline, Candidate: spec.Candidate,
		Dataset: spec.Dataset, Model: spec.Model, Runtime: spec.Runtime,
		Domain: spec.Domain, Parameters: spec.Parameters, Budget: spec.Budget,
		History: []ExpHistoryEvent{{
			At: time.Now().UTC().Format(time.RFC3339), Kind: "proposed",
			From: "", To: StateProposed, By: "proposer", Note: "experiment proposed",
		}},
	}
	if err := os.MkdirAll(EvolveDir(root), 0o755); err != nil {
		return Experiment{}, err
	}
	path := filepath.Join(EvolveDir(root), id+".json")
	raw, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return Experiment{}, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return Experiment{}, fmt.Errorf("evolve: experiment %s already exists", id)
		}
		return Experiment{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return Experiment{}, err
	}
	return exp, f.Close()
}

// LoadExperiment reads one stored experiment.
func LoadExperiment(root, id string) (Experiment, error) {
	data, err := os.ReadFile(filepath.Join(EvolveDir(root), id+".json"))
	if err != nil {
		return Experiment{}, fmt.Errorf("evolve: %v", err)
	}
	var exp Experiment
	if err := json.Unmarshal(data, &exp); err != nil {
		return Experiment{}, fmt.Errorf("evolve: malformed experiment %s: %v", id, err)
	}
	return exp, nil
}

// save writes the experiment back.
func (e Experiment) save(root string) error {
	raw, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(EvolveDir(root), e.ID+".json"), append(raw, '\n'), 0o644)
}

// expAllowed maps each state to its legal successors.
var expAllowed = map[string][]string{
	StateProposed:  {StateRunning},
	StateRunning:   {StateEvaluated},
	StateEvaluated: {StateAdopted, StateRejected},
	StateAdopted:   {},
	StateRejected:  {},
}

// SetOutcome records the evaluation outcomes on a RUNNING experiment.
// Outcomes never move the state by themselves; Advance does that.
func (e *Experiment) SetOutcome(results, regression, decision string) error {
	if e.State != StateRunning {
		return fmt.Errorf("evolve: outcomes are recorded while RUNNING, not %s", e.State)
	}
	e.Results = results
	e.RegressionAnalysis = regression
	e.Decision = decision
	return nil
}

// Advance validates one state edge, appends it to the history and stores
// the experiment. Adoption additionally requires the T5 human-approval
// gate: this transition alone never adopts.
func (e *Experiment) Advance(root, to string) error {
	next, ok := expAllowed[e.State]
	if !ok {
		return fmt.Errorf("evolve: unknown experiment state %q", e.State)
	}
	for _, n := range next {
		if n == to {
			e.History = append(e.History, ExpHistoryEvent{
				At: time.Now().UTC().Format(time.RFC3339), Kind: "transition",
				From: e.State, To: to, By: "evolution", Note: "stage advanced",
			})
			e.State = to
			return e.save(root)
		}
	}
	return fmt.Errorf("evolve: experiment transition %s -> %s is not allowed", e.State, to)
}

// CheckExperimentIntegrity revalidates a stored experiment: state/history
// agreement, chain continuity and schema.
func CheckExperimentIntegrity(e Experiment) []string {
	var warns []string
	if e.Schema != ExperimentSchema {
		warns = append(warns, fmt.Sprintf("integrity: unknown schema %d", e.Schema))
	}
	if len(e.History) == 0 {
		return append(warns, "integrity: empty history")
	}
	prev := ""
	for i, h := range e.History {
		if i == 0 {
			if h.Kind != "proposed" || h.To != StateProposed {
				warns = append(warns, "integrity: history must open with proposed")
			}
		} else if h.From != prev {
			warns = append(warns, fmt.Sprintf("integrity: broken chain at event %d (%s -> %s)", i, prev, h.From))
		}
		prev = h.To
	}
	if e.State != prev {
		warns = append(warns, fmt.Sprintf("integrity: state/history mismatch (state %s, history ends %s)", e.State, prev))
	}
	return warns
}
