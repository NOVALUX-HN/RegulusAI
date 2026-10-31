package sextant

import (
	"strings"
	"testing"
)

const goodChart = `# Feature

Domain: go
Class: STANDARD

## Contract

- Objective: ship the thing
- Scope: cli package only
- Constraints: no new dependencies
- Acceptance: the happy path
- Artifacts: internal/cli/x.go
- Dependencies: none
- Risk: standard
- Verification: gauntlet + receipts
- Completion: scenarios PROVEN with receipt ids

## Acceptance

### Scenario: the happy path

- Given a clean project
- When the change runs
- Then the exit code is 0

## Tasks

| ID | Spec | Work | Route | Depends | Status |
|----|------|------|-------|---------|--------|
| T1 | the happy path | implement | inline | - | todo |

## Log

- L1: authorized.
`

func TestParseClassLine(t *testing.T) {
	c, ok := ParseClassLine(goodChart)
	if !ok || c != ClassStandard {
		t.Fatalf("want STANDARD present, got %v/%v", c, ok)
	}
	if _, ok := ParseClassLine("# No class here\n"); ok {
		t.Fatal("absent Class line must report false")
	}
	if _, ok := ParseClassLine("Class: MEDIUM\n"); ok {
		t.Fatal("invalid Class value must report false")
	}
}

func TestParseContractValid(t *testing.T) {
	ct, violations := ParseContract(goodChart)
	if len(violations) != 0 {
		t.Fatalf("valid contract must not violate: %v", violations)
	}
	if ct.Objective == "" || ct.Class != ClassStandard {
		t.Fatalf("contract must carry objective and class, got %+v", ct)
	}
	if len(ct.AcceptanceNames) != 1 || ct.AcceptanceNames[0] != "the happy path" {
		t.Fatalf("acceptance must name the scenario, got %v", ct.AcceptanceNames)
	}
}

func TestParseContractMissingSection(t *testing.T) {
	doc := "# Legacy\n\nClass: TRIVIAL\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n"
	ct, violations := ParseContract(doc)
	if len(violations) != 0 {
		t.Fatalf("absent contract section must not violate, got %v", violations)
	}
	if ct.Present {
		t.Fatal("absent section must report Present=false")
	}
}

func TestParseContractRequiredKeysByLevel(t *testing.T) {
	// STANDARD without Scope/Acceptance/Verification must fail naming lines.
	doc := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n"
	_, violations := ParseContract(doc)
	if len(violations) == 0 {
		t.Fatal("missing STANDARD keys must violate")
	}
	joined := strings.Join(violations, "\n")
	for _, want := range []string{"Scope", "Acceptance", "Verification"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("violations must name %q, got:\n%s", want, joined)
		}
	}
	for _, v := range violations {
		if !strings.HasPrefix(v, "line ") {
			t.Fatalf("violations must cite line numbers, got %q", v)
		}
	}
	// CRITICAL additionally demands Constraints + Verifier + Completion.
	crit := "# F\n\nClass: CRITICAL\n\n## Contract\n\n- Objective: x\n- Scope: y\n- Acceptance: s\n- Verification: g\n- Artifacts: a\n- Dependencies: none\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n"
	_, violations = ParseContract(crit)
	joined = strings.Join(violations, "\n")
	for _, want := range []string{"Constraints", "Verifier", "Completion"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("critical violations must name %q, got:\n%s", want, joined)
		}
	}
}

func TestParseContractAcceptanceTraceability(t *testing.T) {
	// Named scenario that does not exist must fail.
	doc := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n- Scope: y\n- Acceptance: ghost scenario\n- Verification: g\n\n## Acceptance\n\n### Scenario: real scenario\n\n- Given a\n- When b\n- Then c\n"
	_, violations := ParseContract(doc)
	if len(violations) == 0 || !strings.Contains(strings.Join(violations, "\n"), "ghost scenario") {
		t.Fatalf("missing scenario must be named, got %v", violations)
	}
	// Acceptance: all requires at least one scenario.
	noScenarios := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n- Scope: y\n- Acceptance: all\n- Verification: g\n\n## Acceptance\n\nno scenarios here\n"
	_, violations = ParseContract(noScenarios)
	if len(violations) == 0 {
		t.Fatal("Acceptance: all with no scenarios must violate")
	}
}

func TestParseContractRiskClassConsistency(t *testing.T) {
	doc := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n- Scope: y\n- Acceptance: s\n- Verification: g\n- Risk: critical\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n"
	_, violations := ParseContract(doc)
	if len(violations) == 0 || !strings.Contains(strings.Join(violations, "\n"), "Risk") {
		t.Fatalf("Risk/Class mismatch must be flagged, got %v", violations)
	}
}

func TestParseContractUnknownKeysRejected(t *testing.T) {
	doc := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n- Scope: y\n- Acceptance: s\n- Verification: g\n- Magic: z\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n"
	_, violations := ParseContract(doc)
	if len(violations) == 0 || !strings.Contains(strings.Join(violations, "\n"), "Magic") {
		t.Fatalf("unknown keys must be rejected, got %v", violations)
	}
}
