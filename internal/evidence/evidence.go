// Package evidence reads back the gauntlet's append-only receipts for the
// human audit (PRD R-G4, R-G6): the reviewer reads receipts, never diffs.
package evidence

import (
	"path/filepath"
	"sort"

	"regulus-ai/internal/receipts"
)

// Entry pairs one receipt with the file that holds it.
type Entry struct {
	Path    string
	Receipt receipts.Receipt
}

// Dir returns the evidence root under a project.
func Dir(root string) string {
	return filepath.Join(root, ".regulus", "evidence")
}

// TaskDir returns the evidence directory of one task.
func TaskDir(root, task string) string {
	return filepath.Join(Dir(root), receipts.Slug(task))
}

// List returns receipts ordered by path (task slug, then the nanosecond
// slot — chronological). A task of "" lists every task's receipts.
func List(root, task string) ([]Entry, error) {
	var paths []string
	var err error
	if task == "" {
		paths, err = filepath.Glob(filepath.Join(Dir(root), "*", "*.json"))
	} else {
		paths, err = filepath.Glob(filepath.Join(TaskDir(root, task), "*.json"))
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	entries := make([]Entry, 0, len(paths))
	for _, p := range paths {
		rec, err := receipts.ReadFile(p)
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{Path: p, Receipt: rec})
	}
	return entries, nil
}
