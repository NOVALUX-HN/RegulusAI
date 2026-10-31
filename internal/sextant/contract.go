// Structured task contracts (Phase 2, S2-S3): the chart's ## Contract
// section parsed and validated. Absent sections never violate (legacy
// charts keep working); present sections must be complete for their class,
// reference existing acceptance scenarios, and agree with the Class line.
package sextant

import (
	"fmt"
	"regexp"
	"strings"
)

// Contract is one chart's structured task contract.
type Contract struct {
	Present         bool
	SectionLine     int
	Class           Class
	Objective       string
	Scope           string
	Constraints     string
	Acceptance      string
	AcceptanceNames []string
	Artifacts       string
	Dependencies    string
	Risk            string
	Verification    string
	Completion      string
	Verifier        string
}

// Known contract keys; anything else is rejected to keep contracts
// machine-readable.
var contractKeys = map[string]bool{
	"Objective": true, "Scope": true, "Constraints": true,
	"Acceptance": true, "Artifacts": true, "Dependencies": true,
	"Risk": true, "Verification": true, "Completion": true,
	"Verifier": true,
}

var classLineRe = regexp.MustCompile(`(?m)^Class:\s*(\S+)\s*$`)
var contractHeadingRe = regexp.MustCompile(`(?m)^## Contract\s*$`)
var acceptanceHeadingRe = regexp.MustCompile(`(?m)^## Acceptance\s*$`)
var scenarioHeadingRe = regexp.MustCompile(`(?m)^###\s+(?:Scenario:\s*)?(.+?)\s*$`)
var bulletRe = regexp.MustCompile(`^-\s*([A-Za-z]+):\s*(.*)$`)

// ParseClassLine reads the top-level `Class: <name>` line (the `Domain:`
// convention). It reports false when absent or invalid.
func ParseClassLine(doc string) (Class, bool) {
	m := classLineRe.FindStringSubmatch(doc)
	if m == nil {
		return "", false
	}
	c, err := ParseClass(strings.TrimSpace(m[1]))
	if err != nil {
		return "", false
	}
	return c, true
}

// ParseContract parses and validates the ## Contract section. A missing
// section yields Present=false with no violations.
func ParseContract(doc string) (Contract, []string) {
	var ct Contract
	var violations []string
	add := func(line int, format string, args ...any) {
		violations = append(violations, fmt.Sprintf("line %d: "+format, append([]any{line}, args...)...))
	}
	loc := contractHeadingRe.FindStringIndex(doc)
	if loc == nil {
		return ct, nil
	}
	ct.Present = true
	ct.SectionLine = lineOf(doc, loc[0])
	class, ok := ParseClassLine(doc)
	if !ok {
		add(ct.SectionLine, "contract requires a valid top-level Class: line")
		class = ClassStandard
	}
	ct.Class = class
	end := len(doc)
	if m := acceptanceHeadingRe.FindStringIndex(doc); m != nil && m[0] > loc[0] {
		end = m[0]
	}
	fields := map[string]string{}
	fieldLine := map[string]int{}
	for i, line := range strings.Split(doc[loc[0]:end], "\n") {
		ln := ct.SectionLine + i
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "## ") {
			continue
		}
		m := bulletRe.FindStringSubmatch(t)
		if m == nil {
			continue
		}
		key := m[1]
		if !contractKeys[key] {
			add(ln, "unknown contract key %q (want Objective, Scope, Constraints, Acceptance, Artifacts, Dependencies, Risk, Verification, Completion, Verifier)", key)
			continue
		}
		fields[key] = strings.TrimSpace(m[2])
		fieldLine[key] = ln
	}
	set := func(key string, dst *string) { *dst = fields[key] }
	set("Objective", &ct.Objective)
	set("Scope", &ct.Scope)
	set("Constraints", &ct.Constraints)
	set("Acceptance", &ct.Acceptance)
	set("Artifacts", &ct.Artifacts)
	set("Dependencies", &ct.Dependencies)
	set("Risk", &ct.Risk)
	set("Verification", &ct.Verification)
	set("Completion", &ct.Completion)
	set("Verifier", &ct.Verifier)
	required := []string{"Objective"}
	switch class {
	case ClassStandard:
		required = append(required, "Scope", "Acceptance", "Verification")
	case ClassComplex:
		required = append(required, "Scope", "Acceptance", "Verification", "Artifacts", "Dependencies")
	case ClassCritical:
		required = append(required, "Scope", "Acceptance", "Verification", "Artifacts", "Dependencies", "Constraints", "Verifier", "Completion")
	case ClassTrivial:
	}
	for _, key := range required {
		if strings.TrimSpace(fields[key]) == "" {
			add(ct.SectionLine, "class %s requires contract key %q", class, key)
		}
	}
	ct.AcceptanceNames = splitNames(ct.Acceptance)
	scenarios := scenarioNames(doc)
	if strings.TrimSpace(ct.Acceptance) != "" {
		if len(ct.AcceptanceNames) == 1 && strings.EqualFold(ct.AcceptanceNames[0], "all") {
			if len(scenarios) == 0 {
				add(ct.SectionLine, "contract claims Acceptance: all but ## Acceptance names no scenarios")
			}
		} else {
			for _, name := range ct.AcceptanceNames {
				if !hasScenario(scenarios, name) {
					add(fieldLineOr(fieldLine, "Acceptance", ct.SectionLine), "contract Acceptance names %q, which no ## Acceptance scenario declares", name)
				}
			}
		}
	} else if len(scenarios) > 0 && ct.Present {
		add(ct.SectionLine, "chart declares %d acceptance scenario(s) but the contract names none", len(scenarios))
	}
	if strings.TrimSpace(ct.Risk) != "" {
		want := map[string]Class{"trivial": ClassTrivial, "standard": ClassStandard, "complex": ClassComplex, "critical": ClassCritical}
		if wantClass, known := want[strings.ToLower(strings.TrimSpace(ct.Risk))]; known {
			if wantClass != class {
				add(fieldLineOr(fieldLine, "Risk", ct.SectionLine), "contract Risk %q disagrees with Class %s", ct.Risk, class)
			}
		} else {
			add(fieldLineOr(fieldLine, "Risk", ct.SectionLine), "contract Risk %q must be trivial, standard, complex or critical", ct.Risk)
		}
	}
	return ct, violations
}

func fieldLineOr(m map[string]int, key string, fallback int) int {
	if ln, ok := m[key]; ok {
		return ln
	}
	return fallback
}

func lineOf(doc string, offset int) int {
	return strings.Count(doc[:offset], "\n") + 1
}

func splitNames(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func scenarioNames(doc string) []string {
	m := acceptanceHeadingRe.FindStringIndex(doc)
	if m == nil {
		return nil
	}
	rest := doc[m[0]:]
	// Cut at the next top-level section (## Tasks, ## Log, ...).
	if next := regexp.MustCompile(`(?m)^## (Tasks|Log|Contract)\s*$`).FindStringIndex(rest); next != nil {
		rest = rest[:next[0]]
	}
	var out []string
	for _, sm := range scenarioHeadingRe.FindAllStringSubmatch(rest, -1) {
		out = append(out, strings.TrimSpace(sm[1]))
	}
	return out
}

func hasScenario(scenarios []string, name string) bool {
	for _, s := range scenarios {
		if strings.EqualFold(s, name) {
			return true
		}
	}
	return false
}
