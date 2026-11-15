// NOV-64: close traceability. Fixtures prove: valid closure links,
// missing commit blocks, multi-issue needs justification, the
// reviewer must differ from the implementer, and out-of-scope diffs
// block without justification.
package sextant

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regulus-ai/internal/testenv"
	"strings"
	"testing"

	"regulus-ai/internal/guardians"
)

func traceGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func initTraceRepo(t *testing.T) string {
	t.Helper()
	testenv.Require(t, "git")
	root := t.TempDir()
	traceGit(t, root, "init", "-q", "-b", "main")
	traceGit(t, root, "config", "user.email", "dev@example.com")
	traceGit(t, root, "config", "user.name", "dev")
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "ok.go"), []byte("package ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	traceGit(t, root, "add", "-A")
	traceGit(t, root, "commit", "-qm", "init")
	return root
}

func traceCommit(t *testing.T, root, file, body, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	traceGit(t, root, "add", "-A")
	traceGit(t, root, "commit", "-qm", msg)
}

func writeEvals(t *testing.T, root, task string, evals []guardians.Evaluation) {
	t.Helper()
	p := guardians.EvaluationsFile(root, task)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range evals {
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func traceScope() Scope {
	// Cleaned form, as ParseScope stores it (trailing slash folds).
	return Scope{Declared: true, Allows: []string{"internal"}}
}

func traceOpts(issue string) TraceOptions {
	return TraceOptions{Issue: issue, Implementer: "dev", ChartBase: "demo", TaskID: "T9", Scope: traceScope()}
}

func TestTraceValidCloseLinks(t *testing.T) {
	root := initTraceRepo(t)
	traceCommit(t, root, "internal/a.go", "package a\n", "feat: thing (NOV-64)")
	writeEvals(t, root, "T9", []guardians.Evaluation{{Guardian: "spec-keeper", Status: "PASS", Note: "scope ok", At: "now"}})
	opts := traceOpts("NOV-64")
	opts.IndependentReview = true
	if unmet := CheckTraceability(root, opts); len(unmet) != 0 {
		t.Fatalf("valid close must link, got %v", unmet)
	}
}

func TestTraceNoCommitBlocks(t *testing.T) {
	root := initTraceRepo(t)
	if unmet := CheckTraceability(root, traceOpts("NOV-64")); len(unmet) == 0 {
		t.Fatal("close without commit must block")
	} else if !strings.Contains(unmet[0], "no commit cites NOV-64") {
		t.Fatalf("must name the gap, got %v", unmet)
	}
}

func TestTraceMultiIssueNeedsJustification(t *testing.T) {
	root := initTraceRepo(t)
	traceCommit(t, root, "internal/a.go", "package a\n", "feat: two things (NOV-64, NOV-63)")
	if unmet := CheckTraceability(root, traceOpts("NOV-64")); len(unmet) == 0 {
		t.Fatal("unjustified multi-issue commit must block")
	}
	opts := traceOpts("NOV-64")
	opts.Justified = "shared helper touched by both"
	if unmet := CheckTraceability(root, opts); len(unmet) != 0 {
		t.Fatalf("flag justification must cover, got %v", unmet)
	}
	traceCommit(t, root, "internal/b.go", "package b\n", "feat: pair (NOV-64, NOV-62)\n\nJustified: same rename both issues need")
	// The trailer covers its own commit; the earlier unjustified
	// commit still blocks and must name NOV-63.
	if unmet := CheckTraceability(root, traceOpts("NOV-64")); len(unmet) == 0 {
		t.Fatal("earlier unjustified commit must still block")
	} else if !strings.Contains(strings.Join(unmet, " "), "NOV-63") {
		t.Fatalf("must name the unjustified issue, got %v", unmet)
	}
}

func TestTraceReviewerMustDiffer(t *testing.T) {
	root := initTraceRepo(t)
	traceCommit(t, root, "internal/a.go", "package a\n", "feat: thing (NOV-64)")
	writeEvals(t, root, "T9", []guardians.Evaluation{{Guardian: "dev", Status: "PASS", Note: "self review", At: "now"}})
	opts := traceOpts("NOV-64")
	opts.IndependentReview = true
	if unmet := CheckTraceability(root, opts); len(unmet) == 0 {
		t.Fatal("reviewer equal to implementer must refuse")
	} else if !strings.Contains(strings.Join(unmet, " "), "other than") {
		t.Fatalf("must name independence, got %v", unmet)
	}
	opts2 := traceOpts("NOV-64")
	if unmet := CheckTraceability(root, opts2); len(unmet) != 0 {
		t.Fatalf("without independent-review the self review must not block, got %v", unmet)
	}
}

func TestTraceIndependentNeedsImplementer(t *testing.T) {
	root := initTraceRepo(t)
	traceCommit(t, root, "internal/a.go", "package a\n", "feat: thing (NOV-64)")
	writeEvals(t, root, "T9", []guardians.Evaluation{{Guardian: "spec-keeper", Status: "PASS", Note: "ok", At: "now"}})
	opts := traceOpts("NOV-64")
	opts.Implementer = ""
	opts.IndependentReview = true
	if unmet := CheckTraceability(root, opts); len(unmet) == 0 {
		t.Fatal("independent review without implementer must block")
	}
}

func TestTraceOutOfScopeBlocks(t *testing.T) {
	root := initTraceRepo(t)
	traceCommit(t, root, "outside.txt", "x\n", "feat: thing (NOV-64)")
	if unmet := CheckTraceability(root, traceOpts("NOV-64")); len(unmet) == 0 {
		t.Fatal("out-of-scope diff must block")
	}
}
