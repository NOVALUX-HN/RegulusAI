// NOV-65: the status coverage line is a pinned declaration —
// recount the repo's own test files and fail on drift.
package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func repoRootOf(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func countTestFuncs(t *testing.T, root, prefix string) int {
	t.Helper()
	re := regexp.MustCompile(`(?m)^func ` + prefix + `\w*\(`)
	n := 0
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := filepath.Base(p)
			if name != "internal" && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		n += len(re.FindAll(data, -1))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestStatusCountsMatchSuite(t *testing.T) {
	root := filepath.Join(repoRootOf(t), "internal")
	if got := countTestFuncs(t, root, "Fuzz"); got != statusFuzzTargets {
		t.Fatalf("fuzz targets = %d, status declares %d (update both together)", got, statusFuzzTargets)
	}
	if got := countTestFuncs(t, root, "Benchmark"); got != statusBenchmarks {
		t.Fatalf("benchmarks = %d, status declares %d (update both together)", got, statusBenchmarks)
	}
}
