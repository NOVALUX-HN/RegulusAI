// Task delegation governance (Phase 2, S4): the chart's ## Tasks table
// parsed, with routing rules per class. Delegation records carry objective
// (Work), scope (Spec), responsibility (Route), constraints (class rules),
// context (Spec link) and expected output (Status tracking); the core
// validates all of it.
package sextant

import (
	"fmt"
	"regexp"
	"strings"
)

// Task statuses.
const (
	StatusTodo    = "todo"
	StatusDoing   = "doing"
	StatusDone    = "done"
	StatusBlocked = "blocked"
)

// Task is one row of the chart's ## Tasks table.
type Task struct {
	ID            string
	Spec          string
	Work          string
	Route         string
	Depends       []string
	Status        string
	BlockedReason string
	Line          int
}

// Crew routes that may take a bounded unit.
var crewRoutes = map[string]bool{
	"inline": true, "@regulus": true,
	"@regulus-scout": true, "@regulus-worker": true,
	"@regulus-verifier": true, "@regulus-proposer": true,
	"regulus-scout": true, "regulus-worker": true,
	"regulus-verifier": true, "regulus-proposer": true,
	"regulus": true,
}

// Read-only guardian routes: they verify, never take units.
var guardianRoutes = map[string]bool{
	"@regulus-gate-verifier": true, "@regulus-spec-keeper": true,
	"@regulus-mutation-auditor": true, "@regulus-crap-analyst": true,
	"@regulus-evidence-auditor": true, "@regulus-reviewer": true,
	"regulus-gate-verifier": true, "regulus-spec-keeper": true,
	"regulus-mutation-auditor": true, "regulus-crap-analyst": true,
	"regulus-evidence-auditor": true, "regulus-reviewer": true,
}

var tasksHeadingRe = regexp.MustCompile(`(?m)^## Tasks\s*$`)

func normalizeHeader(h string) string {
	return strings.ToLower(strings.TrimSpace(h))
}

// ParseTasks parses the ## Tasks pipe-table. Columns match by header name
// (ID required; Spec, Work, Route, Depends, Status optional; anything else
// ignored), so legacy 4-column tables parse with Status=todo and no deps.
// A missing section yields no tasks and no violations.
func ParseTasks(doc string) ([]Task, []string) {
	var tasks []Task
	m := tasksHeadingRe.FindStringIndex(doc)
	if m == nil {
		return nil, nil
	}
	baseLine := lineOf(doc, m[0])
	rest := doc[m[0]:]
	// Cut at the next top-level section.
	if next := regexp.MustCompile(`(?m)^## [A-Z]`).FindStringIndex(rest); next != nil && next[0] > 0 {
		rest = rest[:next[0]]
	}
	lines := strings.Split(rest, "\n")
	// Find the header row: first pipe line followed by a dashes row.
	hi := -1
	for i, line := range lines {
		if isPipeRow(line) && i+1 < len(lines) && isDashesRow(lines[i+1]) {
			hi = i
			break
		}
	}
	if hi < 0 {
		return nil, nil
	}
	headers := splitPipeRow(lines[hi])
	col := map[string]int{}
	for i, h := range headers {
		n := normalizeHeader(h)
		if _, dup := col[n]; dup {
			continue
		}
		col[n] = i
	}
	idCol, ok := col["id"]
	if !ok {
		return nil, []string{fmt.Sprintf("line %d: tasks table needs an ID column", baseLine+hi)}
	}
	cell := func(cells []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(cells) {
			return ""
		}
		return strings.TrimSpace(cells[i])
	}
	for i := hi + 2; i < len(lines); i++ {
		if !isPipeRow(lines[i]) {
			break
		}
		cells := splitPipeRow(lines[i])
		if idCol >= len(cells) || strings.TrimSpace(cells[idCol]) == "" {
			continue
		}
		t := Task{
			ID:    strings.TrimSpace(cells[idCol]),
			Spec:  cell(cells, "spec"),
			Work:  cell(cells, "work"),
			Route: cell(cells, "route"),
			Line:  baseLine + i,
		}
		if t.Route == "" {
			t.Route = "inline"
		}
		dep := cell(cells, "depends")
		if dep == "" {
			dep = cell(cells, "depend")
		}
		t.Depends = filterPlaceholderDeps(splitNames(dep))
		status, reason := parseStatus(cell(cells, "status"))
		t.Status = status
		t.BlockedReason = reason
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// filterPlaceholderDeps drops the conventional no-dependency markers so
// `-` and `none` never read as a task id.
func filterPlaceholderDeps(deps []string) []string {
	var out []string
	for _, d := range deps {
		switch strings.ToLower(d) {
		case "-", "none", "—", "n/a":
			continue
		}
		out = append(out, d)
	}
	return out
}

func parseStatus(s string) (string, string) {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return StatusTodo, ""
	}
	if t == StatusBlocked || strings.HasPrefix(t, StatusBlocked+":") || strings.HasPrefix(t, StatusBlocked+" ") {
		reason := strings.TrimSpace(s[len(StatusBlocked):])
		reason = strings.TrimLeft(reason, ": ")
		return StatusBlocked, reason
	}
	switch t {
	case StatusTodo, StatusDoing, StatusDone:
		return t, ""
	default:
		return t, ""
	}
}

func isPipeRow(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "|") && strings.HasSuffix(t, "|")
}

func isDashesRow(line string) bool {
	t := strings.TrimSpace(line)
	if !isPipeRow(t) {
		return false
	}
	inner := strings.Trim(t, "|")
	if inner == "" {
		return false
	}
	for _, cell := range strings.Split(inner, "|") {
		c := strings.TrimSpace(cell)
		if c == "" {
			continue
		}
		for _, r := range c {
			if r != '-' && r != ':' {
				return false
			}
		}
	}
	return true
}

func splitPipeRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	return strings.Split(t, "|")
}

// CheckDelegations validates routing per class: known agents only,
// guardians never take units, subagents carry their spec context, TRIVIAL
// never delegates, CRITICAL demands a verifier task in the chart.
func CheckDelegations(tasks []Task, class Class) []string {
	var violations []string
	hasVerifier := false
	for _, t := range tasks {
		if t.Route == "@regulus-verifier" || t.Route == "regulus-verifier" {
			hasVerifier = true
		}
	}
	for _, t := range tasks {
		route := t.Route
		switch {
		case crewRoutes[route]:
		case guardianRoutes[route]:
			violations = append(violations, fmt.Sprintf("line %d: task %s routes to %s, which is read-only — guardians verify, they never take units", t.Line, t.ID, route))
		default:
			violations = append(violations, fmt.Sprintf("line %d: task %s routes to unknown agent %q", t.Line, t.ID, route))
		}
		if route != "inline" && crewRoutes[route] && t.Spec == "" {
			violations = append(violations, fmt.Sprintf("line %d: delegated task %s carries no Spec — the context the unit works against", t.Line, t.ID))
		}
		if class == ClassTrivial && route != "inline" {
			violations = append(violations, fmt.Sprintf("line %d: TRIVIAL task %s must run inline — a worker costs more than the work", t.Line, t.ID))
		}
	}
	if class == ClassCritical {
		needsVerifier := false
		for _, t := range tasks {
			if t.Route == "@regulus-worker" || t.Route == "regulus-worker" {
				needsVerifier = true
			}
		}
		if needsVerifier && !hasVerifier {
			violations = append(violations, "critical work routed to a worker with no @regulus-verifier task in the chart")
		}
	}
	return violations
}
