package almanac

// NOV-30 (S15) bundle integrity: every export writes MANIFEST.json —
// format version plus sha256+size per data file — and every bundle
// import verifies it before planning or writing anything. A tampered
// file refuses with `MANIFEST mismatch: <file>`, a newer format with
// `unsupported bundle vN`, both leaving the target tree intact.
// Bundles without MANIFEST predate NOV-30 and still import (backward
// compatible, documented in the Almanac runbook). INDEX.md is a
// rendered digest, never imported, so the manifest covers only the
// four restorable JSONL data files.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// BundleFormatVersion is the MANIFEST.json format this binary writes
// and understands. A bundle with a higher version refuses (it may use
// a layout this binary cannot restore).
const BundleFormatVersion = 1

// ManifestFileName is the integrity sidecar written into every export
// bundle directory by Export and ExportDomain.
const ManifestFileName = "MANIFEST.json"

// bundleDataFiles are the restorable bundle files the manifest covers:
// every file importBundle reads. INDEX.md is excluded on purpose — it
// is a rendered digest, never imported, so tampering with it cannot
// corrupt a restore.
var bundleDataFiles = []string{"almanac.jsonl", "pins.jsonl", "relations.jsonl", "status.jsonl"}

// ManifestEntry pins one bundle file: its sha256 hex digest and byte size.
type ManifestEntry struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// BundleManifest is the MANIFEST.json shape: the format version, the
// pinned data files, when the bundle was written and from which project.
type BundleManifest struct {
	Format    int                      `json:"format"`
	Files     map[string]ManifestEntry `json:"files"`
	CreatedAt time.Time                `json:"created_at"`
	Source    string                   `json:"source"`
}

// WriteManifest hashes the bundle data files in dir and writes
// MANIFEST.json there, returning its path. Every listed file must
// exist — Export and ExportDomain always write the full shape (missing
// store files export as empty files, never omissions), so a missing
// file means an incomplete bundle and fails closed.
func WriteManifest(dir, source string) (string, error) {
	m := BundleManifest{
		Format:    BundleFormatVersion,
		Files:     map[string]ManifestEntry{},
		CreatedAt: time.Now().UTC(),
		Source:    source,
	}
	for _, name := range bundleDataFiles {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", fmt.Errorf("backup: cannot manifest %s: %w", name, err)
		}
		sum := sha256.Sum256(raw)
		m.Files[name] = ManifestEntry{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw))}
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, ManifestFileName)
	if err := os.WriteFile(dst, append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// VerifyBundle checks the MANIFEST.json in dir against the files on
// disk: every listed file must hash and measure exactly as pinned,
// otherwise `MANIFEST mismatch: <file>`. A manifest newer than this
// binary refuses with `unsupported bundle vN`. A missing MANIFEST.json
// is not an error — pre-NOV-30 bundles carry none and still import.
// A present but unreadable manifest fails closed (`invalid MANIFEST`).
func VerifyBundle(dir string) error {
	raw, err := os.ReadFile(filepath.Join(dir, ManifestFileName))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var m BundleManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("invalid MANIFEST: %v", err)
	}
	if m.Format > BundleFormatVersion {
		return fmt.Errorf("unsupported bundle v%d", m.Format)
	}
	if m.Format < BundleFormatVersion {
		return fmt.Errorf("invalid MANIFEST: unknown format %d", m.Format)
	}
	names := make([]string, 0, len(m.Files))
	for name := range m.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		want := m.Files[name]
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("MANIFEST mismatch: %s", name)
		}
		sum := sha256.Sum256(raw)
		got := ManifestEntry{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw))}
		if got != want {
			return fmt.Errorf("MANIFEST mismatch: %s", name)
		}
	}
	return nil
}
