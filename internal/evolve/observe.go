// Observation sources for the OBSERVE stage (Phase 6, S2). Receipt-backed
// sources (failed executions, repeated failures, execution costs, task
// outcomes, gauntlet results) are collected from the task evidence dir by
// Observe. Guardian findings and memory insights arrive through Note: their
// stores live in other packages and the caller attests the evidence; the
// support rule below applies to every source equally.
package evolve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"regulus-ai/internal/receipts"
)

// Observation sources.
const (
	SourceFailedExecutions = "failed-executions"
	SourceRepeatedFailures = "repeated-failures"
	SourceGuardianFindings = "guardian-findings"
	SourceGauntletResults  = "gauntlet-results"
	SourceExecutionCosts   = "execution-costs"
	SourceMemoryInsights   = "memory-insights"
	SourceTaskOutcomes     = "task-outcomes"
)

// MinSupport is the evidence bar against generalization: one isolated
// observation is never a general rule (S2).
const MinSupport = 2

// ErrIsolated is returned when nothing generalizable supports a hypothesis.
var ErrIsolated = errors.New("evolve: isolated observation cannot support a general rule (needs repeated evidence)")

// Observation is one improvement opportunity with its supporting evidence.
type Observation struct {
	Source   string
	Task     string
	Summary  string
	Evidence []string
	Count    int
}

// Generalizable reports whether the observation carries repeated support.
func (o Observation) Generalizable() bool {
	return o.Count >= MinSupport
}

// CheckSupport refuses an observation set with nothing generalizable in it.
func CheckSupport(obs []Observation) error {
	for _, o := range obs {
		if o.Generalizable() {
			return nil
		}
	}
	return ErrIsolated
}

// Note admits an externally-sourced observation (guardian findings, memory
// insights, gauntlet summaries): the source name is validated and the
// evidence list is attested by the caller. Count is the number of
// independent supporting items; 1 means isolated.
func Note(source, task, summary string, evidence []string) (Observation, error) {
	switch source {
	case SourceFailedExecutions, SourceRepeatedFailures, SourceGuardianFindings,
		SourceGauntletResults, SourceExecutionCosts, SourceMemoryInsights, SourceTaskOutcomes:
	default:
		return Observation{}, fmt.Errorf("evolve: unknown observation source %q", source)
	}
	if strings.TrimSpace(task) == "" || strings.TrimSpace(summary) == "" {
		return Observation{}, errors.New("evolve: observation needs a task and a summary")
	}
	return Observation{Source: source, Task: task, Summary: summary, Evidence: evidence, Count: len(evidence)}, nil
}

// Observe collects receipt-backed observations for a task: per-gate failure
// groups (failed-executions, promoted to repeated-failures at MinSupport),
// a cost summary, and a pass/fail outcome summary.
func Observe(root, task string) ([]Observation, error) {
	if strings.TrimSpace(task) == "" {
		return nil, errors.New("evolve: task must not be empty")
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	recs, err := readTaskReceipts(root, task)
	if err != nil {
		return nil, err
	}
	var out []Observation
	byGate := map[string][]string{}
	var passes, fails int
	var totalMS int64
	for _, r := range recs {
		totalMS += r.DurationMS
		if strings.EqualFold(r.Verdict, "pass") {
			passes++
			continue
		}
		fails++
		byGate[r.Gate] = append(byGate[r.Gate], r.ID)
	}
	gates := make([]string, 0, len(byGate))
	for g := range byGate {
		gates = append(gates, g)
	}
	sort.Strings(gates)
	for _, g := range gates {
		ids := byGate[g]
		src := SourceFailedExecutions
		if len(ids) >= MinSupport {
			src = SourceRepeatedFailures
		}
		out = append(out, Observation{
			Source:   src,
			Task:     task,
			Summary:  fmt.Sprintf("gate %s failed %d time(s)", g, len(ids)),
			Evidence: ids,
			Count:    len(ids),
		})
	}
	out = append(out, Observation{
		Source:   SourceExecutionCosts,
		Task:     task,
		Summary:  fmt.Sprintf("%d receipt(s), %dms total execution", len(recs), totalMS),
		Evidence: []string{fmt.Sprintf("%d receipts", len(recs))},
		Count:    1,
	})
	out = append(out, Observation{
		Source:   SourceTaskOutcomes,
		Task:     task,
		Summary:  fmt.Sprintf("%d pass, %d fail", passes, fails),
		Evidence: []string{fmt.Sprintf("pass=%d fail=%d", passes, fails)},
		Count:    1,
	})
	return out, nil
}

// readTaskReceipts reads every gate receipt for a task (probe contrasts
// live in probe/ and are gauntlet detail, not observations).
func readTaskReceipts(root, task string) ([]receipts.Receipt, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".regulus", "evidence", receipts.Slug(task), "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []receipts.Receipt
	for _, p := range paths {
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			continue
		}
		r, err := receipts.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("evolve: unreadable receipt %s: %v", p, err)
		}
		out = append(out, r)
	}
	return out, nil
}
