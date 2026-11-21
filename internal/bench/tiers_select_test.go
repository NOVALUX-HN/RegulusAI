package bench

import "testing"

func testTierCorpus() ([]AgentTask, AgentSplits) {
	corpus := []AgentTask{
		{ID: "AT1", Title: "t1", Lang: "go"},
		{ID: "AT14", Title: "t14", Lang: "node"},
		{ID: "AT19", Title: "t19", Lang: "shell"},
		{ID: "AT23", Title: "t23", Lang: "python"},
		{ID: "AT99", Title: "extra", Lang: "go"},
	}
	splits := AgentSplits{Version: "v", Seed: 1, Exploration: []string{"AT1", "AT14", "AT19", "AT23", "AT99"}, Holdout: []string{"H1"}}
	// Holdout H1 must exist in corpus for the unknown-ref check to pass otherwise;
	// for happy paths we use a corpus that includes it.
	corpusWithHoldout := append(append([]AgentTask(nil), corpus...), AgentTask{ID: "H1", Title: "h", Lang: "go"})
	return corpusWithHoldout, splits
}

// Covers SelectTierTasks error branches without losing precision.
func TestSelectTierTasksErrors(t *testing.T) {
	corpus, splits := testTierCorpus()

	// Unknown splits reference.
	badSplits := AgentSplits{Version: "v", Exploration: []string{"NOPE"}, Holdout: []string{}}
	if _, err := SelectTierTasks(TierL0, corpus, badSplits, nil, ""); err == nil {
		t.Fatal("unknown splits ref must fail")
	}

	// Unknown tier.
	if _, err := SelectTierTasks("bogus", corpus, splits, nil, ""); err == nil {
		t.Fatal("unknown tier must fail")
	}

	// Smoke task not in corpus.
	thinCorpus := []AgentTask{{ID: "AT1", Title: "t1", Lang: "go"}}
	thinSplits := AgentSplits{Version: "v", Exploration: []string{"AT1"}, Holdout: []string{"H1"}}
	thinCorpus = append(thinCorpus, AgentTask{ID: "H1", Title: "h", Lang: "go"})
	if _, err := SelectTierTasks(TierL0, thinCorpus, thinSplits, nil, ""); err == nil {
		t.Fatal("smoke missing task must fail")
	}

	// Smoke task sealed in holdout.
	sealedCorpus := []AgentTask{
		{ID: "AT1", Title: "t1", Lang: "go"},
		{ID: "AT14", Title: "t14", Lang: "node"},
		{ID: "AT19", Title: "t19", Lang: "shell"},
		{ID: "AT23", Title: "t23", Lang: "python"},
		{ID: "H1", Title: "h", Lang: "go"},
	}
	sealedSplits := AgentSplits{Version: "v", Exploration: []string{"AT14", "AT19", "AT23"}, Holdout: []string{"AT1", "H1"}}
	if _, err := SelectTierTasks(TierL0, sealedCorpus, sealedSplits, nil, ""); err == nil {
		t.Fatal("smoke holdout task must fail")
	}

	// L1 filter matches nothing (docs-only diff maps to ["docs"], no task carries it).
	if _, err := SelectTierTasks(TierL1, corpus, splits, []string{"README.md"}, ""); err == nil {
		t.Fatal("L1 empty match must fail")
	}

	// L1 happy with capability + L2/L3 happy cover omit + seal.
	if _, err := SelectTierTasks(TierL1, corpus, splits, nil, "go"); err != nil {
		t.Fatalf("L1 go must pass: %v", err)
	}
	if _, err := SelectTierTasks(TierL2, corpus, splits, nil, ""); err != nil {
		t.Fatalf("L2 must pass: %v", err)
	}
	sel, err := SelectTierTasks(TierL3, corpus, splits, nil, "")
	if err != nil {
		t.Fatalf("L3 must pass: %v", err)
	}
	if len(sel.Included) == 0 {
		t.Fatal("L3 must include tasks")
	}
}
