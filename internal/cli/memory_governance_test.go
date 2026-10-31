package cli

import (
	"os"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
)

func govRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func govSave(t *testing.T, typ, title string) string {
	t.Helper()
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", typ, "--title", title, "content about " + title}); code != 0 {
		t.Fatalf("save exit = %d, want 0", code)
	}
	all, err := almanac.All(".")
	if err != nil || len(all) == 0 {
		t.Fatal("want a saved observation")
	}
	return all[len(all)-1].ID
}

func TestMemoryVerifyAndStatus(t *testing.T) {
	govRoot(t)
	id := govSave(t, "discovery", "ttl")
	if err := os.MkdirAll(".regulus/evidence/t", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".regulus/evidence/t/rec_1.json", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "verify", id, "--receipt", ".regulus/evidence/t/rec_1.json"}); code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "VERIFIED") {
		t.Fatalf("output = %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "status", id}); code != 0 {
		t.Fatalf("status exit = %d, output:\n%s", code, out2.String())
	}
	if !strings.Contains(out2.String(), "VERIFIED") {
		t.Fatalf("status must show VERIFIED, got:\n%s", out2.String())
	}
	// Idempotent: verifying again is a clean no-op.
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"memory", "verify", id, "--receipt", ".regulus/evidence/t/rec_1.json"}); code != 0 {
		t.Fatalf("re-verify exit = %d, output:\n%s", code, out3.String())
	}
	if !strings.Contains(out3.String(), "already") {
		t.Fatalf("re-verify must be a no-op, got:\n%s", out3.String())
	}
}

func TestMemoryVerifyNeedsEvidence(t *testing.T) {
	govRoot(t)
	id := govSave(t, "discovery", "ttl")
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "verify", id, "--receipt", ".regulus/evidence/t/missing.json"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "missing evidence") {
		t.Fatalf("stderr must name the missing evidence, got %q", errb.String())
	}
}

func TestMemoryReject(t *testing.T) {
	govRoot(t)
	id := govSave(t, "discovery", "ttl")
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "reject", id}); code != 2 {
		t.Fatalf("reject without reason must be usage error, got %d", code)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "reject", id, "--reason", "superseded by field work", "--by", "ana"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "REJECTED") {
		t.Fatalf("output = %q", out2.String())
	}
	st, err := almanac.EffectiveStatus(".", id)
	if err != nil || st != almanac.StatusRejected {
		t.Fatalf("want REJECTED, got %q/%v", st, err)
	}
}

func TestMemoryCheckContradictions(t *testing.T) {
	govRoot(t)
	a := govSave(t, "discovery", "a-claim")
	b := govSave(t, "discovery", "b-claim")
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "relate", "--id", a, "--relation", "conflicts_with", "--to", b}); code != 0 {
		t.Fatalf("relate exit = %d", code)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "check"}); code != 1 {
		t.Fatalf("live contradiction must exit 1, output:\n%s", out2.String())
	}
	if !strings.Contains(out2.String(), "conflicts_with") {
		t.Fatalf("check must name the relation, got:\n%s", out2.String())
	}
	// Resolving one side clears the check.
	c3, _, _ := newCLI(nil)
	if code := c3.Run([]string{"memory", "reject", b, "--reason", "wrong", "--by", "ana"}); code != 0 {
		t.Fatal("reject must succeed")
	}
	c4, out4, _ := newCLI(nil)
	if code := c4.Run([]string{"memory", "check"}); code != 0 {
		t.Fatalf("resolved check must exit 0, output:\n%s", out4.String())
	}
}

func TestMemoryImportStaysProposed(t *testing.T) {
	govRoot(t)
	if err := os.WriteFile("b.jsonl", []byte("{\"id\":\"obs_9\",\"type\":\"discovery\",\"title\":\"t\",\"content\":\"c\",\"saved_at\":\"2026-01-01T00:00:00Z\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "import", "b.jsonl"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	st, err := almanac.EffectiveStatus(".", "obs_9")
	if err != nil || st != almanac.StatusProposed {
		t.Fatalf("imports stay PROPOSED until reviewed, got %q/%v", st, err)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "status", "obs_9"}); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	if !strings.Contains(out2.String(), "PROPOSED") {
		t.Fatalf("status must show PROPOSED, got:\n%s", out2.String())
	}
	// Review promotes: verify with evidence.
	if err := os.MkdirAll(".regulus/evidence/t", 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(".regulus/evidence/t/r.json", []byte("{}"), 0o644)
	c3, _, _ := newCLI(nil)
	if code := c3.Run([]string{"memory", "verify", "obs_9", "--receipt", ".regulus/evidence/t/r.json"}); code != 0 {
		t.Fatal("verify must succeed")
	}
	st, _ = almanac.EffectiveStatus(".", "obs_9")
	if st != almanac.StatusVerified {
		t.Fatalf("reviewed import verifies, got %q", st)
	}
	_ = time.Now
}
