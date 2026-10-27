package surface

import (
	"os"
	"path/filepath"
	"regulus-ai/internal/budget"
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
		".opencode/discipline.md",
		".opencode/tool-guidance.md",
		"docs/x.md",
		"docs/architecture/y.md",
		"scripts/other.bash",
	} {
		if _, err := Propose(t.TempDir(), ok, "content", "reason", t0); err != nil {
			t.Fatalf("whitelisted %q must be accepted, got %v", ok, err)
		}
	}
	// scripts/build.bash sits inside the whitelist but is reserved:
	// release infrastructure changes by hand, never via surface.
	if _, err := Propose(t.TempDir(), "scripts/build.bash", "content", "reason", t0); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("want reserved refusal, got %v", err)
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

func TestSurfaceBudgetCapAndRecording(t *testing.T) {
	root := t.TempDir()
	// set cap to 1
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "regulus.json"), []byte(`{"gates":[{"name":"typecheck","layer":1,"command":"true"}],"evolution":{"max_per_day":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	// first propose: fits inside the cap
	e1, err := Propose(root, "docs/a.md", "first", "r1", now)
	if err != nil {
		t.Fatal(err)
	}
	// budget: surface-proposed recorded
	entries, _ := budget.Show(root)
	if len(entries) != 1 || entries[0].Kind != budget.KindSurface || entries[0].Outcome != budget.OutcomeSurfaceProposed {
		t.Fatalf("ledger = %+v", entries)
	}
	// second propose: cap reached → refused
	_, err = Propose(root, "docs/b.md", "second", "r2", now.Add(time.Minute))
	if err == nil || !strings.Contains(err.Error(), "evolution cap reached") {
		t.Fatalf("want cap refusal, got %v", err)
	}
	// the refusal is recorded
	entries, _ = budget.Show(root)
	if len(entries) != 2 || entries[1].Outcome != budget.OutcomeRefusedCap {
		t.Fatalf("the refusal must be recorded, got %+v", entries)
	}
	// apply the first (mechanical — no cap consumption)
	applied, err := Apply(root, e1.ID, "regulus-reviewer", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if applied.Status != "applied" {
		t.Fatalf("applied = %+v", applied)
	}
	// the application is recorded (surface-applied)
	entries, _ = budget.Show(root)
	if len(entries) != 3 || entries[2].Outcome != budget.OutcomeSurfaceApplied {
		t.Fatalf("ledger after apply = %+v", entries)
	}
	// the cap count is still 1 (application doesn't consume)
	count, _ := budget.TodayEvolutionCount(root, now.Add(2*time.Minute))
	if count != 1 {
		t.Fatalf("cap count = %d, want 1 (application is mechanical)", count)
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

func TestProposeRefusesReservedFiles(t *testing.T) {
	_, err := Propose(t.TempDir(), "scripts/build.bash", "content", "reason", t0)
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("want reserved refusal, got %v", err)
	}
	// other scripts stay editable
	if _, err := Propose(t.TempDir(), "scripts/other.bash", "content", "reason", t0); err != nil {
		t.Fatalf("non-reserved scripts must stay editable, got %v", err)
	}
	if !IsReserved("scripts/build.bash") || IsReserved("scripts/other.bash") || IsReserved("docs/x.md") {
		t.Fatal("IsReserved must match exactly the reserved list")
	}
}
