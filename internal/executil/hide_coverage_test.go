package executil

// Every production spawn site must route through HideWindow: an
// unmarked exec.Command flashes a terminal window on Windows hosts
// without a console (the opencode background server), once per tool
// call, trace record and gate. Test files are exempt (they run inside
// a console already); this package defines the marking itself.
// See docs/troubleshooting.md (terminal flashing).
import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEveryProductionSpawnHidesWindow(t *testing.T) {
	_, self, _, _ := runtime.Caller(0)
	root := filepath.Dir(self)
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		root = filepath.Dir(root)
	}
	var bad []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		if filepath.Dir(p) == filepath.Join(root, "internal", "executil") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "exec.Command") && !strings.Contains(string(raw), "HideWindow") {
			rel, _ := filepath.Rel(root, p)
			bad = append(bad, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) > 0 {
		t.Fatalf("production spawn sites without HideWindow (terminal flash on Windows):\n  %s", strings.Join(bad, "\n  "))
	}
}
