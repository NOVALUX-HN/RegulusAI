package almanac

// T1 (macizo-tokens): the compaction resume bound lives in two places —
// the Go core defaults (DefaultContextLimit/DefaultContextBudget) and
// the plugin mirror (COMPACTION_CONTEXT_LIMIT/COMPACTION_CONTEXT_BUDGET
// in .opencode/lib/domain.ts). Either side drifting fails this test in
// red: change both, never one.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
)

func t1RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate this test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

var t1TsDefaultRe = regexp.MustCompile(`COMPACTION_CONTEXT_(LIMIT|BUDGET)\s*=\s*(\d+)`)

func t1TypeScriptDefault(t *testing.T, path, name string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading TypeScript mirror %s: %v (the Go core and the plugin must pin the same compaction bound)", path, err)
	}
	for _, m := range t1TsDefaultRe.FindAllSubmatch(data, -1) {
		if string(m[1]) == name {
			n, err := strconv.Atoi(string(m[2]))
			if err != nil {
				t.Fatalf("COMPACTION_CONTEXT_%s is not a number in %s", name, path)
			}
			return n
		}
	}
	t.Fatalf("COMPACTION_CONTEXT_%s not found in %s (the mirror must export the pinned bound)", name, path)
	return 0
}

func TestCompactionDefaultsParity(t *testing.T) {
	tsPath := filepath.Join(t1RepoRoot(t), ".opencode", "lib", "domain.ts")
	if got := t1TypeScriptDefault(t, tsPath, "LIMIT"); got != DefaultContextLimit {
		t.Fatalf("compaction bound drift: Go DefaultContextLimit = %d but the TypeScript mirror speaks %d (change both, never one)", DefaultContextLimit, got)
	}
	if got := t1TypeScriptDefault(t, tsPath, "BUDGET"); got != DefaultContextBudget {
		t.Fatalf("compaction bound drift: Go DefaultContextBudget = %d but the TypeScript mirror speaks %d (change both, never one)", DefaultContextBudget, got)
	}
}
