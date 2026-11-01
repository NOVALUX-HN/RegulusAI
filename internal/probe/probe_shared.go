// Shared probe phases for Contrast and Compose (debt-paydown-2 D2).
//
// ContrastWithRunner and ComposeWithRunner shared the same evolution
// cap-check shape with different "probe contrast"/"probe compose"
// prefixes. This file hosts the shared check plus the phase helpers
// (validation, load, loop runs, budget-record) so both orchestrators
// stay under 20 lines with identical messages and receipts.
package probe

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/reflect"
)

// validateProbeTask checks the task id for probe ops (shared shape).
func validateProbeTask(task, op string) error {
	if strings.TrimSpace(task) == "" {
		return fmt.Errorf("%s: task must not be empty", op)
	}
	if !taskIDRe.MatchString(task) || strings.Contains(task, "..") {
		return fmt.Errorf("%s: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')", op, task)
	}
	return nil
}

// validateProbeGate checks the gate name for probe ops (shared shape).
func validateProbeGate(gate, op string) error {
	if strings.TrimSpace(gate) == "" {
		return fmt.Errorf("%s: gate must not be empty", op)
	}
	return nil
}

// validateContrastCmds checks the two contrast variant commands.
func validateContrastCmds(cmdA, cmdB, op string) error {
	if strings.TrimSpace(cmdA) == "" || strings.TrimSpace(cmdB) == "" {
		return fmt.Errorf("%s: both --a and --b commands must not be empty", op)
	}
	return nil
}

// validateComposeCmds checks the base + two compose variant commands.
func validateComposeCmds(base, cmdA, cmdB, op string) error {
	if strings.TrimSpace(base) == "" || strings.TrimSpace(cmdA) == "" || strings.TrimSpace(cmdB) == "" {
		return fmt.Errorf("%s: --base, --a and --b commands must not be empty", op)
	}
	return nil
}

// validateContrastInputs runs the full contrast validation phase.
func validateContrastInputs(task, gate, cmdA, cmdB string) error {
	const op = "probe contrast"
	if err := validateProbeTask(task, op); err != nil {
		return err
	}
	if err := validateProbeGate(gate, op); err != nil {
		return err
	}
	return validateContrastCmds(cmdA, cmdB, op)
}

// validateComposeInputs runs the full compose validation phase.
func validateComposeInputs(task, gate, base, cmdA, cmdB string) error {
	const op = "probe compose"
	if err := validateProbeTask(task, op); err != nil {
		return err
	}
	if err := validateProbeGate(gate, op); err != nil {
		return err
	}
	return validateComposeCmds(base, cmdA, cmdB, op)
}

// normalizeProbeEnv applies the shared defaults for probe orchestrators.
func normalizeProbeEnv(root string, now time.Time, r gates.CmdRunner) (string, time.Time, gates.CmdRunner) {
	if r == nil {
		r = gates.ShRunner{}
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return root, now, r
}

// checkDailyCap enforces the daily evolution cap (shared shape).
func checkDailyCap(root, task, op string, now time.Time) error {
	cap := reflect.EvolutionCap(root)
	today, err := budget.TodayEvolutionCount(root, now)
	if err != nil {
		return err
	}
	if today < cap {
		return nil
	}
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindProbe,
		Timestamp: now.UTC(),
		Task:      task,
		Outcome:   budget.OutcomeRefusedCap,
		Detail:    fmt.Sprintf("cap %d reached", cap),
	}); err != nil {
		return err
	}
	return fmt.Errorf("%s: evolution cap reached: %d run(s) today, cap %d (regulus.json evolution.max_per_day)", op, today, cap)
}

// checkTaskCap enforces the per-task evolution cap (shared shape).
func checkTaskCap(root, task, op string, now time.Time) error {
	tcap := reflect.TaskCap(root)
	tcount, err := budget.TaskEvolutionCount(root, task, now)
	if err != nil {
		return err
	}
	if tcount < tcap {
		return nil
	}
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindProbe,
		Timestamp: now.UTC(),
		Task:      task,
		Outcome:   budget.OutcomeRefusedCap,
		Detail:    fmt.Sprintf("task cap %d reached", tcap),
	}); err != nil {
		return err
	}
	return fmt.Errorf("%s: task evolution cap reached: task %q has %d run(s) today, task cap %d (regulus.json evolution.max_per_task)", op, task, tcount, tcap)
}

// checkProbeCaps is the shared cap-check for Contrast and Compose.
func checkProbeCaps(root, task, op string, now time.Time) error {
	if err := checkDailyCap(root, task, op, now); err != nil {
		return err
	}
	return checkTaskCap(root, task, op, now)
}

// loadProbeGate loads regulus.json and resolves one gate (shared shape).
func loadProbeGate(root, gate, op string) (gates.Gate, error) {
	cfg, err := gates.Load(filepath.Join(root, "regulus.json"))
	if err != nil {
		return gates.Gate{}, fmt.Errorf("%s: %v", op, err)
	}
	for i := range cfg.Gates {
		if cfg.Gates[i].Name == gate {
			return cfg.Gates[i], nil
		}
	}
	return gates.Gate{}, fmt.Errorf("%s: unknown gate %q", op, gate)
}

// openProbeGate runs the shared env + cap + load preamble for probes.
func openProbeGate(root, task, gate, op string, now time.Time, r gates.CmdRunner) (string, time.Time, gates.CmdRunner, gates.Gate, error) {
	root, now, r = normalizeProbeEnv(root, now, r)
	if err := checkProbeCaps(root, task, op, now); err != nil {
		return "", time.Time{}, nil, gates.Gate{}, err
	}
	orig, err := loadProbeGate(root, gate, op)
	if err != nil {
		return "", time.Time{}, nil, gates.Gate{}, err
	}
	return root, now, r, orig, nil
}

// probeEvidenceDir returns the probe subdirectory for a task.
func probeEvidenceDir(root, task string) string {
	return filepath.Join(root, ".regulus", "evidence", receipts.Slug(task), "probe")
}

// runProbeVariants runs each command through the gate path in order.
func runProbeVariants(op string, orig gates.Gate, evidenceDir, task, head string, r gates.CmdRunner, names, cmds []string) ([]receipts.Receipt, error) {
	recs := make([]receipts.Receipt, 0, len(cmds))
	for i, cmd := range cmds {
		rec, err := runProbeReceipt(op, orig, evidenceDir, task, head, cmd, r)
		if err != nil {
			return nil, fmt.Errorf("%s: run %s: %v", op, names[i], err)
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

// runContrastPair runs the A/B contrast variants (loop of 2).
func runContrastPair(orig gates.Gate, evidenceDir, task, head string, r gates.CmdRunner, cmdA, cmdB string) (receipts.Receipt, receipts.Receipt, error) {
	recs, err := runProbeVariants("probe contrast", orig, evidenceDir, task, head, r, []string{"A", "B"}, []string{cmdA, cmdB})
	if err != nil {
		return receipts.Receipt{}, receipts.Receipt{}, err
	}
	return recs[0], recs[1], nil
}

// runComposeTrio runs the base/A/B compose variants (loop of 3).
func runComposeTrio(orig gates.Gate, evidenceDir, task, head string, r gates.CmdRunner, base, cmdA, cmdB string) (receipts.Receipt, receipts.Receipt, receipts.Receipt, error) {
	recs, err := runProbeVariants("probe compose", orig, evidenceDir, task, head, r, []string{"base", "A", "B"}, []string{base, cmdA, cmdB})
	if err != nil {
		return receipts.Receipt{}, receipts.Receipt{}, receipts.Receipt{}, err
	}
	return recs[0], recs[1], recs[2], nil
}

// recordContrastBudget records one contrast probe-run entry.
func recordContrastBudget(root, task, gate, diff string, now time.Time) error {
	sum := sha256.Sum256([]byte(diff))
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindProbe,
		Timestamp: now.UTC(),
		Task:      task,
		Outcome:   budget.OutcomeProbeRun,
		Detail:    fmt.Sprintf("contrast task %s gate %s sha256 %s", task, gate, hex.EncodeToString(sum[:])),
	}); err != nil {
		return fmt.Errorf("probe contrast: %v", err)
	}
	return nil
}

// recordComposeBudget records one compose probe-run entry.
func recordComposeBudget(root, task, gate, report string, conflicts bool, now time.Time) error {
	sum := sha256.Sum256([]byte(report))
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindProbe,
		Timestamp: now.UTC(),
		Task:      task,
		Outcome:   budget.OutcomeProbeRun,
		Detail:    fmt.Sprintf("compose task %s gate %s sha256 %s conflicts=%t", task, gate, hex.EncodeToString(sum[:]), conflicts),
	}); err != nil {
		return fmt.Errorf("probe compose: %v", err)
	}
	return nil
}

// finishContrast diffs the pair and records the probe-run entry.
func finishContrast(root, task, gate, cmdA, cmdB string, now time.Time, recA, recB receipts.Receipt) (string, bool, error) {
	diff := Diff(recA, recB, cmdA, cmdB, gate, task)
	if err := recordContrastBudget(root, task, gate, diff, now); err != nil {
		return "", false, err
	}
	return diff, true, nil
}

// finishCompose renders the trio merge and records the entry.
func finishCompose(root, task, gate, base, cmdA, cmdB string, now time.Time, recBase, recA, recB receipts.Receipt) (string, bool, bool, error) {
	report, conflicts := renderComposed(recBase, recA, recB, base, cmdA, cmdB, gate, task)
	if err := recordComposeBudget(root, task, gate, report, conflicts, now); err != nil {
		return "", false, false, err
	}
	return report, conflicts, true, nil
}

// probeHead runs the shared git-head + evidence-dir preamble.
// The HEAD probe keeps the S3 bound (gitHeadBounded): only the probe
// runs keep their own timeouts, never the HEAD lookup.
func probeHead(root, task string, r gates.CmdRunner) (string, string) {
	return gitHeadBounded(r), probeEvidenceDir(root, task)
}
