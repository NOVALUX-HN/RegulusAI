// Branch probing for manifests and the dashboard (V3, charts/dashboard-vision.md):
// the current checkout branch (and short HEAD) resolved with pure Go
// (os/exec, argument list, never a shell), bounded by a short timeout,
// degrading to Unknown ("—") outside git instead of failing.
package manifest

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"regulus-ai/internal/executil"
)

// Unknown is the best-effort fallback when the branch (or HEAD) cannot
// be resolved: outside git, git missing, or the probe timing out.
// Historic manifests written before V3 carry no branch and read back
// as "" — callers display those as Unknown too.
const Unknown = "—"

// branchProbeTimeout bounds each `git rev-parse` probe. Rev-parse is
// local (no network); a wedged git must never hang a manifest build
// or an HTTP handler, so the probe dies fast and the caller falls back
// to Unknown.
const branchProbeTimeout = 5 * time.Second

// BranchOf returns the current branch of the checkout holding root
// (`git rev-parse --abbrev-ref HEAD` run with cwd root), or Unknown
// when it cannot be resolved.
func BranchOf(root string) string {
	if out := runGit(root, "rev-parse", "--abbrev-ref", "HEAD"); out != "" {
		return out
	}
	return Unknown
}

// HeadOf returns the short HEAD of the checkout holding root
// (`git rev-parse --short HEAD` run with cwd root), or Unknown when
// it cannot be resolved.
func HeadOf(root string) string {
	if out := runGit(root, "rev-parse", "--short", "HEAD"); out != "" {
		return out
	}
	return Unknown
}

// runGit runs `git args...` with cwd root (never a shell: the argument
// list goes straight to the binary) and returns the trimmed stdout, or
// "" on any failure (missing git, outside a checkout, timeout, empty
// output). It never fails: every error path is "not resolvable".
func runGit(root string, args ...string) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), branchProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	executil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
