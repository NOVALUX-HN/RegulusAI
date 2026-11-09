package almanac

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Prune selects the observations that fail keep (candidates for
// removal) without touching disk — the dry-run half. Pinned
// observations are always kept: pins protect intent. Active lessons
// are evidence and stay immune; retired lessons (superseded,
// invalidated, rejected or expired) prune like any other content when
// unpinned — GC without touching live knowledge. The caller executes
// with ApplyPrune.
func Prune(root string, keep func(Observation) bool) ([]Observation, error) {
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	pins, err := PinnedIDs(root)
	if err != nil {
		return nil, err
	}
	retired, err := retiredIDs(root)
	if err != nil {
		return nil, err
	}
	var out []Observation
	for _, o := range all {
		if pins[o.ID] {
			continue
		}
		if o.Type == TypeLesson && !retired[o.ID] {
			continue
		}
		if !keep(o) {
			out = append(out, o)
		}
	}
	return out, nil
}

// retiredIDs folds every retirement signal for lessons: superseded and
// invalidated relations (endpoints must exist), REJECTED governance
// state, and expired temporal validity. Active knowledge is absent.
func retiredIDs(root string) (map[string]bool, error) {
	out := map[string]bool{}
	sup, err := SupersededIDs(root)
	if err != nil {
		return nil, err
	}
	for id := range sup {
		out[id] = true
	}
	inv, err := InvalidatedIDs(root)
	if err != nil {
		return nil, err
	}
	for id := range inv {
		out[id] = true
	}
	ends, err := statusEnds(root)
	if err != nil {
		return nil, err
	}
	for id, to := range ends {
		if to == StatusRejected {
			out[id] = true
		}
	}
	now := time.Now().UTC()
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	for _, o := range all {
		if o.Type == TypeLesson && IsExpired(&o, now) {
			out[o.ID] = true
		}
	}
	return out, nil
}

// ApplyPrune rewrites the log without exactly the given observations
// (matched by id), atomically. Pins and relations are untouched:
// pruning drops content, never provenance events. M2: the rewrite runs
// under the store lock (serialized with Save/Import/Reindex); the file
// still lands via tmp+Rename.
func ApplyPrune(root string, removed []Observation) error {
	if len(removed) == 0 {
		return nil
	}
	return withStoreLock(root, func() error {
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
	})
}
