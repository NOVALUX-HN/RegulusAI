// MergeBranch tests: pure helpers with injected fixed-string git runners
// (no real git), plus the MergeBranch delegate through a stubbed runner.
// Every message and code path mirrors production byte-identically.
package orchestrate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// mergeCall is one scripted git invocation: fixed output and error.
type mergeCall struct {
	out string
	err error
}

// stubMergeRun replays fixed git outputs keyed by "git <args>"; any
// unscripted call fails loudly so missing branches surface as errors.
func stubMergeRun(calls map[string]mergeCall) gitRunner {
	return func(ctx context.Context, dir, name string, args ...string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		key := name + " " + strings.Join(args, " ")
		if s, ok := calls[key]; ok {
			return s.out, s.err
		}
		return "", errors.New("unexpected git call: " + key)
	}
}

func TestParseConflictFilesPure(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want []string
	}{
		{"empty", "", nil},
		{"blank lines drop out", "a.go\n \n\nb.go\n", []string{"a.go", "b.go"}},
		{"sorted deterministic", "b.go\na.go\n", []string{"a.go", "b.go"}},
		{"whitespace trimmed", "  a.go  \r\n\tb.go\t\n", []string{"a.go", "b.go"}},
		{"single file", "only.go\n", []string{"only.go"}},
	}
	for _, tc := range cases {
		got := parseConflictFiles(tc.out)
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: parseConflictFiles(%q) = %v, want %v", tc.name, tc.out, got, tc.want)
		}
	}
}

func TestEnsureMergeBaseBranches(t *testing.T) {
	ctx := context.Background()
	t.Run("rev-parse failure keeps message", func(t *testing.T) {
		run := stubMergeRun(map[string]mergeCall{
			"git rev-parse --abbrev-ref HEAD": {out: "partial\n", err: errors.New("exit status 128")},
		})
		err := ensureMergeBase(ctx, "/repo", "main", run)
		if err == nil || !strings.Contains(err.Error(), "orchestrate parallel: cannot read current branch") {
			t.Fatalf("err = %v, want cannot-read-current-branch", err)
		}
		if !strings.Contains(err.Error(), "exit status 128") || !strings.Contains(err.Error(), "(partial)") {
			t.Fatalf("err = %v, want exit code and trimmed output preserved", err)
		}
	})
	t.Run("already on base skips checkout", func(t *testing.T) {
		run := stubMergeRun(map[string]mergeCall{
			"git rev-parse --abbrev-ref HEAD": {out: "main\n"},
		})
		if err := ensureMergeBase(ctx, "/repo", "main", run); err != nil {
			t.Fatalf("already on base: %v", err)
		}
	})
	t.Run("HEAD base never checks out", func(t *testing.T) {
		run := stubMergeRun(map[string]mergeCall{
			"git rev-parse --abbrev-ref HEAD": {out: "feature-x\n"},
		})
		if err := ensureMergeBase(ctx, "/repo", "HEAD", run); err != nil {
			t.Fatalf("HEAD base: %v", err)
		}
	})
	t.Run("checkout success", func(t *testing.T) {
		run := stubMergeRun(map[string]mergeCall{
			"git rev-parse --abbrev-ref HEAD": {out: "main\n"},
			"git checkout release":            {out: "Switched to branch 'release'\n"},
		})
		if err := ensureMergeBase(ctx, "/repo", "release", run); err != nil {
			t.Fatalf("checkout: %v", err)
		}
	})
	t.Run("checkout failure keeps message", func(t *testing.T) {
		run := stubMergeRun(map[string]mergeCall{
			"git rev-parse --abbrev-ref HEAD": {out: "main\n"},
			"git checkout release":            {out: "error: pathspec\n", err: errors.New("exit status 1")},
		})
		err := ensureMergeBase(ctx, "/repo", "release", run)
		if err == nil || !strings.Contains(err.Error(), "orchestrate parallel: cannot check out base release") {
			t.Fatalf("err = %v, want cannot-check-out-base", err)
		}
		if !strings.Contains(err.Error(), "exit status 1") || !strings.Contains(err.Error(), "(error: pathspec)") {
			t.Fatalf("err = %v, want exit code and trimmed output preserved", err)
		}
	})
}

func TestEnsureCleanMergeTreeBranches(t *testing.T) {
	ctx := context.Background()
	t.Run("status failure keeps message", func(t *testing.T) {
		run := stubMergeRun(map[string]mergeCall{
			"git status --porcelain": {out: "fatal: not a repo\n", err: errors.New("exit status 128")},
		})
		err := ensureCleanMergeTree(ctx, "/repo", run)
		if err == nil || !strings.Contains(err.Error(), "orchestrate parallel: cannot verify a clean tree") {
			t.Fatalf("err = %v, want cannot-verify-clean-tree", err)
		}
	})
	t.Run("dirty tree refuses", func(t *testing.T) {
		run := stubMergeRun(map[string]mergeCall{
			"git status --porcelain": {out: " M dirty.go\n"},
		})
		err := ensureCleanMergeTree(ctx, "/repo", run)
		if err == nil || !strings.Contains(err.Error(), "refusing to merge onto a dirty tree (commit or stash first)") {
			t.Fatalf("err = %v, want dirty-tree refusal", err)
		}
	})
	for _, clean := range []string{"", "  \n"} {
		run := stubMergeRun(map[string]mergeCall{
			"git status --porcelain": {out: clean},
		})
		if err := ensureCleanMergeTree(ctx, "/repo", run); err != nil {
			t.Fatalf("clean %q: %v", clean, err)
		}
	}
}

func mergeFlowCalls(extra map[string]mergeCall) map[string]mergeCall {
	calls := map[string]mergeCall{
		"git rev-parse --abbrev-ref HEAD": {out: "main\n"},
		"git status --porcelain":          {},
	}
	for k, v := range extra {
		calls[k] = v
	}
	return calls
}

func TestMergeBranchWithRunnerFlows(t *testing.T) {
	ctx := context.Background()
	t.Run("clean merge returns no conflicts", func(t *testing.T) {
		run := stubMergeRun(mergeFlowCalls(map[string]mergeCall{
			"git merge --no-ff --no-edit parallel/r/t": {out: "Merge made by the 'ort' strategy.\n"},
		}))
		conflicts, err := mergeBranchWithRunner(ctx, "/repo", "parallel/r/t", "main", run)
		if err != nil || len(conflicts) != 0 {
			t.Fatalf("conflicts = %v, err = %v; want clean merge", conflicts, err)
		}
	})
	t.Run("conflict returns sorted files with nil error", func(t *testing.T) {
		run := stubMergeRun(mergeFlowCalls(map[string]mergeCall{
			"git merge --no-ff --no-edit parallel/r/t": {out: "Auto-merging b.go\nCONFLICT\n", err: errors.New("exit status 1")},
			"git diff --name-only --diff-filter=U":     {out: "b.go\na.go\n"},
		}))
		conflicts, err := mergeBranchWithRunner(ctx, "/repo", "parallel/r/t", "main", run)
		if err != nil {
			t.Fatalf("conflict must return nil error, got %v", err)
		}
		if !reflect.DeepEqual(conflicts, []string{"a.go", "b.go"}) {
			t.Fatalf("conflicts = %v, want sorted [a.go b.go]", conflicts)
		}
	})
	t.Run("diff failure wraps merge error", func(t *testing.T) {
		run := stubMergeRun(mergeFlowCalls(map[string]mergeCall{
			"git merge --no-ff --no-edit parallel/r/t": {out: "merge output\n", err: errors.New("exit status 1")},
			"git diff --name-only --diff-filter=U":     {out: "fatal\n", err: errors.New("exit status 128")},
		}))
		conflicts, err := mergeBranchWithRunner(ctx, "/repo", "parallel/r/t", "main", run)
		if err == nil || !strings.Contains(err.Error(), "orchestrate parallel: merge of parallel/r/t failed") {
			t.Fatalf("err = %v, want merge-failed wrap", err)
		}
		if conflicts != nil {
			t.Fatalf("conflicts = %v, want nil on diff failure", conflicts)
		}
		if !strings.Contains(err.Error(), "(merge output)") {
			t.Fatalf("err = %v, want trimmed merge output preserved", err)
		}
	})
	t.Run("empty diff wraps merge error", func(t *testing.T) {
		run := stubMergeRun(mergeFlowCalls(map[string]mergeCall{
			"git merge --no-ff --no-edit parallel/r/t": {out: "merge output\n", err: errors.New("exit status 1")},
			"git diff --name-only --diff-filter=U":     {out: "\n"},
		}))
		_, err := mergeBranchWithRunner(ctx, "/repo", "parallel/r/t", "main", run)
		if err == nil || !strings.Contains(err.Error(), "orchestrate parallel: merge of parallel/r/t failed") {
			t.Fatalf("err = %v, want merge-failed wrap on empty diff", err)
		}
	})
	t.Run("dirty tree stops before merge", func(t *testing.T) {
		run := stubMergeRun(map[string]mergeCall{
			"git rev-parse --abbrev-ref HEAD": {out: "main\n"},
			"git status --porcelain":          {out: " M dirty.go\n"},
		})
		_, err := mergeBranchWithRunner(ctx, "/repo", "parallel/r/t", "main", run)
		if err == nil || !strings.Contains(err.Error(), "refusing to merge onto a dirty tree") {
			t.Fatalf("err = %v, want dirty-tree refusal", err)
		}
	})
	t.Run("unreadable branch stops before merge", func(t *testing.T) {
		run := stubMergeRun(map[string]mergeCall{
			"git rev-parse --abbrev-ref HEAD": {out: "", err: errors.New("exit status 128")},
		})
		_, err := mergeBranchWithRunner(ctx, "/repo", "parallel/r/t", "main", run)
		if err == nil || !strings.Contains(err.Error(), "cannot read current branch") {
			t.Fatalf("err = %v, want cannot-read-current-branch", err)
		}
	})
}

func TestMergeBranchDelegateUsesInjectedRunner(t *testing.T) {
	ctx := context.Background()
	old := mergeGitRun
	defer func() { mergeGitRun = old }()
	mergeGitRun = stubMergeRun(mergeFlowCalls(map[string]mergeCall{
		"git merge --no-ff --no-edit parallel/r/t": {out: "ok\n"},
	}))
	conflicts, err := GitWorktreeOps{}.MergeBranch(ctx, "/repo", "parallel/r/t", "main")
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("delegate clean merge: conflicts = %v, err = %v", conflicts, err)
	}
	mergeGitRun = stubMergeRun(mergeFlowCalls(map[string]mergeCall{
		"git merge --no-ff --no-edit parallel/r/t": {out: "CONFLICT\n", err: errors.New("exit status 1")},
		"git diff --name-only --diff-filter=U":     {out: "a.go\n"},
	}))
	conflicts, err = GitWorktreeOps{}.MergeBranch(ctx, "/repo", "parallel/r/t", "main")
	if err != nil || !reflect.DeepEqual(conflicts, []string{"a.go"}) {
		t.Fatalf("delegate conflict: conflicts = %v, err = %v", conflicts, err)
	}
}
