// NOV-65: benchmarks for the trace hot paths backing the
// record-batch claim (one spawn per flush) and cost folding.
// Baselines in docs/perf-baselines.md; >2x regression fails review.
package trace

import (
	"testing"
	"time"
)

func seedTraceStore(b *testing.B, sessions, per int) string {
	b.Helper()
	dir := b.TempDir()
	now := time.Now().UTC()
	n := 0
	for s := 0; s < sessions; s++ {
		for i := 0; i < per; i++ {
			if _, err := Record(dir, Entry{
				Session: "ses",
				Message: "m",
				Call:    "c",
				Tool:    "read",
				Status:  StatusCompleted,
				Task:    "bench/T1",
			}, now.Add(time.Duration(n)*time.Millisecond)); err != nil {
				b.Fatal(err)
			}
			n++
		}
	}
	return dir
}

func BenchmarkRecord(b *testing.B) {
	dir := b.TempDir()
	now := time.Now().UTC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Record(dir, Entry{
			Session: "ses", Message: "m", Call: "c", Tool: "read",
			Status: StatusCompleted, Task: "bench/T1",
		}, now); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCostByTask(b *testing.B) {
	dir := seedTraceStore(b, 4, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := CostByTask(dir); err != nil {
			b.Fatal(err)
		}
	}
}
