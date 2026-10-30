package gates

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestGofmtClean is the executable form of the `gofmt` gauntlet gate
// (regulus.json layer 12): every .go file under internal/ and cmd/ must
// be gofmt-clean. A bare `gofmt -l` gate always exits 0 (it only lists),
// so the gate runs this test instead — `go test -run TestGofmtClean`
// fails red naming the files. Windows note: gofmt wants LF; a CRLF save
// flags the file — run `gofmt -w` on the listed paths.
func TestGofmtClean(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate this test file")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Fatalf("gofmt not found on PATH (install the Go toolchain): %v", err)
	}
	out, err := exec.Command("gofmt", "-l", filepath.Join(root, "internal"), filepath.Join(root, "cmd")).Output()
	if err != nil {
		t.Fatalf("gofmt -l failed: %v", err)
	}
	if listed := strings.TrimSpace(string(out)); listed != "" {
		t.Fatalf("unformatted Go files (run gofmt -w on them):\n%s", listed)
	}
}
