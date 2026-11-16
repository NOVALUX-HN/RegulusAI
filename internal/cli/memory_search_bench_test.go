// NOV-65: benchmarks for memory search with and without the
// staleness cache (the speed-tokens claim). Baseline in
// docs/perf-baselines.md; >2x regression fails review.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/testenv"
)

func seedMemoryStoreBench(b *testing.B, n int) {
	b.Helper()
	dir := b.TempDir()
	b.Chdir(dir)
	// NOV-79: realistic staleness fixture. Without a git repo the
	// cached variant never exercises rev-list behind-counts and both
	// benchmarks measure the same thing. Build history, save, then pin
	// every observation to an old head so Cached pays real probes
	// (bounded by the process-lifetime cache) while --no-staleness
	// skips them: the pair discriminates instead of tying.
	withGit := benchGitRepo(b, dir)
	c, _, errb := newCLI(nil)
	for i := 0; i < n; i++ {
		code := c.Run([]string{"memory", "save", "--type", "session-note",
			"--title", fmt.Sprintf("bench note %d", i),
			fmt.Sprintf("bench body %d about harness gates", i)})
		if code != 0 {
			b.Fatalf("seed save exit = %d: %s", code, errb.String())
		}
	}
	if withGit {
		pinBenchObsBehind(b, dir)
	}
}

// benchGitRepo builds two commits of history in dir for the staleness
// fixture. False when git is unavailable: callers keep the old
// git-less behavior (both variants tie) instead of failing.
func benchGitRepo(b *testing.B, dir string) bool {
	b.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		if testenv.Strict() {
			b.Fatalf("git not on PATH and REGULUS_STRICT_ENV=1 (skip refused)")
		}
		b.Skip("git not available: staleness fixture disabled")
	}
	run := func(args ...string) bool {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			b.Logf("bench git setup %v: %v (%s)", args, err, out)
			return false
		}
		return true
	}
	if !run("init", "-q") || !run("config", "user.email", "bench@example.com") ||
		!run("config", "user.name", "bench") ||
		!run("commit", "--allow-empty", "-qm", "base") ||
		!run("commit", "--allow-empty", "-qm", "tip") {
		return false
	}
	return true
}

// pinBenchObsBehind points every seeded observation at HEAD~1, so the
// cached variant probes real behind-counts (HEAD advanced past them).
// Precedent: the provenance-patch pattern in cli_test (memory save
// carries no git head for non-lesson types).
func pinBenchObsBehind(b *testing.B, dir string) {
	b.Helper()
	headCmd := exec.Command("git", "rev-parse", "HEAD~1")
	headCmd.Dir = dir
	rawHead, err := headCmd.Output()
	if err != nil {
		b.Fatalf("bench git HEAD~1: %v", err)
	}
	oldHead := strings.TrimSpace(string(rawHead))
	p := filepath.Join(dir, ".regulus", "almanac", "observations.jsonl")
	data, err := os.ReadFile(p)
	if err != nil {
		b.Fatalf("bench read observations: %v", err)
	}
	patched := strings.ReplaceAll(string(data), `"provenance":{"git_head":""`, `"provenance":{"git_head":"`+oldHead+`"`)
	if patched == string(data) {
		b.Fatalf("bench provenance patch found no empty git_head to pin")
	}
	if err := os.WriteFile(p, []byte(patched), 0o644); err != nil {
		b.Fatalf("bench write observations: %v", err)
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
