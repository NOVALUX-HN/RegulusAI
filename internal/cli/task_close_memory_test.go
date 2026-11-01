package cli

// M3 auto-save on close: `task close` records one session observation
// with the summary (id, spec, receipts) as a side effect — the close
// verdict never changes, a repeat close skips by topic, and
// CheckDuplicate (DedupThreshold) refuses saves duplicating pinned or
// lesson-equivalent knowledge.

import (
	"os"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/sextant"
)

func closeTopic(chart, id string) string { return closeNoteTopic(chart, id) }

func TestTaskCloseAutoSavesObservation(t *testing.T) {
	chart := initTaskChart(t, "demo")
	writeTaskReceipt(t, "demo/T1", "build", true)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "close", chart, "T1"}); code != 0 {
		t.Fatalf("close: exit = %d, want 0, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "CLOSED T1") {
		t.Fatalf("close must keep its verdict, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "close observation saved obs_") {
		t.Fatalf("close must auto-save the observation, got:\n%s", out.String())
	}
	hist, err := almanac.History(".", closeTopic("demo", "T1"))
	if err != nil || len(hist) != 1 {
		t.Fatalf("one close note with the unit topic, got %+v (%v)", hist, err)
	}
	note := hist[0]
	if note.Type != almanac.TypeSessionNote {
		t.Fatalf("close note type = %q, want session-note", note.Type)
	}
	for _, want := range []string{"CLOSED demo/T1", "spec", "demo__T1"} {
		if !strings.Contains(note.Content, want) {
			t.Fatalf("close note must summarize %q, got:\n%s", want, note.Content)
		}
	}
	if note.Provenance == nil || note.Provenance.Task != "demo/T1" || len(note.Provenance.Receipts) == 0 {
		t.Fatalf("close note must carry task+receipts provenance, got %+v", note.Provenance)
	}
	// The chart still records the closure exactly like before.
	data, err := os.ReadFile(chart)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "CLOSED T1") || !strings.Contains(string(data), "done") {
		t.Fatalf("chart must record the closure, got:\n%s", data)
	}
}

func TestTaskCloseAutoSaveDedupsRepeatClose(t *testing.T) {
	chart := initTaskChart(t, "demo")
	writeTaskReceipt(t, "demo/T1", "build", true)
	c1, _, _ := newCLI(nil)
	if code := c1.Run([]string{"task", "close", chart, "T1"}); code != 0 {
		t.Fatalf("first close: exit = %d", code)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"task", "close", chart, "T1"}); code != 0 {
		t.Fatalf("repeat close must keep exit 0 (close flow unchanged), got %d", code)
	}
	if !strings.Contains(out2.String(), "already recorded") {
		t.Fatalf("repeat close must skip by topic, got:\n%s", out2.String())
	}
	hist, err := almanac.History(".", closeTopic("demo", "T1"))
	if err != nil || len(hist) != 1 {
		t.Fatalf("repeat close must not duplicate the note, got %d (%v)", len(hist), err)
	}
}

func TestTaskCloseAutoSaveSkipsPinnedEquivalent(t *testing.T) {
	chart := initTaskChart(t, "demo")
	writeTaskReceipt(t, "demo/T1", "build", true)
	raw, err := os.ReadFile(chart)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	res := sextant.CheckClose(doc, ".", "demo", "T1")
	if !res.Closed {
		t.Fatalf("fixture must be closeable, unmet: %v", res.Unmet)
	}
	// A pinned observation carrying exactly what the close would record —
	// without the unit topic, so only the DedupThreshold bar can catch it.
	seed := buildCloseNote("demo", "T1", doc, res, "")
	seed.TopicKey = ""
	saved, err := almanac.Save(".", seed, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := almanac.Pin(".", saved.ID, true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "close", chart, "T1"}); code != 0 {
		t.Fatalf("close verdict must stand despite the skip, got %d", code)
	}
	if !strings.Contains(out.String(), "duplicates "+saved.ID) {
		t.Fatalf("close must name the pinned equivalent it refuses to duplicate, got:\n%s", out.String())
	}
	all, err := almanac.All(".")
	if err != nil || len(all) != 1 {
		t.Fatalf("nothing may duplicate the pinned equivalent, got %d (%v)", len(all), err)
	}
	if hist, _ := almanac.History(".", closeTopic("demo", "T1")); len(hist) != 0 {
		t.Fatalf("no unit-topic note may land, got %+v", hist)
	}
}

func TestBuildCloseNoteShapesSummary(t *testing.T) {
	doc := "# demo\n\nDomain: almanac\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 | M3 | auto-save | inline |\n"
	res := sextant.CloseResult{TaskID: "T1", Class: "COMPLEX", Receipts: []string{"a.json", "b.json"}}
	obs := buildCloseNote("demo", "T1", doc, res, "abc")
	if obs.Title != "task close demo/T1" || obs.TopicKey != "task-close/demo/T1" || obs.Domain != "almanac" {
		t.Fatalf("note shape = %+v", obs)
	}
	for _, want := range []string{"CLOSED demo/T1", "spec: M3", "receipt: a.json", "receipt: b.json"} {
		if !strings.Contains(obs.Content, want) {
			t.Fatalf("note content must carry %q, got:\n%s", want, obs.Content)
		}
	}
	if obs.Provenance == nil || obs.Provenance.Task != "demo/T1" || obs.Provenance.GitHead != "abc" {
		t.Fatalf("provenance = %+v", obs.Provenance)
	}
	// The scaffold placeholder declares no domain — a note never invents one.
	if d := chartDomain("# x\n\nDomain: <name>\n"); d != "" {
		t.Fatalf("placeholder domain must read empty, got %q", d)
	}
	if d := chartDomain("# x\n"); d != "" {
		t.Fatalf("missing Domain line must read empty, got %q", d)
	}
}
