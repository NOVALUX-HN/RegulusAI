// Baselines, versioned adoption and rollback (Phase 6, S7-S8). A named
// baseline pins the stable metrics a candidate compares against. Adopt
// records a version (monotonic number, snapshot, approver) and advances
// the experiment to ADOPTED; Rollback supersedes a version and hands the
// snapshot back. Versions plus experiment histories are the audit trail.
package evolve

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// baselinesDir returns the baseline store under a project root.
func baselinesDir(root string) string {
	return filepath.Join(EvolveDir(root), "baselines")
}

// versionsDir returns the version store under a project root.
func versionsDir(root string) string {
	return filepath.Join(EvolveDir(root), "versions")
}

// SetBaseline pins the stable metrics under name.
func SetBaseline(root, name string, m Metrics) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("evolve: baseline needs a name")
	}
	if err := os.MkdirAll(baselinesDir(root), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(baselinesDir(root), name+".json"), append(raw, '\n'), 0o644)
}

// GetBaseline reads pinned metrics. A missing baseline is an error: there
// is nothing to compare a candidate against.
func GetBaseline(root, name string) (Metrics, error) {
	data, err := os.ReadFile(filepath.Join(baselinesDir(root), name+".json"))
	if err != nil {
		return Metrics{}, fmt.Errorf("evolve: no baseline %q: %v", name, err)
	}
	var m Metrics
	if err := json.Unmarshal(data, &m); err != nil {
		return Metrics{}, fmt.Errorf("evolve: malformed baseline %q: %v", name, err)
	}
	return m, nil
}

// Version is one adopted change.
type Version struct {
	N            int    `json:"n"`
	ExperimentID string `json:"experiment_id"`
	Snapshot     string `json:"snapshot"`
	AdoptedBy    string `json:"adopted_by"`
	At           string `json:"at"`
	Superseded   bool   `json:"superseded"`
	Note         string `json:"note,omitempty"`
}

// Adopt records a versioned adoption: the experiment must be EVALUATED
// with outcomes, the candidate must clear the mandatory-guarantee gate
// against the experiment's named baseline, and a human approver is
// required (no self-approval, S10). The transition alone never adopts.
func Adopt(root, expID string, candidate Metrics, snapshot, approvedBy string) (Version, error) {
	if strings.TrimSpace(snapshot) == "" {
		return Version{}, errors.New("evolve: adoption needs a snapshot to roll back to")
	}
	if strings.TrimSpace(approvedBy) == "" {
		return Version{}, errors.New("evolve: adoption needs human approval (no self-approval)")
	}
	exp, err := LoadExperiment(root, expID)
	if err != nil {
		return Version{}, err
	}
	if exp.State != StateEvaluated {
		return Version{}, fmt.Errorf("evolve: only EVALUATED experiments adopt, not %s", exp.State)
	}
	if strings.TrimSpace(exp.Results) == "" {
		return Version{}, errors.New("evolve: adoption needs recorded evaluation outcomes")
	}
	base, err := GetBaseline(root, exp.Baseline)
	if err != nil {
		return Version{}, err
	}
	if err := Adoptable(candidate, base); err != nil {
		return Version{}, err
	}
	versions, err := ListVersions(root)
	if err != nil {
		return Version{}, err
	}
	v := Version{
		N: len(versions) + 1, ExperimentID: exp.ID, Snapshot: snapshot,
		AdoptedBy: approvedBy, At: time.Now().UTC().Format(time.RFC3339),
	}
	if err := os.MkdirAll(versionsDir(root), 0o755); err != nil {
		return Version{}, err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Version{}, err
	}
	if err := os.WriteFile(filepath.Join(versionsDir(root), fmt.Sprintf("%d.json", v.N)), append(raw, '\n'), 0o644); err != nil {
		return Version{}, err
	}
	exp.History = append(exp.History, ExpHistoryEvent{
		At: v.At, Kind: "adopted", From: exp.State, To: StateAdopted,
		By: approvedBy, Note: fmt.Sprintf("adopted as version %d", v.N),
	})
	exp.State = StateAdopted
	if err := exp.save(root); err != nil {
		return Version{}, err
	}
	return v, nil
}

// ListVersions returns every version in number order.
func ListVersions(root string) ([]Version, error) {
	paths, err := filepath.Glob(filepath.Join(versionsDir(root), "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []Version
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var v Version
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("evolve: malformed version %s: %v", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// Rollback supersedes version n and returns it: the caller restores the
// snapshot. The reason is recorded on the version (audit trail).
func Rollback(root string, n int, reason string) (Version, error) {
	if strings.TrimSpace(reason) == "" {
		return Version{}, errors.New("evolve: rollback needs a reason")
	}
	path := filepath.Join(versionsDir(root), fmt.Sprintf("%d.json", n))
	data, err := os.ReadFile(path)
	if err != nil {
		return Version{}, fmt.Errorf("evolve: no version %d: %v", n, err)
	}
	var v Version
	if err := json.Unmarshal(data, &v); err != nil {
		return Version{}, fmt.Errorf("evolve: malformed version %d: %v", n, err)
	}
	if v.Superseded {
		return Version{}, fmt.Errorf("evolve: version %d already superseded", n)
	}
	v.Superseded = true
	v.Note = "rollback: " + reason
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Version{}, err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return Version{}, err
	}
	return v, nil
}
