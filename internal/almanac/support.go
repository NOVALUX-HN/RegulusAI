package almanac

import (
	"regulus-ai/internal/evidence"
)

// Support states for one observation's cited provenance: supported
// (cited receipts exist and verify), rot (a cited receipt fails
// verification), unchecked (nothing cited, nothing to check, or the
// store is unreadable). Fail-open everywhere except a proven failure:
// an unverifiable store must never accuse. Engram ranks by vector
// similarity plus model judgments; Almanac grades by support —
// evidence over opinion.
const (
	SupportOK        = "supported"
	SupportRot       = "rot"
	SupportUnchecked = "unchecked"
)

// SupportOf grades one observation's cited support, sharing the
// evidence.CitedRot semantics: only a cited failure rots.
func SupportOf(root string, o Observation) string {
	if o.Provenance == nil {
		return SupportUnchecked
	}
	rot, checked := evidence.CitedRot(root, o.Provenance.Task, o.Provenance.Receipts)
	if !checked {
		return SupportUnchecked
	}
	if rot {
		return SupportRot
	}
	return SupportOK
}
