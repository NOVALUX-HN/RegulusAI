// Evolution domains (Phase 6, S9): INV improvements change how the agent
// works (harness, instructions, tools, workflows); DOM improvements change
// what a practitioner knows. The boundary is structural: a DOM candidate
// that touches harness fundamentals is a breach and must be reclassified
// as INV or narrowed — it can never silently modify the guarantees.
package evolve

import (
	"errors"
	"fmt"
	"strings"
)

// FundamentalComponents are the harness guarantees a DOM improvement must
// never silently touch.
var FundamentalComponents = []string{
	"gates", "evidence", "receipts", "budget", "manifest", "verdict",
	"guardians", "decisions", "trace", "surface", "probe", "regulus.json",
	"execution policy", "almanac", "sextant",
}

// ErrDomainBreach refuses a DOM candidate over fundamentals.
var ErrDomainBreach = errors.New("evolve: DOM improvement touches harness fundamentals (reclassify as INV or narrow the candidate)")

// TouchesFundamentals names the fundamentals a candidate text reaches.
func TouchesFundamentals(candidate string) []string {
	lower := strings.ToLower(candidate)
	var hit []string
	for _, f := range FundamentalComponents {
		if strings.Contains(lower, f) {
			hit = append(hit, f)
		}
	}
	return hit
}

// CheckDomain enforces the INV/DOM separation on a stored experiment.
func CheckDomain(exp Experiment) error {
	if exp.Domain != DomainDOM {
		return nil
	}
	if hit := TouchesFundamentals(exp.Candidate); len(hit) != 0 {
		return fmt.Errorf("%w: %s", ErrDomainBreach, strings.Join(hit, ", "))
	}
	return nil
}
