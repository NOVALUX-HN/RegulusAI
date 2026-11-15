// NOV-65: benchmarks for the "speed-tokens" claim paths. Baselines
// (2026-11-14, linux/amd64, Go 1.27, 200-obs store) live in
// docs/perf-baselines.md; a >2x regression vs baseline fails review
// (rerun, do not hand-edit the numbers).
package almanac

import (
	"fmt"
	"testing"
	"time"
)

func seedBenchStore(b *testing.B, n int) string {
	b.Helper()
	dir := b.TempDir()
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		if _, err := Save(dir, &Observation{
			Type:    TypeSessionNote,
			Title:   fmt.Sprintf("note %d about harness gates", i),
			Content: fmt.Sprintf("observation body %d with receipts and verdicts", i),
			Domain:  "bench",
		}, now.Add(time.Duration(i)*time.Second)); err != nil {
			b.Fatal(err)
		}
	}
	return dir
}

func BenchmarkNotes(b *testing.B) {
	dir := seedBenchStore(b, 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Notes(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContext(b *testing.B) {
	dir := seedBenchStore(b, 200)
	now := time.Now().UTC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Context(dir, now); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHybridSearch(b *testing.B) {
	dir := seedBenchStore(b, 200)
	q := Query{Terms: []string{"harness", "gates"}, Limit: 10}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := HybridSearch(dir, q); err != nil {
			b.Fatal(err)
		}
	}
}
