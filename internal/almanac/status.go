// Knowledge governance (Phase 2, S5): observations enter as OBSERVED
// (saved) or PROPOSED (imported, pending review); lessons admitted through
// reflect are ACCEPTED by construction. VERIFIED marks a check against
// cited evidence, REJECTED a refusal with reason, SUPERSEDED is computed
// from supersedes relations. Transitions append to an append-only status
// log — agents never promote silently, and the log is the audit trail.
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

// Governance states.
const (
	StatusObserved   = "OBSERVED"
	StatusProposed   = "PROPOSED"
	StatusVerified   = "VERIFIED"
	StatusAccepted   = "ACCEPTED"
	StatusRejected   = "REJECTED"
	StatusSuperseded = "SUPERSEDED"
)

// StatusEvent is one appended governance transition.
type StatusEvent struct {
	ID   string    `json:"id"`
	From string    `json:"from"`
	To   string    `json:"to"`
	By   string    `json:"by"`
	Note string    `json:"note"`
	At   time.Time `json:"at"`
}

// StatusFile returns the append-only governance log path.
func StatusFile(root string) string {
	return filepath.Join(root, ".regulus", "almanac", "status.jsonl")
}

// allowedTransitions gates every status move. Terminal states never move;
// SUPERSEDED is computed from relations, never recorded.
var allowedTransitions = map[string][]string{
	StatusObserved: {StatusVerified, StatusAccepted, StatusRejected},
	StatusProposed: {StatusVerified, StatusAccepted, StatusRejected},
	StatusVerified: {StatusAccepted, StatusRejected},
	StatusAccepted: {},
	StatusRejected: {},
}

// RecordStatus validates a transition from the observation's effective
// status and appends it to the status log.
func RecordStatus(root, id, to, by, note string, now time.Time) error {
	if strings.TrimSpace(by) == "" {
		return errors.New("status transitions need an actor (by)")
	}
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
	from, err := EffectiveStatus(root, id)
	if err != nil {
		return err
	}
	next, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("status %s is terminal or computed", from)
	}
	allowed := false
	for _, n := range next {
		if n == to {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("status transition %s -> %s is not allowed", from, to)
	}
	if err := os.MkdirAll(filepath.Dir(StatusFile(root)), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(StatusFile(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(StatusEvent{ID: id, From: from, To: to, By: by, Note: note, At: now.UTC()})
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// MarkImported records the proposing event for imported observations:
// foreign knowledge enters PROPOSED, never laundered into higher states.
func MarkImported(root string, ids []string, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(StatusFile(root)), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(StatusFile(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, id := range ids {
		line, err := json.Marshal(StatusEvent{ID: id, From: "", To: StatusProposed, By: "import", Note: "imported bundle, pending review", At: now.UTC()})
		if err != nil {
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// statusEnds folds the status log to each observation's latest state.
func statusEnds(root string) (map[string]string, error) {
	data, err := os.ReadFile(StatusFile(root))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := map[string]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var ev StatusEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("malformed status event: %v", err)
		}
		out[ev.ID] = ev.To
	}
	return out, nil
}

// StatusHistory returns one observation's transitions, oldest first.
func StatusHistory(root, id string) ([]StatusEvent, error) {
	data, err := os.ReadFile(StatusFile(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []StatusEvent
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var ev StatusEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("malformed status event: %v", err)
		}
		if ev.ID == id {
			out = append(out, ev)
		}
	}
	return out, nil
}

// lastStatus folds one observation's log to its latest state.
func lastStatus(root, id string) (string, error) {
	hist, err := StatusHistory(root, id)
	if err != nil {
		return "", err
	}
	if len(hist) == 0 {
		return "", nil
	}
	return hist[len(hist)-1].To, nil
}

// EffectiveStatus resolves one observation's governance state: explicit
// log entries win; SUPERSEDED is computed from relations; otherwise the
// type/source defaults apply (lessons ACCEPTED, imports PROPOSED,
// everything else OBSERVED).
func EffectiveStatus(root, id string) (string, error) {
	all, err := All(root)
	if err != nil {
		return "", err
	}
	var found *Observation
	for i, o := range all {
		if o.ID == id {
			found = &all[i]
			break
		}
	}
	if found == nil {
		return "", fmt.Errorf("no observation %q", id)
	}
	if st, err := lastStatus(root, id); err != nil {
		return "", err
	} else if st != "" {
		return st, nil
	}
	sup, err := SupersededIDs(root)
	if err != nil {
		return "", err
	}
	if sup[id] {
		return StatusSuperseded, nil
	}
	switch {
	case found.Type == TypeLesson:
		return StatusAccepted, nil
	case found.Source == SourceImport:
		return StatusProposed, nil
	default:
		return StatusObserved, nil
	}
}
