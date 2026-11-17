// Mission benchmark tests (F13.8/NOV-111): corpus pinning,
// splits discipline, journey outcomes, verdict rules and the
// holdout seal. Tampering with the corpus or touching holdout
// unflagged fails here before it can fail the gate.
package mbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCorpusPinned(t *testing.T) {
	corpus := MissionCorpus()
	if len(corpus) != 13 {
		t.Fatalf("corpus = %d cases (want 10 exploration + 3 holdout)", len(corpus))
	}
	if got := len(Exploration()); got != 10 {
		t.Fatalf("exploration = %d (want 10)", got)
	}
	classes := map[string]bool{}
	for _, c := range Exploration() {
		classes[c.Class] = true
	}
	for _, want := range []string{"bugfix", "feature", "refactor-multi", "permissions", "approvals", "red-gate", "agent-offline", "ambiguity", "rollback", "parallel-conflict"} {
		if !classes[want] {
			t.Fatalf("exploration misses class %q", want)
		}
	}
	if SealHash(corpus) == "" {
		t.Fatal("corpus seal empty")
	}
}

func TestSplitsDiscipline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "splits.json")
	data := `{"version":"mission-splits-v1","seed":20261117,` +
		`"exploration":["MB1","MB2","MB3","MB4","MB5","MB6","MB7","MB8","MB9","MB10"],` +
		`"holdout":["MH1","MH2","MH3"]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	sp, err := LoadMissionSplits(path, MissionCorpus())
	if err != nil {
		t.Fatal(err)
	}
	if len(sp.Holdout) != 3 {
		t.Fatalf("holdout = %d (want 3, >=20%%)", len(sp.Holdout))
	}
	if _, err := SelectSplits(sp, MissionCorpus(), "holdout", false); err == nil {
		t.Fatal("holdout without the explicit flag must be refused")
	}
	if _, err := SelectSplits(sp, MissionCorpus(), "all", false); err == nil {
		t.Fatal("all (includes holdout) without the flag must be refused")
	}
	exp, err := SelectSplits(sp, MissionCorpus(), "exploration", false)
	if err != nil || len(exp) != 10 {
		t.Fatalf("exploration select = %d, %v", len(exp), err)
	}
	hold, err := SelectSplits(sp, MissionCorpus(), "holdout", true)
	if err != nil || len(hold) != 3 {
		t.Fatalf("flagged holdout select = %d, %v", len(hold), err)
	}
	if _, err := SelectSplits(sp, MissionCorpus(), "nope", false); err == nil {
		t.Fatal("unknown split must fail")
	}
}

func TestSplitsRejectOverlap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "splits.json")
	data := `{"version":"mission-splits-v1","seed":1,` +
		`"exploration":["MB1","MB2","MB3","MB4","MB5","MB6","MB7","MB8","MB9","MB10","MH1"],` +
		`"holdout":["MH1","MH2","MH3"]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMissionSplits(path, MissionCorpus()); err == nil {
		t.Fatal("overlapping splits must fail validation")
	}
}

func TestSuitePassesExploration(t *testing.T) {
	if testing.Short() {
		t.Skip("suite journeys need full mode")
	}
	results := RunSuite(Exploration())
	for _, r := range results {
		if !r.AssistedOK || !r.EvidenceValid || r.Violations != 0 {
			t.Fatalf("case %s not green: %+v", r.CaseID, r)
		}
	}
	v := Aggregate(results, MissionCorpus())
	if v.Decision != DecisionPass {
		t.Fatalf("gate = %s (want PASS):\n%s", v.Decision, v.Render())
	}
	if v.N != 10 || v.EvidenceRate != 1 || v.Violations != 0 {
		t.Fatalf("weak verdict: %+v", v)
	}
}

func TestVerdictFailsOnViolation(t *testing.T) {
	results := []CaseResult{{CaseID: "MB4", Class: "permissions", AssistedOK: true, EvidenceValid: true, Violations: 1}}
	v := Aggregate(results, MissionCorpus())
	if v.Decision != DecisionFail {
		t.Fatalf("gate = %s (want FAIL on violations)", v.Decision)
	}
}

func TestVerdictInsufficientOnEmpty(t *testing.T) {
	v := Aggregate(nil, MissionCorpus())
	if v.Decision != DecisionInsufficient {
		t.Fatalf("gate = %s (want INSUFFICIENT_EVIDENCE on N=0)", v.Decision)
	}
}

func TestVerdictInsufficientOnMissingClass(t *testing.T) {
	results := RunSuite(Exploration()[:5])
	v := Aggregate(results, MissionCorpus())
	if v.Decision != DecisionInsufficient {
		t.Fatalf("gate = %s (want INSUFFICIENT_EVIDENCE on uncovered classes)", v.Decision)
	}
}
