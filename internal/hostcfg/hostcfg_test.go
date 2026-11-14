// NOV-69: config ownership. Managed files update (with backup);
// foreign files — including JSONC with comments — are never touched.
package hostcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectClassifies(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.toml")
	if p, err := Inspect(missing); err != nil || !p.Missing {
		t.Fatalf("missing = %+v, %v", p, err)
	}
	managed := filepath.Join(dir, "m.toml")
	if err := os.WriteFile(managed, []byte("# regulus-managed\nkey=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, err := Inspect(managed); err != nil || !p.Managed {
		t.Fatalf("managed = %+v, %v", p, err)
	}
	foreign := filepath.Join(dir, "f.toml")
	if err := os.WriteFile(foreign, []byte("key=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, err := Inspect(foreign); err != nil || !p.Foreign {
		t.Fatalf("foreign = %+v, %v", p, err)
	}
}

func TestForeignJSONCPreservedByteForByte(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".mcp.json")
	// JSONC with comments and custom keys: foreign, must survive intact.
	body := "{\n  // user comment\n  \"mcpServers\": {}, // trailing\n  \"custom\": [1, 2]\n}\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(p, "x"); err == nil {
		t.Fatal("foreign install must refuse")
	} else if !strings.Contains(err.Error(), "preserved") {
		t.Fatalf("refusal must say preserved, got %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("foreign bytes changed:\n%q", got)
	}
}

func TestInstallMissingThenManagedUpdateWithBackup(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "c.toml")
	if _, err := Install(p, "a=1"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), Marker) {
		t.Fatalf("installed file must carry the marker:\n%s", raw)
	}
	bak, err := Install(p, "a=2")
	if err != nil {
		t.Fatal(err)
	}
	if bak == "" {
		t.Fatal("managed update must return a backup")
	}
	raw, _ = os.ReadFile(p)
	if !strings.Contains(string(raw), "a=2") {
		t.Fatalf("update must apply:\n%s", raw)
	}
	if err := Restore(p, bak); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(p)
	if !strings.Contains(string(raw), "a=1") {
		t.Fatalf("restore must roll back:\n%s", raw)
	}
}

func TestUninstallLeavesForeignAlone(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "f.toml")
	if err := os.WriteFile(foreign, []byte("x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if bak, err := Uninstall(foreign); err != nil || bak != "" {
		t.Fatalf("foreign uninstall = %q, %v", bak, err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign file must survive: %v", err)
	}
	managed := filepath.Join(dir, "m.toml")
	if _, err := Install(managed, "a=1"); err != nil {
		t.Fatal(err)
	}
	bak, err := Uninstall(managed)
	if err != nil || bak == "" {
		t.Fatalf("managed uninstall = %q, %v", bak, err)
	}
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Fatal("managed file must be removed")
	}
}
