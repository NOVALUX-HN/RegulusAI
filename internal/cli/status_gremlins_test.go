package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The status gremlins probe must be pure Go (no shell): `command -v`
// does not exist in pwsh/cmd, so a shell probe always fails on Windows
// without sh. These tests pin PATH-first plus the per-OS Go bin fallback.
func TestGremlinsAvailableForStatus(t *testing.T) {
	mkexe := func(dir, name string) {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chmod(filepath.Join(dir, name), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Run("PATH hit wins", func(t *testing.T) {
		bin := t.TempDir()
		mkexe(bin, "gremlins")
		t.Setenv("PATH", bin)
		if !gremlinsAvailableForStatus() {
			t.Fatal("gremlins on PATH must resolve")
		}
	})

	t.Run("Go bin fallback", func(t *testing.T) {
		home := t.TempDir()
		gobin := filepath.Join(home, "go", "bin")
		if err := os.MkdirAll(gobin, 0o755); err != nil {
			t.Fatal(err)
		}
		mkexe(gobin, "gremlins")
		t.Setenv("PATH", "")
		if runtime.GOOS == "windows" {
			t.Setenv("USERPROFILE", home)
		} else {
			t.Setenv("HOME", home)
		}
		if !gremlinsAvailableForStatus() {
			t.Fatal("gremlins in Go bin dir must resolve")
		}
	})

	t.Run("absent everywhere", func(t *testing.T) {
		t.Setenv("PATH", "")
		if runtime.GOOS == "windows" {
			t.Setenv("USERPROFILE", t.TempDir())
		} else {
			t.Setenv("HOME", t.TempDir())
		}
		if gremlinsAvailableForStatus() {
			t.Fatal("no gremlins anywhere must not resolve")
		}
	})
}
