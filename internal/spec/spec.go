// Package spec validates the acceptance-criteria contract of ODD feature
// documents (PRD R-G1): every authorized change declares Gherkin-style
// acceptance criteria before implementation begins, and the human reviews
// these — not diffs.
package spec

import (
	"fmt"
	"strings"
)

// Check validates the Acceptance section of a feature document.
//
// It returns the number of scenarios found and every violation, each citing
// its line number. The document passes the contract when no violation was
// reported. All violations are reported at once, never first-fail.
func Check(doc string) (scenarios int, errs []string) {
	type scenario struct {
		name         string
		line         int
		given        int
		when         int
		then         int
		lastStepType string
	}

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
		if cur.given == 0 {
			violations = append(violations, fmt.Sprintf("line %d: scenario %q: missing Given step", cur.line, cur.name))
		}
		if cur.when == 0 {
			violations = append(violations, fmt.Sprintf("line %d: scenario %q: missing When step", cur.line, cur.name))
		}
		if cur.then == 0 {
			violations = append(violations, fmt.Sprintf("line %d: scenario %q: missing Then step", cur.line, cur.name))
		}
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
			rest := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			if !strings.HasPrefix(rest, "Scenario:") {
				violations = append(violations, fmt.Sprintf("line %d: only Scenario headings are allowed in the Acceptance section", ln))
				continue
			}
			name := strings.TrimSpace(strings.TrimPrefix(rest, "Scenario:"))
			if name == "" {
				violations = append(violations, fmt.Sprintf("line %d: scenario heading needs a name", ln))
				continue
			}
			cur = &scenario{name: name, line: ln}
			scenCount++
		case strings.HasPrefix(line, "- "):
			body := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if cur == nil {
				violations = append(violations, fmt.Sprintf("line %d: step outside a scenario", ln))
				continue
			}
			var stepType string
			switch {
			case strings.HasPrefix(body, "Given "):
				stepType = "Given"
			case strings.HasPrefix(body, "When "):
				stepType = "When"
			case strings.HasPrefix(body, "Then "):
				stepType = "Then"
			case body == "Given" || body == "When" || body == "Then":
				violations = append(violations, fmt.Sprintf("line %d: %s step needs text", ln, body))
				continue
			case strings.HasPrefix(body, "And "):
				if cur.lastStepType == "" {
					violations = append(violations, fmt.Sprintf("line %d: And step has no preceding Given/When/Then step", ln))
					continue
				}
				stepType = cur.lastStepType
			default:
				violations = append(violations, fmt.Sprintf("line %d: unrecognized step (want Given/When/Then/And): %s", ln, truncate(body, 40)))
				continue
			}
			switch stepType {
			case "Given":
				cur.given++
			case "When":
				cur.when++
			case "Then":
				cur.then++
			}
			cur.lastStepType = stepType
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
