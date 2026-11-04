package cli

// P7 (pending-7): `evolve validate` renders the deterministic verdict
// per admitted lesson — keep, drop, or unreviewable — from evidence
// alone. Read-only: it proposes nothing, retires nothing, judges
// nothing beyond support. The guardian acts on the lines.
import (
	"strings"
	"testing"
)

func TestEvolveValidateUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evolve", "validate", "extra"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evolve validate") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestEvolveValidateEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "validate"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"keep: 0", "drop: 0", "unreviewable: 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("empty validate must show %q, got %q", want, got)
		}
	}
}
