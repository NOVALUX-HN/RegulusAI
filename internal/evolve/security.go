// Untrusted-proposal security (Phase 6, S10): every proposal is treated
// as untrusted until it proves otherwise. The six-point checklist is
// attested while the experiment runs and verified structurally by Adopt:
// isolation, provenance, validations, regressions, policies and
// reversibility must all hold, plus human approval. Nothing self-approves.
package evolve

import (
	"errors"
	"fmt"
	"strings"
)

// TrustCheck is the six-point untrusted-proposal checklist.
type TrustCheck struct {
	Isolated    bool   `json:"isolated"`
	Provenance  string `json:"provenance"`
	Validations bool   `json:"validations"`
	Regressions bool   `json:"regressions"`
	Policies    bool   `json:"policies"`
	Reversible  bool   `json:"reversible"`
}

// Verify refuses any checklist gap, naming the missing item.
func (c TrustCheck) Verify() error {
	if !c.Isolated {
		return errors.New("evolve: untrusted proposal: not isolated")
	}
	if strings.TrimSpace(c.Provenance) == "" {
		return errors.New("evolve: untrusted proposal: no provenance")
	}
	if !c.Validations {
		return errors.New("evolve: untrusted proposal: validations not passed")
	}
	if !c.Regressions {
		return errors.New("evolve: untrusted proposal: regressions not passed")
	}
	if !c.Policies {
		return errors.New("evolve: untrusted proposal: policies not met")
	}
	if !c.Reversible {
		return errors.New("evolve: untrusted proposal: not reversible")
	}
	return nil
}

// AttestTrust records the checklist on a RUNNING experiment. Partial
// attestations store fine; Adopt verifies completeness, so a gap can be
// recorded but never adopted through.
func (e *Experiment) AttestTrust(root string, check TrustCheck) error {
	if e.State != StateRunning {
		return fmt.Errorf("evolve: trust is attested while RUNNING, not %s", e.State)
	}
	e.Trust = check
	return e.save(root)
}
