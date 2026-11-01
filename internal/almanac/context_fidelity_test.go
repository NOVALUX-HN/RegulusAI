package almanac

// T1 (macizo-tokens): the bounded default resume must stay faithful —
// pins, latest decisions and active-domain lessons survive the cap while
// the block shrinks. This test compares the explicit-full resume against
// the bounded one on essential content.

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBoundedResumeKeepsEssentials(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	// 30 notes and 25 typed items: both lists exceed the 20/20 defaults.
	var oldestNote, oldestTyped Observation
	for i := 1; i <= 30; i++ {
		o, err := Save(root, &Observation{
			Type: TypeSessionNote, Title: fmt.Sprintf("note-%02d", i),
			Content: fmt.Sprintf("fidelity-note-%02d body", i),
		}, at)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			oldestNote = o
		}
		at = at.Add(time.Minute)
	}
	for i := 1; i <= 25; i++ {
		typ := TypeDecision
		if i%2 == 0 {
			typ = TypeDiscovery
		}
		o, err := Save(root, &Observation{
			Type: typ, Title: fmt.Sprintf("typed-%02d", i),
			Content: fmt.Sprintf("fidelity-typed-%02d body", i),
		}, at)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			oldestTyped = o
		}
		at = at.Add(time.Minute)
	}
	// Lessons in two domains: neither list is capped, both survive.
	for _, d := range []string{"go", "ts"} {
		if _, err := Save(root, &Observation{
			Type: TypeLesson, Title: d + " lesson", Content: d + " content", Domain: d,
			Lesson: &Lesson{Cue: "fidelity-cue-" + d, Lesson: "fidelity-lesson-" + d, Evidence: []string{"r.json"}, Confidence: "high"},
		}, at); err != nil {
			t.Fatal(err)
		}
		at = at.Add(time.Minute)
	}
	// Pins must surface even when their items fall outside the cap.
	if err := Pin(root, oldestNote.ID, true, at); err != nil {
		t.Fatal(err)
	}
	if err := Pin(root, oldestTyped.ID, true, at); err != nil {
		t.Fatal(err)
	}
	now := at.Add(time.Hour)

	full, err := ContextScoped(root, now, 0, ContextScope{})
	if err != nil {
		t.Fatal(err)
	}
	bounded, err := ContextScoped(root, now, DefaultContextLimit, ContextScope{Budget: DefaultContextBudget})
	if err != nil {
		t.Fatal(err)
	}

	// 1. The bounded block is strictly smaller.
	if len(bounded) >= len(full) {
		t.Fatalf("bounded resume must shrink: full %d bytes, bounded %d bytes", len(full), len(bounded))
	}

	// 2. Pins survive even though they are the oldest items.
	for _, id := range []string{oldestNote.ID, oldestTyped.ID} {
		if !strings.Contains(bounded, id) {
			t.Fatalf("bounded resume must keep pinned %s, got:\n%s", id, bounded)
		}
	}

	// 3. Latest work survives on both lists.
	for _, want := range []string{"fidelity-note-30 body", "fidelity-typed-25 body"} {
		if !strings.Contains(bounded, want) {
			t.Fatalf("bounded resume must keep latest %q, got:\n%s", want, bounded)
		}
	}

	// 4. Both domains' lessons survive with cue AND content.
	for _, want := range []string{"fidelity-cue-go", "fidelity-lesson-go", "fidelity-cue-ts", "fidelity-lesson-ts"} {
		if !strings.Contains(bounded, want) {
			t.Fatalf("bounded resume must keep lesson content %q, got:\n%s", want, bounded)
		}
	}

	// 5. Every cap is named with its repair.
	if !strings.Contains(bounded, fmt.Sprintf("showing latest %d of 30", DefaultContextLimit)) {
		t.Fatalf("bounded resume must name the notes cap, got:\n%s", bounded)
	}
	if !strings.Contains(bounded, "narrow with --task/--path/--system") {
		t.Fatalf("bounded resume must name the decisions repair, got:\n%s", bounded)
	}

	// 6. The oldest unpinned items are cut from the lists (pins keep
	// them visible only under Pinned: — exactly once each outside it).
	if strings.Contains(bounded, "fidelity-note-01 body") {
		t.Fatalf("bounded resume must cut the oldest note from the list, got:\n%s", bounded)
	}
	if strings.Contains(bounded, "fidelity-typed-01 body") {
		t.Fatalf("bounded resume must cut the oldest typed item from the list, got:\n%s", bounded)
	}

	// 7. Explicit 0/0 keeps the full store (the escape hatch).
	for _, want := range []string{"fidelity-note-01 body", "fidelity-typed-01 body"} {
		if !strings.Contains(full, want) {
			t.Fatalf("explicit-full resume must show %q, got:\n%s", want, full)
		}
	}
	if strings.Contains(full, "showing latest") || strings.Contains(full, "scoped:") {
		t.Fatalf("explicit-full resume must not claim truncation, got:\n%s", full)
	}
}
