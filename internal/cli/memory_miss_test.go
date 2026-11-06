package cli

// A3 (almanac-prime): zero-hit searches log their terms; the doctor
// reports the miss count plus the most-missed sets — the repair
// pointer (save the fact, reword the query, reindex) writes itself.
import (
	"strings"
	"testing"
)

func TestMemorySearchMissLogs(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "--lexical", "zebracorn-nothing-here"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "no matching observations") {
		t.Fatalf("output = %q", out.String())
	}
	// Hybrid zero-hit records too (same miss log, no --lexical flag).
	cH, outH, _ := newCLI(nil)
	if code := cH.Run([]string{"memory", "search", "zebracorn-other-nothing"}); code != 0 {
		t.Fatalf("hybrid exit = %d, want 0", code)
	}
	if !strings.Contains(outH.String(), "no matching observations") {
		t.Fatalf("hybrid output = %q", outH.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("doctor exit = %d, want 0", code)
	}
	got := out2.String()
	if !strings.Contains(got, "misses:") || !strings.Contains(got, "zebracorn-nothing-here") || !strings.Contains(got, "zebracorn-other-nothing") {
		t.Fatalf("doctor must report both misses and their terms, got:\n%s", got)
	}
}

func TestMemoryDoctorNoMisses(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("doctor exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "misses:") {
		t.Fatalf("doctor must always show the misses line, got:\n%s", out.String())
	}
}
