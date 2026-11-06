package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/executil"
)

// rootGitTimeout bounds the `git rev-parse --show-toplevel` probe used
// for project-root detection. Rev-parse is local (no network); a wedged
// git must never hang a CLI command, so the probe dies fast and the
// caller falls back to the regulus.json walk-up, then to the cwd.
const rootGitTimeout = 2 * time.Second

// ResolveRoot auto-detects the project root for a directory:
//
//  1. git top-level via `git rev-parse --show-toplevel` run with cwd
//     (tolerates failure: missing git, outside a checkout, timeout),
//  2. otherwise the nearest ancestor (inclusive) holding regulus.json,
//  3. otherwise the input unchanged (today's behavior outside
//     git/regulus.json).
//
// The empty string means the process cwd. When the resolved root equals
// the input directory, the input is returned verbatim (so "." stays "."
// at the top level and absolute operator paths keep their spelling);
// otherwise the resolved absolute root is returned. It never fails:
// every error path falls back to the input.
//
// Callers: CLI.dir (every store command) and `regulus serve` (the
// default "." and every --projects entry), so running from a
// subdirectory shares exactly the same <root>/.regulus/ stores as
// running from the root.
func ResolveRoot(cwd string) string {
	start := cwd
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		start = wd
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return cwd
	}
	abs = filepath.Clean(abs)
	if top := gitTopLevel(abs); top != "" {
		if sameDir(top, abs) {
			return cwd
		}
		return top
	}
	if dir, ok := ancestorWithFile(abs, "regulus.json"); ok {
		if sameDir(dir, abs) {
			return cwd
		}
		return dir
	}
	return cwd
}

// gitTopLevel runs `git rev-parse --show-toplevel` in dir and returns
// the cleaned top-level path, or "" on any failure (no git, outside a
// checkout, timeout, non-directory output).
func gitTopLevel(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), rootGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	executil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	top := filepath.Clean(strings.TrimSpace(string(out)))
	if top == "" {
		return ""
	}
	fi, err := os.Stat(top)
	if err != nil || !fi.IsDir() {
		return ""
	}
	return top
}

// ancestorWithFile walks dir and its parents for the first directory
// holding a regular file named name (the directory itself included).
func ancestorWithFile(dir, name string) (string, bool) {
	for d := dir; ; {
		if fi, err := os.Stat(filepath.Join(d, name)); err == nil && !fi.IsDir() {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

// sameDir reports whether two cleaned paths name the same directory.
// Comparison is case-insensitive so a Windows checkout (where git
// spells the drive/directory case differently than Getwd) still
// matches; the two spellings can only differ in case when they resolve
// the same location, since git only succeeds against the real dir.
func sameDir(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
