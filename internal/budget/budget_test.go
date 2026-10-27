package budget

import (
	"path/filepath"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)

func TestFile(t *testing.T) {
	if got, want := File("root"), filepath.Join("root", ".regulus", "budget.json"); got != want {
		t.Fatalf("File = %q, want %q", got, want)
	}
}

func TestRecordAppendsAndShowReadsAll(t *testing.T) {
	root := t.TempDir()
	e1 := Entry{Kind: "reflect", Timestamp: t0, Task: "feature/T1", Domain: "go", Outcome: "candidate-admitted", Detail: "2 receipts"}
	e2 := Entry{Kind: "reflect", Timestamp: t0.Add(time.Minute), Task: "feature/T2", Domain: "go", Outcome: "candidate-admitted"}
	if err := Record(root, e1); err != nil {
		t.Fatal(err)
	}
	if err := Record(root, e2); err != nil {
		t.Fatal(err)
	}
	entries, err := Show(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %+v", entries)
	}
	if entries[0].Task != "feature/T1" || entries[1].Task != "feature/T2" {
		t.Fatalf("entries must be oldest first, got %+v", entries)
	}
	if entries[0].Detail != "2 receipts" || entries[0].Domain != "go" {
		t.Fatalf("entry fields must round-trip, got %+v", entries[0])
	}
}

func TestShowMissingLedgerIsEmpty(t *testing.T) {
	entries, err := Show(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("want an empty ledger, got %+v", entries)
	}
}

func TestTodayEvolutionCountExcludesRefusedOtherKindsAndDays(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	if err := Record(root, Entry{Kind: KindReflect, Timestamp: base, Outcome: OutcomeCandidateProposed}); err != nil {
		t.Fatal(err)
	}
	if err := Record(root, Entry{Kind: KindProbe, Timestamp: base.Add(time.Hour), Outcome: OutcomeProbeRun}); err != nil {
		t.Fatal(err)
	}
	if err := Record(root, Entry{Kind: KindReflect, Timestamp: base.Add(2 * time.Hour), Outcome: OutcomeRefusedCap}); err != nil {
		t.Fatal(err)
	}
	if err := Record(root, Entry{Kind: "other", Timestamp: base.Add(3 * time.Hour), Outcome: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := Record(root, Entry{Kind: KindReflect, Timestamp: base.Add(-25 * time.Hour), Outcome: OutcomeCandidateProposed}); err != nil {
		t.Fatal(err)
	}
	n, err := TodayEvolutionCount(root, base.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("TodayEvolutionCount = %d, want 2 (refused, other kinds and yesterday excluded)", n)
	}
	if n, _ := TodayEvolutionCount(root, base.Add(-48*time.Hour)); n != 0 {
		t.Fatalf("a day with no entries = %d, want 0", n)
	}
	if n, _ := TodayEvolutionCount(t.TempDir(), base); n != 0 {
		t.Fatalf("empty ledger count = %d, want 0", n)
	}
}
