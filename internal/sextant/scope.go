// Worker scope enforcement (Phase 2, S11/NOV-32): the chart declares the
// machine-checkable boundary its tasks inherit, and task close refuses
// worktree files outside it. Scope is chart-level (one ## Contract per
// chart): every task row inherits the same boundary. The human who
// approves the chart authorizes the boundary; a worker never widens its
// own scope (widening means editing Scope plus a REPLAN log line).
package sextant

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Scope is one chart's machine-checkable worker boundary: repo-relative
// allow/deny globs (prefix/path, case-insensitive) read from the
// ## Contract section's Scope Allow:/Deny: lines. Declared reports
// whether the chart states any machine scope; without it there is no
// enforcement and legacy charts keep working.
type Scope struct {
	Declared bool
	Allows   []string
	Denys    []string
}

// scopeSectionRe bounds the ## Contract section for scope parsing: the
// next top-level section ends it (Scope lines live inside Contract).
var scopeSectionRe = regexp.MustCompile(`(?m)^## `)

// ParseScope reads the chart's machine scope from the ## Contract
// section's Scope Allow:/Deny: lines (documented in docs/sextant.md).
// No Allow:/Deny: lines means no declared scope: enforcement stays off
// so legacy charts keep working. A bad declaration (traversal,
// absolute, drive-anchored or empty pattern) is a hard error: the
// boundary must be rejected, not silently neutralized.
func ParseScope(doc string) (Scope, error) {
	var sc Scope
	loc := contractHeadingRe.FindStringIndex(doc)
	if loc == nil {
		return sc, nil
	}
	start := loc[1]
	end := len(doc)
	if m := scopeSectionRe.FindStringIndex(doc[start:]); m != nil {
		end = start + m[0]
	}
	for _, line := range strings.Split(doc[start:end], "\n") {
		t := strings.TrimSpace(line)
		t = strings.TrimPrefix(t, "- ")
		key, value, ok := strings.Cut(t, ":")
		if !ok {
			continue
		}
		var deny bool
		switch {
		case strings.EqualFold(strings.TrimSpace(key), "allow"):
		case strings.EqualFold(strings.TrimSpace(key), "deny"):
			deny = true
		default:
			continue
		}
		norm, err := cleanScopePattern(value)
		if err != nil {
			kind := "Allow"
			if deny {
				kind = "Deny"
			}
			return Scope{}, fmt.Errorf("scope: invalid %s pattern %q: %v", kind, strings.TrimSpace(value), err)
		}
		sc.Declared = true
		if deny {
			sc.Denys = append(sc.Denys, norm)
		} else {
			sc.Allows = append(sc.Allows, norm)
		}
	}
	return sc, nil
}

// cleanScopePattern normalizes one declared pattern the way the jail
// refuses escapes (see evidence.TaskDirSafe): repo-relative, Clean +
// ToSlash, no ".." segments before or after cleaning, no absolutes, no
// Windows drive anchors. Matching is case-insensitive, so the stored
// form is folded; "" marks the repo root (Allow: .).
func cleanScopePattern(pattern string) (string, error) {
	t := strings.TrimSpace(pattern)
	if t == "" || strings.Contains(t, "\x00") {
		return "", fmt.Errorf("must be a non-empty repo-relative path or prefix glob")
	}
	slashed := strings.ReplaceAll(t, "\\", "/")
	if err := refuseScopeEscape(slashed); err != nil {
		return "", err
	}
	cleaned := path.Clean(slashed)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("must be repo-relative, no absolute paths or ..")
	}
	return strings.ToLower(cleaned), nil
}

// refuseScopeEscape rejects absolute, drive-anchored and parent-escaping
// lexemes — the same refusal shape TaskDirSafe enforces, applied to
// scope declarations and worktree paths alike.
func refuseScopeEscape(slashed string) error {
	if slashed == "" || strings.Contains(slashed, "\x00") {
		return fmt.Errorf("must be repo-relative, no absolute paths or ..")
	}
	if strings.HasPrefix(slashed, "/") || scopeIsWindowsAbsolute(slashed) ||
		slashed == "~" || strings.HasPrefix(slashed, "~/") {
		return fmt.Errorf("must be repo-relative, no absolute paths or ..")
	}
	for _, seg := range strings.Split(slashed, "/") {
		if seg == ".." {
			return fmt.Errorf("must be repo-relative, no absolute paths or ..")
		}
	}
	return nil
}

// scopeIsWindowsAbsolute reports drive-anchored paths ("C:\x", "C:/x",
// "C:x"), mirroring evidence.checkSlug so verdicts match on any host.
func scopeIsWindowsAbsolute(s string) bool {
	return len(s) >= 2 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && s[1] == ':'
}

// CheckScopeFiles refuses worktree files outside the scope, naming the
// first offender as `fuera de scope: <path>`. Deny wins over Allow; when
// Allows is non-empty a file must match at least one. Every entry is
// evaluated separately, so a rename contributes its two paths. Without
// a declared scope there is no enforcement (nil). Paths resolve against
// the process working directory.
func (s Scope) CheckScopeFiles(files []string) error {
	return s.checkFiles("", files)
}

// CheckScopeFilesIn is CheckScopeFiles with worktree paths resolved
// against root (the CLI passes the project root: git lists
// root-relative paths and the process cwd may be a subdirectory).
func (s Scope) CheckScopeFilesIn(root string, files []string) error {
	return s.checkFiles(root, files)
}

func (s Scope) checkFiles(root string, files []string) error {
	if !s.Declared {
		return nil
	}
	base := root
	if base == "" {
		var err error
		base, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("fuera de scope: cannot resolve worktree: %v", err)
		}
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("fuera de scope: cannot resolve worktree: %v", err)
	}
	// Resolve the base itself: TempDir/macOS hosts often nest the
	// worktree under a symlinked parent (/var -> /private/var), and
	// EvalSymlinks below resolves those too — both sides must agree.
	if resolved, rerr := filepath.EvalSymlinks(absBase); rerr == nil {
		absBase = resolved
	}
	for _, f := range files {
		for _, part := range splitScopeRename(f) {
			norm, ok := resolveScopeFile(absBase, part)
			if !ok {
				return fmt.Errorf("fuera de scope: %s", strings.TrimSpace(part))
			}
			if matchAnyScopePattern(s.Denys, norm) {
				return fmt.Errorf("fuera de scope: %s", strings.TrimSpace(part))
			}
			if len(s.Allows) > 0 && !matchAnyScopePattern(s.Allows, norm) {
				return fmt.Errorf("fuera de scope: %s", strings.TrimSpace(part))
			}
		}
	}
	return nil
}

// splitScopeRename evaluates rename entries as their two paths
// separately ("old -> new"); ordinary entries pass through unchanged.
func splitScopeRename(entry string) []string {
	if old, new, ok := strings.Cut(entry, " -> "); ok {
		var out []string
		for _, part := range []string{old, new} {
			if t := strings.TrimSpace(part); t != "" {
				out = append(out, t)
			}
		}
		if len(out) != 0 {
			return out
		}
	}
	return []string{entry}
}

// resolveScopeFile normalizes one worktree path to its folded,
// root-relative form. A path that exists is normalized through
// filepath.EvalSymlinks so a symlink escaping the worktree (or into
// denied ground) is judged by its target; a missing path is judged by
// its clean lexeme. Traversal, absolute and drive-anchored forms never
// resolve (false), following the TaskDirSafe refusal pattern.
func resolveScopeFile(absBase, f string) (string, bool) {
	if strings.Contains(f, "\x00") {
		return "", false
	}
	t := strings.TrimSpace(f)
	if t == "" {
		return "", false
	}
	full := t
	if !filepath.IsAbs(full) && !scopeIsWindowsAbsolute(full) {
		full = filepath.Join(absBase, t)
	}
	if _, err := os.Lstat(full); err == nil {
		if resolved, rerr := filepath.EvalSymlinks(full); rerr == nil {
			rel, relErr := filepath.Rel(absBase, resolved)
			if relErr != nil || rel == "" || rel == ".." ||
				strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
				filepath.IsAbs(rel) || scopeIsWindowsAbsolute(rel) {
				return "", false
			}
			t = rel
		}
		// A dangling symlink (EvalSymlinks fails) falls back to the
		// clean lexeme below, as if the path did not exist.
	}
	slashed := strings.ReplaceAll(t, "\\", "/")
	if err := refuseScopeEscape(slashed); err != nil {
		return "", false
	}
	cleaned := path.Clean(slashed)
	if cleaned == "." {
		return "", true
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return strings.ToLower(cleaned), true
}

// matchAnyScopePattern reports whether the folded file matches any
// folded pattern.
func matchAnyScopePattern(patterns []string, file string) bool {
	for _, p := range patterns {
		if matchScopePattern(p, file) {
			return true
		}
	}
	return false
}

// matchScopePattern matches one simple prefix/path glob: "" is the repo
// root, a trailing "/*" names a directory and everything under it, a
// trailing "*" is a prefix, an inner "*" is a path.Match glob, and a
// bare path names a file or the directory tree under it.
func matchScopePattern(pattern, file string) bool {
	if pattern == "" {
		return true
	}
	if file == pattern {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return file == prefix || strings.HasPrefix(file, prefix+"/")
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(file, strings.TrimSuffix(pattern, "*"))
	}
	if strings.Contains(pattern, "*") {
		ok, err := path.Match(pattern, file)
		return err == nil && ok
	}
	return strings.HasPrefix(file, pattern+"/")
}
