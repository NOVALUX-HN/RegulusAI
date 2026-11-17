// Durable mission store (F13.1/NOV-104): one directory per
// mission holding mission.json plus a manifest carrying its
// sha256. Writes are atomic (temp file + rename) under an
// inter-process lock; every generation bump is checked against
// the stored one, so two coordinators racing the same mission
// fail loudly instead of overwriting silently. Reads verify the
// sha before use and fail closed: an edited file is not
// evidence, load refuses it rather than guessing. Version drift
// migrates explicitly (v0 -> v1) or refuses explicitly.
package mission

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
)

// Store file names inside one mission directory.
const (
	MissionFile = "mission.json"
	MissionMeta = "manifest.json"
	maxStore    = 4 * 1024 * 1024
)

// MissionManifest cites the stored bytes: sha256 of mission.json
// plus the version and timestamps that reproduce the write.
type MissionManifest struct {
	Version     string    `json:"version"`
	MissionID   string    `json:"mission_id"`
	MissionFile string    `json:"mission_file"`
	MissionSHA  string    `json:"mission_sha256"`
	Generation  int64     `json:"generation"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// StoreDir resolves the durable directory for one mission under a
// project root. Mission ids are validated so no traversal escapes
// .regulus/missions.
func StoreDir(root, missionID string) (string, error) {
	if err := ValidateID(missionID); err != nil {
		return "", err
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Join(root, ".regulus", "missions", missionID), nil
}

// saveLocked persists one validated mission with generation
// expected+1 under the caller's dir lock, atomically with its
// manifest. Shared by create/save/update so every write path
// seals the same checksum discipline.
func saveLocked(dir string, m *Mission, expected int64, now time.Time) (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	m.Version = MissionVersion
	m.Generation = expected + 1
	if m.Effects == nil {
		m.Effects = map[string]EffectResult{}
	}
	m.UpdatedAt = now.UTC()
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	if err := atomicWrite(filepath.Join(dir, MissionFile), raw); err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	man := MissionManifest{
		Version: MissionVersion, MissionID: m.ID, MissionFile: MissionFile,
		MissionSHA: hex.EncodeToString(sum[:]), Generation: m.Generation,
		CreatedAt: m.CreatedAt.UTC(), UpdatedAt: m.UpdatedAt.UTC(),
	}
	mraw, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return "", err
	}
	manPath := filepath.Join(dir, MissionMeta)
	if err := atomicWrite(manPath, append(mraw, '\n')); err != nil {
		return "", err
	}
	return manPath, nil
}

// CreateMission persists a brand-new mission: an existing store
// with the same id refuses (create never overwrites, not even at
// generation 0, so two coordinators creating the same id fail
// loudly instead of clobbering each other). The sealed record
// carries generation 1.
func CreateMission(root string, m *Mission, now time.Time) (string, error) {
	if m == nil {
		return "", fmt.Errorf("mission: cannot store a nil mission")
	}
	if err := m.Validate(); err != nil {
		return "", err
	}
	dir, err := StoreDir(root, m.ID)
	if err != nil {
		return "", err
	}
	release, err := fsx.LockDir(dir)
	if err != nil {
		return "", err
	}
	defer release()
	if _, _, lerr := loadLocked(dir, m.ID); lerr == nil {
		return "", fmt.Errorf("mission: mission %q already exists (create refuses to overwrite; advance the stored one)", m.ID)
	} else if !os.IsNotExist(lerr) {
		return "", lerr
	}
	return saveLocked(dir, m, 0, now)
}

// SaveMission persists one mission record atomically with its
// manifest. expectedGen is the generation the caller loaded: when
// the stored generation differs, another coordinator won the race
// and the save refuses with a conflict instead of overwriting.
// On success the generation advances by exactly one and UpdatedAt
// moves to now, so status can quote freshness honestly.
func SaveMission(root string, m *Mission, expectedGen int64, now time.Time) (string, error) {
	if m == nil {
		return "", fmt.Errorf("mission: cannot store a nil mission")
	}
	if err := m.Validate(); err != nil {
		return "", err
	}
	dir, err := StoreDir(root, m.ID)
	if err != nil {
		return "", err
	}
	release, err := fsx.LockDir(dir)
	if err != nil {
		return "", err
	}
	defer release()
	if _, stored, lerr := loadLocked(dir, m.ID); lerr == nil {
		if stored.Generation != expectedGen {
			return "", fmt.Errorf("mission: mission %q generation conflict (stored %d, caller holds %d): another coordinator mutated it, reload and retry", m.ID, stored.Generation, expectedGen)
		}
	} else if !os.IsNotExist(lerr) {
		return "", lerr
	} else if expectedGen != 0 {
		return "", fmt.Errorf("mission: mission %q does not exist (nothing to update)", m.ID)
	}
	return saveLocked(dir, m, expectedGen, now)
}

// loadLocked reads one mission record inside the caller's dir lock,
// verifying its sha before use and migrating legacy shapes
// explicitly. Callers persist the migrated record on their next
// save; load itself never writes, so a read-only crash
// inspection cannot corrupt the store.
func loadLocked(dir, missionID string) (MissionManifest, *Mission, error) {
	mraw, err := os.ReadFile(filepath.Join(dir, MissionMeta))
	if err != nil {
		if os.IsNotExist(err) {
			return MissionManifest{}, nil, err
		}
		return MissionManifest{}, nil, fmt.Errorf("mission: cannot read mission %q: %w", missionID, err)
	}
	var man MissionManifest
	dec := json.NewDecoder(strings.NewReader(string(mraw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&man); err != nil {
		return MissionManifest{}, nil, fmt.Errorf("mission: malformed manifest for mission %q: %w", missionID, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, MissionFile))
	if err != nil {
		return MissionManifest{}, nil, fmt.Errorf("mission: cannot read mission %q: %w", missionID, err)
	}
	if len(raw) > maxStore {
		return MissionManifest{}, nil, fmt.Errorf("mission: mission %q exceeds %d bytes", missionID, maxStore)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != man.MissionSHA {
		return MissionManifest{}, nil, fmt.Errorf("mission: mission %q sha mismatch (edited store is not evidence)", missionID)
	}
	switch man.Version {
	case MissionVersion:
		return loadV1(missionID, man, raw)
	case LegacyVersion:
		return migrateV0(missionID, man, raw)
	default:
		return MissionManifest{}, nil, fmt.Errorf("mission: mission %q version %q unsupported (want %q; migrate explicitly)", missionID, man.Version, MissionVersion)
	}
}

// loadV1 decodes a current-shape record, refusing drift.
func loadV1(missionID string, man MissionManifest, raw []byte) (MissionManifest, *Mission, error) {
	var m Mission
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return MissionManifest{}, nil, fmt.Errorf("mission: malformed mission %q: %w", missionID, err)
	}
	if m.Version != MissionVersion {
		return MissionManifest{}, nil, fmt.Errorf("mission: mission %q version %q, want %q", missionID, m.Version, MissionVersion)
	}
	if m.ID != missionID || m.ID != man.MissionID {
		return MissionManifest{}, nil, fmt.Errorf("mission: mission %q id drift (file %q manifest %q)", missionID, m.ID, man.MissionID)
	}
	if m.Generation != man.Generation {
		return MissionManifest{}, nil, fmt.Errorf("mission: mission %q generation drift (file %d manifest %d)", missionID, m.Generation, man.Generation)
	}
	if m.Effects == nil {
		m.Effects = map[string]EffectResult{}
	}
	if err := m.Validate(); err != nil {
		return MissionManifest{}, nil, err
	}
	return man, &m, nil
}

// legacyV0 is the pre-F13 shape: identity, intent and state, but
// no risk tier, budget, links, effects, generation or audit trail.
type legacyV0 struct {
	MissionID   string    `json:"mission_id"`
	Objective   string    `json:"objective"`
	Description string    `json:"description"`
	Repo        string    `json:"repo"`
	Workspace   string    `json:"workspace"`
	Constraints []string  `json:"constraints"`
	Criteria    []string  `json:"acceptance_criteria"`
	Creator     string    `json:"creator"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// migrateV0 upgrades a legacy record explicitly: defaults fill
// what v0 never knew (risk medium, zero budget, empty links and
// ledger, generation 0) and the migration itself enters the audit
// history, so nothing is ever lost silently.
func migrateV0(missionID string, man MissionManifest, raw []byte) (MissionManifest, *Mission, error) {
	var old legacyV0
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&old); err != nil {
		return MissionManifest{}, nil, fmt.Errorf("mission: malformed legacy mission %q: %w", missionID, err)
	}
	if old.MissionID != missionID || old.MissionID != man.MissionID {
		return MissionManifest{}, nil, fmt.Errorf("mission: legacy mission %q id drift (file %q manifest %q)", missionID, old.MissionID, man.MissionID)
	}
	if !knownState(old.State) {
		return MissionManifest{}, nil, fmt.Errorf("mission: legacy mission %q holds unknown state %q (migrate explicitly, never guess)", missionID, old.State)
	}
	eid, err := mintEventID()
	if err != nil {
		return MissionManifest{}, nil, err
	}
	m := &Mission{
		Version: MissionVersion, ID: old.MissionID, Objective: old.Objective,
		Description: old.Description, Repo: old.Repo, Workspace: old.Workspace,
		Constraints: append([]string(nil), old.Constraints...),
		Criteria:    append([]string(nil), old.Criteria...),
		Risk:        RiskMedium, Creator: old.Creator,
		CreatedAt: old.CreatedAt.UTC(), UpdatedAt: old.UpdatedAt.UTC(),
		State: old.State, Generation: 0, Effects: map[string]EffectResult{},
		History: []Event{{EventID: eid, From: LegacyVersion, To: old.State, Reason: "migrated mission-v0 to mission-v1 (risk=medium default, empty links/ledger)", At: time.Now().UTC()}},
	}
	if err := m.Validate(); err != nil {
		return MissionManifest{}, nil, err
	}
	return man, m, nil
}

// LoadMission reads one mission record, verifying its sha before
// use. A migrated (v0) record loads as v1 in memory; the caller
// persists it with SaveMission to seal the migration durably.
// A missing store refuses before locking, so a read of an
// unknown id never litters the tree with empty directories.
func LoadMission(root, missionID string) (MissionManifest, *Mission, error) {
	dir, err := StoreDir(root, missionID)
	if err != nil {
		return MissionManifest{}, nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, MissionMeta)); err != nil {
		if os.IsNotExist(err) {
			return MissionManifest{}, nil, fmt.Errorf("mission: cannot read mission %q: %w", missionID, err)
		}
	}
	release, err := fsx.LockDir(dir)
	if err != nil {
		return MissionManifest{}, nil, err
	}
	defer release()
	man, m, err := loadLocked(dir, missionID)
	if err != nil {
		return MissionManifest{}, nil, err
	}
	return man, m, nil
}

// UpdateMission is the single-coordinator mutation path: lock,
// load the confirmed state, apply fn in memory, persist with the
// loaded generation as the expected one. A concurrent coordinator
// that saved first makes this update refuse with a generation
// conflict instead of overwriting silently. fn errors abort
// before anything is written: the store keeps the previous
// confirmed state.
func UpdateMission(root, missionID string, now time.Time, fn func(*Mission) error) (*Mission, error) {
	dir, err := StoreDir(root, missionID)
	if err != nil {
		return nil, err
	}
	release, err := fsx.LockDir(dir)
	if err != nil {
		return nil, err
	}
	defer release()
	man, m, err := loadLocked(dir, missionID)
	if err != nil {
		return nil, err
	}
	_ = man
	expected := m.Generation
	if err := fn(m); err != nil {
		return nil, err
	}
	if _, err := saveLocked(dir, m, expected, now); err != nil {
		return nil, err
	}
	return m, nil
}

// ListEntry is one mission summary plus its load problem, if any:
// corrupt entries are reported explicitly, never skipped
// silently and never fatal to their healthy siblings.
type ListEntry struct {
	ID         string
	State      string
	Objective  string
	Generation int64
	UpdatedAt  time.Time
	Problem    string
}

// ListMissions summarizes every mission directory under root.
// Missing stores list as empty; corrupt entries carry Problem
// (the exact refusal) instead of disappearing.
func ListMissions(root string) ([]ListEntry, error) {
	base := filepath.Join(root, ".regulus", "missions")
	names, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []ListEntry
	for _, n := range names {
		if !n.IsDir() {
			continue
		}
		id := n.Name()
		man, m, lerr := LoadMission(root, id)
		_ = man
		if lerr != nil {
			out = append(out, ListEntry{ID: id, Problem: lerr.Error()})
			continue
		}
		out = append(out, ListEntry{
			ID: m.ID, State: m.State, Objective: m.Objective,
			Generation: m.Generation, UpdatedAt: m.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// atomicWrite stores data via temp-plus-rename so a crash mid-write
// leaves either the previous bytes or the new ones, never a half
// file the next load would misread.
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
