package sextant

import (
	"strings"
	"testing"
)

const tasksChart = `# Feature

Domain: go
Class: COMPLEX

## Contract

- Objective: x
- Scope: y
- Acceptance: the happy path
- Verification: g
- Artifacts: a
- Dependencies: none

## Acceptance

### Scenario: the happy path

- Given a
- When b
- Then c

## Tasks

| ID | Spec | Work | Route | Depends | Status |
|----|------|------|-------|---------|--------|
| T1 | the happy path | map | @regulus-scout | - | done |
| T2 | the happy path | build | @regulus-worker | T1 | doing |
| T3 | the happy path | verify | @regulus-verifier | T2 | todo |

## Log

- L1: authorized.
- CLOSED T1 verified task T1 closed with receipts PASS.
`

func TestParseTasksFullColumns(t *testing.T) {
	tasks, violations := ParseTasks(tasksChart)
	if len(violations) != 0 {
		t.Fatalf("valid table must parse, got %v", violations)
	}
	if len(tasks) != 3 {
		t.Fatalf("want 3 tasks, got %+v", tasks)
	}
	if tasks[0].Route != "@regulus-scout" || tasks[1].Depends[0] != "T1" {
		t.Fatalf("columns must map, got %+v", tasks)
	}
	if tasks[0].Status != StatusDone || tasks[1].Status != StatusDoing {
		t.Fatalf("status must parse, got %+v", tasks)
	}
}

func TestParseTasksLegacyFourColumns(t *testing.T) {
	doc := "# F\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| F1 | S1 | work | @regulus-worker |\n"
	tasks, violations := ParseTasks(doc)
	if len(violations) != 0 {
		t.Fatalf("legacy table must parse, got %v", violations)
	}
	if len(tasks) != 1 || tasks[0].Status != StatusTodo || len(tasks[0].Depends) != 0 {
		t.Fatalf("legacy defaults wrong, got %+v", tasks)
	}
}

func TestParseTasksNoSection(t *testing.T) {
	tasks, violations := ParseTasks("# F\n\nno tasks here\n")
	if len(violations) != 0 || len(tasks) != 0 {
		t.Fatalf("missing section must be empty-clean, got %v/%v", tasks, violations)
	}
}

func TestParseTasksBlockedReason(t *testing.T) {
	doc := "# F\n\n## Tasks\n\n| ID | Spec | Work | Route | Status |\n|----|------|------|-------|--------|\n| T1 | S1 | w | inline | blocked: waiting on API key |\n"
	tasks, violations := ParseTasks(doc)
	if len(violations) != 0 {
		t.Fatalf("blocked with reason must parse, got %v", violations)
	}
	if tasks[0].Status != StatusBlocked || tasks[0].BlockedReason == "" {
		t.Fatalf("blocker reason must be kept, got %+v", tasks[0])
	}
}

func TestCheckDelegations(t *testing.T) {
	cases := []struct {
		name   string
		doc    string
		class  Class
		badSub string
	}{
		{"unknown agent", "# F\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 | S1 | w | @regulus-ghost |\n", ClassStandard, "unknown agent"},
		{"guardian route", "# F\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 | S1 | w | @regulus-gate-verifier |\n", ClassStandard, "read-only"},
		{"trivial delegates", "# F\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 | S1 | w | @regulus-worker |\n", ClassTrivial, "TRIVIAL"},
		{"critical without verifier", "# F\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 | S1 | w | @regulus-worker |\n", ClassCritical, "verifier"},
		{"worker without spec", "# F\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 |  | w | @regulus-worker |\n", ClassStandard, "Spec"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tasks, perrs := ParseTasks(c.doc)
			if len(perrs) != 0 {
				t.Fatalf("parse must succeed, got %v", perrs)
			}
			violations := CheckDelegations(tasks, c.class)
			if len(violations) == 0 || !strings.Contains(strings.Join(violations, "\n"), c.badSub) {
				t.Fatalf("want violation naming %q, got %v", c.badSub, violations)
			}
		})
	}
}

func TestCheckDelegationsClean(t *testing.T) {
	tasks, perrs := ParseTasks(tasksChart)
	if len(perrs) != 0 {
		t.Fatal(perrs)
	}
	if violations := CheckDelegations(tasks, ClassComplex); len(violations) != 0 {
		t.Fatalf("crew routing with specs must be clean, got %v", violations)
	}
	// Inline without spec is fine: the unit carries its own context.
	doc := "# F\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 |  | w | inline |\n"
	tasks, _ = ParseTasks(doc)
	if violations := CheckDelegations(tasks, ClassStandard); len(violations) != 0 {
		t.Fatalf("inline must not need a spec, got %v", violations)
	}
}
