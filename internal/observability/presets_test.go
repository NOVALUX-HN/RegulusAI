package observability

// NOV-39 (S14): context presets pin one ceiling per task class —
// trivial 10/10, standard 20/20 (the almanac default), complex 40/40,
// critical full-store with a required justification. Unknown classes
// fail closed: no silent full-store resume.

import (
	"strings"
	"testing"
)

func TestContextPresetsPinCeilings(t *testing.T) {
	cases := []struct {
		class               ContextClass
		limit, budget       int
		full, justification bool
	}{
		{ClassTrivial, 10, 10, false, false},
		{ClassStandard, 20, 20, false, false},
		{ClassComplex, 40, 40, false, false},
		{ClassCritical, 0, 0, true, true},
	}
	for _, tc := range cases {
		p, err := ContextPresetFor(tc.class)
		if err != nil {
			t.Fatalf("ContextPresetFor(%q): %v", tc.class, err)
		}
		if p.Limit != tc.limit || p.Budget != tc.budget {
			t.Errorf("class %q ceiling = %d/%d, want %d/%d", tc.class, p.Limit, p.Budget, tc.limit, tc.budget)
		}
		if p.Full != tc.full || p.RequiresJustification != tc.justification {
			t.Errorf("class %q full=%v justification=%v, want full=%v justification=%v",
				tc.class, p.Full, p.RequiresJustification, tc.full, tc.justification)
		}
		if strings.TrimSpace(p.Escalation) == "" {
			t.Errorf("class %q must document its escalation path", tc.class)
		}
	}
}

func TestContextPresetStandardMirrorsAlmanacDefault(t *testing.T) {
	p, err := ContextPresetFor(ClassStandard)
	if err != nil {
		t.Fatalf("ContextPresetFor(standard): %v", err)
	}
	// The standard ceiling IS the bounded default every restart
	// injects (almanac DefaultContextLimit/DefaultContextBudget 20/20):
	// 20/20 pinned literally so drift fails here, never silently.
	if p.Limit != 20 || p.Budget != 20 {
		t.Fatalf("standard must mirror the 20/20 almanac default, got %d/%d", p.Limit, p.Budget)
	}
}

func TestContextPresetUnknownFailsClosed(t *testing.T) {
	if _, err := ContextPresetFor("ultra"); err == nil {
		t.Fatal("unknown class must fail closed, got nil")
	} else if !strings.Contains(err.Error(), "ultra") {
		t.Fatalf("unknown-class error must name the class, got: %v", err)
	}
	if _, err := ContextPresetFor(""); err == nil {
		t.Fatal("empty class must fail closed, got nil")
	}
}

func TestContextPresetEscalationNamesNextStep(t *testing.T) {
	next := map[ContextClass]string{
		ClassTrivial:  "standard",
		ClassStandard: "complex",
		ClassComplex:  "critical",
		ClassCritical: "full",
	}
	for class, want := range next {
		p, err := ContextPresetFor(class)
		if err != nil {
			t.Fatalf("ContextPresetFor(%q): %v", class, err)
		}
		if !strings.Contains(strings.ToLower(p.Escalation), want) {
			t.Errorf("class %q escalation must name %q, got %q", class, want, p.Escalation)
		}
	}
}
