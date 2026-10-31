// Package guardians is the machine side of guardian governance (Phase 3):
// the six read-only guardians with bounded responsibilities, and the only
// path for their assessments into the evidence store. Guardians never write
// code, receipts, charts or policy — Evaluate only reads and appends to an
// evaluations log. The core consolidates via the verdict package; an
// evaluation is recorded, never authoritative.
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

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/verdict"
)

// Guardian is one guardian's governance contract: identity, bounded
// responsibility and scope, read-only permissions, accepted inputs,
// required evidence, the output schema it speaks, and the acceptance,
// rejection and abstention criteria for its own assessments.
type Guardian struct {
	ID               string
	Responsibility   string
	Scope            string
	Permissions      string
	Inputs           string
	RequiredEvidence string
	OutputSchema     string
	Acceptance       string
	Rejection        string
	Abstention       string
}

// Registry returns the six guardians, mirroring docs/agents.md. Crew and
// primary agents are deliberately absent: they implement, they do not judge.
func Registry() []Guardian {
	ro := "read-only: deny all edits, allow-listed verification commands only"
	out := "verdict PASS|FAIL|BLOCKED|INCONCLUSIVE + cited receipt ids + note"
	return []Guardian{
		{
			ID: "regulus-gate-verifier", Responsibility: "verify evidence receipts against acceptance criteria",
			Scope: "one task: spec contract, full gauntlet, evidence audit", Permissions: ro,
			Inputs: "chart acceptance scenarios, gate receipts", RequiredEvidence: "one receipt per acceptance scenario",
			OutputSchema: out, Acceptance: "every scenario PROVEN with existing PASS receipts",
			Rejection: "any NOT COVERED scenario or failing gate", Abstention: "no receipts for the task",
		},
		{
			ID: "regulus-spec-keeper", Responsibility: "judge acceptance criteria quality, never the implementation",
			Scope: "chart acceptance sections before implementation", Permissions: ro,
			Inputs: "chart draft", RequiredEvidence: "the chart text itself",
			OutputSchema: out, Acceptance: "criteria externally observable, not mirroring the implementation",
			Rejection: "criteria that restate code instead of behavior", Abstention: "no chart provided",
		},
		{
			ID: "regulus-mutation-auditor", Responsibility: "prove tests catch wrong behavior via mutation testing",
			Scope: "the gauntlet's mutation gate and its survivors", Permissions: ro,
			Inputs: "mutation receipts, surviving mutants", RequiredEvidence: "mutation gate receipt",
			OutputSchema: out, Acceptance: "efficacy floor met, survivors explained",
			Rejection: "unkilled mutants that change specified behavior", Abstention: "mutation gate did not run",
		},
		{
			ID: "regulus-crap-analyst", Responsibility: "locate risky functions via CRAP and name the cure",
			Scope: "CRAP gate violations", Permissions: ro,
			Inputs: "CRAP report, coverage profile", RequiredEvidence: "CRAP gate receipt",
			OutputSchema: out, Acceptance: "every violation cured (COVER or SPLIT) or waived with reason",
			Rejection: "uncovered complex code with no cure", Abstention: "CRAP gate did not run",
		},
		{
			ID: "regulus-evidence-auditor", Responsibility: "audit claims against receipts: every done cites rec_ ids that exist and pass",
			Scope: "completion claims and their cited evidence", Permissions: ro,
			Inputs: "claim text, cited receipt ids", RequiredEvidence: "the cited receipts, hash-verified",
			OutputSchema: out, Acceptance: "every claim backed by an existing PASS receipt",
			Rejection: "cited receipt missing, failing, or hash-broken", Abstention: "claim cites no receipts",
		},
		{
			ID: "regulus-reviewer", Responsibility: "senior read of the code the change touches, not just the diff",
			Scope: "the change's touched files and their callers", Permissions: ro,
			Inputs: "touched paths, diff stat", RequiredEvidence: "trace files entry or diff listing",
			OutputSchema: out, Acceptance: "no unneeded code, Necessity Ladder honored",
			Rejection: "reuse ignored, validation cut, or scope sprawl", Abstention: "no touched paths recorded",
		},
	}
}

// Crew roles implement bounded units; they are recognized but forbidden
// from evaluating — the implementer cannot judge its own work.
var crewRoles = map[string]bool{
	"regulus": true, "regulus-scout": true, "regulus-worker": true,
	"regulus-verifier": true, "regulus-proposer": true,
}

func normalize(id string) string {
	return strings.TrimPrefix(strings.TrimSpace(id), "@")
}

// ByID resolves a guardian id (with or without @).
func ByID(id string) (Guardian, bool) {
	n := normalize(id)
	for _, g := range Registry() {
		if g.ID == n {
			return g, true
		}
	}
	return Guardian{}, false
}

// IsGuardian reports registry membership.
func IsGuardian(id string) bool {
	_, ok := ByID(id)
	return ok
}

// IsCrew reports implementer roles, which evaluations refuse.
func IsCrew(id string) bool {
	return crewRoles[normalize(id)]
}

// Evaluation is one recorded guardian assessment. It carries an opinion,
// never authority: consolidation belongs to the core verdict.
type Evaluation struct {
	Guardian string   `json:"guardian"`
	Status   string   `json:"status"`
	Note     string   `json:"note"`
	Evidence []string `json:"evidence"`
	At       string   `json:"at"`
}

// EvaluationsFile is the append-only log of assessments in a task dir.
func EvaluationsFile(root, task string) string {
	return filepath.Join(evidence.TaskDir(root, task), "evaluations.jsonl")
}

// Evaluate validates a guardian assessment and appends it to the task's
// evaluations log. Validation rejects: unknown guardians, crew evaluators,
// unparseable verdicts, empty notes, and evidence refs that resolve to
// nothing in the task dir. It never modifies receipts, charts, or policy.
func Evaluate(root, task, guardianID, status, note string, evidenceRefs []string) (Evaluation, error) {
	id := normalize(guardianID)
	if _, ok := ByID(id); !ok {
		if IsCrew(id) {
			return Evaluation{}, fmt.Errorf("gate evaluate: %q is not a guardian (crew implements, guardians judge)", id)
		}
		return Evaluation{}, fmt.Errorf("gate evaluate: unknown guardian %q", id)
	}
	st, err := verdict.ParseStatus(strings.ToUpper(strings.TrimSpace(status)))
	if err != nil {
		return Evaluation{}, fmt.Errorf("gate evaluate: invalid verdict %q (want PASS, FAIL, BLOCKED or INCONCLUSIVE)", status)
	}
	if strings.TrimSpace(note) == "" {
		return Evaluation{}, fmt.Errorf("gate evaluate: note is required (what was judged and why)")
	}
	entries, err := evidence.List(root, task)
	if err != nil {
		return Evaluation{}, fmt.Errorf("gate evaluate: %v", err)
	}
	byID := map[string]bool{}
	byFile := map[string]bool{}
	for _, e := range entries {
		byID[e.Receipt.ID] = true
		byFile[filepath.Base(e.Path)] = true
	}
	for _, ref := range evidenceRefs {
		r := strings.TrimSpace(ref)
		if r == "" {
			continue
		}
		if !byID[r] && !byFile[filepath.Base(r)] {
			return Evaluation{}, fmt.Errorf("gate evaluate: missing evidence %q (no such receipt in task %s)", r, task)
		}
	}
	ev := Evaluation{
		Guardian: id, Status: string(st), Note: strings.TrimSpace(note),
		Evidence: append([]string(nil), evidenceRefs...),
		At:       time.Now().UTC().Format(time.RFC3339),
	}
	path := EvaluationsFile(root, task)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Evaluation{}, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Evaluation{}, err
	}
	defer f.Close()
	raw, err := json.Marshal(ev)
	if err != nil {
		return Evaluation{}, err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return Evaluation{}, err
	}
	return ev, nil
}

// LoadEvaluations reads a task's recorded assessments, oldest first.
// A missing log is empty, not an error.
func LoadEvaluations(root, task string) ([]Evaluation, error) {
	data, err := os.ReadFile(EvaluationsFile(root, task))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Evaluation
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev Evaluation
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("evaluations log: malformed entry: %v", err)
		}
		out = append(out, ev)
	}
	return out, sc.Err()
}
