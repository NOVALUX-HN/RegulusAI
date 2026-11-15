// NOV-65: benchmarks for memory search with and without the
// staleness cache (the speed-tokens claim). Baseline in
// docs/perf-baselines.md; >2x regression fails review.
package cli

import (
	"fmt"
	"testing"
)

func seedMemoryStoreBench(b *testing.B, n int) {
	b.Helper()
	b.Chdir(b.TempDir())
	c, _, errb := newCLI(nil)
	for i := 0; i < n; i++ {
		code := c.Run([]string{"memory", "save", "--type", "session-note",
			"--title", fmt.Sprintf("bench note %d", i),
			fmt.Sprintf("bench body %d about harness gates", i)})
		if code != 0 {
			b.Fatalf("seed save exit = %d: %s", code, errb.String())
		}
	}
}

func runMemorySearchBench(b *testing.B, extra ...string) {
	b.Helper()
	args := append([]string{"memory", "search", "harness"}, extra...)
	c, _, _ := newCLI(nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code := c.Run(args); code != 0 {
			b.Fatalf("search exit = %d", code)
		}
	}
}

func BenchmarkMemorySearchCached(b *testing.B) {
	seedMemoryStoreBench(b, 50)
	runMemorySearchBench(b)
}

func BenchmarkMemorySearchNoStaleness(b *testing.B) {
	seedMemoryStoreBench(b, 50)
	runMemorySearchBench(b, "--no-staleness")
}
