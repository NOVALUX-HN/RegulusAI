package evolve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	ID          string `json:"id"`
	Domain      string `json:"domain"`
	Cue         string `json:"cue"`
	ProxyTokens int64  `json:"proxy_tokens"`
	Evidence    string `json:"evidence"`
}

// Report is the lesson economics of the store (one domain, or all
// when domain is ""): per-lesson rows in stable order (domain, cue),
// totals, and retire lines for rot lessons. Rot proposes retirement;
// the guardian still disposes.
type Report struct {
	Domain     string      `json:"domain"`
	Lessons    []LessonRow `json:"lessons"`
	Admitted   int         `json:"admitted"`
	TotalProxy int64       `json:"total_proxy"`
	Rot        int         `json:"rot"`
	Retire     []string    `json:"retire"`
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

// lessonEvidence grades one lesson's cited support, sharing the
// evidence.CitedRot semantics with almanac SupportOf: only a cited
// failure rots.
func lessonEvidence(root string, l library.Lesson) string {
	rot, checked := evidence.CitedRot(root, l.Task, l.Evidence)
	if !checked {
		return EvidenceUnchecked
	}
	if rot {
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

// ReportRow is one lesson's evolution across two reports: proxy
// delta, evidence-state change, and added/dropped membership. Zero
// values on one side mean the lesson lives only on the other.
type ReportRow struct {
	ID           string `json:"id"`
	Domain       string `json:"domain"`
	Cue          string `json:"cue"`
	BaseProxy    int64  `json:"base_proxy"`
	CandProxy    int64  `json:"cand_proxy"`
	DeltaProxy   int64  `json:"delta_proxy"`
	BaseEvidence string `json:"base_evidence"`
	CandEvidence string `json:"cand_evidence"`
	Added        bool   `json:"added"`
	Dropped      bool   `json:"dropped"`
}

// ReportComparison is the machine verdict over two reports: rows in
// stable order (domain, cue) plus the totals that tell whether the
// store got cheaper and cleaner or costlier and deader.
type ReportComparison struct {
	Rows          []ReportRow `json:"rows"`
	DeltaProxy    int64       `json:"delta_proxy"`
	DeltaAdmitted int         `json:"delta_admitted"`
}

// CompareReports contrasts a candidate report against a baseline,
// row by row. Pure: both sides are values, the store is untouched.
func CompareReports(base, cand Report) ReportComparison {
	byID := map[string]*ReportRow{}
	var order []string
	get := func(id string) *ReportRow {
		if r, ok := byID[id]; ok {
			return r
		}
		r := &ReportRow{ID: id}
		byID[id] = r
		order = append(order, id)
		return r
	}
	for _, l := range base.Lessons {
		r := get(l.ID)
		r.Domain, r.Cue, r.BaseProxy, r.BaseEvidence = l.Domain, l.Cue, l.ProxyTokens, l.Evidence
	}
	for _, l := range cand.Lessons {
		r := get(l.ID)
		r.Domain, r.Cue, r.CandProxy, r.CandEvidence = l.Domain, l.Cue, l.ProxyTokens, l.Evidence
	}
	c := ReportComparison{}
	for _, id := range order {
		r := byID[id]
		r.DeltaProxy = r.CandProxy - r.BaseProxy
		r.Added = !hasLesson(base, id) && hasLesson(cand, id)
		r.Dropped = hasLesson(base, id) && !hasLesson(cand, id)
		c.Rows = append(c.Rows, *r)
	}
	sort.Slice(c.Rows, func(i, j int) bool {
		if c.Rows[i].Domain != c.Rows[j].Domain {
			return c.Rows[i].Domain < c.Rows[j].Domain
		}
		return c.Rows[i].Cue < c.Rows[j].Cue
	})
	c.DeltaProxy = cand.TotalProxy - base.TotalProxy
	c.DeltaAdmitted = cand.Admitted - base.Admitted
	return c
}

func hasLesson(r Report, id string) bool {
	for _, l := range r.Lessons {
		if l.ID == id {
			return true
		}
	}
	return false
}

// SaveReport writes one report as indented JSON (temp+rename: a torn
// snapshot is worse than none). Snapshots compound economics over
// time: pin today, compare tomorrow.
func SaveReport(path string, rep Report) error {
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "report-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// LoadReport reads one report written by SaveReport.
func LoadReport(path string) (Report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var rep Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		return Report{}, fmt.Errorf("%s: malformed evolution report: %v", path, err)
	}
	return rep, nil
}
