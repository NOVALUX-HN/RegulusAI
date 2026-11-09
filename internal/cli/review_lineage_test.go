package cli

// T7 (pro-harness RDD, chart pro-harness-rdd): the review stops being
// advisory-only and becomes a transaction — `review start` freezes the
// candidate bytes under a lineage, `review approve` binds approval to
// those exact bytes (drift refutes and consumes one of 2 correction
// rounds; past budget only a human decides), `review refute` sorts a
// finding into candidate-caused (in the frozen set) vs pre-existing
// (outside it: follow-up, never a blocker).
import (
	"os"
	"strings"
	"testing"
)

func TestReviewLineageLifecycle(t *testing.T) {
	t.Chdir(t.TempDir())
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package a\n")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"review", "start", "--task", "t1"}); code != 0 {
		t.Fatalf("start exit = %d, want 0", code)
	}
	lineage := strings.TrimSpace(out.String())
	if !strings.HasPrefix(lineage, "rev_") {
		t.Fatalf("start must print the lineage id, got %q", lineage)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"review", "approve", "--task", "t1", "--lineage", lineage, "--verdict", "pass"}); code != 0 {
		t.Fatalf("clean approve exit = %d, want 0\n%s", code, out2.String())
	}
	if !strings.Contains(out2.String(), "approved") {
		t.Fatalf("approve must say approved, got %q", out2.String())
	}
	write("a.go", "package a\n// drift\n")
	c3, _, errb3 := newCLI(nil)
	if code := c3.Run([]string{"review", "approve", "--task", "t1", "--lineage", lineage, "--verdict", "pass"}); code != 1 {
		t.Fatalf("drifted approve exit = %d, want 1", code)
	}
	if !strings.Contains(errb3.String(), "drifted") || !strings.Contains(errb3.String(), "1/2") {
		t.Fatalf("drift must name itself + consume round 1/2, got %q", errb3.String())
	}
	c4, _, errb4 := newCLI(nil)
	if code := c4.Run([]string{"review", "approve", "--task", "t1", "--lineage", lineage, "--verdict", "pass"}); code != 1 {
		t.Fatalf("second drift exit = %d, want 1", code)
	}
	if !strings.Contains(errb4.String(), "2/2") {
		t.Fatalf("second drift must consume round 2/2, got %q", errb4.String())
	}
	c5, _, errb5 := newCLI(nil)
	if code := c5.Run([]string{"review", "approve", "--task", "t1", "--lineage", lineage, "--verdict", "pass"}); code != 1 {
		t.Fatalf("third drift exit = %d, want 1", code)
	}
	if !strings.Contains(errb5.String(), "budget") || !strings.Contains(errb5.String(), "human") {
		t.Fatalf("past budget must demand a human, got %q", errb5.String())
	}
}

func TestReviewRefuteSortsFindings(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("a.go", []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"review", "start", "--task", "t2"}); code != 0 {
		t.Fatalf("start exit = %d, want 0", code)
	}
	lineage := strings.TrimSpace(out.String())
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"review", "refute", "--task", "t2", "--lineage", lineage, "--file", "a.go"}); code != 0 {
		t.Fatalf("in-set refute exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "candidate-caused") {
		t.Fatalf("in-set file must read candidate-caused, got %q", out2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"review", "refute", "--task", "t2", "--lineage", lineage, "--file", "other/b.go"}); code != 0 {
		t.Fatalf("out-of-set refute exit = %d, want 0", code)
	}
	if !strings.Contains(out3.String(), "pre-existing") || !strings.Contains(out3.String(), "follow-up") {
		t.Fatalf("out-of-set file must read pre-existing follow-up, got %q", out3.String())
	}
}

func TestReviewApproveUnknownLineage(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"review", "approve", "--task", "t9", "--lineage", "rev_nope", "--verdict", "pass"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "no such lineage") {
		t.Fatalf("stderr = %q", errb.String())
	}
}
