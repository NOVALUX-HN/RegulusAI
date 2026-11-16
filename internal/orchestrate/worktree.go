// Git worktree isolation (F12.2/NOV-97): one isolated checkout per
// worker over native `git worktree`, plus orphan detection.
//
// Every worker gets its own branch (parallel/<run>/<task>), its own
// directory (no shared files with sibling workers), its own pinned
// workspace (F11.8: the delegate workspace IS the worktree path, so
// path containment anchors per worker) and its own restricted scopes
// (F11.8 Delegate: the child keeps only the intersection, never more).
// The production path shells out to real git; tests inject fakes
// through WorktreeOps. Nothing here invents a parallel git: branch,
// worktree and merge are the native commands.
package orchestrate

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"regulus-ai/internal/mcp"
)

// DefaultParallelBranchPrefix names every worker branch; the orphan
// sweeper only touches branches under this prefix, never user work.
const DefaultParallelBranchPrefix = "parallel"

// WorkerIsolation is the per-worker sandbox: branch, directory,
// pinned workspace and restricted scopes. Host/User ride the parent
// attribution (server-side, never client-claimed).
type WorkerIsolation struct {
	TaskID    string   `json:"task_id"`
	Agent     string   `json:"agent"`
	Branch    string   `json:"branch"`
	Path      string   `json:"path"`
	Workspace string   `json:"workspace"`
	Host      string   `json:"host"`
	User      string   `json:"user"`
	Scopes    []string `json:"scopes"`
}

// sanitizeParallelSegment folds arbitrary text onto the branch/path
// alphabet: letters, digits, '.', '_', '-' (and '/' for branches).
// Anything else becomes '-', runs collapse, edges trim, empty folds
// to "task".
func sanitizeParallelSegment(s string, keepSlash bool) string {
	var b strings.Builder
	for _, r := range s {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			continue
		}
		if keepSlash && r == '/' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-./")
	if out == "" {
		return "task"
	}
	return out
}

// ParallelBranch names the worker branch: prefix/run/task, every
// segment sanitized, bounded to 255 bytes (the git ref limit the
// sweeper relies on to never mistake user branches).
func ParallelBranch(prefix, runID, taskID string) string {
	if strings.TrimSpace(prefix) == "" {
		prefix = DefaultParallelBranchPrefix
	}
	branch := sanitizeParallelSegment(prefix, true) + "/" +
		sanitizeParallelSegment(runID, false) + "/" +
		sanitizeParallelSegment(taskID, true)
	if len(branch) > 255 {
		branch = branch[:255]
		branch = strings.Trim(branch, "-./")
	}
	return branch
}

// ParallelWorktreePath maps a worker branch onto its directory: the
// branch with '/' folded to '-' under the worktree root, so one root
// lists every worker without nested checkouts.
func ParallelWorktreePath(worktreeRoot, branch string) string {
	seg := strings.ReplaceAll(branch, "/", "-")
	seg = sanitizeParallelSegment(seg, false)
	if len(seg) > 128 {
		seg = seg[:128]
		seg = strings.Trim(seg, "-.")
	}
	return worktreeRoot + "/" + seg
}

// IsolateParallelWorker derives one worker sandbox from the calling
// principal (F11.8): Host/User/Workspace pinned, scopes intersected
// (delegating only restricts — unknown requests are dropped, never
// granted). The workspace IS the worktree path: containment checks
// anchor per worker, so sibling workers share nothing.
func IsolateParallelWorker(parent mcp.Principal, task ParallelTask, branch, path string) WorkerIsolation {
	agent := strings.TrimSpace(task.Agent)
	if agent == "" {
		agent = "parallel-worker"
	}
	requested := task.Scopes
	if len(requested) == 0 {
		requested = parent.GrantedScopes()
	}
	child := parent.Delegate(agent, requested)
	return WorkerIsolation{
		TaskID: task.ID, Agent: child.Agent,
		Branch: branch, Path: path, Workspace: path,
		Host: child.Host, User: child.User,
		Scopes: child.GrantedScopes(),
	}
}

// WorktreeEntry is one `git worktree list --porcelain` record.
type WorktreeEntry struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
	Bare   bool   `json:"bare"`
}

// ParseWorktreeList parses `git worktree list --porcelain`: records
// split on blank lines, `worktree <path>`, `HEAD <sha>`, `branch
// refs/heads/<b>` (detached entries carry no branch), `bare`.
func ParseWorktreeList(porcelain string) []WorktreeEntry {
	var out []WorktreeEntry
	var cur WorktreeEntry
	flush := func() {
		if strings.TrimSpace(cur.Path) != "" {
			out = append(out, cur)
		}
		cur = WorktreeEntry{}
	}
	for _, line := range strings.Split(porcelain, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			if strings.TrimSpace(cur.Path) != "" {
				flush()
			}
			cur.Path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "HEAD "):
			cur.Commit = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			cur.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "bare":
			cur.Bare = true
		case line == "detached":
			cur.Branch = ""
		}
	}
	flush()
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// WorktreeOps is the git seam the scheduler runs through: production
// uses GitWorktreeOps (real git), tests inject fakes that simulate
// directories, conflicts and orphans without a repo.
type WorktreeOps interface {
	Create(ctx context.Context, repoRoot, base, branch, path string) error
	Remove(ctx context.Context, repoRoot, path string) error
	DeleteBranch(ctx context.Context, repoRoot, branch string) error
	List(ctx context.Context, repoRoot string) ([]WorktreeEntry, error)
	MergeBranch(ctx context.Context, repoRoot, branch, base string) (conflicts []string, err error)
	Prune(ctx context.Context, repoRoot string) error
}

// GitWorktreeOps runs native git: `worktree add -b`, `worktree
// remove --force`, `branch -D`, `worktree list --porcelain`,
// `merge --no-ff` onto a clean base, `worktree prune`.
type GitWorktreeOps struct{}

// gitRun executes one git command in dir and returns combined output.
func gitRun(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Create adds one isolated checkout: `git worktree add <path> -b
// <branch> <base>`. The branch is new per run (prefix/run/task), so a
// pre-existing branch fails closed instead of reusing state.
func (GitWorktreeOps) Create(ctx context.Context, repoRoot, base, branch, path string) error {
	if strings.TrimSpace(base) == "" {
		base = "HEAD"
	}
	out, err := gitRun(ctx, repoRoot, "git", "worktree", "add", path, "-b", branch, base)
	if err != nil {
		return fmt.Errorf("orchestrate parallel: git worktree add %s: %v (%s)", branch, err, strings.TrimSpace(out))
	}
	return nil
}

// Remove detaches one worktree: `git worktree remove --force <path>`.
func (GitWorktreeOps) Remove(ctx context.Context, repoRoot, path string) error {
	out, err := gitRun(ctx, repoRoot, "git", "worktree", "remove", "--force", path)
	if err != nil {
		return fmt.Errorf("orchestrate parallel: git worktree remove %s: %v (%s)", path, err, strings.TrimSpace(out))
	}
	return nil
}

// DeleteBranch drops one worker branch after a clean merge.
func (GitWorktreeOps) DeleteBranch(ctx context.Context, repoRoot, branch string) error {
	out, err := gitRun(ctx, repoRoot, "git", "branch", "-D", branch)
	if err != nil {
		return fmt.Errorf("orchestrate parallel: git branch -D %s: %v (%s)", branch, err, strings.TrimSpace(out))
	}
	return nil
}

// List returns every registered worktree.
func (GitWorktreeOps) List(ctx context.Context, repoRoot string) ([]WorktreeEntry, error) {
	out, err := gitRun(ctx, repoRoot, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("orchestrate parallel: git worktree list: %v (%s)", err, strings.TrimSpace(out))
	}
	return ParseWorktreeList(out), nil
}

// MergeBranch merges one worker branch into base with `--no-ff`.
// Preconditions fail closed: base must be checked out and the tree
// clean (unrelated dirt never hides inside a parallel merge). On a
// clean merge it returns no conflicts; on a conflict it leaves the
// tree conflicted for the integrator review, reports the unmerged
// files (`diff --name-only --diff-filter=U`) and returns them with a
// nil error — a conflict is a review state, not a git failure.
func (GitWorktreeOps) MergeBranch(ctx context.Context, repoRoot, branch, base string) ([]string, error) {
	if out, err := gitRun(ctx, repoRoot, "git", "rev-parse", "--abbrev-ref", "HEAD"); err != nil {
		return nil, fmt.Errorf("orchestrate parallel: cannot read current branch: %v (%s)", err, strings.TrimSpace(out))
	} else if strings.TrimSpace(out) != base && base != "HEAD" {
		if out2, err := gitRun(ctx, repoRoot, "git", "checkout", base); err != nil {
			return nil, fmt.Errorf("orchestrate parallel: cannot check out base %s: %v (%s)", base, err, strings.TrimSpace(out2))
		}
	}
	if out, err := gitRun(ctx, repoRoot, "git", "status", "--porcelain"); err != nil {
		return nil, fmt.Errorf("orchestrate parallel: cannot verify a clean tree: %v (%s)", err, strings.TrimSpace(out))
	} else if strings.TrimSpace(out) != "" {
		return nil, fmt.Errorf("orchestrate parallel: refusing to merge onto a dirty tree (commit or stash first)")
	}
	out, err := gitRun(ctx, repoRoot, "git", "merge", "--no-ff", "--no-edit", branch)
	if err == nil {
		return nil, nil
	}
	confOut, cerr := gitRun(ctx, repoRoot, "git", "diff", "--name-only", "--diff-filter=U")
	if cerr != nil {
		return nil, fmt.Errorf("orchestrate parallel: merge of %s failed: %v (%s)", branch, err, strings.TrimSpace(out))
	}
	var conflicts []string
	for _, line := range strings.Split(confOut, "\n") {
		if f := strings.TrimSpace(line); f != "" {
			conflicts = append(conflicts, f)
		}
	}
	sort.Strings(conflicts)
	if len(conflicts) == 0 {
		return nil, fmt.Errorf("orchestrate parallel: merge of %s failed: %v (%s)", branch, err, strings.TrimSpace(out))
	}
	return conflicts, nil
}

// Prune drops stale worktree metadata.
func (GitWorktreeOps) Prune(ctx context.Context, repoRoot string) error {
	if out, err := gitRun(ctx, repoRoot, "git", "worktree", "prune"); err != nil {
		return fmt.Errorf("orchestrate parallel: git worktree prune: %v (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

// FindParallelOrphans filters worktree entries to our abandoned
// workers: branch under prefix/ whose branch is not in the active
// set. Bare and detached entries are never ours. Sorted by path.
func FindParallelOrphans(entries []WorktreeEntry, activeBranches map[string]bool, prefix string) []WorktreeEntry {
	if strings.TrimSpace(prefix) == "" {
		prefix = DefaultParallelBranchPrefix
	}
	prefix += "/"
	var out []WorktreeEntry
	for _, e := range entries {
		if e.Bare || strings.TrimSpace(e.Branch) == "" {
			continue
		}
		if !strings.HasPrefix(e.Branch, prefix) {
			continue
		}
		if activeBranches[e.Branch] {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// PruneParallelOrphans removes every orphan worktree plus its branch
// and prunes stale metadata. It returns the removed paths in order.
// A removal failure stops the sweep and reports which orphan blocked
// it; already-removed orphans stay removed (best effort per orphan,
// fail closed across orphans).
func PruneParallelOrphans(ctx context.Context, ops WorktreeOps, repoRoot string, activeBranches map[string]bool, prefix string) ([]string, error) {
	entries, err := ops.List(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	orphans := FindParallelOrphans(entries, activeBranches, prefix)
	var removed []string
	for _, o := range orphans {
		if err := ops.Remove(ctx, repoRoot, o.Path); err != nil {
			return removed, err
		}
		// The branch may already be merged away; a branch-delete
		// failure after a successful worktree removal must not
		// resurrect the orphan as an error.
		_ = ops.DeleteBranch(ctx, repoRoot, o.Branch)
		removed = append(removed, o.Path)
	}
	if err := ops.Prune(ctx, repoRoot); err != nil {
		return removed, err
	}
	return removed, nil
}
