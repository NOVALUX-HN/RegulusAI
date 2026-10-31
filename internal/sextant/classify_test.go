// Package sextant is the machine-checkable engineering discipline
// (Phase 2): task classification, structured contracts, delegation rules,
// progress governance and core-verified completion — the Sextant workflow
// (authorize, chart, build, prove, learn) with proportional depth.
package sextant

import (
	"testing"
)

func TestParseClass(t *testing.T) {
	for _, c := range []Class{ClassTrivial, ClassStandard, ClassComplex, ClassCritical} {
		got, err := ParseClass(string(c))
		if err != nil || got != c {
			t.Fatalf("ParseClass(%q) = %v, %v", c, got, err)
		}
	}
	if _, err := ParseClass("MEDIUM"); err == nil {
		t.Fatal("unknown class must be rejected")
	}
	if _, err := ParseClass(""); err == nil {
		t.Fatal("empty class must be rejected")
	}
}

func TestClassifyMatrix(t *testing.T) {
	cases := []struct {
		name    string
		signals Signals
		want    Class
	}{
		{"read-only investigation", Signals{}, ClassTrivial},
		{"docs-only prose", Signals{}, ClassTrivial},
		{"single code unit", Signals{TouchesCode: true}, ClassStandard},
		{"two units", Signals{TouchesCode: true, Units: 2}, ClassComplex},
		{"with dependencies", Signals{TouchesCode: true, Dependencies: 1}, ClassComplex},
		{"contract change", Signals{TouchesCode: true, ContractConsumed: true}, ClassCritical},
		{"concurrency", Signals{TouchesCode: true, Concurrency: true}, ClassCritical},
		{"delivery", Signals{TouchesCode: true, Delivery: true}, ClassCritical},
		{"destructive", Signals{TouchesCode: true, Destructive: true}, ClassCritical},
		{"security", Signals{TouchesCode: true, Security: true}, ClassCritical},
		{"unclear risk", Signals{TouchesCode: true, UnclearRisk: true}, ClassCritical},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.signals); got != c.want {
				t.Fatalf("Classify(%+v) = %s, want %s", c.signals, got, c.want)
			}
		})
	}
}

func TestTrivialNeverEvadesSecurity(t *testing.T) {
	// Even with zero code touched and a single unit, security and
	// destructive markers escalate: a trivial label must never bypass
	// mandatory policies.
	for _, s := range []Signals{
		{Destructive: true},
		{Security: true},
		{UnclearRisk: true},
		{ContractConsumed: true, Units: 1},
		{Concurrency: true},
		{Delivery: true},
	} {
		if got := Classify(s); got != ClassCritical {
			t.Fatalf("Classify(%+v) = %s, want CRITICAL (no evasion)", s, got)
		}
	}
}

func TestClassifyPrecedence(t *testing.T) {
	// Risk beats size: a destructive multi-unit task is CRITICAL, and a
	// complex-but-safe task stays COMPLEX.
	if got := Classify(Signals{TouchesCode: true, Units: 5, Dependencies: 3, Destructive: true}); got != ClassCritical {
		t.Fatalf("risk must beat size, got %s", got)
	}
	if got := Classify(Signals{TouchesCode: true, Units: 5, Dependencies: 3}); got != ClassComplex {
		t.Fatalf("multi-unit safe work is COMPLEX, got %s", got)
	}
}

func TestRequirementsCoverAllDimensions(t *testing.T) {
	for _, c := range []Class{ClassTrivial, ClassStandard, ClassComplex, ClassCritical} {
		r := Requirements(c)
		checks := map[string]string{
			"planning":      r.Planning,
			"specification": r.Specification,
			"execution":     r.Execution,
			"verification":  r.Verification,
			"documentation": r.Documentation,
			"completion":    r.Completion,
		}
		for dim, text := range checks {
			if text == "" {
				t.Fatalf("%s requirements must define %s", c, dim)
			}
		}
	}
}

func TestRequirementsDepthProportionalToRisk(t *testing.T) {
	// Higher classes demand strictly more: critical requires independent
	// verification and evidence; trivial requires none of that.
	triv, crit := Requirements(ClassTrivial), Requirements(ClassCritical)
	if containsFold(crit.Verification, "independent") == containsFold(triv.Verification, "independent") {
		t.Fatal("critical must demand independent verification beyond trivial")
	}
	if !containsFold(crit.Completion, "evidence") {
		t.Fatal("critical completion must name evidence")
	}
	if containsFold(triv.Execution, "verifier") {
		t.Fatal("trivial must not require a verifier")
	}
	// Standard sits between: gauntlet yes, independent verifier no.
	std := Requirements(ClassStandard)
	if !containsFold(std.Verification, "gauntlet") {
		t.Fatal("standard verification must name the gauntlet")
	}
	if containsFold(std.Verification, "independent") {
		t.Fatal("standard must not require an independent verifier")
	}
}
