//go:build windows

package executil

import (
	"os/exec"
	"testing"
)

func TestHideWindowSetsFlag(t *testing.T) {
	cmd := exec.Command("go", "version")
	HideWindow(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow must set SysProcAttr.HideWindow on Windows")
	}
}
