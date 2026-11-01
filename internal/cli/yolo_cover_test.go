package cli

import (
	"os"
	"strings"
	"testing"
)

// COVER (gap-sweep/G3): yoloCmd carried CRAP 25.72 at 80.3% coverage
// (missing-permission-block branch never exercised). Pure coverage, no
// behavior change.
func TestYoloMissingPermissionBlockCover(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("opencode.json", []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"yolo", "on"}); code != 1 {
		t.Fatalf("missing permission exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "no permission block") {
		t.Fatalf("stderr = %q, want permission-block guidance", errb.String())
	}
}
