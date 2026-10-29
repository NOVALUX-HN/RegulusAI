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
// (verbatim copy of the log) and .regulus-export/INDEX.md (a rendered
// digest grouped by type, with pinned and relations sections).
func Export(root string) (paths []string, err error) {
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
	return []string{jsonl, index}, nil
}
