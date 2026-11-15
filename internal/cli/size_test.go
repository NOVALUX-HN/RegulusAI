// NOV-67: `regulus size check` fails on real violations and passes
// on a clean tree with a readable summary.
package cli

import (
	"strings"
	"testing"
)

func TestSizeCheckPassesRepo(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"size", "check"}); code != 0 {
		t.Fatalf("size check exit = %d, want 0 on this tree", code)
	}
	if !strings.Contains(out.String(), "size OK") {
		t.Fatalf("missing OK summary, got %q", out.String())
	}
}
