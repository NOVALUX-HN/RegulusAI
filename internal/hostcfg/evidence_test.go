// F11.7: redacted config snapshots. Reads never write; secrets
// never survive.
package hostcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotForEvidenceRedacts(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	body := Marker + "\nmodel = \"opus\"\napi_key = \"sk-live_no-leak-here\"\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := SnapshotForEvidence(p)
	if snap.State != "managed" {
		t.Fatalf("managed file state = %q, want managed", snap.State)
	}
	if strings.Contains(snap.Redacted, "sk-live_no-leak-here") {
		t.Fatalf("snapshot leaks a secret:\n%s", snap.Redacted)
	}
	if !strings.Contains(snap.Redacted, "opus") {
		t.Fatalf("snapshot must preserve benign values:\n%s", snap.Redacted)
	}
	// The file itself is untouched by the read-only snapshot.
	raw, _ := os.ReadFile(p)
	if string(raw) != body {
		t.Fatalf("snapshot must not modify the file:\n%q", raw)
	}
}

func TestSnapshotForeignPreservedAndRedacted(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".mcp.json")
	body := "{\n  // user comment\n  \"token\": \"ghp_fixture-token-1\"\n}\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := SnapshotForEvidence(p)
	if snap.State != "foreign" {
		t.Fatalf("state = %q, want foreign", snap.State)
	}
	if strings.Contains(snap.Redacted, "ghp_fixture-token-1") {
		t.Fatalf("foreign snapshot leaks a secret:\n%s", snap.Redacted)
	}
	raw, _ := os.ReadFile(p)
	if string(raw) != body {
		t.Fatalf("foreign bytes changed:\n%q", raw)
	}
}

func TestSnapshotMissing(t *testing.T) {
	snap := SnapshotForEvidence(filepath.Join(t.TempDir(), "nope.toml"))
	if snap.State != "missing" || snap.Redacted != "" {
		t.Fatalf("missing = %+v, want state missing with empty content", snap)
	}
}

func TestSnapshotTruncates(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.toml")
	body := Marker + "\n" + strings.Repeat("x", MaxSnapshotBytes+100) + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := SnapshotForEvidence(p)
	if !strings.Contains(snap.Redacted, "[truncated]") {
		t.Fatal("oversize snapshot must carry the truncation marker")
	}
	if len(snap.Redacted) > MaxSnapshotBytes+64 {
		t.Fatalf("snapshot too long: %d bytes", len(snap.Redacted))
	}
}

func TestConfigRefsNeverStoresContents(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(secret), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("api_key = \"sk-live_no-leak-here\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refs := ConfigRefs("codex", dir)
	raw := strings.ToLower(strings.TrimSpace(strings.Join([]string{refs["host"].(string)}, " ")))
	if raw != "codex" {
		t.Fatalf("refs host = %q", raw)
	}
	_ = refs
	for k, v := range refs {
		if s, ok := v.(string); ok && strings.Contains(s, "sk-live_no-leak-here") {
			t.Fatalf("refs[%s] leaks a secret", k)
		}
	}
	states, ok := refs["states"].(map[string]string)
	if !ok || states[secret] != "foreign" {
		t.Fatalf("codex secret must classify foreign: %v", refs["states"])
	}
}
