package cli

// Snapshots compound evolution economics over time: --save pins
// today's report, --compare diffs a saved baseline against the live
// store. Both are deterministic files-in/files-out; the store is only
// read, never written by comparing.
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvolveReportSaveAndCompare(t *testing.T) {
	t.Chdir(t.TempDir())
	save := filepath.Join(t.TempDir(), "base.json")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "report", "--save", save}); code != 0 {
		t.Fatalf("save exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "saved") {
		t.Fatalf("save must say where, got %q", out.String())
	}
	raw, err := os.ReadFile(save)
	if err != nil {
		t.Fatalf("saved snapshot missing: %v", err)
	}
	var rep map[string]any
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("snapshot is not JSON: %v", err)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"evolve", "report", "--compare", save}); code != 0 {
		t.Fatalf("compare exit = %d, want 0", code)
	}
	got := out2.String()
	for _, want := range []string{"delta proxy", "delta admitted"} {
		if !strings.Contains(got, want) {
			t.Fatalf("compare must show %q, got %q", want, got)
		}
	}
}
