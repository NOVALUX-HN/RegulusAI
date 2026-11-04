package evolve

import (
	"sort"
)

// Lesson verdicts for `evolve validate`: keep (evidence verifies),
// drop (evidence rot — proposed, the guardian still disposes),
// unreviewable (nothing verifiable cited — kept by default, never
// accused). Cost is reported alongside, never judged: a costly lesson
// that pays is fine, a cheap one with rot support is not.
const (
	VerdictKeep         = "keep"
	VerdictDrop         = "drop"
	VerdictUnreviewable = "unreviewable"
)

// LessonVerdict is one lesson's deterministic verdict plus the
// reason a human or script needs to act on it.
type LessonVerdict struct {
	ID          string
	Domain      string
	Cue         string
	Verdict     string
	Reason      string
	ProxyTokens int64
}

// Validate renders verdicts over admitted lessons (one domain, one
// lesson id, or everything when both are empty). Read-only: it
// proposes nothing and retires nothing. An unknown lesson id yields
// no verdicts, never an error — absence is not failure.
func Validate(root, domain, lessonID string) ([]LessonVerdict, error) {
	rep, err := BuildReport(root, domain)
	if err != nil {
		return nil, err
	}
	var out []LessonVerdict
	for _, l := range rep.Lessons {
		if lessonID != "" && l.ID != lessonID {
			continue
		}
		v := LessonVerdict{ID: l.ID, Domain: l.Domain, Cue: l.Cue, ProxyTokens: l.ProxyTokens}
		switch l.Evidence {
		case EvidenceRot:
			v.Verdict = VerdictDrop
			v.Reason = "cites receipts that no longer verify — propose retirement, guardian disposes"
		case EvidenceOK:
			v.Verdict = VerdictKeep
			v.Reason = "cited evidence verifies"
		default:
			v.Verdict = VerdictUnreviewable
			v.Reason = "no verifiable evidence cited — kept by default"
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].Cue < out[j].Cue
	})
	return out, nil
}
