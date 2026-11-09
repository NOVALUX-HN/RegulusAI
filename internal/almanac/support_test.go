package almanac

// A1 (almanac-prime): SupportOf grades one observation by its cited
// support — supported (cited receipts exist and verify), rot (a cited
// receipt fails verification), unchecked (nothing cited, nothing to
// check, or the store is unreadable). Fail-open everywhere except a
// proven failure: an unverifiable store must never accuse.
import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSupportReceipt(t *testing.T, dir, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSupportOfGrades(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	taskDir := filepath.Join(root, ".regulus", "evidence", "t1")
	ok := writeSupportReceipt(t, taskDir, "rec_ok.json", `{"schema_version":1,"stdout":"ok","stdout_sha256":"2689367b205c16ce32ed4200942b8b8b1e262dfc70d9bc9fbc77c49699a4f1df","stderr":"","stderr_sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}`)
	t.Log(ok)
	_ = writeSupportReceipt(t, taskDir, "rec_bad.json", "{not json")
	mk := func(prov *Provenance) Observation {
		o, err := Save(root, &Observation{
			Type: TypeDiscovery, Title: "t", Content: "c", Provenance: prov,
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second)
		return o
	}
	supported := mk(&Provenance{Task: "t1", Receipts: []string{ok}})
	if got := SupportOf(root, supported); got != SupportOK {
		t.Errorf("cited receipt verifying = %q, want supported", got)
	}
	rot := mk(&Provenance{Task: "t1", Receipts: []string{filepath.Join(taskDir, "rec_bad.json")}})
	if got := SupportOf(root, rot); got != SupportRot {
		t.Errorf("cited receipt failing = %q, want rot", got)
	}
	plain := mk(nil)
	if got := SupportOf(root, plain); got != SupportUnchecked {
		t.Errorf("no provenance = %q, want unchecked", got)
	}
	ghost := mk(&Provenance{Task: "ghost", Receipts: []string{"rec_x/1"}})
	if got := SupportOf(root, ghost); got != SupportUnchecked {
		t.Errorf("missing evidence dir = %q, want unchecked", got)
	}
}
