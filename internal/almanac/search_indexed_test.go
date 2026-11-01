package almanac

// M2 phase 2: lecturas acotadas — parity indexed↔legacy (conducta
// idéntica) + bounded-reads tripwire (líneas leídas << total).

import (
	"os"
	"testing"
	"time"
)

// parityFixture seeds one root with every bucket kind: topics, domains,
// types, lesson dimensions, provenance paths, pins, supersede hiding,
// invalidated/rejected/expired staleness. Returned for id reference.
type parityFixture struct {
	noteK1    Observation
	noteFree  Observation
	decGo     Observation
	decTs     Observation
	discGoK1  Observation
	lessonDom Observation
	lessonInv Observation
	expired   Observation
	rejected  Observation
	invalid   Observation
	oldSup    Observation
	newSup    Observation
}

func seedParityFixture(t *testing.T, root string) parityFixture {
	t.Helper()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	must := func(o *Observation, at time.Time) Observation {
		t.Helper()
		saved, err := Save(root, o, at)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	var f parityFixture
	f.noteK1 = must(&Observation{Type: TypeSessionNote, Title: "postgres pooling note", Content: "pool sizing today", TopicKey: "k1"}, base)
	f.noteFree = must(&Observation{Type: TypeSessionNote, Title: "random errand", Content: "buy milk"}, base.Add(time.Minute))
	f.decGo = must(&Observation{Type: TypeDecision, Title: "chose postgres pooling", Content: "pooling for the db", Domain: "go",
		Provenance: &Provenance{Paths: []string{"internal/db"}}}, base.Add(2*time.Minute))
	f.decTs = must(&Observation{Type: TypeDecision, Title: "chose bun runtime", Content: "fast startup", Domain: "ts"}, base.Add(3*time.Minute))
	f.discGoK1 = must(&Observation{Type: TypeDiscovery, Title: "pool leak found", Content: "postgres pool under load", TopicKey: "k1", Domain: "go",
		Provenance: &Provenance{Paths: []string{"internal/db"}}}, base.Add(4*time.Minute))
	f.lessonDom = must(&Observation{Type: TypeLesson, Title: "pool lesson", Content: "size pools evidence",
		Lesson: &Lesson{Cue: "pooling", Lesson: "measure first", Evidence: []string{"rec_1"}, Confidence: "high", Dimension: "dom"}}, base.Add(5*time.Minute))
	f.lessonInv = must(&Observation{Type: TypeLesson, Title: "gate lesson", Content: "receipt first always",
		Lesson: &Lesson{Cue: "gates", Lesson: "prove it", Evidence: []string{"rec_2"}, Confidence: "low", Dimension: "inv"}}, base.Add(6*time.Minute))
	past := base.Add(-time.Hour)
	f.expired = must(&Observation{Type: TypeDiscovery, Title: "stale cache fact", Content: "ttl zero", ExpiresAt: &past}, base.Add(7*time.Minute))
	f.rejected = must(&Observation{Type: TypeDecision, Title: "rejected approach", Content: "too slow"}, base.Add(8*time.Minute))
	if err := RecordStatus(root, f.rejected.ID, StatusRejected, "ana", "too slow", base.Add(9*time.Minute)); err != nil {
		t.Fatal(err)
	}
	f.invalid = must(&Observation{Type: TypeDiscovery, Title: "invalidated claim", Content: "wrong metric"}, base.Add(10*time.Minute))
	f.oldSup = must(&Observation{Type: TypeDecision, Title: "old postgres choice", Content: "first pooling pick"}, base.Add(11*time.Minute))
	f.newSup = must(&Observation{Type: TypeDecision, Title: "new postgres choice", Content: "second pooling pick"}, base.Add(12*time.Minute))
	if err := Relate(root, f.newSup.ID, RelationSupersedes, f.oldSup.ID, base.Add(13*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, f.newSup.ID, RelationInvalidatedBy, f.invalid.ID, base.Add(14*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Pin(root, f.decGo.ID, true, base.Add(15*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return f
}

func parityQueries() []Query {
	return []Query{
		{Terms: []string{"postgres"}},
		{Terms: []string{"pooling", "postgres"}},
		{Terms: []string{"pooling"}, Any: true},
		{TopicKey: "k1"},
		{Domain: "go"},
		{Domain: "ts"},
		{Type: TypeDecision},
		{Type: TypeLesson, Dimension: "dom"},
		{Dimension: "inv"},
		{System: SystemDecision},
		{System: SystemEvolution},
		{Path: "internal/db"},
		{Terms: []string{"pool"}, Domain: "go", Limit: 2},
		{Terms: []string{"choice"}, Latest: true},
		{Limit: 3},
		{Terms: []string{"postgres"}, IncludeStale: true},
		{Terms: []string{"postgres"}, IncludeSuperseded: true},
		{Terms: []string{"postgres"}, IncludeStale: true, IncludeSuperseded: true, Limit: 10, Latest: true},
		{Terms: []string{"zzz-no-match"}},
	}
}

func idsOf(hits []Observation) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.ID)
	}
	return out
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSearchParityIndexedVsLegacy: the indexed path (fresh v2) returns
// bit-identical orders to the legacy full scan across every bucket,
// flag and limit combination. Conducta idéntica, probada.
func TestSearchParityIndexedVsLegacy(t *testing.T) {
	root := t.TempDir()
	seedParityFixture(t, root)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	for i, q := range parityQueries() {
		indexed, lines, ok, err := SearchWithStats(root, q)
		if err != nil {
			t.Fatalf("q%d: indexed err %v", i, err)
		}
		if !ok {
			t.Fatalf("q%d: fresh index must serve indexed", i)
		}
		_ = lines
		if err := os.Remove(IndexPath(root)); err != nil {
			t.Fatal(err)
		}
		legacy, _, _, err := SearchWithStats(root, q)
		if err != nil {
			t.Fatalf("q%d: legacy err %v", i, err)
		}
		if _, _, err := Reindex(root); err != nil {
			t.Fatal(err)
		}
		if !equalIDs(idsOf(indexed), idsOf(legacy)) {
			t.Fatalf("q%d %+v: indexed %v vs legacy %v", i, q, idsOf(indexed), idsOf(legacy))
		}
	}
}

// TestHistoryParityIndexedVsLegacy: the chain reads identical in file
// order from offsets or from the full scan.
func TestHistoryParityIndexedVsLegacy(t *testing.T) {
	root := t.TempDir()
	seedParityFixture(t, root)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	indexed, served := historyIndexed(root, "k1")
	if !served {
		t.Fatal("fresh index must serve history indexed")
	}
	if err := os.Remove(IndexPath(root)); err != nil {
		t.Fatal(err)
	}
	legacy, err := History(root, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if !equalIDs(idsOf(indexed), idsOf(legacy)) {
		t.Fatalf("history indexed %v vs legacy %v", idsOf(indexed), idsOf(legacy))
	}
	if len(legacy) != 2 {
		t.Fatalf("chain k1 = %d, want 2", len(legacy))
	}
}

// TestHybridCorpusParityIndexedVsLegacy: the indexed corpus carries the
// same observations and the same hidden/stale sets as the legacy single
// load (relations + status + expiry included).
func TestHybridCorpusParityIndexedVsLegacy(t *testing.T) {
	root := t.TempDir()
	seedParityFixture(t, root)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	entries, ok := freshEntriesIfPinned(root)
	if !ok {
		t.Fatal("index must be fresh")
	}
	for _, q := range []Query{{}, {Domain: "go"}, {Terms: []string{"postgres"}}} {
		ic, served, err := indexedCorpusFor(root, q, entries, now)
		if err != nil || !served {
			t.Fatalf("q %+v: indexed corpus served=%v err=%v", q, served, err)
		}
		legacy, err := loadHybridCorpus(root, now)
		if err != nil {
			t.Fatal(err)
		}
		if !equalIDs(idsOf(ic.all), idsOf(legacy.all)) {
			t.Fatalf("q %+v: corpus %v vs %v", q, idsOf(ic.all), idsOf(legacy.all))
		}
		if !equalIDs(setIDs(ic.hidden), setIDs(legacy.hidden)) {
			t.Fatalf("q %+v: hidden %v vs %v", q, setIDs(ic.hidden), setIDs(legacy.hidden))
		}
		if !equalIDs(setIDs(ic.stale), setIDs(legacy.stale)) {
			t.Fatalf("q %+v: stale %v vs %v", q, setIDs(ic.stale), setIDs(legacy.stale))
		}
	}
}

func setIDs(m map[string]bool) []string {
	var out []string
	for id, on := range m {
		if on {
			out = append(out, id)
		}
	}
	// Order-insensitive compare: sort both via insertion into a slice.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// TestBoundedReadsDomainScoped: on a 1k log with 100 domain hits, the
// scoped query parses ~100 lines indexed vs 1000 legacy — lecturas
// acotadas, medidas.
func TestBoundedReadsDomainScoped(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		o := &Observation{Type: TypeDecision, Title: "filler decision", Content: "background noise"}
		if i%10 == 0 {
			o.Domain = "go"
			o.Title = "go target decision"
			o.Content = "postgres pooling target"
		}
		if _, err := Save(root, o, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	q := Query{Domain: "go", Terms: []string{"postgres"}}
	hits, lines, indexed, err := SearchWithStats(root, q)
	if err != nil {
		t.Fatal(err)
	}
	if !indexed {
		t.Fatal("fresh index must serve scoped query indexed")
	}
	if len(hits) != 100 {
		t.Fatalf("hits = %d, want 100", len(hits))
	}
	t.Logf("scoped read: lines=%d total=1000 hits=%d", lines, len(hits))
	if lines != 100 {
		t.Fatalf("scoped query must read exactly the 100 candidates, read %d", lines)
	}
	if lines*4 >= 1000 {
		t.Fatalf("bounded reads tripwire: lines=%d must be << total=1000", lines)
	}
	// Unscoped terms-only still serves (candidates = all): parity holds.
	all, allLines, indexedAll, err := SearchWithStats(root, Query{Terms: []string{"postgres"}})
	if err != nil || !indexedAll {
		t.Fatalf("unscoped indexed serve: hits=%d indexed=%v err=%v", len(all), indexedAll, err)
	}
	if allLines != 1000 || len(all) != 100 {
		t.Fatalf("unscoped: lines=%d hits=%d, want 1000/100", allLines, len(all))
	}
}

// TestSearchFallbackOnSkew: a prune without reindex skews offsets —
// Search falls back to the legacy path with identical results, and the
// stats path reports indexed=false.
func TestSearchFallbackOnSkew(t *testing.T) {
	root := t.TempDir()
	seedParityFixture(t, root)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPrune(root, []Observation{all[0]}); err != nil {
		t.Fatal(err)
	}
	q := Query{Terms: []string{"postgres"}}
	hits, lines, indexed, err := SearchWithStats(root, q)
	if err != nil {
		t.Fatal(err)
	}
	if indexed {
		t.Fatal("skewed index must NOT serve indexed (fallback to legacy)")
	}
	if lines == 0 {
		t.Fatal("fallback must still read the log")
	}
	// The fallback serves the CURRENT log exactly like the legacy path.
	current, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	want, err := rankFiltered(current, root, q)
	if err != nil {
		t.Fatal(err)
	}
	if !equalIDs(idsOf(hits), idsOf(want)) {
		t.Fatalf("fallback %v vs legacy %v", idsOf(hits), idsOf(want))
	}
}
