package bench

import (
	"testing"
)

func TestQualityCostSeparate(t *testing.T) {
	rep := controlled(Run("test", "abc123"))
	if len(rep.Tasks) != 14 {
		t.Fatalf("tasks = %d, want 14", len(rep.Tasks))
	}
	if !rep.ProxyKnown {
		t.Fatal("Report must mark proxy known (measured-local), host stays unknown")
	}
	if rep.TotalProxyTokens <= 0 {
		t.Fatalf("TotalProxyTokens = %d, want > 0", rep.TotalProxyTokens)
	}
	for _, tr := range rep.Tasks {
		if !tr.ProxyKnown {
			t.Fatalf("task %s must mark proxy known", tr.ID)
		}
		if tr.ProxyTokens <= 0 {
			t.Fatalf("task %s proxy = %d, want > 0", tr.ID, tr.ProxyTokens)
		}
	}
	if rep.Passed > 0 && rep.QualityPerKProxy <= 0 {
		t.Fatalf("QualityPerKProxy = %v, want > 0 when passes exist", rep.QualityPerKProxy)
	}
	// Unknown host tokens are never conflated: Report carries no host
	// token totals, only the measured-local proxy with its known flag.
}

func TestCompareReportsProxyDeltaWithoutJudging(t *testing.T) {
	base := controlled(Run("base", "abc"))
	cand := base
	cmp := Compare(base, cand)
	if cmp.DeltaProxy != 0 {
		t.Fatalf("identical reports delta = %d, want 0", cmp.DeltaProxy)
	}
	if cmp.Verdict != "ADOPT" {
		t.Fatalf("identical reports verdict = %q, want ADOPT", cmp.Verdict)
	}
	// Cost delta must not flip a quality verdict: same passes, different
	// proxy still ADOPTs because cost is reported, never judged.
	cand.TotalProxyTokens += 500
	cand.Tasks[0].ProxyTokens += 500
	cmp2 := Compare(base, cand)
	if cmp2.Verdict != "ADOPT" {
		t.Fatalf("proxy-only delta verdict = %q, want ADOPT", cmp2.Verdict)
	}
	if cmp2.DeltaProxy != 500 {
		t.Fatalf("proxy delta = %d, want 500", cmp2.DeltaProxy)
	}
}
