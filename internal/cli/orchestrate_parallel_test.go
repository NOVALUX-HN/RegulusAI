// CLI tests for `regulus orchestrate parallel` / `orphans`
// (F12.2/NOV-97): usage errors stay byte-identical, dry-run plans
// are deterministic and touch nothing, hostile plans fail closed.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeParallelTasks(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOrchestrateParallelNeedsTasks(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"orchestrate", "parallel"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus orchestrate parallel") {
		t.Fatalf("missing tasks must print the parallel usage, got %q", errb.String())
	}
}

func TestOrchestrateParallelUnknownSubcommandStillUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"orchestrate", "frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus orchestrate agents") {
		t.Fatalf("umbrella usage keeps its prefix, got %q", errb.String())
	}
}

// Dry-run plans are byte-identical across invocations and create
// nothing (no worktrees, no trace writes, no report dir).
func TestOrchestrateParallelDryRunDeterministic(t *testing.T) {
	dir := t.TempDir()
	tasks := writeParallelTasks(t, dir, "tasks.json", `[
		{"id":"a","title":"A","files":["a.txt"],"command":"echo a"},
		{"id":"b","title":"B","files":["b.txt"],"command":"echo b"}
	]`)
	run := func() (string, string, int) {
		c, out, errb := newCLI(nil)
		c.Dir = dir
		code := c.Run([]string{"orchestrate", "parallel", "--tasks", tasks, "--dry-run"})
		return out.String(), errb.String(), code
	}
	out1, err1, code1 := run()
	out2, err2, code2 := run()
	if code1 != 0 || code2 != 0 {
		t.Fatalf("dry-run exits 0, got %d/%d (stderr %q %q)", code1, code2, err1, err2)
	}
	if out1 != out2 {
		t.Fatalf("dry-run reports diverged:\n%s\n%s", out1, out2)
	}
	if !strings.Contains(out1, "dry-run: plan no ejecutado, nada registrado") {
		t.Fatalf("dry-run must say it recorded nothing, got %q", out1)
	}
	if _, err := os.Stat(filepath.Join(dir, ".regulus")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write trace stores, stat err = %v", err)
	}
}

// A colliding pair serializes into two levels on dry-run.
func TestOrchestrateParallelDryRunSerializesCollisions(t *testing.T) {
	dir := t.TempDir()
	tasks := writeParallelTasks(t, dir, "tasks.json", `[
		{"id":"b","title":"B","files":["shared.txt"]},
		{"id":"a","title":"A","files":["shared.txt"]}
	]`)
	c, out, _ := newCLI(nil)
	c.Dir = dir
	if code := c.Run([]string{"orchestrate", "parallel", "--tasks", tasks, "--dry-run", "--json"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var rep struct {
		Levels     [][]string  `json:"levels"`
		Serialized [][2]string `json:"serialized"`
		DryRun     bool        `json:"dry_run"`
	}
	if err := json.Unmarshal([]byte(out.String()), &rep); err != nil {
		t.Fatalf("report must be JSON: %v", err)
	}
	if !rep.DryRun || len(rep.Levels) != 2 {
		t.Fatalf("colliding tasks serialize into two levels, got %+v", rep)
	}
	if len(rep.Serialized) != 1 || rep.Serialized[0] != [2]string{"a", "b"} {
		t.Fatalf("report must name the serialized pair, got %+v", rep.Serialized)
	}
}

// Hostile plans fail closed with exit 1 and a naming reason.
func TestOrchestrateParallelRefusesHostile(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"cycle.json": `[{"id":"a","title":"A","depends_on":["b"]}, {"id":"b","title":"B","depends_on":["a"]}]`,
		"abs.json":   `[{"id":"a","title":"A","files":["/abs"]}]`,
		"dup.json":   `[{"id":"a","title":"A"}, {"id":"a","title":"A2"}]`,
	} {
		tasks := writeParallelTasks(t, dir, name, body)
		c, _, errb := newCLI(nil)
		c.Dir = dir
		if code := c.Run([]string{"orchestrate", "parallel", "--tasks", tasks, "--dry-run"}); code != 1 {
			t.Fatalf("%s: exit = %d, want 1 (stderr %q)", name, code, errb.String())
		}
	}
}

// Malformed JSON fails closed.
func TestOrchestrateParallelMalformedFile(t *testing.T) {
	dir := t.TempDir()
	tasks := writeParallelTasks(t, dir, "bad.json", `[{"id":`)
	c, _, _ := newCLI(nil)
	c.Dir = dir
	if code := c.Run([]string{"orchestrate", "parallel", "--tasks", tasks, "--dry-run"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

// `orphans` lists nothing outside a repo gracefully: git is absent
// or the dir is not a repo, so it fails with exit 1, never 2.
func TestOrchestrateOrphansOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	c, _, errb := newCLI(nil)
	c.Dir = dir
	code := c.Run([]string{"orchestrate", "orphans"})
	if code != 0 && code != 1 {
		t.Fatalf("exit = %d, want 0 or 1 outside a repo", code)
	}
	if code == 1 && !strings.Contains(errb.String(), "orchestrate orphans:") {
		t.Fatalf("failure must name the command, got %q", errb.String())
	}
}

func TestOrchestrateOrphansRejectsExtraArgs(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"orchestrate", "orphans", "extra"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus orchestrate parallel") {
		t.Fatalf("extra args must print the parallel usage, got %q", errb.String())
	}
}
