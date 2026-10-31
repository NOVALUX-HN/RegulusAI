package sextant

import (
	"strings"
	"testing"
)

func TestCheckProgressDoneRequiresClose(t *testing.T) {
	// T1 done but no CLOSED record: unverified completion.
	tasks, perrs := ParseTasks(tasksChart)
	if len(perrs) != 0 {
		t.Fatal(perrs)
	}
	// tasksChart Log has CLOSED T1: only T1 done is verified. Mark T2 done
	// without a record and expect a violation.
	tasks[1].Status = StatusDone
	violations := CheckProgress(tasks, tasksChart)
	joined := strings.Join(violations, "\n")
	if !strings.Contains(joined, "T2") || !strings.Contains(joined, "close") {
		t.Fatalf("unclosed done must name task + close, got %v", violations)
	}
	// T1 has its CLOSED record: no violation for T1.
	for _, v := range violations {
		if strings.Contains(v, "T1") {
			t.Fatalf("closed task must not violate, got %v", violations)
		}
	}
}

func TestCheckProgressDependencies(t *testing.T) {
	cases := []struct {
		name   string
		rows   string
		badSub string
	}{
		{"unknown dep", "| T1 | S1 | w | inline | T9 | todo |", "T9"},
		{"self dep", "| T1 | S1 | w | inline | T1 | todo |", "itself"},
		{"cycle", "| T1 | S1 | w | inline | T2 | todo |\n| T2 | S1 | w | inline | T1 | todo |", "cycle"},
		{"doing before dep", "| T1 | S1 | w | inline | - | todo |\n| T2 | S1 | w | inline | T1 | doing |", "T2"},
		{"done before dep", "| T1 | S1 | w | inline | - | todo |\n| T2 | S1 | w | inline | T1 | done |", "T2"},
		{"duplicate id", "| T1 | S1 | w | inline | - | todo |\n| T1 | S1 | w | inline | - | todo |", "duplicate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := "# F\n\n## Tasks\n\n| ID | Spec | Work | Route | Depends | Status |\n|----|------|------|-------|---------|--------|\n" + c.rows + "\n"
			tasks, perrs := ParseTasks(doc)
			if len(perrs) != 0 {
				t.Fatalf("parse must succeed, got %v", perrs)
			}
			violations := CheckProgress(tasks, doc)
			if len(violations) == 0 || !strings.Contains(strings.Join(violations, "\n"), c.badSub) {
				t.Fatalf("want violation naming %q, got %v", c.badSub, violations)
			}
		})
	}
}

func TestCheckProgressBlockedNeedsReason(t *testing.T) {
	doc := "# F\n\n## Tasks\n\n| ID | Spec | Work | Route | Status |\n|----|------|------|-------|--------|\n| T1 | S1 | w | inline | blocked |\n"
	tasks, perrs := ParseTasks(doc)
	if len(perrs) != 0 {
		t.Fatal(perrs)
	}
	violations := CheckProgress(tasks, doc)
	if len(violations) == 0 || !strings.Contains(strings.Join(violations, "\n"), "blocker") {
		t.Fatalf("bare blocked must demand a reason, got %v", violations)
	}
}

func TestCheckLogReplanFormat(t *testing.T) {
	good := "# F\n\n## Log\n\n- L1: authorized.\n- REPLAN: scope narrowed to cli — gremlins out of scope for this unit.\n"
	if violations := CheckLog(good); len(violations) != 0 {
		t.Fatalf("well-formed REPLAN must pass, got %v", violations)
	}
	bad := "# F\n\n## Log\n\n- REPLAN: x\n"
	if violations := CheckLog(bad); len(violations) == 0 {
		t.Fatal("justification-free REPLAN must be flagged")
	}
	// No Log section at all is fine (nothing to replan yet).
	if violations := CheckLog("# F\n"); len(violations) != 0 {
		t.Fatalf("missing Log must not violate, got %v", violations)
	}
}

func TestClosedTasks(t *testing.T) {
	closed := ClosedTasks(tasksChart)
	if !closed["T1"] || closed["T2"] {
		t.Fatalf("want {T1}, got %v", closed)
	}
	if len(ClosedTasks("# F\n")) != 0 {
		t.Fatal("no records means none closed")
	}
}
