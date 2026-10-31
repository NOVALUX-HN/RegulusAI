// Task classification (Phase 2, S1): depth proportional to risk.
package sextant

import (
	"fmt"
	"strings"
)

// Class is the task complexity level.
type Class string

// Task classes, lightest first.
const (
	ClassTrivial  Class = "TRIVIAL"
	ClassStandard Class = "STANDARD"
	ClassComplex  Class = "COMPLEX"
	ClassCritical Class = "CRITICAL"
)

// ParseClass validates a class name.
func ParseClass(s string) (Class, error) {
	switch Class(s) {
	case ClassTrivial, ClassStandard, ClassComplex, ClassCritical:
		return Class(s), nil
	default:
		return "", fmt.Errorf("unknown task class %q (want TRIVIAL, STANDARD, COMPLEX or CRITICAL)", s)
	}
}

// Signals are the charter-declared facts classification reads. They mirror
// the Phase 0 high-risk inventory: any risk marker escalates, and unclear
// risk escalates too — a trivial label must never bypass mandatory policy.
type Signals struct {
	TouchesCode      bool
	Destructive      bool
	Security         bool
	ContractConsumed bool
	Concurrency      bool
	Delivery         bool
	Units            int
	Dependencies     int
	UnclearRisk      bool
}

// Classify maps signals to the lightest class that still covers the risk.
func Classify(s Signals) Class {
	if s.Destructive || s.Security || s.UnclearRisk ||
		s.ContractConsumed || s.Concurrency || s.Delivery {
		return ClassCritical
	}
	if s.Units > 1 || s.Dependencies > 0 {
		return ClassComplex
	}
	if s.TouchesCode {
		return ClassStandard
	}
	return ClassTrivial
}

// LevelReq is the per-level process contract: what planning, specification,
// execution, verification, documentation and completion demand.
type LevelReq struct {
	Planning      string
	Specification string
	Execution     string
	Verification  string
	Documentation string
	Completion    string
}

// Requirements returns the process contract for one class.
func Requirements(c Class) LevelReq {
	switch c {
	case ClassCritical:
		return LevelReq{
			Planning:      "Chart + decomposed units with explicit dependencies; blast-radius note; rollback plan before writing code.",
			Specification: "Contract with acceptance scenarios (Given/When/Then), named verifier task, and prohibited outcomes alongside required ones.",
			Execution:     "One bounded unit per worker; destructive steps behind explicit confirmation; independent verifier reproduces the failure and the fix.",
			Verification:  "Full gauntlet + independent verifier receipts + evidence audit; every acceptance scenario PROVEN with receipt ids.",
			Documentation: "Chart Log narrates each unit; Almanac lesson admitted only through reflect with evidence.",
			Completion:    "Core close only: spec OK, task receipts PASS, verifier task receipts PASS — evidence, never self-report.",
		}
	case ClassComplex:
		return LevelReq{
			Planning:      "Chart with units and dependencies; scout map first when the ground is unknown.",
			Specification: "Contract with acceptance scenarios; dependencies named between tasks.",
			Execution:     "One bounded unit per worker; scout before building on unknown ground.",
			Verification:  "Full gauntlet per unit + evidence audit; scenarios PROVEN with receipt ids.",
			Documentation: "Chart Log per unit; session note at close.",
			Completion:    "Core close only: spec OK and task receipts PASS — evidence, never self-report.",
		}
	case ClassStandard:
		return LevelReq{
			Planning:      "Chart with one unit; no ceremony beyond the acceptance scenarios.",
			Specification: "Contract with at least one acceptance scenario.",
			Execution:     "Inline or one worker; gauntlet after each meaningful step.",
			Verification:  "Full gauntlet + evidence audit; scenarios PROVEN with receipt ids.",
			Documentation: "Chart Log entry at close.",
			Completion:    "Core close only: spec OK and task receipts PASS — evidence, never self-report.",
		}
	default:
		return LevelReq{
			Planning:      "None beyond the request itself; read-only work needs no chart.",
			Specification: "Acceptance stated in the request or reply; no chart required.",
			Execution:     "Inline; never delegate (a worker costs more than the work).",
			Verification:  "Author re-reads the outcome; gauntlet only if code changed (then it is not trivial).",
			Documentation: "None required; a session note only if future work depends on it.",
			Completion:    "Done when stated and shown; code changes reclassify to STANDARD or higher.",
		}
	}
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
