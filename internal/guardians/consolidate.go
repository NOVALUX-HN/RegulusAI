// Verdict consolidation (Phase 3, S5-S7): the core reads every receipt and
// every recorded evaluation and produces one verdict. Receipts decide;
// evaluations are shown with their dispute flag and counted, never applied.
// Rates are measured proxies (disagreement, inconclusive, completeness):
// true false-positive/false-negative rates need an oracle, which the
// harness does not invent.
package guardians

import (
	"fmt"
	"os"
	"path/filepath"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/verdict"
)

// GateVerdict is the consolidated view of one executed gate.
type GateVerdict struct {
	Gate      string
	Layer     int
	Status    string
	Reason    string
	ReceiptID string
}

// EvalView is one recorded evaluation with its dispute flag against the
// consolidated verdict.
type EvalView struct {
	Guardian string
	Status   string
	Note     string
	Evidence []string
	Disputed bool
}

// Consolidation is the core's verdict over one task's evidence.
type Consolidation struct {
	Task                 string
	Verdict              string
	Reason               string
	Gates                []GateVerdict
	Evaluations          []EvalView
	Agree                int
	Dispute              int
	GatesInconclusive    int
	EvalsInconclusive    int
	Legacy               int
	EvidenceTotal        int
	EvidenceLinked       int
	EvidenceCompleteness float64
	DisagreementRate     float64
}

// Consolidate reads a task's receipts and recorded evaluations and derives
// the core verdict: FAIL beats BLOCKED beats INCONCLUSIVE beats PASS.
// Legacy receipts (no canonical status) derive from pass/fail. A task with
// no receipts at all cannot consolidate.
func Consolidate(root, task string) (Consolidation, error) {
	entries, err := evidence.List(root, task)
	if err != nil {
		return Consolidation{}, err
	}
	if len(entries) == 0 {
		return Consolidation{}, fmt.Errorf("gate verdict: no evidence for task %s", task)
	}
	con := Consolidation{Task: task}
	for _, e := range entries {
		status, reason := canonicalStatus(e)
		if e.Receipt.Schema == 0 {
			con.Legacy++
		}
		con.EvidenceTotal++
		if linked(root, task, e.Receipt.Manifest) {
			con.EvidenceLinked++
		}
		if status == string(verdict.StatusInconclusive) {
			con.GatesInconclusive++
		}
		con.Gates = append(con.Gates, GateVerdict{
			Gate: e.Receipt.Gate, Layer: e.Receipt.Layer, Status: status, Reason: reason, ReceiptID: e.Receipt.ID,
		})
	}
	con.Verdict, con.Reason = overall(con.Gates)
	evals, err := LoadEvaluations(root, task)
	if err != nil {
		return Consolidation{}, err
	}
	for _, ev := range evals {
		disputed := ev.Status != con.Verdict
		if disputed {
			con.Dispute++
		} else {
			con.Agree++
		}
		if ev.Status == string(verdict.StatusInconclusive) {
			con.EvalsInconclusive++
		}
		con.Evaluations = append(con.Evaluations, EvalView{
			Guardian: ev.Guardian, Status: ev.Status, Note: ev.Note,
			Evidence: append([]string(nil), ev.Evidence...), Disputed: disputed,
		})
	}
	if n := len(evals); n > 0 {
		con.DisagreementRate = float64(con.Dispute) / float64(n)
	}
	if con.EvidenceTotal > 0 {
		con.EvidenceCompleteness = float64(con.EvidenceLinked) / float64(con.EvidenceTotal)
	}
	return con, nil
}

func canonicalStatus(e evidence.Entry) (string, string) {
	if e.Receipt.Status != "" {
		if _, err := verdict.ParseStatus(e.Receipt.Status); err == nil {
			reason := e.Receipt.Reason
			if reason == "" {
				reason = string(verdict.ReasonOK)
			}
			return e.Receipt.Status, reason
		}
	}
	if e.Receipt.Verdict == "pass" {
		return string(verdict.StatusPass), string(verdict.ReasonOK)
	}
	return string(verdict.StatusFail), string(verdict.ReasonDeterministicFailure)
}

func overall(gates []GateVerdict) (string, string) {
	st, rs := string(verdict.StatusPass), string(verdict.ReasonOK)
	for _, g := range gates {
		switch g.Status {
		case string(verdict.StatusFail):
			return string(verdict.StatusFail), string(verdict.ReasonDeterministicFailure)
		case string(verdict.StatusBlocked):
			st, rs = string(verdict.StatusBlocked), g.Reason
		case string(verdict.StatusInconclusive):
			if st == string(verdict.StatusPass) {
				st, rs = string(verdict.StatusInconclusive), g.Reason
			}
		}
	}
	return st, rs
}

// linked reports whether a receipt's manifest claim resolves to a file in
// the task dir. Substance is evidence.Verify's job; consolidation counts
// the claim.
func linked(root, task, manifestFile string) bool {
	if manifestFile == "" {
		return false
	}
	fi, err := os.Stat(filepath.Join(evidence.TaskDir(root, task), filepath.Base(manifestFile)))
	return err == nil && !fi.IsDir()
}
