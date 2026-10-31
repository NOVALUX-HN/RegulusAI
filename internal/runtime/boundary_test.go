package runtime

import (
	"os/exec"
	"strings"
	"testing"
)

// The core boundary is pinned: `go list -m all` must show nothing but the
// main module. Any future external (agent-SDK or otherwise) dependency
// fails this test by construction.
func TestCoreHasNoExternalDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "all").Output()
	if err != nil {
		t.Fatalf("go list -m all: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		mod := strings.Fields(line)[0]
		if mod != "regulus-ai" {
			t.Fatalf("core depends on external module %q (want stdlib-only core)", mod)
		}
	}
}

// The adapter surface stays dependency-free too: this package must import
// only stdlib and regulus-ai/internal packages.
func TestRuntimePackageImportsAreCoreOnly(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", `{{join .Imports "\n"}}`, "regulus-ai/internal/runtime").Output()
	if err != nil {
		t.Fatalf("go list imports: %v", err)
	}
	for _, imp := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if imp == "" {
			continue
		}
		if strings.HasPrefix(imp, "regulus-ai/internal/") {
			continue
		}
		if !strings.Contains(imp, ".") {
			continue // stdlib
		}
		t.Fatalf("runtime imports external package %q (want stdlib + internal only)", imp)
	}
}
