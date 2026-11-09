package gates

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T5 RED: repeated runs with unchanged config+HEAD skip coverage
// recomputation — the unit-tests command runs once, the second run
// reuses .regulus/coverage.out and still passes.
func TestCoverageCacheHitSkipsRecompute(t *testing.T) {
	root := t.TempDir()
	cfgJSON := `{"gates":[{"name":"unit-tests","layer":4,"command":"go test ./... -coverprofile .regulus/coverage.out"}]}`
	cfg, err := Load(writeConfigJSON(t, cfgJSON))
	if err != nil {
		t.Fatal(err)
	}
	cfgBytes := []byte(cfgJSON)
	head := "abc123deadbeef"
	r := &fakeRunner{results: map[string]fakeResult{
		"git rev-parse --verify HEAD": {stdout: head + "\n"},
		"git diff HEAD -- .":          {stdout: "DIFF"},
	}}
	// Simulate the coverage file the real `go test -coverprofile` writes.
	if err := os.MkdirAll(filepath.Join(root, ".regulus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".regulus", "coverage.out"), []byte("mode: set\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res1, _, err := RunTask(context.Background(), cfg, cfgBytes, root, "feat/T5", "v", "c", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if res1.Failed {
		t.Fatalf("first run must pass: %+v", res1)
	}
	callsAfterFirst := len(r.calls)
	res2, _, err := RunTask(context.Background(), cfg, cfgBytes, root, "feat/T5", "v", "c", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Failed {
		t.Fatalf("cached run must pass: %+v", res2)
	}
	for _, c := range r.calls[callsAfterFirst:] {
		if strings.Contains(c, "go test") {
			t.Fatalf("cache hit must skip recomputation, second-run calls: %v", r.calls[callsAfterFirst:])
		}
	}
}

func writeConfigJSON(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "regulus.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
