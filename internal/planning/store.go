// Versioned plan store (F13.2/NOV-105): one directory per mission
// holding every plan revision plus a manifest with per-file sha256.
// Writes are atomic (temp file + rename) under an inter-process
// lock; reads verify the sha before use and fail closed. History
// is append-only: replan adds a version, never rewrites or deletes
// one, so the original plan and every change reason survive.
package planning

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/fsx"
	"regulus-ai/internal/mission"
)

// Store file names inside one mission plans directory.
const (
	PlanMeta    = "manifest.json"
	maxPlanSize = 2 * 1024 * 1024
)

// PlanEntry cites one stored revision: the file, its sha and the
// reason the revision exists.
type PlanEntry struct {
	Version      int       `json:"version"`
	File         string    `json:"file"`
	SHA          string    `json:"sha256"`
	CreatedAt    time.Time `json:"created_at"`
	ChangeReason string    `json:"change_reason,omitempty"`
	Status       string    `json:"status,omitempty"`
	Kind         string    `json:"kind,omitempty"`
}

// PlanManifest is the append-only index over every revision.
type PlanManifest struct {
	Version   string      `json:"version"`
	MissionID string      `json:"mission_id"`
	Versions  []PlanEntry `json:"versions"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// PlansDir resolves the plans directory of one mission. Mission ids
// are validated so no traversal escapes .regulus/missions.
func PlansDir(root, missionID string) (string, error) {
	if err := mission.ValidateID(missionID); err != nil {
		return "", err
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Join(root, ".regulus", "missions", missionID, "plans"), nil
}

// planFile names one revision file: zero-padded so lexical order
// is version order.
func planFile(version int) string {
	return fmt.Sprintf("plan-%04d.json", version)
}

// SavePlan persists one plan revision atomically with its manifest
// entry. An existing file for the same version with different bytes
// refuses (replan bumps, never overwrites); identical bytes are an
// idempotent no-op success.
func SavePlan(root string, p *Plan) (string, error) {
	if p == nil {
		return "", fmt.Errorf("planning: cannot store a nil plan")
	}
	if err := mission.ValidateID(p.MissionID); err != nil {
		return "", err
	}
	if p.Version != PlanVersion {
		return "", fmt.Errorf("planning: plan version %q, want %q", p.Version, PlanVersion)
	}
	if p.PlanVersion <= 0 {
		return "", fmt.Errorf("planning: plan revision must be >= 1, got %d", p.PlanVersion)
	}
	dir, err := PlansDir(root, p.MissionID)
	if err != nil {
		return "", err
	}
	release, err := fsx.LockDir(dir)
	if err != nil {
		return "", err
	}
	defer release()
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	sum := sha256.Sum256(raw)
	hexSum := hex.EncodeToString(sum[:])
	name := planFile(p.PlanVersion)
	existing, rerr := os.ReadFile(filepath.Join(dir, name))
	if rerr == nil {
		esum := sha256.Sum256(existing)
		if hex.EncodeToString(esum[:]) != hexSum {
			return "", fmt.Errorf("planning: plan v%d of mission %q already stored with different bytes (replan bumps, never overwrites)", p.PlanVersion, p.MissionID)
		}
	} else if !os.IsNotExist(rerr) {
		return "", fmt.Errorf("planning: cannot read stored plan: %w", rerr)
	} else if err := atomicWrite(filepath.Join(dir, name), raw); err != nil {
		return "", err
	}
	man, merr := loadManifestLocked(dir, p.MissionID)
	if merr != nil {
		return "", merr
	}
	found := false
	for i := range man.Versions {
		if man.Versions[i].Version == p.PlanVersion {
			man.Versions[i] = PlanEntry{
				Version: p.PlanVersion, File: name, SHA: hexSum,
				CreatedAt: p.CreatedAt.UTC(), ChangeReason: p.ChangeReason,
				Status: p.Status, Kind: p.Kind,
			}
			found = true
		}
	}
	if !found {
		man.Versions = append(man.Versions, PlanEntry{
			Version: p.PlanVersion, File: name, SHA: hexSum,
			CreatedAt: p.CreatedAt.UTC(), ChangeReason: p.ChangeReason,
			Status: p.Status, Kind: p.Kind,
		})
	}
	sort.Slice(man.Versions, func(i, j int) bool { return man.Versions[i].Version < man.Versions[j].Version })
	man.UpdatedAt = time.Now().UTC()
	if err := writeManifestLocked(dir, man); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// LoadPlan reads one revision, verifying its sha before use.
func LoadPlan(root, missionID string, version int) (*Plan, error) {
	dir, err := PlansDir(root, missionID)
	if err != nil {
		return nil, err
	}
	release, err := fsx.LockDir(dir)
	if err != nil {
		return nil, err
	}
	defer release()
	man, err := loadManifestLocked(dir, missionID)
	if err != nil {
		return nil, err
	}
	want := ""
	wantSHA := ""
	for _, e := range man.Versions {
		if e.Version == version {
			want = e.File
			wantSHA = e.SHA
		}
	}
	if want == "" {
		return nil, fmt.Errorf("planning: mission %q holds no plan v%d (%d revision(es) stored)", missionID, version, len(man.Versions))
	}
	raw, err := os.ReadFile(filepath.Join(dir, want))
	if err != nil {
		return nil, fmt.Errorf("planning: cannot read plan v%d of mission %q: %w", version, missionID, err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != wantSHA {
		return nil, fmt.Errorf("planning: plan v%d of mission %q sha mismatch (edited store is not evidence)", version, missionID)
	}
	return decodePlan(missionID, version, want, raw)
}

// LatestPlan reads the highest stored revision.
func LatestPlan(root, missionID string) (*Plan, error) {
	entries, err := ListPlans(root, missionID)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("planning: mission %q holds no plan (usa mission plan primero)", missionID)
	}
	return LoadPlan(root, missionID, entries[len(entries)-1].Version)
}

// ListPlans summarizes every stored revision in version order.
// A missing plans directory lists as empty.
func ListPlans(root, missionID string) ([]PlanEntry, error) {
	dir, err := PlansDir(root, missionID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, PlanMeta)); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("planning: cannot read plans of mission %q: %w", missionID, err)
	}
	release, err := fsx.LockDir(dir)
	if err != nil {
		return nil, err
	}
	defer release()
	man, err := loadManifestLocked(dir, missionID)
	if err != nil {
		return nil, err
	}
	return append([]PlanEntry(nil), man.Versions...), nil
}

// loadManifestLocked reads the index inside the caller's lock; a
// missing manifest starts an empty index (first plan seals it).
func loadManifestLocked(dir, missionID string) (PlanManifest, error) {
	man := PlanManifest{Version: PlanVersion, MissionID: missionID}
	raw, err := os.ReadFile(filepath.Join(dir, PlanMeta))
	if err != nil {
		if os.IsNotExist(err) {
			return man, nil
		}
		return PlanManifest{}, fmt.Errorf("planning: cannot read plan manifest of mission %q: %w", missionID, err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&man); err != nil {
		return PlanManifest{}, fmt.Errorf("planning: malformed plan manifest of mission %q: %w", missionID, err)
	}
	if man.MissionID != "" && man.MissionID != missionID {
		return PlanManifest{}, fmt.Errorf("planning: plan manifest id drift (%q, want %q)", man.MissionID, missionID)
	}
	man.MissionID = missionID
	if man.Version != PlanVersion {
		return PlanManifest{}, fmt.Errorf("planning: plan manifest version %q unsupported (want %q)", man.Version, PlanVersion)
	}
	return man, nil
}

// writeManifestLocked seals the index atomically.
func writeManifestLocked(dir string, man PlanManifest) error {
	raw, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, PlanMeta), append(raw, '\n'))
}

// decodePlan verifies size, sha-shape and strict shape before use.
func decodePlan(missionID string, version int, file string, raw []byte) (*Plan, error) {
	if len(raw) > maxPlanSize {
		return nil, fmt.Errorf("planning: plan v%d of mission %q exceeds %d bytes", version, missionID, maxPlanSize)
	}
	var p Plan
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("planning: malformed plan v%d of mission %q: %w", version, missionID, err)
	}
	if p.Version != PlanVersion {
		return nil, fmt.Errorf("planning: plan v%d version %q, want %q", version, p.Version, PlanVersion)
	}
	if p.MissionID != missionID {
		return nil, fmt.Errorf("planning: plan v%d id drift (%q, want %q)", version, p.MissionID, missionID)
	}
	if p.PlanVersion != version {
		return nil, fmt.Errorf("planning: plan file %s holds revision %d (want %d)", file, p.PlanVersion, version)
	}
	return &p, nil
}

// atomicWrite stores data via temp-plus-rename so a crash leaves
// either the previous bytes or the new ones, never a half file.
func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
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
