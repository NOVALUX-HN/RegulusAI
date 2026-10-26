// Package budget records the evolution budget ledger: every lesson
// admission the core accepts (architecture §8; PRD R-E4 — no unbounded
// self-modification).
package budget

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry is one ledger record.
type Entry struct {
	Kind      string    `json:"kind"`
	Timestamp time.Time `json:"timestamp"`
	Task      string    `json:"task"`
	Domain    string    `json:"domain"`
	Outcome   string    `json:"outcome"`
	Detail    string    `json:"detail,omitempty"`
}

// File returns the ledger path under a project root.
func File(root string) string {
	return filepath.Join(root, ".regulus", "budget.json")
}

// Record appends one entry with an atomic rewrite (single writer: the core).
func Record(root string, e Entry) error {
	entries, err := Show(root)
	if err != nil {
		return err
	}
	entries = append(entries, e)
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		return err
	}
	tmp := File(root) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, File(root))
}

// Show returns every entry, oldest first. A missing ledger is empty.
func Show(root string) ([]Entry, error) {
	data, err := os.ReadFile(File(root))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("malformed budget ledger: %w", err)
	}
	return entries, nil
}
