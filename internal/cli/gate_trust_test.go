package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGateRunFooterLines(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"typecheck","layer":1,"command":"echo hi"},
		{"name":"unit-tests","layer":2,"command":"echo ok"}
	]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "regulus-ai-v1/T2"}); code != 0 {
		t.Fatalf("exit = %d, want 0; stdout %q", code, out.String())
	}
	s := out.String()
	// existing per-layer lines unchanged
	if !strings.Contains(s, "layer 1 typecheck") || !strings.Contains(s, "layer 2 unit-tests") {
		t.Fatalf("per-layer lines must be preserved, got:\n%s", s)
	}
	if !strings.Contains(s, "gauntlet: 2/2 gates passed") {
		t.Fatalf("gauntlet line must be preserved, got:\n%s", s)
	}
	// additive footer
	if !strings.Contains(s, "verdict: PASS (ok)") {
		t.Fatalf("footer must carry verdict PASS (ok), got:\n%s", s)
	}
	if !strings.Contains(s, "manifest: ") {
		t.Fatalf("footer must carry manifest path, got:\n%s", s)
	}
	// manifest file must exist
	var mpath string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "manifest: ") {
			mpath = strings.TrimSpace(strings.TrimPrefix(line, "manifest: "))
		}
	}
	if mpath == "" {
		t.Fatal("no manifest line")
	}
	if _, err := os.Stat(mpath); err != nil {
		t.Fatalf("manifest %q must exist: %v", mpath, err)
	}
}

func TestGateRunFailedFooter(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"typecheck","layer":1,"command":"boom"}
	]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"boom":                        {code: 1, stderr: "type error"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "demo/T1"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	s := out.String()
	if !strings.Contains(s, "stopped at gate") {
		t.Fatalf("stop line must be preserved, got:\n%s", s)
	}
	if !strings.Contains(s, "verdict: FAIL (deterministic_failure)") {
		t.Fatalf("failed footer must carry verdict FAIL, got:\n%s", s)
	}
	if !strings.Contains(s, "manifest: ") {
		t.Fatalf("failed run must still print manifest, got:\n%s", s)
	}
}

func TestGateRunDryRunCreatesNoFiles(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"typecheck","layer":1,"command":"echo hi"}
	]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
		t.Fatalf("dry-run exit = %d, want 0", code)
	}
	s := out.String()
	if !strings.Contains(s, "typecheck") || !strings.Contains(s, "echo hi") {
		t.Fatalf("dry-run must print gate details, got:\n%s", s)
	}
	if len(r.calls) != 0 {
		t.Fatalf("dry-run must execute NOTHING, calls=%v", r.calls)
	}
	// no writes: no .regulus dir at all
	if _, err := os.Stat(filepath.Join(dir, ".regulus")); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(filepath.Join(dir, ".regulus"))
		t.Fatalf("dry-run must create no files, .regulus contains %v", entries)
	}
}
