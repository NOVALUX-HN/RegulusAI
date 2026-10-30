package executil

import (
	"os/exec"
	"testing"
)

func TestHideWindowKeepsCommandUsable(t *testing.T) {
	cmd := exec.Command("go", "version")
	HideWindow(cmd)
	if cmd.Path == "" {
		t.Fatal("HideWindow must not clear the command path")
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("command must still run after HideWindow: %v", err)
	}
}
