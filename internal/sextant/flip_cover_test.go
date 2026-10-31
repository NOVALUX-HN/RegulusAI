package sextant

import (
	"strings"
	"testing"
)

// S6/C7: flipStatusCell was cov69. Covers cells, legacy tables and errors.
func TestFlipNoTasksHeading(t *testing.T) {
	doc := "# F\n\nno tasks here\n"
	if got := flipStatusCell(doc, "T1"); got != doc {
		t.Fatalf("missing ## Tasks must return doc unchanged, got:\n%s", got)
	}
}

func TestFlipNoHeaderRow(t *testing.T) {
	doc := "# F\n\n## Tasks\n\nno table here\n"
	if got := flipStatusCell(doc, "T1"); got != doc {
		t.Fatalf("missing table must return doc unchanged, got:\n%s", got)
	}
}

func TestFlipNoIDColumn(t *testing.T) {
	doc := "# F\n\n## Tasks\n\n| Spec | Work |\n|------|------|\n| s | w |\n\n## Log\n\n- L1.\n"
	if got := flipStatusCell(doc, "T1"); got != doc {
		t.Fatalf("table without ID must return doc unchanged, got:\n%s", got)
	}
}

func TestFlipUnknownTaskLeavesDoc(t *testing.T) {
	doc := closeChart("STANDARD", "")
	got := flipStatusCell(doc, "T9")
	if strings.Contains(got, "| done |") && !strings.Contains(doc, "| done |") {
		t.Fatalf("unknown task must not flip any row, got:\n%s", got)
	}
	tasks, _ := ParseTasks(got)
	for _, task := range tasks {
		if task.Status == StatusDone {
			t.Fatalf("unknown task must not mark %s done", task.ID)
		}
	}
}

func TestFlipLegacyTableWithoutStatus(t *testing.T) {
	doc := "# F\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 | s | w | inline |\n| T2 | s | w | inline |\n\n## Log\n\n- L1.\n"
	got := flipStatusCell(doc, "T1")
	if !strings.Contains(got, "Status") {
		t.Fatalf("legacy table must gain a Status column, got:\n%s", got)
	}
	tasks, _ := ParseTasks(got)
	var t1, t2 *Task
	for i := range tasks {
		switch tasks[i].ID {
		case "T1":
			t1 = &tasks[i]
		case "T2":
			t2 = &tasks[i]
		}
	}
	if t1 == nil || t2 == nil {
		t.Fatalf("rows must survive, got %+v", tasks)
	}
	if t1.Status != StatusDone {
		t.Fatalf("T1 status = %q, want done", t1.Status)
	}
	if t2.Status == StatusDone {
		t.Fatalf("T2 must stay %q, got done", t2.Status)
	}
}

func TestFlipShortRowExtends(t *testing.T) {
	// Header has 6 columns; the T1 row carries only 4 cells, so the
	// extender (len(cells) <= statusCol) must run.
	doc := "# F\n\n## Tasks\n\n| ID | Spec | Work | Route | Depends | Status |\n|----|------|------|-------|---------|--------|\n| T1 | s | w | inline |\n\n## Log\n\n- L1.\n"
	got := flipStatusCell(doc, "T1")
	tasks, perrs := ParseTasks(got)
	if len(perrs) != 0 {
		t.Fatalf("extended row must still parse: %v\n%s", perrs, got)
	}
	if len(tasks) != 1 || tasks[0].ID != "T1" || tasks[0].Status != StatusDone {
		t.Fatalf("short row must flip to done, got %+v\n%s", tasks, got)
	}
}

func TestFlipOnlyTargetRow(t *testing.T) {
	doc := closeChart("STANDARD", "")
	got := flipStatusCell(doc, "T2")
	tasks, _ := ParseTasks(got)
	for _, task := range tasks {
		switch task.ID {
		case "T1":
			if task.Status == StatusDone {
				t.Fatalf("T1 must not flip when T2 is the target")
			}
		case "T2":
			if task.Status != StatusDone {
				t.Fatalf("T2 status = %q, want done", task.Status)
			}
		}
	}
}
