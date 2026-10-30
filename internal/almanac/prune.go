package almanac

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Prune selects the observations that fail keep (candidates for
// removal) without touching disk — the dry-run half. Pinned
// observations and lessons are always kept: pins protect intent,
// lessons are evidence. The caller executes with ApplyPrune.
func Prune(root string, keep func(Observation) bool) ([]Observation, error) {
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	pins, err := PinnedIDs(root)
	if err != nil {
		return nil, err
	}
	var out []Observation
	for _, o := range all {
		if pins[o.ID] || o.Type == TypeLesson {
			continue
		}
		if !keep(o) {
			out = append(out, o)
		}
	}
	return out, nil
}

// ApplyPrune rewrites the log without exactly the given observations
// (matched by id), atomically. Pins and relations are untouched:
// pruning drops content, never provenance events.
func ApplyPrune(root string, removed []Observation) error {
	if len(removed) == 0 {
		return nil
	}
	drop := map[string]bool{}
	for _, o := range removed {
		drop[o.ID] = true
	}
	all, err := All(root)
	if err != nil {
		return err
	}
	var lines []string
	for _, o := range all {
		if drop[o.ID] {
			continue
		}
		raw, err := json.Marshal(o)
		if err != nil {
			return err
		}
		lines = append(lines, string(raw))
	}
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(File(root)), "observations-*.jsonl")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	body := strings.Join(lines, "\n")
	if body != "" {
		body += "\n"
	}
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, File(root))
}
