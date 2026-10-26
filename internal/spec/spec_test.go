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
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- And another given\n- When b\n- Then c\n- And another then\n"
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
