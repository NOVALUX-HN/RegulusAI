// Package evidence reads back the gauntlet's append-only receipts for the
// human audit (PRD R-G4, R-G6): the reviewer reads receipts, never diffs.
package evidence

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
//
// It is the raw join (Dir + Slug): callers handling untrusted task input
// must use TaskDirSafe, which refuses traversal/absolute escapes (S2 jail).
func TaskDir(root, task string) string {
	return filepath.Join(Dir(root), receipts.Slug(task))
}

// TaskDirSafe resolves the evidence directory of one task, refusing inputs
// that could escape Dir(root) (S2 jail, defense in depth: callers already
// validate task ids upstream, but Plan/Verify/List must not trust them).
// The refusal error always contains "evidence: task escapes evidence dir".
func TaskDirSafe(root, task string) (string, error) {
	slug := receipts.Slug(task)
	if err := checkSlug(task, slug); err != nil {
		return "", err
	}
	dir := Dir(root)
	joined := filepath.Join(dir, slug)
	// Backstop: even if a future Slug change reintroduced separators, the
	// resolved dir must stay at or inside the evidence root.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if absJoined != absDir && !strings.HasPrefix(absJoined, absDir+string(filepath.Separator)) {
		return "", fmt.Errorf("evidence: task escapes evidence dir: %q", task)
	}
	return joined, nil
}

// checkSlug refuses directory names that look like traversal or absolute
// paths. Slug maps "/" to "__", so a slugged "..__evil" or "__abs" would
// land harmlessly inside the evidence root — yet the input was an escape
// attempt and must be refused, not neutralized silently.
func checkSlug(task, slug string) error {
	refuse := func() error {
		return fmt.Errorf("evidence: task escapes evidence dir: %q", task)
	}
	if strings.Contains(task, "\x00") || strings.Contains(slug, "\x00") {
		return refuse()
	}
	// Residual separators: Slug only maps "/"; a "\" is a separator on
	// Windows and never legitimate in a task id. A "/" surviving in the
	// slug would mean Slug changed: refuse too.
	if strings.Contains(task, "\\") || strings.Contains(slug, "\\") ||
		strings.Contains(slug, "/") {
		return refuse()
	}
	// Absolute paths: unix "/abs" plus Windows drive/UNC forms ("C:\x",
	// "C:/x", "C:x"). filepath.IsAbs covers the platform-native form;
	// the drive pattern covers Windows forms on any platform, and the
	// leading-slash check covers unix forms on Windows (where Go does not
	// deem "/abs" absolute), so verdicts are identical everywhere.
	// "~" (home expansion attempt) too.
	if filepath.IsAbs(task) || filepath.IsAbs(slug) ||
		strings.HasPrefix(task, "/") ||
		isWindowsAbsolute(task) || isWindowsAbsolute(slug) ||
		task == "~" || slug == "~" ||
		strings.HasPrefix(task, "~/") || strings.HasPrefix(slug, "~/") ||
		strings.HasPrefix(task, `~\`) || strings.HasPrefix(slug, `~\`) {
		return refuse()
	}
	// ".." anywhere in the slug: covers "..", "../evil" (slugged to
	// "..__evil"), "a/../b", and pre-slugged "..__evil" alike. No
	// legitimate chart task id contains a double dot.
	if strings.Contains(slug, "..") {
		return refuse()
	}
	return nil
}

// isWindowsAbsolute reports drive-anchored paths ("C:\x", "C:/x", "C:x").
func isWindowsAbsolute(s string) bool {
	return len(s) >= 2 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && s[1] == ':'
}

// listPaths returns receipt candidate paths ordered by path (task slug,
// then the nanosecond slot — chronological). Candidate manifests
// (manifest-*.json) live in the same directories but are indexes, not
// receipts, and callers skip them.
func listPaths(root, task string) ([]string, error) {
	var paths []string
	var err error
	if task == "" {
		paths, err = filepath.Glob(filepath.Join(Dir(root), "*", "*.json"))
	} else {
		var dir string
		dir, err = TaskDirSafe(root, task)
		if err != nil {
			return nil, err
		}
		paths, err = filepath.Glob(filepath.Join(dir, "*.json"))
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// List returns receipts ordered by path (task slug, then the nanosecond
// slot — chronological). A task of "" lists every task's receipts.
// Candidate manifests (manifest-*.json) live in the same directories but
// are indexes, not receipts, and are skipped here.
//
// List is strict: the first malformed receipt aborts with an error.
// Tolerant readers (the envfacts miner, H2) use ListSkipping instead,
// which skips corrupt files with a stderr warning and counts them.
func List(root, task string) ([]Entry, error) {
	paths, err := listPaths(root, task)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(paths))
	for _, p := range paths {
		if strings.HasPrefix(filepath.Base(p), "manifest-") {
			continue
		}
		rec, err := receipts.ReadFile(p)
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{Path: p, Receipt: rec})
	}
	return entries, nil
}

// isMalformedReceipt reports whether err came from a corrupt receipt
// document (receipts.ReadFile wraps JSON failures as "malformed
// receipt"). I/O failures are not malformed: they still abort the
// caller.
func isMalformedReceipt(err error) bool {
	return err != nil && strings.Contains(err.Error(), "malformed receipt")
}

// ListSkipping is the tolerant H2 reader for the envfacts miner: like
// List but a malformed receipt file is skipped — counted in the
// returned corrupt total with a "path:line" warning on stderr — while
// healthy receipts keep flowing. Non-malformed failures (I/O, jail
// refusal, glob) still abort. Line is always 1: one receipt file holds
// one JSON document.
func ListSkipping(root, task string) ([]Entry, int, error) {
	paths, err := listPaths(root, task)
	if err != nil {
		return nil, 0, err
	}
	entries := make([]Entry, 0, len(paths))
	corrupt := 0
	for _, p := range paths {
		if strings.HasPrefix(filepath.Base(p), "manifest-") {
			continue
		}
		rec, err := receipts.ReadFile(p)
		if err != nil {
			if !isMalformedReceipt(err) {
				return nil, corrupt, err
			}
			corrupt++
			fmt.Fprintf(os.Stderr, "evidence: skipping corrupt receipt %s:1: %v\n", p, err)
			continue
		}
		entries = append(entries, Entry{Path: p, Receipt: rec})
	}
	return entries, corrupt, nil
}
