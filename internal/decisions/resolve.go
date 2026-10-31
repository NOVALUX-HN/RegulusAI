// Resolution governance (Phase 4, S7): assessment advises, policy gates,
// humans approve. ACCEPT requires zero blocking findings; critical
// decisions additionally require a recorded human approval; INCONCLUSIVE is
// always available (abstention); supersede is atomic with the replacing
// accept so no scope is ever decided twice.
package decisions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ResolveOptions selects one resolution.
type ResolveOptions struct {
	Accept       bool
	Reject       bool
	Inconclusive bool
	// Supersede names an ACCEPTED same-scope decision replaced atomically
	// by this accept. Requires human approval.
	Supersede string
	// ApproveBy records human approval as "nick: reason".
	ApproveBy string
	// Note carries the rationale for reject/inconclusive.
	Note string
}

// List returns every stored decision ordered by id.
func List(root string) ([]Decision, error) {
	paths, err := filepath.Glob(filepath.Join(Dir(root), "dec_*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []Decision
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var d Decision
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("decision: malformed %s: %v", filepath.Base(p), err)
		}
		out = append(out, d)
	}
	return out, nil
}

// accepted returns the currently ACCEPTED decisions for obsolete checks.
func accepted(root string, exceptID string) []Decision {
	all, err := List(root)
	if err != nil {
		return nil
	}
	var out []Decision
	for _, d := range all {
		if d.State == StateAccepted && d.ID != exceptID {
			out = append(out, d)
		}
	}
	return out
}

func validApproval(s string) bool {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}

// Resolve applies one governed resolution and persists it. Refusals change
// nothing. Assessment runs fresh inside every accept: stale approvals of
// changed evidence are impossible by construction.
func Resolve(root, id string, opt ResolveOptions) (Decision, error) {
	actions := 0
	if opt.Accept {
		actions++
	}
	if opt.Reject {
		actions++
	}
	if opt.Inconclusive {
		actions++
	}
	if actions != 1 {
		return Decision{}, fmt.Errorf("decide: choose exactly one of --accept, --reject, --inconclusive")
	}
	d, err := Load(root, id)
	if err != nil {
		return Decision{}, err
	}
	if d.State != StateProposed && d.State != StateUnderReview {
		return Decision{}, fmt.Errorf("decide: %s is %s (terminal or already resolved)", id, d.State)
	}
	by := "core"
	if strings.TrimSpace(opt.ApproveBy) != "" {
		if !validApproval(opt.ApproveBy) {
			return Decision{}, fmt.Errorf("decide: --approve-by must look like \"nick: reason\"")
		}
		by = strings.TrimSpace(opt.ApproveBy)
	}
	switch {
	case opt.Reject:
		if strings.TrimSpace(opt.Note) == "" {
			return Decision{}, fmt.Errorf("decide: --reject needs --note (the rationale)")
		}
		if d.State == StateProposed {
			if err := d.ApplyTransition(StateUnderReview, by, "review started", "assessed"); err != nil {
				return Decision{}, err
			}
		}
		if err := d.ApplyTransition(StateRejected, by, opt.Note, "resolved"); err != nil {
			return Decision{}, err
		}
		return d, d.Save(root)
	case opt.Inconclusive:
		if strings.TrimSpace(opt.Note) == "" {
			return Decision{}, fmt.Errorf("decide: --inconclusive needs --note (why the system abstains)")
		}
		if d.State == StateProposed {
			if err := d.ApplyTransition(StateUnderReview, by, "review started", "assessed"); err != nil {
				return Decision{}, err
			}
		}
		if err := d.ApplyTransition(StateInconclusive, by, opt.Note, "resolved"); err != nil {
			return Decision{}, err
		}
		return d, d.Save(root)
	}

	// Accept: fresh assessment, policy gates, optional atomic supersede.
	p := Assess(root, d, accepted(root, d.ID))
	if p.Policy.ObsoleteBy != "" && strings.TrimSpace(opt.Supersede) == "" {
		return Decision{}, fmt.Errorf("decide: scope already decided by %s: supersede it with --supersede %s or narrow scope", p.Policy.ObsoleteBy, p.Policy.ObsoleteBy)
	}
	if p.Policy.HasBlocking {
		addressed := strings.TrimSpace(opt.Supersede) != "" && strings.TrimSpace(opt.Supersede) == p.Policy.ObsoleteBy && onlyObsoleteBlocking(p)
		if !addressed {
			return Decision{}, fmt.Errorf("decide: blocked by policy (%s); resolve the findings or abstain with --inconclusive", firstBlocking(p))
		}
	}
	if d.Contract.Critical && strings.TrimSpace(opt.ApproveBy) == "" {
		return Decision{}, fmt.Errorf("decide: critical decisions need human approval (--approve-by \"nick: reason\")")
	}
	if strings.TrimSpace(opt.Supersede) != "" {
		if strings.TrimSpace(opt.ApproveBy) == "" {
			return Decision{}, fmt.Errorf("decide: superseding needs human approval (--approve-by \"nick: reason\")")
		}
		old, err := Load(root, opt.Supersede)
		if err != nil {
			return Decision{}, fmt.Errorf("decide: supersede target: %v", err)
		}
		if old.State != StateAccepted {
			return Decision{}, fmt.Errorf("decide: can only supersede an ACCEPTED decision (got %s %s)", old.ID, old.State)
		}
		if !strings.EqualFold(strings.TrimSpace(old.Contract.Scope), strings.TrimSpace(d.Contract.Scope)) {
			return Decision{}, fmt.Errorf("decide: supersede scope mismatch (%q vs %q)", old.Contract.Scope, d.Contract.Scope)
		}
		if err := old.ApplyTransition(StateSuperseded, by, "superseded by "+d.ID, "resolved"); err != nil {
			return Decision{}, err
		}
		if err := old.Save(root); err != nil {
			return Decision{}, err
		}
	}
	if d.State == StateProposed {
		if err := d.ApplyTransition(StateUnderReview, by, "review started", "assessed"); err != nil {
			return Decision{}, err
		}
	}
	if err := d.ApplyTransition(StateAccepted, by, p.Rationale, "resolved"); err != nil {
		return Decision{}, err
	}
	return d, d.Save(root)
}

func onlyObsoleteBlocking(p Pipeline) bool {
	for _, f := range p.Findings {
		if f.Severity == SeverityBlocking && f.Code != FindingObsolete {
			return false
		}
	}
	return true
}

func firstBlocking(p Pipeline) string {
	for _, f := range p.Findings {
		if f.Severity == SeverityBlocking {
			return f.Code + ": " + f.Detail
		}
	}
	return "policy"
}
