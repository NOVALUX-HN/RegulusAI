// Package surface is the mechanical gate for harness-surface edits
// (PRD R-E3): adopted lessons may change the harness itself — a skill
// text, a guardian prompt, a plugin line — but only through a proposal
// that cites evidence, is validated by a named guardian, and consumes
// the evolution budget. Nothing edits the surface outside this gate.
package surface

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/reflect"
)

// Edit is one proposed or applied surface edit.
type Edit struct {
	ID        string    `json:"id"`      // sfx_<hex>
	Target    string    `json:"target"`  // repo-relative file inside the whitelist
	Content   string    `json:"content"` // the text to append
	Reason    string    `json:"reason"`  // why — cites the lesson/evidence
	Status    string    `json:"status"`  // proposed | applied
	Guardian  string    `json:"guardian,omitempty"`
	At        time.Time `json:"at"`
	AppliedAt time.Time `json:"applied_at,omitempty"`
}

// Whitelist defines the harness surface: the files an adopted lesson may
// edit. Everything else is refused.
var Whitelist = []string{
	".opencode/skills/",
	".opencode/agents/",
	".opencode/plugins/",
	".opencode/discipline.md",
	".opencode/tool-guidance.md",
	"docs/",
	"scripts/",
}

func File(root string) string {
	return filepath.Join(root, ".regulus", "surface", "edits.jsonl")
}

func validateTarget(root, target string) error {
	normalized := filepath.ToSlash(filepath.Clean(target))
	for _, prefix := range Whitelist {
		if strings.HasPrefix(normalized, prefix) {
			return nil
		}
	}
	return fmt.Errorf("%q is outside the harness surface whitelist (%s)", target, strings.Join(Whitelist, ", "))
}

func readAll(root string) ([]Edit, error) {
	data, err := os.ReadFile(File(root))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var edits []Edit
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var e Edit
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("malformed surface edit: %v", err)
		}
		edits = append(edits, e)
	}
	return edits, nil
}

func appendEdit(root string, e Edit) error {
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(File(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// Reserved lists harness files the surface gate never edits, even
// though they sit inside the whitelist: release and delivery
// infrastructure changes by hand (human decision), never as a
// side effect of an adopted lesson.
var Reserved = []string{
	"scripts/build.bash",
}

// IsReserved reports whether a whitelisted target is reserved from
// surface edits (release/delivery infrastructure).
func IsReserved(target string) bool {
	normalized := filepath.ToSlash(filepath.Clean(target))
	for _, r := range Reserved {
		if normalized == r {
			return true
		}
	}
	return false
}

// Propose records a surface edit proposal. The target must be inside
// the whitelist; the content and reason must be non-empty; the daily
// evolution cap must not be reached. The proposal is recorded in the
// budget ledger (KindSurface, surface-proposed).
func Propose(root, target, content, reason string, now time.Time) (Edit, error) {
	if err := validateTarget(root, target); err != nil {
		return Edit{}, err
	}
	if IsReserved(target) {
		return Edit{}, fmt.Errorf("%q is a reserved harness file (release infrastructure): edit it by hand, not through surface edits", target)
	}
	if strings.TrimSpace(content) == "" {
		return Edit{}, errors.New("content must not be empty")
	}
	if strings.TrimSpace(reason) == "" {
		return Edit{}, errors.New("reason must not be empty — cite the lesson or evidence")
	}
	// cap check (PRD R-E3: surface edits under budget caps)
	cap := reflect.EvolutionCap(root)
	today, err := budget.TodayEvolutionCount(root, now)
	if err != nil {
		return Edit{}, err
	}
	if today >= cap {
		if err := budget.Record(root, budget.Entry{
			Kind:      budget.KindSurface,
			Timestamp: now.UTC(),
			Outcome:   budget.OutcomeRefusedCap,
			Detail:    fmt.Sprintf("cap %d reached", cap),
		}); err != nil {
			return Edit{}, err
		}
		return Edit{}, fmt.Errorf("evolution cap reached: %d run(s) today, cap %d (regulus.json evolution.max_per_day)", today, cap)
	}
	e := Edit{
		ID:      "sfx_" + fmt.Sprintf("%x", now.UnixNano()),
		Target:  target,
		Content: content,
		Reason:  reason,
		Status:  "proposed",
		At:      now.UTC(),
	}
	if err := appendEdit(root, e); err != nil {
		return Edit{}, err
	}
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindSurface,
		Timestamp: now.UTC(),
		Outcome:   budget.OutcomeSurfaceProposed,
		Detail:    target,
	}); err != nil {
		return Edit{}, err
	}
	return e, nil
}

// Apply executes a proposal after guardian validation: appends the
// content to the target file (bounded, marker-delimited) and records
// who validated.
func Apply(root, id, guardian string, now time.Time) (Edit, error) {
	edits, err := readAll(root)
	if err != nil {
		return Edit{}, err
	}
	var found *Edit
	for i := range edits {
		if edits[i].ID == id {
			if edits[i].Status == "applied" {
				return Edit{}, fmt.Errorf("surface edit %q is already applied (guardian %s)", id, edits[i].Guardian)
			}
			if edits[i].Status == "proposed" && found == nil {
				found = &edits[i]
			}
		}
	}
	if found == nil {
		return Edit{}, fmt.Errorf("no proposed surface edit %q", id)
	}
	if strings.TrimSpace(guardian) == "" {
		return Edit{}, errors.New("guardian must not be empty: only a guardian-validated edit is applied")
	}
	// execute: append to the target, marker-delimited (creating parent dirs
	// so a surface edit can add a new file)
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, filepath.FromSlash(found.Target))), 0o755); err != nil {
		return Edit{}, fmt.Errorf("creating target dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(root, filepath.FromSlash(found.Target)), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Edit{}, fmt.Errorf("opening target: %w", err)
	}
	defer f.Close()
	block := fmt.Sprintf("\n<!-- regulus:surface-edit %s (%s) -->\n%s\n<!-- /regulus:surface-edit -->\n", found.ID, guardian, found.Content)
	if _, err := f.WriteString(block); err != nil {
		return Edit{}, err
	}
	// record the applied state (append the applied copy — the proposal stays)
	found.Status = "applied"
	found.Guardian = guardian
	found.AppliedAt = now.UTC()
	if err := appendEdit(root, *found); err != nil {
		return Edit{}, err
	}
	// record the application in the budget (mechanical — no cap consumption)
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindSurface,
		Timestamp: now.UTC(),
		Outcome:   budget.OutcomeSurfaceApplied,
		Detail:    "guardian " + guardian,
	}); err != nil {
		return Edit{}, err
	}
	return *found, nil
}

// List returns every edit (proposals and applied), oldest first.
func List(root string) ([]Edit, error) {
	return readAll(root)
}
