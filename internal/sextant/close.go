// Completion integrity (Phase 2, S6): an agent never closes a task by
// self-report. CheckClose verifies the acceptance contract, the task's
// receipts (hash-verified, all PASS) and the level requirements; ApplyClose
// records the closure in the chart. The core is the authority — the table
// status alone closes nothing.
package sextant

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/manifest"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/spec"
)

// CloseResult is the outcome of one completion check.
type CloseResult struct {
	Closed   bool
	TaskID   string
	RunID    string
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
	task := findCloseTask(tasks, taskID)
	if task == nil {
		res.Unmet = append(res.Unmet, fmt.Sprintf("unknown task %q: no Tasks row declares it", taskID))
		return res
	}
	if class == ClassTrivial {
		res.Closed = true
		res.Verdict = "PASS"
		return res
	}
	ct := checkContract(doc, &res)
	checkDeps(doc, task, &res)
	checkEvidence(root, chartBase, taskID, &res)
	checkVerifier(root, chartBase, class, ct, &res)
	finishClose(&res)
	return res
}

func findCloseTask(tasks []Task, taskID string) *Task {
	for i, t := range tasks {
		if t.ID == taskID {
			return &tasks[i]
		}
	}
	return nil
}

func finishClose(res *CloseResult) {
	res.Closed = len(res.Unmet) == 0
	if res.Closed {
		res.Verdict = "PASS"
	}
}

func checkContract(doc string, res *CloseResult) Contract {
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
	return ct
}

func checkDeps(doc string, task *Task, res *CloseResult) {
	closed := ClosedTasks(doc)
	for _, d := range task.Depends {
		if !closed[d] {
			res.Unmet = append(res.Unmet, fmt.Sprintf("dependency %s is not closed", d))
		}
	}
}

func checkEvidence(root, chartBase, taskID string, res *CloseResult) {
	for _, cand := range []string{chartBase + "/" + taskID, taskID} {
		m, _, paths, unmet, found := latestManifestEvidence(root, cand)
		if !found {
			continue
		}
		res.RunID = m.RunID
		res.Receipts = append([]string(nil), paths...)
		res.Unmet = append(res.Unmet, unmet...)
		return
	}
	res.Unmet = append(res.Unmet, fmt.Sprintf("no receipts for task: run the gauntlet as %s/%s (no manifest)", chartBase, taskID))
}

func checkVerifier(root, chartBase string, class Class, ct Contract, res *CloseResult) {
	if class != ClassCritical || !ct.Present {
		return
	}
	verifier := strings.TrimSpace(ct.Verifier)
	if verifier == "" {
		res.Unmet = append(res.Unmet, "critical work requires a Verifier task in the contract")
		return
	}
	// The verifier gates by evidence, not by its own closure: its
	// latest manifest must verify. Its closure follows afterwards.
	for _, cand := range []string{chartBase + "/" + verifier, verifier} {
		_, _, _, unmet, found := latestManifestEvidence(root, cand)
		if !found {
			continue
		}
		for _, u := range unmet {
			res.Unmet = append(res.Unmet, "verifier "+u)
		}
		return
	}
	res.Unmet = append(res.Unmet, fmt.Sprintf("verifier task %s has no receipts", verifier))
}

// latestManifestEvidence selects the latest manifest of one evidence task:
// max CreatedAt, tie-break lexicographic FileName(RunID). It fails closed
// on any malformed candidate (no silent fallback), on Schema!=1, on empty
// Artifacts or any Skipped (see gates/runtask_split.go:112-156: Artifacts
// pin receipt bytes, Skipped records omitted gates — an empty or partial
// run never proves the contract), and on any evidence.Verify failure whose
// file (always via filepath.Base) belongs to the closed set {manifest plus
// basenames of its Artifacts} in both directions (receipt→manifest link
// and manifest→disk SHA256).
func latestManifestEvidence(root, evidTask string) (manifest.Manifest, string, []string, []string, bool) {
	dir, err := evidence.TaskDirSafe(root, evidTask)
	if err != nil {
		return manifest.Manifest{}, "", nil, []string{err.Error()}, true
	}
	matches, err := filepath.Glob(filepath.Join(dir, "manifest-*.json"))
	if err != nil {
		return manifest.Manifest{}, "", nil, []string{err.Error()}, true
	}
	if len(matches) == 0 {
		return manifest.Manifest{}, "", nil, nil, false
	}
	sort.Strings(matches)
	mlist := make([]manifest.Manifest, 0, len(matches))
	for _, p := range matches {
		m, err := manifest.ReadFile(p)
		if err != nil {
			return manifest.Manifest{}, p, nil, []string{fmt.Sprintf("evidence verify: %s: manifest_malformed: %s", filepath.Base(p), err.Error())}, true
		}
		mlist = append(mlist, m)
	}
	idx := 0
	for i := 1; i < len(mlist); i++ {
		if mlist[i].CreatedAt.After(mlist[idx].CreatedAt) || (mlist[i].CreatedAt.Equal(mlist[idx].CreatedAt) && manifest.FileName(mlist[i].RunID) > manifest.FileName(mlist[idx].RunID)) {
			idx = i
		}
	}
	m := mlist[idx]
	mpath := matches[idx]
	base := filepath.Base(mpath)
	if m.Schema != manifest.SchemaVersion {
		return m, mpath, nil, []string{fmt.Sprintf("evidence verify: %s: unsupported_schema", base)}, true
	}
	if len(m.Artifacts) == 0 {
		return m, mpath, nil, []string{fmt.Sprintf("evidence verify: %s: empty manifest: no artifacts", base)}, true
	}
	if len(m.Skipped) > 0 {
		return m, mpath, nil, []string{fmt.Sprintf("evidence verify: %s: skipped gates: %s", base, strings.Join(m.Skipped, ", "))}, true
	}
	var paths []string
	var unmet []string
	var failed []string
	for _, a := range m.Artifacts {
		fpath := filepath.Join(dir, filepath.Base(a.File))
		paths = append(paths, fpath)
		rec, err := receipts.ReadFile(fpath)
		if err != nil {
			unmet = append(unmet, fmt.Sprintf("evidence verify: %s: artifact_missing: %s", filepath.Base(a.File), err.Error()))
			continue
		}
		if rec.Verdict != "pass" {
			failed = append(failed, fmt.Sprintf("%s (exit %d)", rec.Gate, rec.ExitCode))
		}
	}
	if len(failed) != 0 {
		unmet = append(unmet, fmt.Sprintf("failing gates: %s", strings.Join(failed, ", ")))
	}
	rep, err := evidence.Verify(root, evidTask, false)
	if err != nil {
		sort.Strings(paths)
		return m, mpath, paths, append(unmet, err.Error()), true
	}
	closed := map[string]bool{base: true}
	for _, a := range m.Artifacts {
		closed[filepath.Base(a.File)] = true
	}
	for _, f := range rep.Failed {
		if closed[filepath.Base(f.File)] {
			unmet = append(unmet, fmt.Sprintf("evidence verify: %s: %s", filepath.Base(f.File), f.Reason))
		}
	}
	sort.Strings(paths)
	return m, mpath, paths, unmet, true
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
	if res.RunID != "" {
		record = fmt.Sprintf("- CLOSED %s %s spec OK + %d receipt(s) PASS (%s).", taskID, res.Class, len(res.Receipts), res.RunID)
	}
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
