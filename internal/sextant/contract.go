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

// requireContractClass reads the top-level Class line a contract
// requires. A missing or invalid line violates and defaults to STANDARD.
func requireContractClass(doc string, sectionLine int, add func(int, string, ...any)) Class {
	class, ok := ParseClassLine(doc)
	if !ok {
		add(sectionLine, "contract requires a valid top-level Class: line")
		class = ClassStandard
	}
	return class
}

// contractSectionEnd bounds the ## Contract section: the next
// ## Acceptance heading past the section start, or end of document.
func contractSectionEnd(doc string, start int) int {
	end := len(doc)
	if m := acceptanceHeadingRe.FindStringIndex(doc); m != nil && m[0] > start {
		end = m[0]
	}
	return end
}

// collectContractFields parses the "- Key: value" bullets of the contract
// section, rejecting unknown keys. Later duplicates win, as before.
func collectContractFields(doc string, start, end, base int, add func(int, string, ...any)) (map[string]string, map[string]int) {
	fields := map[string]string{}
	fieldLine := map[string]int{}
	for i, line := range strings.Split(doc[start:end], "\n") {
		ln := base + i
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
	return fields, fieldLine
}

// assignContractFields copies the parsed bullets onto the contract.
func assignContractFields(ct *Contract, fields map[string]string) {
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
}

// checkContractRequired enforces the per-class mandatory keys.
func checkContractRequired(ct Contract, fields map[string]string, add func(int, string, ...any)) {
	required := []string{"Objective"}
	switch ct.Class {
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
			add(ct.SectionLine, "class %s requires contract key %q", ct.Class, key)
		}
	}
}

// checkContractAcceptance records the acceptance names and checks them
// against the scenarios the ## Acceptance section declares.
func checkContractAcceptance(doc string, ct *Contract, fieldLine map[string]int, add func(int, string, ...any)) {
	ct.AcceptanceNames = splitNames(ct.Acceptance)
	scenarios := scenarioNames(doc)
	if strings.TrimSpace(ct.Acceptance) == "" {
		if len(scenarios) > 0 && ct.Present {
			add(ct.SectionLine, "chart declares %d acceptance scenario(s) but the contract names none", len(scenarios))
		}
		return
	}
	if len(ct.AcceptanceNames) == 1 && strings.EqualFold(ct.AcceptanceNames[0], "all") {
		if len(scenarios) == 0 {
			add(ct.SectionLine, "contract claims Acceptance: all but ## Acceptance names no scenarios")
		}
		return
	}
	for _, name := range ct.AcceptanceNames {
		if !hasScenario(scenarios, name) {
			add(fieldLineOr(fieldLine, "Acceptance", ct.SectionLine), "contract Acceptance names %q, which no ## Acceptance scenario declares", name)
		}
	}
}

// checkContractRisk enforces Risk/Class consistency when Risk is set.
func checkContractRisk(ct Contract, fieldLine map[string]int, add func(int, string, ...any)) {
	if strings.TrimSpace(ct.Risk) == "" {
		return
	}
	want := map[string]Class{"trivial": ClassTrivial, "standard": ClassStandard, "complex": ClassComplex, "critical": ClassCritical}
	if wantClass, known := want[strings.ToLower(strings.TrimSpace(ct.Risk))]; known {
		if wantClass != ct.Class {
			add(fieldLineOr(fieldLine, "Risk", ct.SectionLine), "contract Risk %q disagrees with Class %s", ct.Risk, ct.Class)
		}
	} else {
		add(fieldLineOr(fieldLine, "Risk", ct.SectionLine), "contract Risk %q must be trivial, standard, complex or critical", ct.Risk)
	}
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
	ct.Class = requireContractClass(doc, ct.SectionLine, add)
	end := contractSectionEnd(doc, loc[0])
	fields, fieldLine := collectContractFields(doc, loc[0], end, ct.SectionLine, add)
	assignContractFields(&ct, fields)
	checkContractRequired(ct, fields, add)
	checkContractAcceptance(doc, &ct, fieldLine, add)
	checkContractRisk(ct, fieldLine, add)
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
