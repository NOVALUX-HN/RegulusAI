package evolve

import (
	"sort"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/library"
)

// Lesson evidence states for the evolution report: ok (task receipts
// verify), rot (they fail — the lesson's support is gone), unchecked
// (no task to check, or the store is unreadable — fail-open, never
// accused).
const (
	EvidenceOK        = "ok"
	EvidenceRot       = "rot"
	EvidenceUnchecked = "unchecked"
)

// LessonRow is one admitted lesson's economics: its proxy-token price
// (chars/4 over the rendered cue+lesson+evidence — what it adds to
// every context assembly that injects it) and its evidence state.
type LessonRow struct {
	ID          string
	Domain      string
	Cue         string
	ProxyTokens int64
	Evidence    string
}

// Report is the lesson economics of the store (one domain, or all
// when domain is ""): per-lesson rows in stable order (domain, cue),
// totals, and retire lines for rot lessons. Rot proposes retirement;
// the guardian still disposes.
type Report struct {
	Domain     string
	Lessons    []LessonRow
	Admitted   int
	TotalProxy int64
	Rot        int
	Retire     []string
}

// lessonProxy prices one lesson the way the context prices text:
// chars/4 rounded up over cue+lesson+evidence. Same convention as
// trace.EstimateLocalTokens and the B11 golden.
func lessonProxy(l library.Lesson) int64 {
	n := int64(len(l.Cue) + len(l.Lesson))
	for _, e := range l.Evidence {
		n += int64(len(e))
	}
	if n <= 0 {
		return 0
	}
	return (n + 3) / 4
}

// lessonEvidence checks one lesson's support: lessons without a task
// cannot be checked; an unreadable store fails open to unchecked; only
// failed receipt verification reads as rot.
func lessonEvidence(root string, l library.Lesson) string {
	if l.Task == "" {
		return EvidenceUnchecked
	}
	rep, err := evidence.Verify(root, l.Task, false)
	if err != nil {
		return EvidenceUnchecked
	}
	if len(rep.Failed) > 0 {
		return EvidenceRot
	}
	return EvidenceOK
}

// Report renders the economics over the store's admitted lessons. A
// store with no library reads as an empty report, never an error: a
// fresh repo has nothing to economize yet.
func BuildReport(root, domain string) (Report, error) {
	rep := Report{Domain: domain}
	domains, err := library.Domains(root)
	if err != nil {
		return rep, nil
	}
	for _, d := range domains {
		if domain != "" && d != domain {
			continue
		}
		lessons, err := library.List(root, d)
		if err != nil {
			continue
		}
		for _, l := range lessons {
			ev := lessonEvidence(root, l)
			row := LessonRow{
				ID: l.ID, Domain: d, Cue: l.Cue,
				ProxyTokens: lessonProxy(l), Evidence: ev,
			}
			rep.Lessons = append(rep.Lessons, row)
			rep.Admitted++
			rep.TotalProxy += row.ProxyTokens
			if ev == EvidenceRot {
				rep.Rot++
				rep.Retire = append(rep.Retire, l.ID)
			}
		}
	}
	sort.Slice(rep.Lessons, func(i, j int) bool {
		if rep.Lessons[i].Domain != rep.Lessons[j].Domain {
			return rep.Lessons[i].Domain < rep.Lessons[j].Domain
		}
		return rep.Lessons[i].Cue < rep.Lessons[j].Cue
	})
	return rep, nil
}
