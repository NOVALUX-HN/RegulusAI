package almanac

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pin toggles whether an observation is pinned. Pins are append-only
// events (last event wins); the id must exist.
func Pin(root, id string, pinned bool, now time.Time) error {
	all, err := All(root)
	if err != nil {
		return err
	}
	found := false
	for _, o := range all {
		if o.ID == id {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no observation %q", id)
	}
	type pinEvent struct {
		ID     string    `json:"id"`
		Pinned bool      `json:"pinned"`
		At     time.Time `json:"at"`
	}
	if err := os.MkdirAll(filepath.Dir(pinsFile(root)), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(pinsFile(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(pinEvent{ID: id, Pinned: pinned, At: now.UTC()})
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// PinnedIDs returns the ids whose last pin event is pinned.
func PinnedIDs(root string) (map[string]bool, error) {
	data, err := os.ReadFile(pinsFile(root))
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	pins := map[string]bool{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var ev struct {
			ID     string `json:"id"`
			Pinned bool   `json:"pinned"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("malformed pin event: %v", err)
		}
		pins[ev.ID] = ev.Pinned
	}
	return pins, nil
}

// Relation types.
const (
	RelationSupersedes    = "supersedes"
	RelationConflictsWith = "conflicts_with"
	RelationRelated       = "related"
)

// Relation is one recorded relation between two observations.
type Relation struct {
	From     string    `json:"from"`
	Relation string    `json:"relation"`
	To       string    `json:"to"`
	At       time.Time `json:"at"`
}

// Relate records a validated relation between two existing observations.
func Relate(root, from, relation, to string, now time.Time) error {
	switch relation {
	case RelationSupersedes, RelationConflictsWith, RelationRelated:
	default:
		return fmt.Errorf("relation %q is not one of supersedes, conflicts_with, related", relation)
	}
	if from == to {
		return errors.New("an observation cannot relate to itself")
	}
	all, err := All(root)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, o := range all {
		have[o.ID] = true
	}
	if !have[from] {
		return fmt.Errorf("no observation %q", from)
	}
	if !have[to] {
		return fmt.Errorf("no observation %q", to)
	}
	if err := os.MkdirAll(filepath.Dir(relationsFile(root)), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(relationsFile(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(Relation{From: from, Relation: relation, To: to, At: now.UTC()})
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// Relations returns every recorded relation, oldest first.
func Relations(root string) ([]Relation, error) {
	data, err := os.ReadFile(relationsFile(root))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rels []Relation
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var r Relation
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("malformed relation: %v", err)
		}
		rels = append(rels, r)
	}
	return rels, nil
}
