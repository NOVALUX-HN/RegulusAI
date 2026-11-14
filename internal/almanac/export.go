package almanac

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/executil"
)

// NormalizeProjectName renders a git remote URL as owner/name, or the
// directory name when the remote is empty.
func NormalizeProjectName(remoteURL, dirName string) string {
	remote := strings.ToLower(strings.TrimSpace(remoteURL))
	remote = strings.TrimSuffix(remote, ".git")
	remote = strings.TrimPrefix(remote, "https://")
	remote = strings.TrimPrefix(remote, "http://")
	remote = strings.TrimPrefix(remote, "git@")
	if i := strings.Index(remote, ":"); i >= 0 && !strings.Contains(remote[:i], "/") {
		remote = remote[i+1:]
	} else if i := strings.Index(remote, "/"); i >= 0 {
		remote = remote[i+1:]
	}
	parts := strings.Split(strings.Trim(remote, "/"), "/")
	if len(parts) < 2 {
		return strings.ToLower(dirName)
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

// ProjectName resolves the project name from its git remote origin,
// falling back to the directory name.
func ProjectName(root string) string {
	cmd := exec.Command("git", "-C", root, "config", "--get", "remote.origin.url")
	executil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return strings.ToLower(filepath.Base(root))
	}
	return NormalizeProjectName(strings.TrimSpace(string(out)), filepath.Base(root))
}

// Export writes the committed team bundle: .regulus-export/almanac.jsonl
// (verbatim copy of the log), pins.jsonl, relations.jsonl and
// status.jsonl (verbatim copies of their stores, so a clean tree can
// restore the full Almanac), and .regulus-export/INDEX.md (a rendered
// digest grouped by type, with pinned and relations sections). A store
// with no file yet exports as an empty file — the bundle shape is
// stable, and `memory import` of an old bundle without the extra files
// still restores the observations. The whole snapshot runs under the
// store lock (the same lock appends serialize on), so concurrent saves
// cannot slip between the reads and leave a mixed bundle; MANIFEST.json
// (NOV-30 integrity) is pinned under the same lock. The manifest is
// written alongside the bundle but not counted in the returned paths —
// the five bundle files are the stable shape older tooling expects.
func Export(root string) (paths []string, err error) {
	err = withStoreLock(root, func() error {
		var inner error
		paths, inner = exportLocked(root)
		if inner != nil {
			return inner
		}
		_, inner = WriteManifest(filepath.Join(root, ".regulus-export"), ProjectName(root))
		return inner
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// exportLocked writes the five bundle files. The caller holds the store
// lock, so the reads below are one atomic snapshot.
func exportLocked(root string) (paths []string, err error) {
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	pins, err := PinnedIDs(root)
	if err != nil {
		return nil, err
	}
	rels, err := Relations(root)
	if err != nil {
		return nil, err
	}
	exportDir := filepath.Join(root, ".regulus-export")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return nil, err
	}
	// verbatim copy of the log
	src, err := os.ReadFile(File(root))
	if os.IsNotExist(err) {
		src = []byte("")
	} else if err != nil {
		return nil, err
	}
	jsonl := filepath.Join(exportDir, "almanac.jsonl")
	if err := os.WriteFile(jsonl, src, 0o644); err != nil {
		return nil, err
	}
	// rendered INDEX
	var b strings.Builder
	fmt.Fprintf(&b, "# Almanac export — %s\n\n", ProjectName(root))
	fmt.Fprintf(&b, "%d observation(s), %d pinned, %d relation(s).\n\n", len(all), len(pins), len(rels))
	b.WriteString("## Pinned\n\n")
	any := false
	for _, o := range all {
		if pins[o.ID] {
			any = true
			fmt.Fprintf(&b, "- `%s` %s: %s\n", o.ID, o.Type, o.Title)
		}
	}
	if !any {
		b.WriteString("_(none)_\n")
	}
	b.WriteString("\n## Relations\n\n")
	if len(rels) == 0 {
		b.WriteString("_(none)_\n")
	} else {
		for _, r := range rels {
			fmt.Fprintf(&b, "- `%s` **%s** `%s`\n", r.From, r.Relation, r.To)
		}
	}
	b.WriteString("\n## Observations\n\n")
	for _, o := range all {
		fmt.Fprintf(&b, "### `%s` — %s: %s\n\n- saved: %s\n", o.ID, o.Type, o.Title, o.SavedAt.Format(time.RFC3339))
		if o.Lesson != nil {
			fmt.Fprintf(&b, "- cue: %s\n- lesson: %s\n- confidence: %s\n", o.Lesson.Cue, o.Lesson.Lesson, o.Lesson.Confidence)
		}
		b.WriteString("\n")
	}
	index := filepath.Join(exportDir, "INDEX.md")
	if err := os.WriteFile(index, []byte(b.String()), 0o644); err != nil {
		return nil, err
	}
	paths = []string{jsonl, index}
	// Verbatim copies of the event stores, re-importable as-is: each
	// bundle file mirrors its store line for line (missing store file
	// means an empty bundle file, never an omission).
	stores := []struct {
		src  string
		name string
	}{
		{pinsFile(root), "pins.jsonl"},
		{relationsFile(root), "relations.jsonl"},
		{StatusFile(root), "status.jsonl"},
	}
	for _, s := range stores {
		raw, err := os.ReadFile(s.src)
		if os.IsNotExist(err) {
			raw = []byte("")
		} else if err != nil {
			return nil, err
		}
		dst := filepath.Join(exportDir, s.name)
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, dst)
	}
	return paths, nil
}
