// Package spec validates the acceptance-criteria contract of ODD feature
// documents (PRD R-G1): every authorized change declares Gherkin-style
// acceptance criteria before implementation begins, and the human reviews
// these — not diffs.
package spec

import (
	"fmt"
	"strings"
)

// scenario is one acceptance scenario being scanned.
type scenario struct {
	name         string
	line         int
	given        int
	when         int
	then         int
	lastStepType string
}

// violations returns the missing-step violations of a closed scenario,
// each citing its header line.
func (s *scenario) violations() []string {
	var vs []string
	if s.given == 0 {
		vs = append(vs, fmt.Sprintf("line %d: scenario %q: missing Given step", s.line, s.name))
	}
	if s.when == 0 {
		vs = append(vs, fmt.Sprintf("line %d: scenario %q: missing When step", s.line, s.name))
	}
	if s.then == 0 {
		vs = append(vs, fmt.Sprintf("line %d: scenario %q: missing Then step", s.line, s.name))
	}
	return vs
}

// parseScenarioHeading classifies a "### " line inside the Acceptance
// section. It returns the scenario name and no violation for a named
// Scenario heading, or "" and a violation otherwise.
func parseScenarioHeading(ln int, line string) (name, violation string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "### "))
	if !strings.HasPrefix(rest, "Scenario:") {
		return "", fmt.Sprintf("line %d: only Scenario headings are allowed in the Acceptance section", ln)
	}
	name = strings.TrimSpace(strings.TrimPrefix(rest, "Scenario:"))
	if name == "" {
		return "", fmt.Sprintf("line %d: scenario heading needs a name", ln)
	}
	return name, ""
}

// parseStepLine classifies one step line ("- …") inside a scenario and
// updates its counters. It returns a violation when the step is invalid.
func parseStepLine(ln int, body string, sc *scenario) string {
	var stepType string
	switch {
	case strings.HasPrefix(body, "Given "):
		stepType = "Given"
	case strings.HasPrefix(body, "When "):
		stepType = "When"
	case strings.HasPrefix(body, "Then "):
		stepType = "Then"
	case body == "Given" || body == "When" || body == "Then":
		return fmt.Sprintf("line %d: %s step needs text", ln, body)
	case strings.HasPrefix(body, "And "):
		if sc.lastStepType == "" {
			return fmt.Sprintf("line %d: And step has no preceding Given/When/Then step", ln)
		}
		stepType = sc.lastStepType
	default:
		return fmt.Sprintf("line %d: unrecognized step (want Given/When/Then/And): %s", ln, truncate(body, 40))
	}
	switch stepType {
	case "Given":
		sc.given++
	case "When":
		sc.when++
	case "Then":
		sc.then++
	}
	sc.lastStepType = stepType
	return ""
}

// Check validates the Acceptance section of a feature document.
//
// It returns the number of scenarios found and every violation, each citing
// its line number. The document passes the contract when no violation was
// reported. All violations are reported at once, never first-fail.
func Check(doc string) (scenarios int, errs []string) {
	var (
		inSection   bool
		sectionLine int
		cur         *scenario
		scenCount   int
		violations  []string
	)

	closeScenario := func() {
		if cur == nil {
			return
		}
		violations = append(violations, cur.violations()...)
		cur = nil
	}

	for i, raw := range strings.Split(doc, "\n") {
		ln := i + 1
		line := strings.TrimSpace(raw)

		switch {
		case line == "## Acceptance":
			if inSection {
				violations = append(violations, fmt.Sprintf("line %d: duplicate Acceptance section", ln))
				continue
			}
			inSection = true
			sectionLine = ln
			continue
		case inSection && strings.HasPrefix(line, "## "):
			closeScenario()
			inSection = false
			continue
		}

		if !inSection {
			continue
		}

		switch {
		case strings.HasPrefix(line, "### "):
			closeScenario()
			name, violation := parseScenarioHeading(ln, line)
			if violation != "" {
				violations = append(violations, violation)
				continue
			}
			cur = &scenario{name: name, line: ln}
			scenCount++
		case strings.HasPrefix(line, "- "):
			if cur == nil {
				violations = append(violations, fmt.Sprintf("line %d: step outside a scenario", ln))
				continue
			}
			body := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if violation := parseStepLine(ln, body, cur); violation != "" {
				violations = append(violations, violation)
			}
		}
	}
	closeScenario()

	if sectionLine == 0 {
		violations = append(violations, "missing `## Acceptance` section")
		return 0, violations
	}
	if scenCount == 0 {
		violations = append(violations, fmt.Sprintf("line %d: the Acceptance section needs at least one scenario", sectionLine))
	}
	return scenCount, violations
}

// truncate keeps violation messages bounded.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
