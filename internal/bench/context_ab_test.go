package bench

// NOV-39 (S14): A/B context budget 20/20 vs 10/10 over one fixture
// store, quality-first. The store reuses the B11 core (2 notes +
// 1 evidence-gated lesson, same titles/cue) scaled past the 10-cap so
// both budgets are exercised; essentials are asserted
// TestBoundedResumeKeepsEssentials-style (pins, latest work, lesson
// cue AND content survive). The 20/20 baseline must keep essentials —
// losing them fails the test. If 10/10 loses essentials the test
// declares NOT ADOPTABLE and still passes: that verdict IS the valid
// result (quality first, savings never judged over fidelity).
// DeltaProxy (proxy20 - proxy10, chars/4) is reported, never judged.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
)

// abFixtureBuilds seeds one deterministic store: the exact B11 core
// trio plus filler notes so the store (12 notes) exceeds the 10-cap
// while staying within the 20-cap. Fixed timestamps keep byte counts
// comparable between the two assemblies (same root, same header).
func abFixtureBuilds(t *testing.T, root string) (oldestID string) {
	t.Helper()
	base := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	notes := []almanac.Observation{
		{Type: almanac.TypeSessionNote, Title: "harbor deploy done", Content: "Deploy finished cleanly; canary watched for ten minutes."},
		{Type: almanac.TypeSessionNote, Title: "rollback drill done", Content: "Rollback drill finished; restoring service is routine now."},
	}
	for i := 3; i <= 12; i++ {
		notes = append(notes, almanac.Observation{
			Type: almanac.TypeSessionNote, Title: fmt.Sprintf("ab-note-%02d", i),
			Content: fmt.Sprintf("ab filler note %02d body; routine deploy log line.", i),
		})
	}
	for i, o := range notes {
		saved, err := almanac.Save(root, &o, base.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatalf("seed note %d: %v", i, err)
		}
		if i == 0 {
			oldestID = saved.ID
		}
	}
	lesson := almanac.Observation{
		Type: almanac.TypeLesson, Title: "harbor rollback first",
		Content: "When the harbor deploy fails, roll back before investigating.",
		Lesson: &almanac.Lesson{Cue: "harbor deploy fails", Lesson: "Roll back first, questions later.",
			Evidence: []string{"rec_fixture/1"}, Confidence: "high", Dimension: "dom"},
	}
	if _, err := almanac.Save(root, &lesson, base.Add(time.Duration(len(notes))*time.Second)); err != nil {
		t.Fatalf("seed lesson: %v", err)
	}
	now := base.Add(time.Duration(len(notes)+1) * time.Second)
	if err := almanac.Pin(root, oldestID, true, now); err != nil {
		t.Fatalf("pin oldest: %v", err)
	}
	return oldestID
}

// abEssentials asserts the fidelity contract on one assembled block:
// the pinned oldest note surfaces, the latest work survives, and the
// lesson keeps its cue AND its content (never a count-only line).
func abEssentials(block, oldestID string) error {
	for _, want := range []string{
		oldestID,                            // pinned oldest survives the cap
		"ab filler note 12 body",            // latest work survives
		"harbor deploy fails",               // lesson cue
		"Roll back first, questions later.", // lesson content
	} {
		if !strings.Contains(block, want) {
			return fmt.Errorf("block lost essential %q (%d chars)", want, len(block))
		}
	}
	return nil
}

func abProxy(block string) int64 {
	return (int64(len(block)) + 3) / 4
}

func TestContextBudgetABQualityFirst(t *testing.T) {
	// B11 still gates its golden: the baseline price the A/B diffs
	// against stays pinned.
	b11, err := checkLessonContextCost()
	if err != nil {
		t.Fatalf("B11 baseline check failed: %v", err)
	}
	t.Logf("B11 baseline: %s", b11)

	root := t.TempDir()
	oldestID := abFixtureBuilds(t, root)
	now := time.Date(2026, 11, 4, 13, 0, 0, 0, time.UTC)

	std, err := almanac.ContextScoped(root, now, 20, almanac.ContextScope{Budget: 20})
	if err != nil {
		t.Fatalf("assemble 20/20: %v", err)
	}
	min, err := almanac.ContextScoped(root, now, 10, almanac.ContextScope{Budget: 10})
	if err != nil {
		t.Fatalf("assemble 10/10: %v", err)
	}

	// The 10-cap must actually bite: 12 notes exceed it.
	if !strings.Contains(min, "showing latest 10 of 12") {
		t.Fatalf("10/10 block must name its notes cap, got:\n%s", min)
	}
	if strings.Contains(std, "showing latest") {
		t.Fatalf("20/20 block must not cap 12 notes, got:\n%s", std)
	}

	// Baseline first: 20/20 losing essentials is a real regression.
	if err := abEssentials(std, oldestID); err != nil {
		t.Fatalf("baseline 20/20 lost essentials (regression): %v", err)
	}

	proxyStd, proxyMin := abProxy(std), abProxy(min)
	delta := proxyStd - proxyMin
	t.Logf("A/B 20/20 vs 10/10: %d chars = %d proxy tokens vs %d chars = %d proxy tokens, DeltaProxy = %d",
		len(std), proxyStd, len(min), proxyMin, delta)

	// Quality-first verdict: 10/10 losing essentials is NOT ADOPTABLE,
	// and that verdict passes the test — savings never override fidelity.
	if err := abEssentials(min, oldestID); err != nil {
		t.Logf("verdict: NOT ADOPTABLE — 10/10 loses essentials (%v); quality first, no savings claimed", err)
		return
	}
	if delta < 0 {
		t.Fatalf("10/10 block larger than 20/20 (+%d proxy): the cap must not fatten the block", -delta)
	}
	t.Logf("verdict: ADOPTABLE (essentials preserved) — DeltaProxy = %d proxy tokens on this fixture; proxy only, host counts unknown", delta)
}
