// Durable store (F12.5/NOV-100): one directory per run holding
// run.json plus a manifest carrying its sha256. Writes are atomic
// (temp file + rename); reads verify the sha before use and fail
// closed like F11.3 tier evidence: an edited run file is not
// evidence, resume refuses it rather than guessing.
package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store file names inside one run directory.
const (
	RunFile  = "run.json"
	RunMeta  = "manifest.json"
	maxStore = 4 * 1024 * 1024
)

// RunManifest cites the stored bytes: sha256 of run.json plus the
// version and timestamps that reproduce the write.
type RunManifest struct {
	Version   string    `json:"version"`
	RunID     string    `json:"run_id"`
	RunFile   string    `json:"run_file"`
	RunSHA256 string    `json:"run_sha256"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StoreDir resolves the durable directory for one run under a
// project root. Run ids are validated so no traversal escapes
// .regulus/recovery.
func StoreDir(root, runID string) (string, error) {
	if err := ValidateRunID(runID); err != nil {
		return "", err
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Join(root, ".regulus", "recovery", runID), nil
}

// SaveRun persists one run record atomically with its manifest.
// The record version is pinned before write; UpdatedAt advances to
// now so status can quote freshness honestly.
func SaveRun(root string, r *RunRecord, now time.Time) (string, error) {
	if r == nil {
		return "", fmt.Errorf("recovery: cannot store a nil run")
	}
	if err := ValidateRunID(r.RunID); err != nil {
		return "", err
	}
	r.Version = RecoveryVersion
	r.TraceRunID = r.RunID
	if r.Checks == nil {
		r.Checks = map[string]*Checkpoint{}
	}
	r.UpdatedAt = now.UTC()
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	dir, err := StoreDir(root, r.RunID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := atomicWrite(filepath.Join(dir, RunFile), raw); err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	m := RunManifest{
		Version: RecoveryVersion, RunID: r.RunID, RunFile: RunFile,
		RunSHA256: hex.EncodeToString(sum[:]),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	mraw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	manPath := filepath.Join(dir, RunMeta)
	if err := atomicWrite(manPath, append(mraw, '\n')); err != nil {
		return "", err
	}
	return manPath, nil
}

// LoadRun reads one run record, verifying its sha before use. An
// edited run.json (sha mismatch), a version drift or an oversized
// file fails closed: resume must never reconstruct intent from
// bytes nobody wrote.
func LoadRun(root, runID string) (RunManifest, *RunRecord, error) {
	dir, err := StoreDir(root, runID)
	if err != nil {
		return RunManifest{}, nil, err
	}
	mraw, err := os.ReadFile(filepath.Join(dir, RunMeta))
	if err != nil {
		return RunManifest{}, nil, fmt.Errorf("recovery: cannot read run %q: %w", runID, err)
	}
	var m RunManifest
	dec := json.NewDecoder(strings.NewReader(string(mraw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return RunManifest{}, nil, fmt.Errorf("recovery: malformed manifest for run %q: %w", runID, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, RunFile))
	if err != nil {
		return RunManifest{}, nil, fmt.Errorf("recovery: cannot read run %q: %w", runID, err)
	}
	if len(raw) > maxStore {
		return RunManifest{}, nil, fmt.Errorf("recovery: run %q exceeds %d bytes", runID, maxStore)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != m.RunSHA256 {
		return RunManifest{}, nil, fmt.Errorf("recovery: run %q sha mismatch (edited store is not evidence)", runID)
	}
	var r RunRecord
	dec2 := json.NewDecoder(strings.NewReader(string(raw)))
	dec2.DisallowUnknownFields()
	if err := dec2.Decode(&r); err != nil {
		return RunManifest{}, nil, fmt.Errorf("recovery: malformed run %q: %w", runID, err)
	}
	if r.Version != RecoveryVersion {
		return RunManifest{}, nil, fmt.Errorf("recovery: run %q version %q, want %q", runID, r.Version, RecoveryVersion)
	}
	if r.RunID != runID || r.RunID != m.RunID {
		return RunManifest{}, nil, fmt.Errorf("recovery: run %q id drift (file %q manifest %q)", runID, r.RunID, m.RunID)
	}
	if r.Checks == nil {
		r.Checks = map[string]*Checkpoint{}
	}
	return m, &r, nil
}

// atomicWrite stores data via temp-plus-rename so a crash mid-write
// leaves either the previous bytes or the new ones, never a half
// file the next resume would misread.
func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		// The directory may not exist yet when callers write the
		// manifest first in tests; surface the error plainly.
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
