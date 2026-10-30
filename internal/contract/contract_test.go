package contract

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

// repoRoot resolves the repository root from this file's location, so the
// test works regardless of the caller's working directory (go test runs
// with CWD set to this package's directory).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate this test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// TestVersionIsPinned locks the plugin<->core protocol version. Bumping
// the contract is a deliberate, reviewed act — never silent drift.
func TestVersionIsPinned(t *testing.T) {
	const want = "regulus.contract/v1"
	if Version != want {
		t.Fatalf("contract Version = %q, want pinned %q", Version, want)
	}
}

// tsContractRe extracts the TypeScript mirror of the contract version from
// .opencode/lib/domain.ts (REGULUS_CONTRACT = "...").
var tsContractRe = regexp.MustCompile(`REGULUS_CONTRACT\s*=\s*"([^"]+)"`)

// typeScriptContract reads the contract version the plugin mirror speaks.
// Any read or parse failure fails the test: a missing mirror is drift.
func typeScriptContract(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading TypeScript mirror %s: %v (the Go core and the plugin must pin the same contract)", path, err)
	}
	m := tsContractRe.FindSubmatch(data)
	if m == nil {
		t.Fatalf("REGULUS_CONTRACT not found in %s (the mirror must export the pinned version)", path)
	}
	return string(m[1])
}

// TestParityWithTypeScriptMirror pins the Go core to the plugin's mirror:
// .opencode/lib/domain.ts REGULUS_CONTRACT must equal contract.Version.
// Either side drifting fails this test in red (the gauntlet stops).
func TestParityWithTypeScriptMirror(t *testing.T) {
	tsPath := filepath.Join(repoRoot(t), ".opencode", "lib", "domain.ts")
	if got := typeScriptContract(t, tsPath); got != Version {
		t.Fatalf("contract drift: Go core speaks %q but the TypeScript mirror speaks %q (change both, never one)", Version, got)
	}
}
