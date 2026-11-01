package envfacts

// H1: unknown-verdict receipts never form a contradiction execution:
// skipped, counted as unattributed, never discarding a live fact.

import (
	"fmt"
	"testing"
	"time"

	"regulus-ai/internal/receipts"
)

func TestUnknownVerdictsNeverContradictFailures(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_f%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	// Unknown verdicts AFTER the failures: under the old
	// non-fail==clean rule these would read as a clean window and
	// falsely contradict the candidate.
	unknowns := []string{"", "error", "skip"}
	for i, v := range unknowns {
		writeReceipt(t, root, "feat/T1", "crap", v,
			fmt.Sprintf("run_u%d", i), base.Add(time.Duration(10+i)*time.Hour))
	}
	rs, st, err := ReceiptSeries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("three fails must form one series, got %+v", rs)
	}
	if len(rs[0].Execs) != 3 {
		t.Fatalf("unknown verdicts must not form executions, got %d", len(rs[0].Execs))
	}
	for _, e := range rs[0].Execs {
		if !e.Failed {
			t.Fatalf("remaining executions must all be failures, got %+v", rs[0].Execs)
		}
	}
	if st.Receipts != 6 || st.ReceiptsUnattributed != 3 {
		t.Fatalf("three unknowns must count as unattributed out of six, got %+v", st)
	}
	if IsContradicted(rs[0]) {
		t.Fatal("unknown verdicts must never contradict a failing series")
	}
	decisions, err := Plan(root, append(rs, mustTrace(t, root)...))
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 || decisions[0].Action != ActionMine {
		t.Fatalf("three fails plus unknowns must still mine, got %+v", decisions)
	}
}

func TestUnknownVerdictsAloneFormNoSeries(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	unknowns := []string{"", "error", "skip"}
	for i, v := range unknowns {
		writeReceipt(t, root, "feat/T1", "crap", v,
			fmt.Sprintf("run_u%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	rs, st, err := ReceiptSeries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 0 {
		t.Fatalf("unknown verdicts alone must form no series, got %+v", rs)
	}
	if st.Receipts != 3 || st.ReceiptsUnattributed != 3 {
		t.Fatalf("all three unknowns must count as unattributed, got %+v", st)
	}
}

func mustTrace(t *testing.T, root string) []Series {
	t.Helper()
	ts, _, err := TraceSeries(root)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}
