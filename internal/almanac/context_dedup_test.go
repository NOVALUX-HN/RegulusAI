package almanac

// harness-total/T2: the resume dedups revision chains against history —
// one topic_key lists only its latest revision, pins still surface the
// old revision, and empty topic keys never collapse.

import (
	"strings"
	"testing"
	"time"
)

func TestDedupHistoryKeepsLatestPerChain(t *testing.T) {
	all := []Observation{
		{ID: "obs_a", TopicKey: "k", Content: "old", SavedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "obs_b", TopicKey: "", Content: "lone", SavedAt: time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)},
		{ID: "obs_c", TopicKey: "k", Content: "new", SavedAt: time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)},
	}
	got := dedupHistory(all)
	if len(got) != 2 {
		t.Fatalf("one chain of 2 + one lone must list 2, got %d: %+v", len(got), got)
	}
	for _, o := range got {
		if o.Content == "old" {
			t.Fatalf("old revision must hide, got %+v", got)
		}
	}
}

func TestContextDedupsHistoryChainsButKeepsPins(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	old, err := Save(root, &Observation{Type: TypeDecision, Title: "chain", Content: "chain-old body", TopicKey: "t2-chain"}, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "chain", Content: "chain-new body", TopicKey: "t2-chain"}, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, &Observation{Type: TypeSessionNote, Title: "lone", Content: "lone-note body"}, base.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Pin(root, old.ID, true, base.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	now := base.Add(time.Hour)
	full, err := ContextScoped(root, now, 0, ContextScope{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(full, "chain-old body") && !strings.Contains(full, "Pinned:") {
		t.Fatalf("old revision must hide from live lists (pins keep it under Pinned:), got:\n%s", full)
	}
	if !strings.Contains(full, "chain-new body") {
		t.Fatalf("latest revision must list, got:\n%s", full)
	}
	if !strings.Contains(full, "Pinned:") || !strings.Contains(full, old.ID) {
		t.Fatalf("pinned old revision must surface under Pinned:, got:\n%s", full)
	}
	if !strings.Contains(full, "lone-note body") {
		t.Fatalf("untagged notes never collapse, got:\n%s", full)
	}
	// Bounded stays bounded and deduped: latest + lone, old only under Pinned:.
	bounded, err := ContextScoped(root, now, DefaultContextLimit, ContextScope{Budget: DefaultContextBudget})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bounded, "chain-new body") || !strings.Contains(bounded, old.ID) {
		t.Fatalf("bounded must keep latest + pinned old, got:\n%s", bounded)
	}
}
