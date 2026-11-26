// Outcome persistence (F17.3 wiring).
//
// StrategyOutcome records live beside the run's manifest, inside the run
// directory the store already owns, so there is one store and one atomic-write
// path instead of a parallel one. A reader that does not know about outcomes is
// unaffected: the file simply exists next to the manifest.
package recovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// outcomesFileName is the sibling document inside a run directory.
const outcomesFileName = "outcomes.json"

// outcomesDoc is the on-disk shape. It carries its own version so a reader can
// refuse a document written by a different format instead of misreading it.
type outcomesDoc struct {
	Version   string            `json:"version"`
	RunID     string            `json:"run_id"`
	UpdatedAt time.Time         `json:"updated_at"`
	Outcomes  []StrategyOutcome `json:"outcomes"`
}

// OutcomesPath returns where a run's outcomes are stored.
func OutcomesPath(root, runID string) (string, error) {
	dir, err := StoreDir(root, runID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, outcomesFileName), nil
}

// SaveOutcomes writes every record after validating each one: an outcome that
// could not have come from a measurement is refused before it reaches disk.
func SaveOutcomes(root, runID string, outcomes []StrategyOutcome, now time.Time) (string, error) {
	for i := range outcomes {
		if err := outcomes[i].Validate(); err != nil {
			return "", err
		}
	}
	path, err := OutcomesPath(root, runID)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(outcomesDoc{
		Version:   OutcomeVersion,
		RunID:     runID,
		UpdatedAt: now.UTC(),
		Outcomes:  outcomes,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := atomicWrite(path, data); err != nil {
		return "", err
	}
	return path, nil
}

// LoadOutcomes reads a run's outcomes. A missing file is not an error: a run
// that never recorded one simply has no history, and inventing a document for
// it would blur "absent" with "empty".
func LoadOutcomes(root, runID string) ([]StrategyOutcome, error) {
	path, err := OutcomesPath(root, runID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc outcomesDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("recovery: outcomes for run %q are unreadable: %w", runID, err)
	}
	if doc.Version != OutcomeVersion {
		return nil, fmt.Errorf("recovery: outcomes for run %q use version %q, want %q", runID, doc.Version, OutcomeVersion)
	}
	for i := range doc.Outcomes {
		if err := doc.Outcomes[i].Validate(); err != nil {
			return nil, fmt.Errorf("recovery: outcomes for run %q failed validation: %w", runID, err)
		}
	}
	return doc.Outcomes, nil
}

// AppendOutcome loads, appends and saves in one call, so a caller cannot
// accidentally drop the existing history.
func AppendOutcome(root, runID string, outcome StrategyOutcome, now time.Time) (string, error) {
	existing, err := LoadOutcomes(root, runID)
	if err != nil {
		return "", err
	}
	return SaveOutcomes(root, runID, append(existing, outcome), now)
}
