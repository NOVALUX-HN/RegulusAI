// T5 (S8,S9) perf budget + zero-deps: the v2.0 budgets from
// docs/memory-v2.md §5 (reindex < 2s, hybrid p95 < 50ms on 10k obs) and
// the zero-dependency invariant (go.mod gains zero require directives).
// Fixed seed corpus, generous ceilings — verdicts are deterministic.
package almanac

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// perfVocab is the fixed word pool for the synthetic 10k corpus.
var perfVocab = []string{
	"postgres", "pooling", "pool", "retrieval", "cache", "sizing",
	"bun", "runtime", "harvester", "connections", "trace", "session",
	"decision", "latency", "worker", "plugin", "picker", "debug",
	"consolidation", "observation", "lesson", "evidence", "receipt",
	"gate", "journey", "fixture", "paraphrase", "typo", "latency",
	"memory", "index", "hybrid", "lexical", "vector", "cosine",
	"decay", "domain", "topic", "chain", "revision", "supersede",
}

// perfQueries is the fixed query set for the p95 measurement.
var perfQueries = [][]string{
	{"postgres", "pooling"},
	{"retrieval", "cache"},
	{"bun", "runtime"},
	{"harvester", "connections"},
	{"trace", "session"},
	{"pooling", "latency"},
	{"hybrid", "lexical"},
	{"vector", "cosine"},
	{"topic", "chain"},
	{"memory", "index"},
}

// perfSeedCorpus writes a deterministic 10k-observation JSONL log
// (math/rand seed 42) and returns the root. Direct JSONL write keeps
// setup fast; Save would reopen the file 10k times.
func perfSeedCorpus(t *testing.T, n int) string {
	t.Helper()
	root := t.TempDir()
	rng := rand.New(rand.NewSource(42))
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	types := []string{TypeDecision, TypeDiscovery, TypeSessionNote, TypePreference}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		title := perfWords(rng, 4+rng.Intn(3))
		content := perfWords(rng, 12+rng.Intn(9))
		o := Observation{
			ID:      fmt.Sprintf("obs_perf_%05d", i),
			Type:    types[i%len(types)],
			Title:   title,
			Content: content,
			SavedAt: base.Add(time.Duration(i) * time.Second),
		}
		if i%7 == 0 {
			o.Domain = "go"
		} else if i%11 == 0 {
			o.Domain = "ts"
		}
		line, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(line)
		sb.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(File(root), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// perfWords draws k words from the fixed vocab.
func perfWords(rng *rand.Rand, k int) string {
	words := make([]string, k)
	for i := range words {
		words[i] = perfVocab[rng.Intn(len(perfVocab))]
	}
	return strings.Join(words, " ")
}

// TestPerfReindexBudget10k: reindex of 10k synthetic observations
// completes in < 2s. Measured 2026-10-29 sandbox: ~180ms (count=10000);
// ceiling is the §5 budget, 10x+ above measured, so the verdict is
// deterministic short of a pathological stall.
func TestPerfReindexBudget10k(t *testing.T) {
	root := perfSeedCorpus(t, 10000)
	start := time.Now()
	count, _, err := Reindex(root)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	t.Logf("reindex 10k: count=%d elapsed=%v ceiling=2s", count, elapsed)
	if count != 10000 {
		t.Fatalf("count = %d, want 10000", count)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("reindex 10k took %v, budget is < 2s", elapsed)
	}
}

// TestPerfHybridP95Budget10k: hybrid HybridSearch p95 over the fixed
// query set on the 10k corpus. Measured 2026-10-29 sandbox after the
// T5-followup single-load optimization (log plus pins/superseded load
// once per HybridSearch, shared by both rank paths; 4 runs): p50~30-31ms,
// p95~39-44ms, max~41-53ms — inside the §5 50ms budget, so the assertion
// is the budget itself; tight enough to catch a real regression (e.g. an
// accidental quadratic in the fusion path).
func TestPerfHybridP95Budget10k(t *testing.T) {
	root := perfSeedCorpus(t, 10000)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	// One warmup search so cold file-cache outliers do not decide the
	// verdict; only the timed loop below counts.
	if _, _, err := HybridSearch(root, Query{Terms: perfQueries[0], Limit: 20}); err != nil {
		t.Fatal(err)
	}
	const repeats = 10
	var durs []time.Duration
	for r := 0; r < repeats; r++ {
		for _, terms := range perfQueries {
			start := time.Now()
			hits, degraded, err := HybridSearch(root, Query{Terms: terms, Limit: 20})
			elapsed := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			if degraded {
				t.Fatal("fresh 10k index must not degrade")
			}
			_ = hits
			durs = append(durs, elapsed)
		}
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p50 := durs[len(durs)/2]
	p95 := durs[int(float64(len(durs))*0.95)-1]
	max := durs[len(durs)-1]
	t.Logf("hybrid 10k: n=%d p50=%v p95=%v max=%v ceiling=50ms (§5 budget 50ms)", len(durs), p50, p95, max)
	if p95 >= 50*time.Millisecond {
		t.Fatalf("hybrid p95 = %v over %d searches, budget is < 50ms (§5; measured ~40ms on 2026-10-29 sandbox after single-load optimization)", p95, len(durs))
	}
}

// TestZeroDepsGoMod: v2.0 ships zero new go.mod deps (S9 invariant) —
// the module file must carry no require directives.
func TestZeroDepsGoMod(t *testing.T) {
	mod := findGoMod(t)
	raw, err := os.ReadFile(mod)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if trimmed == "require (" || strings.HasPrefix(trimmed, "require ") {
			t.Fatalf("go.mod must carry zero require directives (S9), found %q in %s", trimmed, mod)
		}
	}
}

// findGoMod locates the repo go.mod by walking up from this file.
func findGoMod(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		cand := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from perf_test.go")
		}
		dir = parent
	}
}
