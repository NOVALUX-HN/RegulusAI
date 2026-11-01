// S3 hygiene: the per-id CLOSED regexp compiles once (package cache)
// with behavior identical to per-call compilation.
package evidence

import (
	"regexp"
	"sync"
	"testing"
)

func TestDirectReCompilesOnce(t *testing.T) {
	a := directReFor("H1")
	b := directReFor("H1")
	if a != b {
		t.Fatal("directReFor must return the cached regexp for the same id")
	}
	if directReFor("H2") == a {
		t.Fatal("different ids must not share a cached regexp")
	}
	want := regexp.MustCompile(`\b` + regexp.QuoteMeta("H1") + `\b`)
	if a.String() != want.String() {
		t.Fatalf("cached pattern = %q, want %q", a.String(), want.String())
	}
}

func TestDirectReCacheBehaviorEqual(t *testing.T) {
	ids := []string{"H1", "H10", "T2", "C6", "A1"}
	contents := []string{
		"- L2: H1 CLOSED — done\n",
		"- L2: H10 CLOSED\n",
		"- L3: H1-H4 CLOSED 2026-10-31 — all done\n",
		"- L2: A1+A2 CLOSED + PROVE\n",
		"nothing closed here\n",
		"H1 CLOSED and H2 CLOSED on one line\n",
	}
	for _, id := range ids {
		for _, c := range contents {
			got := taskMarkedClosed(c, id)
			// Cache-hit path must agree with itself across repeats.
			if again := taskMarkedClosed(c, id); again != got {
				t.Fatalf("taskMarkedClosed(%q, %q) unstable: %v vs %v", c, id, got, again)
			}
		}
	}
	// Word-boundary behavior is preserved through the cache.
	if !taskMarkedClosed("- L2: H1 CLOSED\n", "H1") {
		t.Fatal("H1 must match H1 CLOSED")
	}
	if taskMarkedClosed("- L2: H10 CLOSED\n", "H1") {
		t.Fatal("H10 must not close H1 through the cached regexp")
	}
}

func TestDirectReConcurrentCache(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "H1"
			if i%2 == 0 {
				id = "T2"
			}
			_ = directReFor(id)
			_ = taskMarkedClosed("- L2: H1 CLOSED\n- L2: T2 CLOSED\n", id)
		}(i)
	}
	wg.Wait()
}
