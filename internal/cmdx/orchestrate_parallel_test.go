// Tests for the parallel/orphans thin orchestrators (CRAP paydown):
// parse/load/resolve/print helpers plus the two Cmds through fakes.
// Portable by construction: t.TempDir, in-memory WorktreeOps and
// stubbed run/mint/diff seams — no shell, no powershell, no git.
package cmdx

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/orchestrate"
)

// fakeOrphanOps is an in-memory WorktreeOps for orphan tests.
type fakeOrphanOps struct {
	entries   []orchestrate.WorktreeEntry
	listErr   error
	removeErr map[string]error
	pruneErr  error
	removed   []string
	deleted   []string
	pruned    int
}

func (f *fakeOrphanOps) Create(ctx context.Context, repoRoot, base, branch, path string) error {
	return errors.New("create not used in orphan tests")
}

func (f *fakeOrphanOps) Remove(ctx context.Context, repoRoot, path string) error {
	if err, ok := f.removeErr[path]; ok && err != nil {
		return err
	}
	f.removed = append(f.removed, path)
	keep := f.entries[:0]
	for _, e := range f.entries {
		if e.Path != path {
			keep = append(keep, e)
		}
	}
	f.entries = keep
	return nil
}

func (f *fakeOrphanOps) DeleteBranch(ctx context.Context, repoRoot, branch string) error {
	f.deleted = append(f.deleted, branch)
	return nil
}

func (f *fakeOrphanOps) List(ctx context.Context, repoRoot string) ([]orchestrate.WorktreeEntry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]orchestrate.WorktreeEntry(nil), f.entries...), nil
}

func (f *fakeOrphanOps) MergeBranch(ctx context.Context, repoRoot, branch, base string) ([]string, error) {
	return nil, errors.New("merge not used in orphan tests")
}

func (f *fakeOrphanOps) Prune(ctx context.Context, repoRoot string) error {
	if f.pruneErr != nil {
		return f.pruneErr
	}
	f.pruned++
	return nil
}

func writeTasksFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func validTasksJSON() string {
	return `[{"id":"a","title":"Task A","files":["pkg/a.go"]},{"id":"b","title":"Task B","files":["pkg/b.go"]}]`
}

func stubParallelSeams(t *testing.T, run func(ctx context.Context, req orchestrate.ParallelRequest) (orchestrate.ParallelReport, error)) {
	t.Helper()
	oldRun := parallelRunFunc
	oldMint := parallelMintRunID
	oldOps := parallelNewOps
	parallelRunFunc = run
	parallelMintRunID = func() (string, error) { return "run_test", nil }
	parallelNewOps = func() orchestrate.WorktreeOps { return &fakeOrphanOps{} }
	t.Cleanup(func() {
		parallelRunFunc = oldRun
		parallelMintRunID = oldMint
		parallelNewOps = oldOps
	})
}

func TestParseOrchestrateParallelFlagsFail(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := cmdIO(dir)
	if code := io.OrchestrateParallelCmd(nil); code != 2 {
		t.Fatalf("missing tasks exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage:") {
		t.Fatalf("parse fail must print usage, got %q", errb.String())
	}
	_, code, ok := parseOrchestrateParallelFlags([]string{"--tasks", "x.json", "extra"}, io.Stderr)
	if ok || code != 2 {
		t.Fatalf("positional exit = %d ok=%v, want 2/false", code, ok)
	}
	_, code, ok = parseOrchestrateParallelFlags([]string{"--tasks", "-bogus"}, io.Stderr)
	if ok || code != 2 {
		t.Fatalf("dash value exit = %d ok=%v, want 2/false", code, ok)
	}
	if _, code, ok := parseOrchestrateParallelFlags([]string{"--nope"}, io.Stderr); ok || code != 2 {
		t.Fatalf("unknown flag exit = %d ok=%v, want 2/false", code, ok)
	}
}

func TestParallelHasFlagValue(t *testing.T) {
	if parallelHasFlagValue(orchestrateParallelOptions{tasks: "t.json"}) {
		t.Fatal("clean values must not look like flags")
	}
	if !parallelHasFlagValue(orchestrateParallelOptions{tasks: "t.json", out: "-o"}) {
		t.Fatal("dash out must read as a flag")
	}
	if !parallelHasFlagValue(orchestrateParallelOptions{tasks: "t.json", worktreeRoot: "-w"}) {
		t.Fatal("dash worktree-root must read as a flag")
	}
}

func TestLoadParallelTasksFail(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := cmdIO(dir)
	if _, code, ok := loadParallelTasks(filepath.Join(dir, "missing.json"), io.Stderr); ok || code != 1 {
		t.Fatalf("missing file exit = %d ok=%v, want 1/false", code, ok)
	}
	bad := writeTasksFile(t, dir, "bad.json", "{not json")
	if _, code, ok := loadParallelTasks(bad, io.Stderr); ok || code != 1 {
		t.Fatalf("malformed exit = %d ok=%v, want 1/false", code, ok)
	}
	if !strings.Contains(errb.String(), "malformed tasks file") {
		t.Fatalf("malformed must say so, got %q", errb.String())
	}
	empty := writeTasksFile(t, dir, "empty.json", `[]`)
	if _, code, ok := loadParallelTasks(empty, io.Stderr); ok || code != 1 {
		t.Fatalf("empty plan exit = %d ok=%v, want 1/false", code, ok)
	}
	unknown := writeTasksFile(t, dir, "unknown.json", `[{"id":"a","title":"A","files":["x.go"],"bogus":1}]`)
	if _, code, ok := loadParallelTasks(unknown, io.Stderr); ok || code != 1 {
		t.Fatalf("unknown field exit = %d ok=%v, want 1/false", code, ok)
	}
	if code := io.OrchestrateParallelCmd([]string{"--tasks", filepath.Join(dir, "missing.json")}); code != 1 {
		t.Fatalf("cmd load fail exit = %d, want 1", code)
	}
}

func TestResolveParallelHelpers(t *testing.T) {
	if got := resolveParallelWorktreeRoot("/repo", "/tmp/wt"); got != "/tmp/wt" {
		t.Fatalf("explicit root = %q, want /tmp/wt", got)
	}
	want := filepath.Join("/repo", ".regulus", "orchestrate", "worktrees")
	if got := resolveParallelWorktreeRoot("/repo", ""); got != want {
		t.Fatalf("default root = %q, want %q", got, want)
	}
	b := resolveParallelBudgets(orchestrateParallelOptions{maxConcurrency: 4})
	if b.MaxQueued != orchestrate.MaxParallelTasks {
		t.Fatalf("default MaxQueued = %d, want %d", b.MaxQueued, orchestrate.MaxParallelTasks)
	}
	b = resolveParallelBudgets(orchestrateParallelOptions{maxConcurrency: 2, maxQueued: 3})
	if b.MaxQueued != 3 {
		t.Fatalf("explicit MaxQueued = %d, want 3", b.MaxQueued)
	}
	if id, err := resolveParallelRunID(true, func() (string, error) { return "x", nil }); err != nil || id != "dry-run" {
		t.Fatalf("dry run id = %q %v, want dry-run", id, err)
	}
	if id, err := resolveParallelRunID(false, func() (string, error) { return "run_1", nil }); err != nil || id != "run_1" {
		t.Fatalf("wet run id = %q %v, want run_1", id, err)
	}
	if _, err := resolveParallelRunID(false, func() (string, error) { return "", errors.New("boom") }); err == nil {
		t.Fatal("mint failure must propagate")
	}
	if !shouldWriteParallelReport(orchestrateParallelOptions{out: "r"}) {
		t.Fatal("wet out must persist")
	}
	if shouldWriteParallelReport(orchestrateParallelOptions{dryRun: true, out: "r"}) {
		t.Fatal("dry-run must never persist --out")
	}
	if shouldWriteParallelReport(orchestrateParallelOptions{}) {
		t.Fatal("empty out must not persist")
	}
}

func TestOrchestrateParallelDryRun(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	tasks := writeTasksFile(t, dir, "tasks.json", validTasksJSON())
	out.Reset()
	errb.Reset()
	if code := io.OrchestrateParallelCmd([]string{"--tasks", tasks, "--dry-run"}); code != 0 {
		t.Fatalf("dry-run exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "dry-run") {
		t.Fatalf("dry-run must say so, got %q", out.String())
	}
	out.Reset()
	if code := io.OrchestrateParallelCmd([]string{"--tasks", tasks, "--dry-run", "--json"}); code != 0 {
		t.Fatalf("dry-run --json exit = %d stderr = %q", code, errb.String())
	}
	var rep orchestrate.ParallelReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("dry-run --json must decode: %v\n%s", err, out.String())
	}
	if !rep.DryRun || rep.RunID != "dry-run" {
		t.Fatalf("dry report must carry the dry run, got %+v", rep)
	}
	// Dry-run ignores --out: plans print, nothing persists.
	outDir := filepath.Join(dir, "reports")
	out.Reset()
	if code := io.OrchestrateParallelCmd([]string{"--tasks", tasks, "--dry-run", "--out", outDir}); code != 0 {
		t.Fatalf("dry-run --out exit = %d stderr = %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "parallel-report.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not persist --out (err=%v)", err)
	}
}

func TestOrchestrateParallelWetFake(t *testing.T) {
	dir := t.TempDir()
	tasks := writeTasksFile(t, dir, "tasks.json", validTasksJSON())
	t.Run("success", func(t *testing.T) {
		io, out, errb := cmdIO(dir)
		var seen orchestrate.ParallelRequest
		stubParallelSeams(t, func(ctx context.Context, req orchestrate.ParallelRequest) (orchestrate.ParallelReport, error) {
			seen = req
			return orchestrate.ParallelReport{RunID: req.RunID, Order: []string{"a"}}, nil
		})
		if code := io.OrchestrateParallelCmd([]string{"--tasks", tasks}); code != 0 {
			t.Fatalf("wet exit = %d stderr = %q", code, errb.String())
		}
		if seen.DryRun || seen.RunID != "run_test" {
			t.Fatalf("wet request must mint the run, got %+v", seen)
		}
		if seen.Ops == nil || seen.Work == nil || seen.Gate == nil || seen.Record == nil {
			t.Fatal("wet request must wire ops/work/gate/record")
		}
		if seen.Budgets.MaxQueued != orchestrate.MaxParallelTasks {
			t.Fatalf("wet budgets must default MaxQueued, got %+v", seen.Budgets)
		}
		if out.Len() == 0 {
			t.Fatal("wet success must print the report")
		}
	})
	t.Run("runError", func(t *testing.T) {
		io, _, errb := cmdIO(dir)
		stubParallelSeams(t, func(ctx context.Context, req orchestrate.ParallelRequest) (orchestrate.ParallelReport, error) {
			return orchestrate.ParallelReport{}, errors.New("boom")
		})
		if code := io.OrchestrateParallelCmd([]string{"--tasks", tasks}); code != 1 {
			t.Fatalf("run error exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "orchestrate parallel: boom") {
			t.Fatalf("run error must name itself, got %q", errb.String())
		}
	})
	t.Run("mintError", func(t *testing.T) {
		io, _, errb := cmdIO(dir)
		oldMint := parallelMintRunID
		parallelMintRunID = func() (string, error) { return "", errors.New("no id") }
		t.Cleanup(func() { parallelMintRunID = oldMint })
		if code := io.OrchestrateParallelCmd([]string{"--tasks", tasks}); code != 1 {
			t.Fatalf("mint error exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "orchestrate parallel: no id") {
			t.Fatalf("mint error must name itself, got %q", errb.String())
		}
	})
	t.Run("failedAndCleanup", func(t *testing.T) {
		io, _, errb := cmdIO(dir)
		stubParallelSeams(t, func(ctx context.Context, req orchestrate.ParallelRequest) (orchestrate.ParallelReport, error) {
			return orchestrate.ParallelReport{Failed: true, CleanupErrors: []string{"a: boom"}}, nil
		})
		if code := io.OrchestrateParallelCmd([]string{"--tasks", tasks}); code != 1 {
			t.Fatalf("failed exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "left for `regulus orchestrate orphans --prune`") {
			t.Fatalf("cleanup must point at orphans --prune, got %q", errb.String())
		}
		if !strings.Contains(errb.String(), "run failed (worktrees preserved for review)") {
			t.Fatalf("failure must name preserved worktrees, got %q", errb.String())
		}
	})
	t.Run("cleanupOnly", func(t *testing.T) {
		io, _, errb := cmdIO(dir)
		stubParallelSeams(t, func(ctx context.Context, req orchestrate.ParallelRequest) (orchestrate.ParallelReport, error) {
			return orchestrate.ParallelReport{CleanupErrors: []string{"a: boom"}}, nil
		})
		if code := io.OrchestrateParallelCmd([]string{"--tasks", tasks}); code != 0 {
			t.Fatalf("cleanup-only exit = %d, want 0 stderr = %q", code, errb.String())
		}
		if !strings.Contains(errb.String(), "1 worktree(s) left") {
			t.Fatalf("cleanup notice must count, got %q", errb.String())
		}
	})
}

func TestOrchestrateParallelOut(t *testing.T) {
	dir := t.TempDir()
	tasks := writeTasksFile(t, dir, "tasks.json", validTasksJSON())
	io, out, errb := cmdIO(dir)
	stubParallelSeams(t, func(ctx context.Context, req orchestrate.ParallelRequest) (orchestrate.ParallelReport, error) {
		return orchestrate.ParallelReport{RunID: "run_test", Order: []string{"a"}}, nil
	})
	outDir := filepath.Join(dir, "reports")
	if code := io.OrchestrateParallelCmd([]string{"--tasks", tasks, "--out", outDir}); code != 0 {
		t.Fatalf("out exit = %d stderr = %q", code, errb.String())
	}
	raw, err := os.ReadFile(filepath.Join(outDir, "parallel-report.json"))
	if err != nil {
		t.Fatalf("out must persist the report: %v", err)
	}
	var rep orchestrate.ParallelReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("persisted report must decode: %v", err)
	}
	if !strings.Contains(out.String(), "orchestrate: parallel report recorded in "+outDir) {
		t.Fatalf("out must confirm the dir, got %q", out.String())
	}
	// Unwritable out fails closed with the same prefix.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	io2, _, errb2 := cmdIO(dir)
	stubParallelSeams(t, func(ctx context.Context, req orchestrate.ParallelRequest) (orchestrate.ParallelReport, error) {
		return orchestrate.ParallelReport{}, nil
	})
	if code := io2.OrchestrateParallelCmd([]string{"--tasks", tasks, "--out", filepath.Join(blocker, "sub")}); code != 1 {
		t.Fatalf("bad out exit = %d, want 1", code)
	}
	if !strings.Contains(errb2.String(), "orchestrate parallel:") {
		t.Fatalf("bad out must name itself, got %q", errb2.String())
	}
}

func TestDiffCheckOutcome(t *testing.T) {
	if got := diffCheckOutcome("", "", 0, errors.New("boom")); !got.Ran || got.Passed {
		t.Fatalf("run error must read ran-without-pass: %+v", got)
	}
	got := diffCheckOutcome("out", "errb", 1, nil)
	if !got.Ran || got.Passed || !strings.Contains(got.Detail, "diff-check failed:") {
		t.Fatalf("nonzero exit must fail the gate: %+v", got)
	}
	big := strings.Repeat("x", 600)
	got = diffCheckOutcome(big, "", 1, nil)
	if len(got.Detail) > len("diff-check failed: ")+500 {
		t.Fatalf("detail must truncate at 500 chars, got %d", len(got.Detail))
	}
	got = diffCheckOutcome("", "", 0, nil)
	if !got.Passed || got.Detail != "diff-check clean" {
		t.Fatalf("clean exit must pass: %+v", got)
	}
}

func TestParallelGateWiring(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	// Empty task runs the diff seam without shells.
	oldDiff := parallelDiffCheckRun
	parallelDiffCheckRun = func(ctx context.Context, repoRoot string) (string, string, int, error) {
		return "", "", 0, nil
	}
	t.Cleanup(func() { parallelDiffCheckRun = oldDiff })
	if got := io.parallelGate("")(context.Background(), dir); !got.Passed {
		t.Fatalf("stubbed diff must pass: %+v", got)
	}
	// Named task without regulus.json fails closed before any runner.
	if got := io.parallelGate("gauntlet")(context.Background(), dir); got.Ran == false || got.Passed || got.Task != "gauntlet" {
		t.Fatalf("missing gauntlet config must fail closed: %+v", got)
	}
	if !strings.Contains(io.parallelGate("gauntlet")(context.Background(), dir).Detail, "gate load:") {
		t.Fatal("gauntlet load failure must name itself")
	}
}

func TestOrphansParseFail(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := cmdIO(dir)
	if code := io.OrchestrateOrphansCmd([]string{"extra"}); code != 2 {
		t.Fatalf("positional exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage:") {
		t.Fatalf("positional must print usage, got %q", errb.String())
	}
	if code := io.OrchestrateOrphansCmd([]string{"--nope"}); code != 2 {
		t.Fatalf("unknown flag exit = %d, want 2", code)
	}
}

func orphanEntries() []orchestrate.WorktreeEntry {
	return []orchestrate.WorktreeEntry{
		{Path: "/wt/a", Branch: "parallel/run/a"},
		{Path: "/wt/b", Branch: "parallel/run/b"},
		{Path: "/wt/user", Branch: "feature/x"},
	}
}

func stubOrphanOps(t *testing.T, ops *fakeOrphanOps) {
	t.Helper()
	old := orphanOpsFunc
	orphanOpsFunc = func() orchestrate.WorktreeOps { return ops }
	t.Cleanup(func() { orphanOpsFunc = old })
}

func TestOrphansList(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := cmdIO(dir)
	stubOrphanOps(t, &fakeOrphanOps{entries: orphanEntries()})
	out.Reset()
	if code := io.OrchestrateOrphansCmd(nil); code != 0 {
		t.Fatalf("list exit = %d", code)
	}
	if !strings.Contains(out.String(), `orphans: 2 worktree(s) under "parallel/"`) {
		t.Fatalf("list must count parallel orphans, got %q", out.String())
	}
	if strings.Contains(out.String(), "feature/x") {
		t.Fatalf("list must not sweep user branches, got %q", out.String())
	}
	out.Reset()
	if code := io.OrchestrateOrphansCmd([]string{"--json"}); code != 0 {
		t.Fatalf("list --json exit = %d", code)
	}
	var listed []orchestrate.WorktreeEntry
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil || len(listed) != 2 {
		t.Fatalf("list --json must decode 2 orphans: %v %q", err, out.String())
	}
	// Active branches stay protected.
	out.Reset()
	if code := io.OrchestrateOrphansCmd([]string{"--active", "parallel/run/a"}); code != 0 {
		t.Fatalf("active exit = %d", code)
	}
	if !strings.Contains(out.String(), "orphans: 1 worktree(s)") {
		t.Fatalf("active must protect one branch, got %q", out.String())
	}
}

func TestOrphansListError(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := cmdIO(dir)
	stubOrphanOps(t, &fakeOrphanOps{listErr: errors.New("no git")})
	if code := io.OrchestrateOrphansCmd(nil); code != 1 {
		t.Fatalf("list error exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "orchestrate orphans: no git") {
		t.Fatalf("list error must name itself, got %q", errb.String())
	}
}

func TestOrphansPrune(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := cmdIO(dir)
	ops := &fakeOrphanOps{entries: orphanEntries()}
	stubOrphanOps(t, ops)
	if code := io.OrchestrateOrphansCmd([]string{"--prune"}); code != 0 {
		t.Fatalf("prune exit = %d", code)
	}
	if !strings.Contains(out.String(), "orphans: pruned 2 worktree(s)") {
		t.Fatalf("prune must count, got %q", out.String())
	}
	if ops.pruned != 1 || len(ops.removed) != 2 {
		t.Fatalf("prune must remove both orphans and prune once: %+v", ops)
	}
	// JSON form decodes to the removed paths.
	ops2 := &fakeOrphanOps{entries: orphanEntries()}
	stubOrphanOps(t, ops2)
	out.Reset()
	if code := io.OrchestrateOrphansCmd([]string{"--prune", "--json"}); code != 0 {
		t.Fatalf("prune --json exit = %d", code)
	}
	var removed []string
	if err := json.Unmarshal(out.Bytes(), &removed); err != nil || len(removed) != 2 {
		t.Fatalf("prune --json must decode 2 paths: %v %q", err, out.String())
	}
}

func TestOrphansPruneError(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := cmdIO(dir)
	stubOrphanOps(t, &fakeOrphanOps{listErr: errors.New("no git")})
	if code := io.OrchestrateOrphansCmd([]string{"--prune"}); code != 1 {
		t.Fatalf("prune list error exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "orchestrate orphans: no git") {
		t.Fatalf("prune list error must name itself, got %q", errb.String())
	}
	ops := &fakeOrphanOps{
		entries:   orphanEntries(),
		removeErr: map[string]error{"/wt/a": errors.New("locked")},
	}
	stubOrphanOps(t, ops)
	errb.Reset()
	if code := io.OrchestrateOrphansCmd([]string{"--prune"}); code != 1 {
		t.Fatalf("prune remove error exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "orchestrate orphans: locked") {
		t.Fatalf("prune remove error must name itself, got %q", errb.String())
	}
}
