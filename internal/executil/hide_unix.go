//go:build !windows

package executil

import "os/exec"

// HideWindow is a no-op outside Windows: child processes share the
// parent's terminal there, so no window can flash.
func HideWindow(_ *exec.Cmd) {}
