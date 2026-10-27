package surface

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)

func TestProposeRefusesOutsideWhitelist(t *testing.T) {
	_, err := Propose(t.TempDir(), "cmd/regulus/main.go", "hack", "r", t0)
	if err == nil || !strings.Contains(err.Error(), "whitelist") {
		t.Fatalf("want whitelist refusal, got %v", err)
	}
	for _, ok := range []string{
		".opencode/skills/x/SKILL.md",
		".opencode/agents/x.md",
		".opencode/plugins/x.ts",
		"docs/x.md",
	} {
		if _, err := Propose(t.TempDir(), ok, "content", "reason", t0); err != nil {
			t.Fatalf("whitelisted %q must be accepted, got %v", ok, err)
		}
	}
}

func TestProposeRefusesEmptyContentOrReason(t *testing.T) {
	if _, err := Propose(t.TempDir(), "docs/x.md", " ", "r", t0); err == nil || !strings.Contains(err.Error(), "content") {
		t.Fatalf("want content refusal, got %v", err)
	}
	if _, err := Propose(t.TempDir(), "docs/x.md", "c", "", t0); err == nil || !strings.Contains(err.Error(), "reason") {
		t.Fatalf("want reason refusal, got %v", err)
	}
}

func TestApplyAppendsMarkerAndRecordsGuardian(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".opencode", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, ".opencode", "skills", "SKILL.md")
	if err := os.WriteFile(target, []byte("## Base skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := Propose(root, ".opencode/skills/SKILL.md", "## Learned: use the clean export", "lesson X says so", t0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(e.ID, "sfx_") || e.Status != "proposed" {
		t.Fatalf("proposal = %+v", e)
	}
	// apply without guardian is refused
	if _, err := Apply(root, e.ID, "  ", t0); err == nil || !strings.Contains(err.Error(), "guardian") {
		t.Fatalf("want guardian refusal, got %v", err)
	}
	applied, err := Apply(root, e.ID, "regulus-reviewer", t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != "applied" || applied.Guardian != "regulus-reviewer" {
		t.Fatalf("applied = %+v", applied)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "## Base skill") {
		t.Fatalf("the base content must survive, got:\n%s", content)
	}
	if !strings.Contains(content, "## Learned: use the clean export") {
		t.Fatalf("the appended content must land, got:\n%s", content)
	}
	if !strings.Contains(content, "regulus:surface-edit "+e.ID) {
		t.Fatalf("the edit must be marker-delimited, got:\n%s", content)
	}
	// the proposal cannot be applied twice
	if _, err := Apply(root, e.ID, "regulus-reviewer", t0.Add(2*time.Minute)); err == nil || !strings.Contains(err.Error(), "already applied") {
		t.Fatalf("want already-applied refusal, got %v", err)
	}
}

func TestListReturnsEverythingOldestFirst(t *testing.T) {
	root := t.TempDir()
	a, _ := Propose(root, "docs/a.md", "one", "r", t0)
	b, _ := Propose(root, "docs/b.md", "two", "r", t0.Add(time.Second))
	if _, err := Apply(root, b.ID, "regulus-gate-verifier", t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	edits, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	// proposal a, proposal b, applied b (the applied copy is appended after)
	if len(edits) != 3 {
		t.Fatalf("want 3 entries (2 proposals + 1 applied), got %d: %+v", len(edits), edits)
	}
	if edits[0].ID != a.ID || edits[2].Status != "applied" {
		t.Fatalf("order = %+v", edits)
	}
}
