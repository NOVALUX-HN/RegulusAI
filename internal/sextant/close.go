// Completion integrity (Phase 2, S6): an agent never closes a task by
// self-report. CheckClose verifies the acceptance contract, the task's
// receipts (hash-verified, all PASS) and the level requirements; ApplyClose
// records the closure in the chart. The core is the authority — the table
// status alone closes nothing.
package sextant

import (
	"fmt"
	"strings"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/spec"
)

// CloseResult is the outcome of one completion check.
type CloseResult struct {
	Closed   bool
	TaskID   string
	Class    Class
	Verdict  string
	Receipts []string
	Unmet    []string
}

// CheckClose verifies whether taskID in the chart doc may close.
// root locates the evidence store; chartBase names the chart (evidence
// tasks resolve as <chart>/<id>, falling back to <id>).
func CheckClose(doc, root, chartBase, taskID string) CloseResult {
	res := CloseResult{TaskID: taskID}
	class, ok := ParseClassLine(doc)
	if !ok {
		class = ClassStandard
	}
	res.Class = class
	tasks, perrs := ParseTasks(doc)
	if len(perrs) != 0 {
		res.Unmet = append(res.Unmet, perrs...)
	}
	var task *Task
	for i, t := range tasks {
		if t.ID == taskID {
			task = &tasks[i]
			break
		}
	}
	if task == nil {
		res.Unmet = append(res.Unmet, fmt.Sprintf("unknown task %q: no Tasks row declares it", taskID))
		return res
	}
	if class == ClassTrivial {
		res.Closed = true
		res.Verdict = "PASS"
		return res
	}
	ct, cviol := ParseContract(doc)
	if ct.Present {
		for _, v := range cviol {
			res.Unmet = append(res.Unmet, "contract: "+v)
		}
	} else {
		res.Unmet = append(res.Unmet, "no structured ## Contract: STANDARD and above require one")
	}
	scenarios, sviol := spec.Check(doc)
	if len(sviol) != 0 {
		for _, v := range sviol {
			res.Unmet = append(res.Unmet, "spec: "+v)
		}
	} else if scenarios == 0 {
		res.Unmet = append(res.Unmet, "spec: no acceptance scenarios declared")
	}
	closed := ClosedTasks(doc)
	for _, d := range task.Depends {
		if !closed[d] {
			res.Unmet = append(res.Unmet, fmt.Sprintf("dependency %s is not closed", d))
		}
	}
	evidTask, receipts, rep, found := taskEvidence(root, chartBase, taskID)
	if !found {
		res.Unmet = append(res.Unmet, fmt.Sprintf("no receipts for task: run the gauntlet as %s/%s", chartBase, taskID))
	} else {
		failed := []string{}
		for _, e := range receipts {
			if e.Receipt.Verdict != "pass" {
				failed = append(failed, fmt.Sprintf("%s (exit %d)", e.Receipt.Gate, e.Receipt.ExitCode))
			}
		}
		if len(failed) != 0 {
			res.Unmet = append(res.Unmet, fmt.Sprintf("failing gates: %s", strings.Join(failed, ", ")))
		}
		for _, f := range rep.Failed {
			res.Unmet = append(res.Unmet, fmt.Sprintf("evidence verify: %s: %s", f.File, f.Reason))
		}
		for _, e := range receipts {
			res.Receipts = append(res.Receipts, e.Path)
		}
		_ = evidTask
	}
	if class == ClassCritical && ct.Present {
		verifier := strings.TrimSpace(ct.Verifier)
		if verifier == "" {
			res.Unmet = append(res.Unmet, "critical work requires a Verifier task in the contract")
		} else {
			// The verifier gates by evidence, not by its own closure: its
			// receipts must exist and pass. Its closure follows the
			// normal rules afterwards (no close-order deadlock).
			_, vrecs, vrep, vfound := taskEvidence(root, chartBase, verifier)
			if !vfound {
				res.Unmet = append(res.Unmet, fmt.Sprintf("verifier task %s has no receipts", verifier))
			} else {
				for _, e := range vrecs {
					if e.Receipt.Verdict != "pass" {
						res.Unmet = append(res.Unmet, fmt.Sprintf("verifier gate %s did not pass", e.Receipt.Gate))
					}
				}
				if len(vrep.Failed) != 0 {
					res.Unmet = append(res.Unmet, fmt.Sprintf("verifier evidence does not verify (%d problem(s))", len(vrep.Failed)))
				}
			}
		}
	}
	res.Closed = len(res.Unmet) == 0
	if res.Closed {
		res.Verdict = "PASS"
	}
	return res
}

// taskEvidence resolves a chart task to its receipts: <chart>/<id> first,
// then the bare id (probe and legacy layouts). The receipts are
// hash-verified through evidence.Verify.
func taskEvidence(root, chartBase, taskID string) (string, []evidence.Entry, evidence.Report, bool) {
	for _, candidate := range []string{chartBase + "/" + taskID, taskID} {
		entries, err := evidence.List(root, candidate)
		if err != nil || len(entries) == 0 {
			continue
		}
		rep, err := evidence.Verify(root, candidate, false)
		if err != nil {
			continue
		}
		return candidate, entries, rep, true
	}
	return "", nil, evidence.Report{}, false
}

// ApplyClose records a verified closure: flips the task row Status to done
// and appends the CLOSED record to ## Log (creating it when absent).
// Call only with res.Closed; otherwise the doc is returned unchanged.
func ApplyClose(doc, taskID string, res CloseResult) string {
	if !res.Closed {
		return doc
	}
	updated := flipStatusCell(doc, taskID)
	record := fmt.Sprintf("- CLOSED %s %s spec OK + %d receipt(s) PASS.", taskID, res.Class, len(res.Receipts))
	updated = appendLogLine(updated, record)
	return updated
}

func flipStatusCell(doc, taskID string) string {
	m := tasksHeadingRe.FindStringIndex(doc)
	if m == nil {
		return doc
	}
	rest := doc[m[0]:]
	lines := strings.Split(rest, "\n")
	hi := -1
	for i, line := range lines {
		if isPipeRow(line) && i+1 < len(lines) && isDashesRow(lines[i+1]) {
			hi = i
			break
		}
	}
	if hi < 0 {
		return doc
	}
	headers := splitPipeRow(lines[hi])
	statusCol := -1
	idCol := -1
	for i, h := range headers {
		switch normalizeHeader(h) {
		case "status":
			statusCol = i
		case "id":
			idCol = i
		}
	}
	if idCol < 0 {
		return doc
	}
	for i := hi + 2; i < len(lines); i++ {
		if !isPipeRow(lines[i]) {
			break
		}
		cells := splitPipeRow(lines[i])
		if idCol >= len(cells) || strings.TrimSpace(cells[idCol]) != taskID {
			continue
		}
		if statusCol < 0 {
			// Legacy table without a Status column: append one.
			headers = append(headers, " Status ")
			for j := hi + 2; j < len(lines) && isPipeRow(lines[j]); j++ {
				c := splitPipeRow(lines[j])
				for len(c) <= len(headers)-1 {
					c = append(c, " ")
				}
				lines[j] = "| " + strings.Join(trimAll(c), " | ") + " |"
			}
			lines[hi] = "| " + strings.Join(trimAll(headers), " | ") + " |"
			statusCol = len(headers) - 1
			cells = splitPipeRow(lines[i])
		}
		for len(cells) <= statusCol {
			cells = append(cells, " ")
		}
		cells[statusCol] = " done "
		lines[i] = "| " + strings.Join(trimAll(cells), " | ") + " |"
	}
	return doc[:m[0]] + strings.Join(lines, "\n")
}

func trimAll(cells []string) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

func appendLogLine(doc, record string) string {
	m := logHeadingRe.FindStringIndex(doc)
	if m == nil {
		if !strings.HasSuffix(strings.TrimRight(doc, "\n"), "\n") {
			doc += "\n"
		}
		return strings.TrimRight(doc, "\n") + "\n\n## Log\n\n" + record + "\n"
	}
	rest := doc[m[0]:]
	lines := strings.Split(rest, "\n")
	// Insert before the next top-level section, or append at the end.
	end := len(lines)
	for i, line := range lines {
		if i > 0 && strings.HasPrefix(line, "## ") {
			end = i
			break
		}
	}
	lines = append(append(append([]string{}, lines[:end]...), record), lines[end:]...)
	return doc[:m[0]] + strings.Join(lines, "\n")
}
