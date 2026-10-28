// Package probe runs contrast probes (G2.7): the same gate twice with
// two command variants, diffing the two real receipts as evidence.
package probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/reflect"
)

// Runner executes the probe in tests; production uses gates.ShRunner.
var Runner gates.CmdRunner

func runner() gates.CmdRunner {
	if Runner != nil {
		return Runner
	}
	return gates.ShRunner{}
}

var taskIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)

// Contrast executes cmdA and cmdB through the SAME path `gate run` uses
// (gates.Run, so each run writes a real receipt under
// .regulus/evidence/<task>/) and diffs the two receipts over command,
// exit code, stdout and stderr. It returns the line-oriented diff with
// A/B markers; an empty diff means the receipts are identical on every
// compared field. One budget entry (Kind=probe, Outcome=probe-run) with
// the diff digest in Detail records the probe. Refusals (empty
// task/gate/command, invalid task id, unknown gate) and run errors
// return an error with ran=false and record nothing.
func Contrast(root, task, gate, cmdA, cmdB string, now time.Time) (string, bool, error) {
	return ContrastWithRunner(root, task, gate, cmdA, cmdB, now, runner())
}

// ContrastWithRunner is Contrast with an injected command runner (the
// CLI passes its own runner so tests can substitute fakes).
func ContrastWithRunner(root, task, gate, cmdA, cmdB string, now time.Time, r gates.CmdRunner) (string, bool, error) {
	if strings.TrimSpace(task) == "" {
		return "", false, fmt.Errorf("probe contrast: task must not be empty")
	}
	if !taskIDRe.MatchString(task) || strings.Contains(task, "..") {
		return "", false, fmt.Errorf("probe contrast: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')", task)
	}
	if strings.TrimSpace(gate) == "" {
		return "", false, fmt.Errorf("probe contrast: gate must not be empty")
	}
	if strings.TrimSpace(cmdA) == "" || strings.TrimSpace(cmdB) == "" {
		return "", false, fmt.Errorf("probe contrast: both --a and --b commands must not be empty")
	}
	if r == nil {
		r = gates.ShRunner{}
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Cap check (PRD R-E4): probes consume the evolution budget like
	// reflect proposals; refusals are recorded but free.
	cap := reflect.EvolutionCap(root)
	today, err := budget.TodayEvolutionCount(root, now)
	if err != nil {
		return "", false, err
	}
	if today >= cap {
		if err := budget.Record(root, budget.Entry{
			Kind:      budget.KindProbe,
			Timestamp: now.UTC(),
			Task:      task,
			Outcome:   budget.OutcomeRefusedCap,
			Detail:    fmt.Sprintf("cap %d reached", cap),
		}); err != nil {
			return "", false, err
		}
		return "", false, fmt.Errorf("probe contrast: evolution cap reached: %d run(s) today, cap %d (regulus.json evolution.max_per_day)", today, cap)
	}
	tcap := reflect.TaskCap(root)
	tcount, err := budget.TaskEvolutionCount(root, task, now)
	if err != nil {
		return "", false, err
	}
	if tcount >= tcap {
		if err := budget.Record(root, budget.Entry{
			Kind:      budget.KindProbe,
			Timestamp: now.UTC(),
			Task:      task,
			Outcome:   budget.OutcomeRefusedCap,
			Detail:    fmt.Sprintf("task cap %d reached", tcap),
		}); err != nil {
			return "", false, err
		}
		return "", false, fmt.Errorf("probe contrast: task evolution cap reached: task %q has %d run(s) today, task cap %d (regulus.json evolution.max_per_task)", task, tcount, tcap)
	}

	cfg, err := gates.Load(filepath.Join(root, "regulus.json"))
	if err != nil {
		return "", false, fmt.Errorf("probe contrast: %v", err)
	}
	var orig *gates.Gate
	for i := range cfg.Gates {
		if cfg.Gates[i].Name == gate {
			orig = &cfg.Gates[i]
			break
		}
	}
	if orig == nil {
		return "", false, fmt.Errorf("probe contrast: unknown gate %q", gate)
	}

	head := gates.GitHead(context.Background(), r)
	// Probe runs land in their own subdirectory: gate evidence stays
	// pure (reflect's evidence gate, gate-centric audits), and consumers
	// (evolve pack, human inspection) can tell behavioral contrasts
	// apart from gate verdicts at a glance.
	evidenceDir := filepath.Join(root, ".regulus", "evidence", receipts.Slug(task), "probe")
	run := func(cmd string) (receipts.Receipt, error) {
		return runProbeReceipt("probe contrast", *orig, evidenceDir, task, head, cmd, r)
	}
	recA, err := run(cmdA)
	if err != nil {
		return "", false, fmt.Errorf("probe contrast: run A: %v", err)
	}
	recB, err := run(cmdB)
	if err != nil {
		return "", false, fmt.Errorf("probe contrast: run B: %v", err)
	}

	diff := Diff(recA, recB, cmdA, cmdB, gate, task)
	sum := sha256.Sum256([]byte(diff))
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindProbe,
		Timestamp: now.UTC(),
		Task:      task,
		Outcome:   budget.OutcomeProbeRun,
		Detail:    fmt.Sprintf("contrast task %s gate %s sha256 %s", task, gate, hex.EncodeToString(sum[:])),
	}); err != nil {
		return "", false, fmt.Errorf("probe contrast: %v", err)
	}
	return diff, true, nil
}

// runProbeReceipt runs one command variant through the SAME path `gate
// run` uses (gates.Run, so the run writes a real receipt under
// evidenceDir) and returns that receipt. op names the calling operation
// for error messages ("probe contrast", "probe compose").
func runProbeReceipt(op string, orig gates.Gate, evidenceDir, task, head, cmd string, r gates.CmdRunner) (receipts.Receipt, error) {
	single := orig
	single.Command = cmd
	res, err := gates.Run(context.Background(), gates.Config{Gates: []gates.Gate{single}}, evidenceDir, task, head, r)
	if err != nil {
		return receipts.Receipt{}, err
	}
	if len(res.Receipts) == 0 {
		return receipts.Receipt{}, fmt.Errorf("%s: gate %q wrote no receipt", op, orig.Name)
	}
	rec, err := receipts.ReadFile(res.Receipts[len(res.Receipts)-1])
	if err != nil {
		return receipts.Receipt{}, err
	}
	return rec, nil
}

// Diff renders the line-oriented receipt contrast between runs A and B
// over command, exit code, stdout and stderr. Identical fields produce
// no section; when every compared field is identical Diff returns "".
func Diff(a, b receipts.Receipt, cmdA, cmdB, gate, task string) string {
	var sb strings.Builder
	section := func(title string, lines ...string) {
		sb.WriteString("@@ " + title + " @@\n")
		for _, l := range lines {
			sb.WriteString(l + "\n")
		}
	}
	if cmdA != cmdB {
		section("command", "- A: "+cmdA, "+ B: "+cmdB)
	}
	if a.ExitCode != b.ExitCode {
		section("exit_code",
			fmt.Sprintf("- A: %d", a.ExitCode),
			fmt.Sprintf("+ B: %d", b.ExitCode))
	}
	if a.Stdout != b.Stdout {
		section("stdout", diffLines(a.Stdout, b.Stdout)...)
	}
	if a.Stderr != b.Stderr {
		section("stderr", diffLines(a.Stderr, b.Stderr)...)
	}
	if sb.Len() == 0 {
		return ""
	}
	head := fmt.Sprintf("contrast gate %q task %q\n--- A: %s\n+++ B: %s\n", gate, task, cmdA, cmdB)
	return head + sb.String()
}

// diffLines is a small LCS line diff: common lines carry a two-space
// prefix, A-only lines a "- A: " marker, B-only lines a "+ B: " marker.
func diffLines(a, b string) []string {
	al := splitLines(a)
	bl := splitLines(b)
	n, m := len(al), len(bl)
	// lcs[i][j] = LCS length of al[i:] and bl[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []string
	for i, j := 0, 0; i < n || j < m; {
		switch {
		case i < n && j < m && al[i] == bl[j]:
			out = append(out, "  "+al[i])
			i++
			j++
		case j < m && (i >= n || lcs[i][j+1] >= lcs[i+1][j]):
			out = append(out, "+ B: "+bl[j])
			j++
		default:
			out = append(out, "- A: "+al[i])
			i++
		}
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
