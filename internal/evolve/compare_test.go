package evolve

// Snapshots let evolution economics compound over time: `evolve
// report --save` pins today's numbers, `--compare` diffs a saved
// baseline against the live store — proxy deltas, evidence-state
// changes, added/dropped lessons. Deterministic from the two reports;
// the store is only read for the live side.
import (
	"testing"
)

func TestCompareReportsDiffs(t *testing.T) {
	base := Report{TotalProxy: 15, Admitted: 2, Lessons: []LessonRow{
		{ID: "a", Domain: "go", Cue: "old", ProxyTokens: 10, Evidence: EvidenceOK},
		{ID: "b", Domain: "go", Cue: "gone", ProxyTokens: 5, Evidence: EvidenceOK},
	}}
	cand := Report{TotalProxy: 19, Admitted: 2, Lessons: []LessonRow{
		{ID: "a", Domain: "go", Cue: "old", ProxyTokens: 12, Evidence: EvidenceRot},
		{ID: "c", Domain: "go", Cue: "new", ProxyTokens: 7, Evidence: EvidenceOK},
	}}
	got := CompareReports(base, cand)
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want 3 (a changed, b dropped, c added)", len(got.Rows))
	}
	byID := map[string]ReportRow{}
	for _, r := range got.Rows {
		byID[r.ID] = r
	}
	if byID["a"].DeltaProxy != 2 || byID["a"].BaseEvidence != EvidenceOK || byID["a"].CandEvidence != EvidenceRot {
		t.Errorf("row a = %+v, want delta +2 ok->rot", byID["a"])
	}
	if !byID["b"].Dropped || byID["b"].BaseProxy != 5 {
		t.Errorf("row b = %+v, want dropped", byID["b"])
	}
	if !byID["c"].Added || byID["c"].CandProxy != 7 {
		t.Errorf("row c = %+v, want added", byID["c"])
	}
	if got.DeltaProxy != 4 || got.DeltaAdmitted != 0 {
		t.Errorf("totals = %+v, want deltaProxy +4 admitted +0", got)
	}
}
