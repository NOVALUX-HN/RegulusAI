package almanac

// A3 (almanac-prime): retrieval misses are logged, not shrugged.
// Every `memory search` with zero hits appends its terms here; the
// doctor reports the count plus the most-missed term sets — the repair
// pointer (save the fact, reword the query, reindex) writes itself.
// Tiny append-only JSONL beside the log: local runtime state like
// traces, never committed, never breaking search when unwritable.
import (
	"testing"
	"time"
)

func TestMissStatsFold(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for i, terms := range [][]string{{"harbor", "rollback"}, {"harbor", "rollback"}, {"zebracorn"}} {
		if err := RecordMiss(root, terms, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	count, top, err := MissStats(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("misses = %d, want 3", count)
	}
	if len(top) == 0 || top[0] != "harbor rollback" {
		t.Fatalf("top missed = %q, want harbor rollback first", top)
	}
}

func TestMissStatsEmpty(t *testing.T) {
	count, top, err := MissStats(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || len(top) != 0 {
		t.Fatalf("empty misses = %d %q, want 0 []", count, top)
	}
}
