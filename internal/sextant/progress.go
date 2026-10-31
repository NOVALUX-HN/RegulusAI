// Execution governance (Phase 2, S5): progress, dependencies, blockers,
// scope control and justified replanning — all read from the chart, all
// machine-checkable. A `done` status without a CLOSED record is an
// unverified claim, never a completion.
package sextant

import (
	"fmt"
	"regexp"
	"strings"
)

var logHeadingRe = regexp.MustCompile(`(?m)^## Log\s*$`)
var closedLineRe = regexp.MustCompile(`(?m)^-\s*CLOSED\s+(\S+)`)
var replanLineRe = regexp.MustCompile(`(?m)^-\s*REPLAN:\s*(.+?)\s*$`)

// ClosedTasks returns the task ids with a CLOSED record in ## Log.
// `task close` writes these; nothing else may.
func ClosedTasks(doc string) map[string]bool {
	out := map[string]bool{}
	for _, m := range closedLineRe.FindAllStringSubmatch(doc, -1) {
		out[strings.TrimSpace(m[1])] = true
	}
	return out
}

// CheckLog validates ## Log discipline: REPLAN entries carry a
// justification (scope + why); a missing Log section is fine.
func CheckLog(doc string) []string {
	var violations []string
	m := logHeadingRe.FindStringIndex(doc)
	if m == nil {
		return nil
	}
	base := lineOf(doc, m[0])
	rest := doc[m[0]:]
	if next := regexp.MustCompile(`(?m)^## [A-Z]`).FindStringIndex(rest); next != nil && next[0] > 0 {
		rest = rest[:next[0]]
	}
	lines := strings.Split(rest, "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "- REPLAN:") {
			continue
		}
		just := strings.TrimSpace(strings.TrimPrefix(t, "- REPLAN:"))
		// A justification names the rescoped work and why: demand substance.
		if len(just) < 10 || !strings.ContainsAny(just, "—-:") {
			violations = append(violations, fmt.Sprintf("line %d: REPLAN must justify the rescope (what changed and why), got %q", base+i, just))
		}
		_ = replanLineRe
	}
	return violations
}

// CheckProgress enforces execution governance over parsed tasks:
// unique ids, known and acyclic dependencies, ordering (doing/done only
// after deps closed), named blockers, and verified completions (done only
// with a CLOSED record).
func CheckProgress(tasks []Task, doc string) []string {
	var violations []string
	byID := map[string]Task{}
	for _, t := range tasks {
		if _, dup := byID[t.ID]; dup {
			violations = append(violations, fmt.Sprintf("line %d: duplicate task id %q", t.Line, t.ID))
			continue
		}
		byID[t.ID] = t
	}
	for _, t := range tasks {
		for _, d := range t.Depends {
			if strings.EqualFold(d, t.ID) {
				violations = append(violations, fmt.Sprintf("line %d: task %s depends on itself", t.Line, t.ID))
				continue
			}
			if _, ok := byID[d]; !ok {
				violations = append(violations, fmt.Sprintf("line %d: task %s depends on unknown task %q", t.Line, t.ID, d))
			}
		}
	}
	if cycle := findCycle(tasks); cycle != "" {
		violations = append(violations, fmt.Sprintf("dependency cycle: %s", cycle))
	}
	closed := ClosedTasks(doc)
	for _, t := range tasks {
		for _, d := range t.Depends {
			dep, ok := byID[d]
			if !ok {
				continue
			}
			depDone := dep.Status == StatusDone && closed[dep.ID]
			if (t.Status == StatusDoing || t.Status == StatusDone) && !depDone {
				violations = append(violations, fmt.Sprintf("line %d: task %s is %s but dependency %s is not closed", t.Line, t.ID, t.Status, d))
			}
		}
		if t.Status == StatusBlocked && strings.TrimSpace(t.BlockedReason) == "" {
			violations = append(violations, fmt.Sprintf("line %d: blocked task %s must name the blocker (Status: blocked: <reason>)", t.Line, t.ID))
		}
		if t.Status == StatusDone && !closed[t.ID] {
			violations = append(violations, fmt.Sprintf("line %d: task %s is marked done without a CLOSED record — run task close, self-report closes nothing", t.Line, t.ID))
		}
	}
	return violations
}

func findCycle(tasks []Task) string {
	byID := map[string]Task{}
	for _, t := range tasks {
		byID[t.ID] = t
	}
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var visit func(id string) string
	visit = func(id string) string {
		color[id] = grey
		stack = append(stack, id)
		for _, d := range byID[id].Depends {
			if _, ok := byID[d]; !ok {
				continue
			}
			if color[d] == grey {
				i := 0
				for i < len(stack) && stack[i] != d {
					i++
				}
				return strings.Join(append(append([]string{}, stack[i:]...), d), " -> ")
			}
			if color[d] == white {
				if c := visit(d); c != "" {
					return c
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return ""
	}
	for _, t := range tasks {
		if color[t.ID] == white {
			if c := visit(t.ID); c != "" {
				return c
			}
		}
	}
	return ""
}
