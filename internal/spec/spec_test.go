package spec

import (
	"strings"
	"testing"
)

const validDoc = `# Feature X

Some context.

## Acceptance

### Scenario: gate runner exits cleanly

- Given a fixture project with all gates configured
- When regulus gate run executes
- Then the exit code is 0
- And a receipt is written under .regulus/evidence/

### Scenario: failing gate stops the gauntlet

- Given a fixture project with a failing typecheck gate
- When regulus gate run executes
- Then the exit code is 1 and no later gate runs

## Tasks

## Log
`

func TestCheckValidDocument(t *testing.T) {
	n, errs := Check(validDoc)
	if len(errs) != 0 {
		t.Fatalf("want no violations, got %v", errs)
	}
	if n != 2 {
		t.Fatalf("want 2 scenarios, got %d", n)
	}
}

func TestCheckMissingSection(t *testing.T) {
	_, errs := Check("# Feature\n\n## Tasks\n\nnothing here\n")
	if len(errs) != 1 || !strings.Contains(errs[0], "missing `## Acceptance` section") {
		t.Fatalf("want exactly the missing-section violation, got %v", errs)
	}
}

func TestCheckEmptySection(t *testing.T) {
	_, errs := Check("# F\n\n## Acceptance\n\n## Log\n")
	if !strings.Contains(strings.Join(errs, "\n"), "at least one scenario") {
		t.Fatalf("want at-least-one-scenario violation, got %v", errs)
	}
}

func TestCheckScenarioMissingThen(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: half\n\n- Given a fixture\n- When regulus runs\n\n## Log\n"
	_, errs := Check(doc)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "missing Then step") || !strings.Contains(joined, `"half"`) {
		t.Fatalf("want missing-Then violation naming the scenario, got %v", errs)
	}
	if !strings.Contains(joined, "line 5") {
		t.Fatalf("violation must cite the scenario header line, got %v", errs)
	}
}

func TestCheckReportsAllViolationsAtOnce(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: a\n\n- Given a fixture\n\n### Scenario: b\n\n- Then something\n\n## Log\n"
	_, errs := Check(doc)
	// a: missing When + missing Then; b: missing Given + missing When.
	if len(errs) < 4 {
		t.Fatalf("want every violation at once (>=4), got %v", errs)
	}
}

func TestCheckStepOutsideScenario(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n- Given floating step\n\n### Scenario: ok\n\n- Given a\n- When b\n- Then c\n"
	_, errs := Check(doc)
	if !strings.Contains(strings.Join(errs, "\n"), "outside a scenario") {
		t.Fatalf("want outside-scenario violation, got %v", errs)
	}
}

func TestCheckEmptyStepText(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given\n- When x\n- Then y\n"
	_, errs := Check(doc)
	if !strings.Contains(strings.Join(errs, "\n"), "needs text") {
		t.Fatalf("want needs-text violation, got %v", errs)
	}
}

func TestCheckAndWithoutPrecedingStep(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- And another thing\n- Given a\n- When b\n- Then c\n"
	_, errs := Check(doc)
	if !strings.Contains(strings.Join(errs, "\n"), "no preceding") {
		t.Fatalf("want And-without-preceding violation, got %v", errs)
	}
}

func TestCheckAndCountsAsPreviousType(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a seeded fixture\n- And another seeded precondition\n- When the checker runs\n- Then it passes\n- And another outcome holds\n"
	n, errs := Check(doc)
	if len(errs) != 0 || n != 1 {
		t.Fatalf("want one clean scenario (And continues the previous type), got %d scenarios, errs %v", n, errs)
	}
}

func TestCheckDuplicateSection(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n\n## Acceptance\n"
	_, errs := Check(doc)
	if !strings.Contains(strings.Join(errs, "\n"), "duplicate") {
		t.Fatalf("want duplicate-section violation, got %v", errs)
	}
}

func TestCheckNonScenarioHeading(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Notes\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n"
	_, errs := Check(doc)
	if !strings.Contains(strings.Join(errs, "\n"), "only Scenario headings") {
		t.Fatalf("want only-Scenario-headings violation, got %v", errs)
	}
}

// P1: one-word steps state no criterion (`Given a` is the anti-example
// from the chart). Each short step is rejected with its line, and a
// rejected step also reads as missing.
func TestCheckRejectsOneWordSteps(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: thin\n\n- Given a\n- When the check runs\n- Then it passes\n"
	_, errs := Check(doc)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "Given step needs at least 2 words") {
		t.Fatalf("want min-length violation for `Given a`, got %v", errs)
	}
	if !strings.Contains(joined, "line 7") {
		t.Fatalf("length violation must cite the step line, got %v", errs)
	}
	if !strings.Contains(joined, `scenario "thin"`) || !strings.Contains(joined, "missing Given step") {
		t.Fatalf("a rejected Given must also read as missing, got %v", errs)
	}
}

// P1: the two-word floor is inclusive — a step with exactly two words
// is a criterion and passes.
func TestCheckTwoWordBoundaryPasses(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When it runs\n- Then it passes\n"
	n, errs := Check(doc)
	if len(errs) != 0 || n != 1 {
		t.Fatalf("want one clean scenario at the two-word floor, got %d scenarios, errs %v", n, errs)
	}
}

// P1: a scenario must open with Given — a leading When or Then is out
// of Given→When→Then order.
func TestCheckRejectsFirstStepNotGiven(t *testing.T) {
	for _, first := range []string{"When the check runs", "Then it passes"} {
		doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- " + first + "\n- Given a fixture\n- Then it passes\n"
		_, errs := Check(doc)
		if !strings.Contains(strings.Join(errs, "\n"), "out of order (want Given→When→Then order)") {
			t.Fatalf("first step %q: want out-of-order violation, got %v", first, errs)
		}
	}
}

// P1: phases never step back — Then before When, or Given after the
// action started, is rejected with its line.
func TestCheckRejectsOutOfOrder(t *testing.T) {
	docs := []string{
		"# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- Then it passes\n- When the check runs\n",
		"# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When the check runs\n- Then it passes\n- Given another fixture\n",
		"# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When the check runs\n- Then it passes\n- When it runs again\n",
	}
	for i, doc := range docs {
		_, errs := Check(doc)
		joined := strings.Join(errs, "\n")
		if !strings.Contains(joined, "out of order (want Given→When→Then order)") {
			t.Fatalf("doc %d: want out-of-order violation, got %v", i, errs)
		}
		if !strings.Contains(joined, "line ") {
			t.Fatalf("doc %d: violation must cite its line, got %v", i, errs)
		}
	}
}

// P1: And continues the previous phase — And after Then is a Then, so a
// When after it steps back and is rejected.
func TestCheckRejectsAndBreakingOrder(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When the check runs\n- Then it passes\n- And another outcome holds\n- When it runs again\n"
	_, errs := Check(doc)
	if !strings.Contains(strings.Join(errs, "\n"), "out of order (want Given→When→Then order)") {
		t.Fatalf("want out-of-order violation after Then-phase And, got %v", errs)
	}
}

// P1: the length floor covers And steps too.
func TestCheckRejectsShortAndStep(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- And more\n- When the check runs\n- Then it passes\n"
	_, errs := Check(doc)
	if !strings.Contains(strings.Join(errs, "\n"), "And step needs at least 2 words") {
		t.Fatalf("want min-length violation for the And step, got %v", errs)
	}
}

// P1: full ordered phases with And continuations stay clean.
func TestCheckAcceptsOrderedMultiStep(t *testing.T) {
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- Given a second precondition\n- When the check runs\n- And the hook fires\n- Then it passes\n- Then the receipt lands\n"
	n, errs := Check(doc)
	if len(errs) != 0 || n != 1 {
		t.Fatalf("want one clean ordered scenario, got %d scenarios, errs %v", n, errs)
	}
}
