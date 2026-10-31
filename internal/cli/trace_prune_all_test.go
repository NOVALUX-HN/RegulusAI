package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/trace"
)

func recordN(t *testing.T, session string, n int) {
	t.Helper()
	now := time.Unix(1700000000, 0).UTC()
	for i := 0; i < n; i++ {
		if _, err := trace.Record(".", trace.Entry{
			Session: session,
			Message: fmt.Sprintf("m-%d", i),
			Call:    fmt.Sprintf("c-%d", i),
			Tool:    fmt.Sprintf("tool-%d", i),
			Status:  trace.StatusCompleted,
		}, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTracePruneAllBulkRotates(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, s := range []string{"bulk-a", "bulk-b", "bulk-c"} {
		recordN(t, s, 5)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "prune", "--all", "--keep", "2"}); code != 0 {
		t.Fatalf("prune --all exit = %d, stderr %q", code, errb.String())
	}
	report := out.String()
	for _, s := range []string{"bulk-a", "bulk-b", "bulk-c"} {
		if !strings.Contains(report, "trace "+s+": 2 entr(y/ies) kept") {
			t.Fatalf("report must contain per-session line for %s, got:\n%s", s, report)
		}
		entries, err := trace.List(".", s)
		if err != nil || len(entries) != 2 {
			t.Fatalf("session %s after prune = %+v (%v), want 2", s, entries, err)
		}
		if entries[0].Tool != "tool-3" || entries[1].Tool != "tool-4" {
			t.Fatalf("session %s must keep tail, got %+v", s, entries)
		}
		// no file deletions: every session file still exists.
		if _, err := os.Stat(trace.File(".", s)); err != nil {
			t.Fatalf("session file %s must still exist: %v", s, err)
		}
	}
	if !strings.Contains(report, "across 3 session(s)") {
		t.Fatalf("report must contain total across 3 sessions, got:\n%s", report)
	}
}

func TestTracePruneAllDefaultKeepNoOp(t *testing.T) {
	t.Chdir(t.TempDir())
	recordN(t, "small", 3)
	before, err := os.ReadFile(trace.File(".", "small"))
	if err != nil {
		t.Fatal(err)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "prune", "--all"}); code != 0 {
		t.Fatalf("prune --all default exit = %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "trace small: 3 entr(y/ies) kept") {
		t.Fatalf("default-200 report must list the session, got:\n%s", out.String())
	}
	after, err := os.ReadFile(trace.File(".", "small"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("default 200 with fewer entries must not touch the file")
	}
	entries, err := trace.List(".", "small")
	if err != nil || len(entries) != 3 {
		t.Fatalf("after default prune = %+v (%v), want 3", entries, err)
	}
}

func TestTracePruneUnknownSessionNoOp(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "prune", "--session", "ghost"}); code != 0 {
		t.Fatalf("prune unknown exit = %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "trace ghost: 0 entr(y/ies) kept") {
		t.Fatalf("unknown session report = %q", out.String())
	}
	if _, err := os.Stat(trace.File(".", "ghost")); !os.IsNotExist(err) {
		t.Fatalf("unknown prune must not create a file, stat err = %v", err)
	}
}

func TestTracePruneWithoutAllRequiresSession(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{
		{"trace", "prune"},
		{"trace", "prune", "--keep", "2"},
	} {
		c, _, errb := newCLI(nil)
		if code := c.Run(args); code != 2 {
			t.Fatalf("args %v: exit = %d, want 2", args, code)
		}
		if !strings.Contains(errb.String(), "usage: regulus trace prune") {
			t.Fatalf("args %v: stderr must print prune usage, got %q", args, errb.String())
		}
	}
	// existing single-session behaviour stays: explicit --session works.
	recordN(t, "solo", 3)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "prune", "--session", "solo", "--keep", "1"}); code != 0 {
		t.Fatalf("single prune exit = %d", code)
	}
	if !strings.Contains(out.String(), "trace solo: 1 entr(y/ies) kept") {
		t.Fatalf("single prune report = %q", out.String())
	}
}
