// Package protocol states the engineering protocol as data.
//
// F20.1 asked for a transversal REQP integrated into the existing workflow, without
// a new reviewer or tooling stack. The audit (docs/reqp-integration-audit.md) found
// that roughly half of the protocol already blocks - required gates, the chart
// barrier, the close contract, receipt hashing, idempotency - and the other half is
// prose: subtract, attack-the-premise, root cause, TDD, behaviour, boundaries.
//
// Those cannot simply be gated. "Attack the premise" has no deterministic oracle,
// and a gate that pretends to measure judgement is exactly the vanity metric the
// phase excludes. So this package does the one thing that is both honest and
// useful: it records, per practice, whether something actually blocks, whether
// something advises, or whether it is a human call - and it pins every citation to
// a marker string that must still exist in the file.
//
// The pin is what makes this more than documentation. Deleting the zero-skipped
// check in sextant.close, or the required flag in gates, breaks
// TestCitedCodeStillExists: the protocol cannot drift away from the code it claims.
package protocol

import (
	"fmt"
	"strings"
)

// Enforcement is how a practice is held, in descending order of teeth.
type Enforcement string

const (
	// Enforced means something blocks: a required gate, a refused close, a
	// check that fails the run.
	Enforced Enforcement = "enforced"
	// Advisory means something reports but gates nothing.
	Advisory Enforcement = "advisory"
	// Judgement means a human or agent decides; there is no oracle to encode.
	Judgement Enforcement = "judgement"
)

// Practice is one rule of the engineering protocol.
//
// File and Marker are the citation: Marker must appear verbatim in File. A
// practice that claims enforcement must carry both, and the test suite verifies
// the marker still exists rather than trusting this table.
type Practice struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Enforcement Enforcement `json:"enforcement"`
	File        string      `json:"file,omitempty"`
	Marker      string      `json:"marker,omitempty"`
	Note        string      `json:"note"`
}

// All returns the protocol. The order is the order of the phase that asked for it
// (NOV-150), so the list can be compared against the requirement directly.
func All() []Practice {
	return []Practice{
		{
			ID: "subtract", Title: "Subtract before you add",
			Enforcement: Judgement,
			File:        "internal/cli/review.go", Marker: "Necessity Ladder",
			Note: "The review lens prints a Necessity Ladder checklist, but printing is not blocking. A subtraction gate would have to judge new code, which is the reviewer's job and not a deterministic check.",
		},
		{
			ID: "premise", Title: "Attack the premise",
			Enforcement: Judgement,
			Note:        "Whether a requirement is the right one cannot be computed. It is recorded as a call so nobody mistakes its absence for an oversight.",
		},
		{
			ID: "root-cause", Title: "Root cause first",
			Enforcement: Judgement,
			Note:        "A fix that treats a symptom passes every test the symptom had. Only reading the diff can catch it.",
		},
		{
			ID: "tdd", Title: "Test-first when viable",
			Enforcement: Judgement,
			Note:        "Coverage floor and CRAP measure the result, not the order. They cannot tell a test written first from one written after, so no gate is claimed here.",
		},
		{
			ID: "small-units", Title: "Small verifiable units",
			Enforcement: Enforced,
			File:        "regulus.json", Marker: `"name": "size"`,
			Note: "The size gate is required, so it is never skipped by profile. It bounds unit size mechanically, which is the part of this practice that has an oracle.",
		},
		{
			ID: "behavior", Title: "Test behaviour, not implementation",
			Enforcement: Judgement,
			Note:        "A test that asserts internals still passes the suite. Judging whether it tests behaviour requires reading it.",
		},
		{
			ID: "boundaries", Title: "Test the boundaries",
			Enforcement: Judgement,
			Note:        "Boundary coverage has no dedicated check. Mutation testing is the closest proxy and runs full-profile only.",
		},
		{
			ID: "idempotency", Title: "Idempotent effects",
			Enforcement: Enforced,
			File:        "internal/almanac/dedup.go", Marker: "func contentHash",
			Note: "A repeated save hashes to the same key and becomes a no-op. Recovery effects are exactly-once per idempotency key.",
		},
		{
			ID: "evidence", Title: "Evidence, not claims",
			Enforcement: Enforced,
			File:        "internal/sextant/close.go", Marker: "len(m.Skipped) > 0",
			Note: "Close refuses on any skipped gate, any non-pass verdict, any missing manifest and any hash drift. Required gates additionally cannot be skipped by profile (gates.Gate.IsRequired).",
		},
		{
			ID: "regression", Title: "No regression",
			Enforcement: Advisory,
			File:        "internal/verify/regression.go", Marker: "func DetectRegression",
			Note: "The window comparison reports; it gates nothing at the top level. It is enforced only where a decision consumes it: context promotion, bench adoption and evolve comparison.",
		},
		{
			ID: "rollback", Title: "Reversible change",
			Enforcement: Advisory,
			File:        "internal/evolve/rollback.go", Marker: "func Rollback",
			Note: "Rollback exists for evolve versions only. There is no generic code-change rollback, and receipts are deliberately never rewritten: a new run is a new receipt.",
		},
	}
}

// Validate reports every defect in the table at once.
//
// It rejects a table that cannot be printed honestly: an unknown enforcement, a
// missing title, a duplicate id, or an enforcement claim with no citation. The
// command refuses to render a protocol that fails this, so a malformed table is a
// failure and never a silent default.
func Validate(practices []Practice) []string {
	var errs []string
	seen := make(map[string]bool, len(practices))
	for i, p := range practices {
		where := fmt.Sprintf("practice %d", i+1)
		if p.ID == "" {
			errs = append(errs, where+": empty id")
		} else {
			if seen[p.ID] {
				errs = append(errs, fmt.Sprintf("%s: duplicate id %q", where, p.ID))
			}
			seen[p.ID] = true
			where = p.ID
		}
		if strings.TrimSpace(p.Title) == "" {
			errs = append(errs, where+": empty title")
		}
		if strings.TrimSpace(p.Note) == "" {
			errs = append(errs, where+": empty note")
		}
		switch p.Enforcement {
		case Enforced, Advisory, Judgement:
		default:
			errs = append(errs, fmt.Sprintf("%s: unknown enforcement %q", where, string(p.Enforcement)))
		}
		if p.Enforcement == Enforced && (p.File == "" || p.Marker == "") {
			errs = append(errs, where+": claims enforcement with no citation")
		}
		if (p.File == "") != (p.Marker == "") {
			errs = append(errs, where+": citation needs both file and marker")
		}
	}
	return errs
}
