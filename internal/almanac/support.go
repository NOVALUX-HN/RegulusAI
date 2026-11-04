package almanac

import (
	"path/filepath"
	"strings"

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

// SupportOf grades one observation's cited support. Receipt references
// match by cleaned path or by base name: provenance cites absolute
// paths, task-relative paths and bare ids across writers, while
// verification reports root-joined paths — the dual match keeps both
// honest without forcing one canonical form.
func SupportOf(root string, o Observation) string {
	if o.Provenance == nil || strings.TrimSpace(o.Provenance.Task) == "" {
		return SupportUnchecked
	}
	if len(o.Provenance.Receipts) == 0 {
		return SupportUnchecked
	}
	taskDir, err := evidence.TaskDirSafe(root, o.Provenance.Task)
	if err != nil {
		return SupportUnchecked
	}
	files, err := filepath.Glob(filepath.Join(taskDir, "*.json"))
	if err != nil || len(files) == 0 {
		return SupportUnchecked
	}
	rep, err := evidence.Verify(root, o.Provenance.Task, false)
	if err != nil {
		return SupportUnchecked
	}
	if len(rep.Failed) == 0 {
		return SupportOK
	}
	failed := map[string]bool{}
	for _, f := range rep.Failed {
		failed[f.File] = true
		failed[filepath.Base(f.File)] = true
	}
	for _, r := range o.Provenance.Receipts {
		if failed[r] || failed[filepath.Base(r)] {
			return SupportRot
		}
	}
	return SupportOK
}
