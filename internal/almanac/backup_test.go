package almanac

// NOV-30 (S15) bundle integrity unit tests: MANIFEST.json pins every
// data file, a 1-byte alteration refuses with the target intact, a
// future format refuses, and pre-NOV-30 bundles without MANIFEST still
// import. All roots are tmp dirs — never the real .regulus/.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var backupBase = time.Date(2026, 11, 30, 9, 0, 0, 0, time.UTC)

// seedBackup builds a two-observation Almanac with one pin.
func seedBackup(t *testing.T, root string) (a, b Observation) {
	t.Helper()
	var err error
	a, err = Save(root, &Observation{Type: TypeDecision, Title: "backup one", Content: "alpha content"}, backupBase)
	if err != nil {
		t.Fatal(err)
	}
	b, err = Save(root, &Observation{Type: TypeDiscovery, Title: "backup two", Content: "beta content"}, backupBase.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := Pin(root, b.ID, true, backupBase.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return a, b
}

// readBundleManifest loads and parses the MANIFEST.json of a bundle dir.
func readBundleManifest(t *testing.T, dir string) BundleManifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ManifestFileName))
	if err != nil {
		t.Fatalf("bundle must carry %s: %v", ManifestFileName, err)
	}
	var m BundleManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("MANIFEST must parse: %v", err)
	}
	return m
}

func TestWriteManifestPinsDataFiles(t *testing.T) {
	root := t.TempDir()
	seedBackup(t, root)
	if _, err := Export(root); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".regulus-export")
	m := readBundleManifest(t, dir)
	if m.Format != 1 {
		t.Fatalf("manifest format = %d, want 1", m.Format)
	}
	if len(m.Files) != 4 {
		t.Fatalf("manifest must pin the 4 data files, got %v", m.Files)
	}
	// Every pinned entry must match an independent recomputation.
	for name, want := range m.Files {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("manifest lists %s but it is missing: %v", name, err)
		}
		sum := sha256.Sum256(raw)
		got := ManifestEntry{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw))}
		if got != want {
			t.Fatalf("%s: manifest = %+v, recomputed = %+v", name, want, got)
		}
	}
	if m.CreatedAt.IsZero() {
		t.Fatal("manifest must carry created_at")
	}
	if strings.TrimSpace(m.Source) == "" {
		t.Fatal("manifest must carry source")
	}
	if err := VerifyBundle(dir); err != nil {
		t.Fatalf("a fresh export must verify: %v", err)
	}
}

func TestTamperedBundleRefuses(t *testing.T) {
	src := t.TempDir()
	seedBackup(t, src)
	if _, err := Export(src); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(src, ".regulus-export")
	// Alter exactly one byte of a data file.
	p := filepath.Join(dir, "almanac.jsonl")
	raw, err := os.ReadFile(p)
	if err != nil || len(raw) == 0 {
		t.Fatalf("bundle almanac.jsonl must be non-empty: %v", err)
	}
	raw[0] ^= 0x20
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBundle(dir); err == nil || !strings.Contains(err.Error(), "MANIFEST mismatch: almanac.jsonl") {
		t.Fatalf("tampered bundle must refuse naming the file, got %v", err)
	}
	dst := t.TempDir()
	if _, _, err := Import(dst, dir); err == nil || !strings.Contains(err.Error(), "MANIFEST mismatch: almanac.jsonl") {
		t.Fatalf("tampered import must refuse naming the file, got %v", err)
	}
	// The refused import writes nothing: the target stays intact.
	if all, _ := All(dst); len(all) != 0 {
		t.Fatalf("a refused import must write no observations, got %d", len(all))
	}
	for _, sp := range []string{File(dst), pinsFile(dst), relationsFile(dst), StatusFile(dst)} {
		if _, err := os.Stat(sp); !os.IsNotExist(err) {
			t.Fatalf("a refused import must not create %s", sp)
		}
	}
}

func TestFutureVersionRefuses(t *testing.T) {
	src := t.TempDir()
	seedBackup(t, src)
	if _, err := Export(src); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(src, ".regulus-export")
	// A newer writer bumps the format; the data files stay valid.
	mp := filepath.Join(dir, ManifestFileName)
	raw, err := os.ReadFile(mp)
	if err != nil {
		t.Fatal(err)
	}
	var m BundleManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Format = 2
	raw, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mp, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBundle(dir); err == nil || !strings.Contains(err.Error(), "unsupported bundle v2") {
		t.Fatalf("future bundle must refuse with its version, got %v", err)
	}
	dst := t.TempDir()
	if _, _, err := Import(dst, dir); err == nil || !strings.Contains(err.Error(), "unsupported bundle v2") {
		t.Fatalf("future import must refuse with its version, got %v", err)
	}
	if all, _ := All(dst); len(all) != 0 {
		t.Fatalf("a refused import must write no observations, got %d", len(all))
	}
}

func TestBundleWithoutManifestStillImports(t *testing.T) {
	// Pre-NOV-30 bundles carry no MANIFEST.json: they import exactly as
	// before (backward compatible, never a refusal).
	dir := t.TempDir()
	obs := `{"id":"obs_legacy1","type":"discovery","title":"old bundle","content":"c","saved_at":"2026-11-30T08:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "almanac.jsonl"), []byte(obs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBundle(dir); err != nil {
		t.Fatalf("a bundle without MANIFEST must verify (compat): %v", err)
	}
	dst := t.TempDir()
	got, skipped, err := Import(dst, dir)
	if err != nil || got != 1 || skipped != 0 {
		t.Fatalf("legacy import = %d/%d (%v), want 1/0", got, skipped, err)
	}
}
