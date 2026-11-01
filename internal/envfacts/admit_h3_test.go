package envfacts

// H3: idempotent discard + per-decision Apply reporting.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/receipts"
)

func countNotes(t *testing.T, root string) int {
	t.Helper()
	all, err := almanac.All(root)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, o := range all {
		if o.Type == almanac.TypeSessionNote {
			n++
		}
	}
	return n
}

// Double Apply of the same discard must not mint a double note: the
// second run reloads liveFactIDs, finds nothing live, and skips.
func TestDoubleApplySameDiscardNoDuplicateNote(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_f%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	first, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root, first, base.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictPass,
			fmt.Sprintf("run_p%d", i), base.Add(time.Duration(10+i)*time.Hour))
	}
	decisions, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Action != ActionDiscard {
		t.Fatalf("contradicted live fact must plan a discard, got %+v", decisions)
	}
	r1, err := Apply(root, decisions, base.Add(20*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if r1[0].NoteID == "" {
		t.Fatal("first discard must record the contradiction note")
	}
	notesBefore := countNotes(t, root)
	r2, err := Apply(root, decisions, base.Add(21*time.Hour))
	if err != nil {
		t.Fatalf("re-applying a completed discard must not error, got %v", err)
	}
	if r2[0].NoteID != "" {
		t.Fatalf("re-applied discard must skip without a new note, got %q", r2[0].NoteID)
	}
	if got := countNotes(t, root); got != notesBefore {
		t.Fatalf("re-apply must not duplicate the note: %d -> %d", notesBefore, got)
	}
}

// A failing decision must not block the rest: Apply accumulates
// per-decision errors, returns the successes, and reports m/n.
func TestApplyFailureDoesNotBlockRest(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_f%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	mine, err := Plan(root, scanAll(t, root))
	if err != nil {
		t.Fatal(err)
	}
	mr, err := Apply(root, mine, base.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	liveFact := mr[0].FactID
	if liveFact == "" {
		t.Fatal("setup mine must record a fact")
	}
	bad := Decision{
		Series:      mine[0].Series,
		Action:      ActionDiscard,
		Reason:      "contradicted",
		LiveFactIDs: []string{liveFact, "no-such-observation"},
	}
	goodSeries := Series{Key: Key{Kind: KindGate, Name: "other-gate", Domain: "sextant"}, Execs: []Execution{
		{Session: "s1", Failed: true, Evidence: "rec_a"},
		{Session: "s2", Failed: true, Evidence: "rec_b"},
		{Session: "s3", Failed: true, Evidence: "rec_c"},
	}}
	good := Decision{Series: goodSeries, Action: ActionMine}
	results, err := Apply(root, []Decision{bad, good}, base.Add(5*time.Hour))
	if err == nil {
		t.Fatal("Apply must report the failing decision")
	}
	msg := err.Error()
	if !strings.Contains(msg, "1/2 failed") {
		t.Fatalf("error must aggregate m/n with per-decision list, got %q", msg)
	}
	if !strings.Contains(msg, "discard") {
		t.Fatalf("error must name the failed decision, got %q", msg)
	}
	if len(results) != 1 || results[0].FactID == "" {
		t.Fatalf("the good decision must still apply, got %+v", results)
	}
	inv, err := almanac.InvalidatedIDs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !inv[liveFact] {
		t.Fatal("the valid target must still be retired even though the decision failed on the bogus id")
	}
}
