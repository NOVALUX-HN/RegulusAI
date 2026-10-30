//go:build windows

package executil

import (
	"os/exec"
	"syscall"
)

// HideWindow marks a child console process so Windows does not flash a
// visible terminal window when the parent has no console of its own
// (the opencode background server on Windows): every regulus tool call,
// trace record and gate spawns a short-lived child, and each one popped
// a window open and shut. No effect on the child's exit code or output.
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}
