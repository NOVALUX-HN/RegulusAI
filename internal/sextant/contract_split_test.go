package sextant

// G4 SPLIT pins: the helpers extracted from ParseContract preserve its
// exact verdicts (same messages, same line numbers, last duplicate wins).

import (
	"strings"
	"testing"
)

func TestCollectContractFieldsDuplicateLastWins(t *testing.T) {
	doc := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: first\n- Objective: second\n\n## Acceptance\n"
	add := func(int, string, ...any) {}
	fields, fieldLine := collectContractFields(doc, strings.Index(doc, "## Contract"), len(doc), 5, add)
	if fields["Objective"] != "second" {
		t.Errorf("duplicate Objective = %q, want last value %q", fields["Objective"], "second")
	}
	// Both bullets sit on lines 7 and 8 of the doc; the recorded line
	// must be the second occurrence.
	if fieldLine["Objective"] != 8 {
		t.Errorf("duplicate Objective line = %d, want 8", fieldLine["Objective"])
	}
}

func TestCollectContractFieldsUnknownKeyReportsLine(t *testing.T) {
	doc := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n- Magic: z\n\n## Acceptance\n"
	var lines []int
	add := func(line int, _ string, _ ...any) { lines = append(lines, line) }
	fields, _ := collectContractFields(doc, strings.Index(doc, "## Contract"), len(doc), 5, add)
	if _, ok := fields["Magic"]; ok {
		t.Error("unknown key must not land in fields")
	}
	if len(lines) != 1 || lines[0] != 8 {
		t.Errorf("unknown key must violate once at line 8, got %v", lines)
	}
}

func TestRequireContractClassDefaultsStandard(t *testing.T) {
	var got []string
	add := func(line int, format string, args ...any) {
		got = append(got, format)
		_ = line
		_ = args
	}
	if class := requireContractClass("# No class here\n", 3, add); class != ClassStandard {
		t.Errorf("missing Class line must default STANDARD, got %v", class)
	}
	if len(got) != 1 {
		t.Fatalf("missing Class line must violate once, got %v", got)
	}
}

func TestContractSectionEndWithoutAcceptance(t *testing.T) {
	doc := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n"
	start := strings.Index(doc, "## Contract")
	if end := contractSectionEnd(doc, start); end != len(doc) {
		t.Errorf("section end without ## Acceptance = %d, want len(doc) %d", end, len(doc))
	}
}

func TestParseContractSplitParityOnViolations(t *testing.T) {
	doc := "# F\n\nClass: CRITICAL\n\n## Contract\n\n- Objective: x\n- Risk: standard\n- Magic: z\n\n## Acceptance\n\n### Scenario: s\n\n- Given a ready fixture\n- When the checker runs\n- Then it passes\n"
	_, violations := ParseContract(doc)
	joined := strings.Join(violations, "\n")
	for _, want := range []string{"Magic", "Risk", "Constraints", "Verifier", "Completion"} {
		if !strings.Contains(joined, want) {
			t.Errorf("split ParseContract must still flag %q, got:\n%s", want, joined)
		}
	}
}
