package almanac

import (
	"time"
)

// InvalidatedIDs returns observations retired by invalidated_by relations
// whose endpoints both exist. Like superseded knowledge, invalidated
// knowledge hides from default retrieval: it was checked and found wrong.
func InvalidatedIDs(root string) (map[string]bool, error) {
	rels, err := Relations(root)
	if err != nil {
		return nil, err
	}
	if len(rels) == 0 {
		return nil, nil
	}
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	exists := map[string]bool{}
	for _, o := range all {
		exists[o.ID] = true
	}
	out := map[string]bool{}
	for _, r := range rels {
		if r.Relation == RelationInvalidatedBy && exists[r.From] && exists[r.To] {
			out[r.To] = true
		}
	}
	return out, nil
}

// Conflict is one live contradiction: a conflicts_with edge whose both
// endpoints are active knowledge (not superseded, invalidated, rejected
// or expired).
type Conflict struct {
	A string
	B string
}

// Contradictions lists live conflicts_with edges. Edges touching retired
// knowledge are history, not live contradictions.
func Contradictions(root string, now time.Time) ([]Conflict, error) {
	rels, err := Relations(root)
	if err != nil {
		return nil, err
	}
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	byID := map[string]Observation{}
	for _, o := range all {
		byID[o.ID] = o
	}
	sup, err := SupersededIDs(root)
	if err != nil {
		return nil, err
	}
	inv, err := InvalidatedIDs(root)
	if err != nil {
		return nil, err
	}
	active := func(id string) bool {
		o, ok := byID[id]
		if !ok {
			return false
		}
		if sup[id] || inv[id] || IsExpired(&o, now) {
			return false
		}
		st, err := EffectiveStatus(root, id)
		if err != nil || st == StatusRejected {
			return false
		}
		return true
	}
	var out []Conflict
	for _, r := range rels {
		if r.Relation != RelationConflictsWith {
			continue
		}
		if active(r.From) && active(r.To) {
			out = append(out, Conflict{A: r.From, B: r.To})
		}
	}
	return out, nil
}
