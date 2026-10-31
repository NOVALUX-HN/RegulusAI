package evolve

import (
	"strings"
	"testing"
)

// S6/C7: Note was cov0. Covers the basic path plus every refusal.
func TestNoteAdmitsEveryValidSource(t *testing.T) {
	for _, src := range []string{
		SourceFailedExecutions, SourceRepeatedFailures, SourceGuardianFindings,
		SourceGauntletResults, SourceExecutionCosts, SourceMemoryInsights, SourceTaskOutcomes,
	} {
		obs, err := Note(src, "demo/T1", "summary for "+src, []string{"e1", "e2"})
		if err != nil {
			t.Fatalf("Note(%s) must hold: %v", src, err)
		}
		if obs.Source != src || obs.Task != "demo/T1" {
			t.Fatalf("Note(%s) = %+v, want source+task echoed", src, obs)
		}
		if obs.Count != 2 || len(obs.Evidence) != 2 {
			t.Fatalf("Note(%s) count = %d, want 2 (len(evidence))", src, obs.Count)
		}
		if !obs.Generalizable() {
			t.Fatalf("Note(%s) with 2 items must be generalizable", src)
		}
	}
}

func TestNoteIsolatedEvidence(t *testing.T) {
	obs, err := Note(SourceGuardianFindings, "demo/T1", "one item", []string{"only"})
	if err != nil {
		t.Fatal(err)
	}
	if obs.Count != 1 {
		t.Fatalf("Count = %d, want 1", obs.Count)
	}
	if obs.Generalizable() {
		t.Fatal("a single item must not be generalizable")
	}
}

func TestNoteEmptyEvidence(t *testing.T) {
	obs, err := Note(SourceMemoryInsights, "demo/T1", "no items", nil)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Count != 0 {
		t.Fatalf("Count = %d, want 0", obs.Count)
	}
}

func TestNoteUnknownSource(t *testing.T) {
	if _, err := Note("NOPE", "demo/T1", "s", []string{"e"}); err == nil {
		t.Fatal("unknown source must be refused")
	} else if !strings.Contains(err.Error(), "unknown observation source") {
		t.Fatalf("refusal must name the source, got %v", err)
	}
}

func TestNoteNeedsTaskAndSummary(t *testing.T) {
	if _, err := Note(SourceFailedExecutions, "  ", "s", []string{"e"}); err == nil {
		t.Fatal("blank task must be refused")
	} else if !strings.Contains(err.Error(), "needs a task and a summary") {
		t.Fatalf("refusal must name task+summary, got %v", err)
	}
	if _, err := Note(SourceFailedExecutions, "demo/T1", "   ", []string{"e"}); err == nil {
		t.Fatal("blank summary must be refused")
	}
	if _, err := Note(SourceFailedExecutions, "", "", nil); err == nil {
		t.Fatal("empty task+summary must be refused")
	}
}
