// In-memory worktree backend (F13.3/NOV-106): the scheduler seam
// for benchmarks and tests. It simulates directories, conflicts and
// orphans without a repo; production wires GitWorktreeOps. Merge
// conflicts are configured per branch; everything else succeeds.
package execute

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"regulus-ai/internal/orchestrate"
)

// memOps is a thread-safe in-memory WorktreeOps.
type memOps struct {
	mu        sync.Mutex
	branches  map[string]string
	conflicts map[string][]string
	mergeErr  map[string]error
	creates   int
	maxActive int
	active    int
}

// newMemOps builds an empty backend.
func newMemOps() *memOps {
	return &memOps{branches: map[string]string{}, conflicts: map[string][]string{}, mergeErr: map[string]error{}}
}

func (f *memOps) Create(ctx context.Context, repoRoot, base, branch, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Err() != nil {
		return fmt.Errorf("memops: %w", ctx.Err())
	}
	f.branches[branch] = path
	f.creates++
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	return nil
}

func (f *memOps) Remove(ctx context.Context, repoRoot, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for b, p := range f.branches {
		if p == path {
			delete(f.branches, b)
			f.active--
			return nil
		}
	}
	return nil
}

func (f *memOps) DeleteBranch(ctx context.Context, repoRoot, branch string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.branches, branch)
	return nil
}

func (f *memOps) List(ctx context.Context, repoRoot string) ([]orchestrate.WorktreeEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []orchestrate.WorktreeEntry
	for b, p := range f.branches {
		out = append(out, orchestrate.WorktreeEntry{Path: p, Branch: b, Commit: "mem"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (f *memOps) MergeBranch(ctx context.Context, repoRoot, branch, base string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, bad := f.mergeErr[branch]; bad {
		return nil, err
	}
	return append([]string(nil), f.conflicts[branch]...), nil
}

func (f *memOps) Prune(ctx context.Context, repoRoot string) error { return nil }
