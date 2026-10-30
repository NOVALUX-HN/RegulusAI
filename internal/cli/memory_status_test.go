package cli

// H3 (S5): a save past the last reindex must not go stale in silence —
// `regulus status` and `regulus memory doctor` name the state with the
// repairing command.

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/doctor"
)

func saveTwoWithReindexBetween(t *testing.T) {
	t.Helper()
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "pooling everywhere"}); code != 0 {
		t.Fatalf("save 1: exit = %d", code)
	}
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	cS, _, _ := newCLI(nil)
	if code := cS.Run([]string{"memory", "save", "--type", "decision", "--title", "second choice", "more content"}); code != 0 {
		t.Fatalf("save 2: exit = %d", code)
	}
}

func TestStatusReportsStaleIndexWithRepair(t *testing.T) {
	t.Chdir(t.TempDir())
	saveTwoWithReindexBetween(t)
	c, out, _ := newCLI(func() doctor.Result {
		return doctor.Result{Detected: "opencode 2.1.4", Major: 2, Supported: true}
	})
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"status"}); code != 0 {
		t.Fatalf("status exit = %d, want 0 (status always reports)", code)
	}
	for _, want := range []string{"memory:", "stale", "regulus memory reindex"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status must report %q for a stale index, got:\n%s", want, out.String())
		}
	}
}

func TestStatusReportsFreshIndex(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "pooling everywhere"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	cS, out, _ := newCLI(func() doctor.Result {
		return doctor.Result{Detected: "opencode 2.1.4", Major: 2, Supported: true}
	})
	cS.Runner = &fakeGateRunner{}
	if code := cS.Run([]string{"status"}); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	if !strings.Contains(out.String(), "memory:     semantic index fresh") {
		t.Fatalf("status must report a fresh index, got:\n%s", out.String())
	}
}

func TestMemoryDoctorReportsStaleIndexWithRepair(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if _, err := almanac.Save(".", &almanac.Observation{Type: almanac.TypeDecision, Title: "a", Content: "c"}, time.Unix(1700000000, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("doctor exit = %d", code)
	}
	for _, want := range []string{"index:      missing", "regulus memory reindex"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("memory doctor must report %q for a never-indexed log, got:\n%s", want, out.String())
		}
	}
	saveTwoWithReindexBetween(t)
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("doctor exit = %d", code)
	}
	for _, want := range []string{"index:      stale", "regulus memory reindex"} {
		if !strings.Contains(out2.String(), want) {
			t.Fatalf("memory doctor must report %q after a save past reindex, got:\n%s", want, out2.String())
		}
	}
}
