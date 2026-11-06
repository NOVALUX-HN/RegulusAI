package evolve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/library"
	"regulus-ai/internal/trace"
)

// Tick finding kinds: repeated tool errors across sessions seed
// micro-skill candidates; stale lessons (task evidence that no longer
// verifies) seed retirement review. Both propose — the guardian still
// owns every admission or retirement.
const (
	FindingRepeatedErrors = "repeated-errors"
	FindingStaleLesson    = "stale-lesson"
)

// Tick thresholds (documented, TE-shaped): a tool earns a candidate
// after TickMinErrors failures spread over TickMinErrorSessions
// sessions — one bad session is noise, a cross-session pattern is a
// fact about the repo. At most TickMaxFindings candidates per tick so
// one noisy weekend cannot flood the pending list.
const (
	TickMinErrors        = 3
	TickMinErrorSessions = 2
	TickMaxFindings      = 3
)

// TickDefaultEvery is the default session cadence: a tick fires once
// three traced sessions accumulate past the last tick (TE's k=3).
const TickDefaultEvery = 3

// TickDomain carries cross-cutting failure findings: they belong to
// how the repo behaves, not to one practice domain. Stale-lesson
// findings keep their lesson's domain.
const TickDomain = "harness"

// Finding is one tick proposal: a cue plus the lesson seed plus the
// evidence that justifies proposing it (session ids for error
// patterns, failed receipt files for stale lessons).
type Finding struct {
	Kind     string
	Domain   string
	Cue      string
	Lesson   string
	Evidence []string
	Sessions []string
}

// TickResult is one tick evaluation: whether the cadence fired, the
// session counts on each side of it, and the findings in stable order
// (repeated errors by tool, then stale lessons by domain).
type TickResult struct {
	Due      bool
	Sessions int
	AtTick   int
	Every    int
	Findings []Finding
}

// tickState is the persisted cadence cursor: the session count the
// last applied tick saw. Missing or corrupt state reads as zero (a
// fresh repo ticks on schedule, never refused for lack of history).
type tickState struct {
	SessionsAtTick int `json:"sessions_at_tick"`
}

func tickStateFile(root string) string {
	return filepath.Join(EvolveDir(root), "tick.json")
}

func loadTickState(root string) tickState {
	var st tickState
	raw, err := os.ReadFile(tickStateFile(root))
	if err != nil {
		return st
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return tickState{}
	}
	if st.SessionsAtTick < 0 {
		st.SessionsAtTick = 0
	}
	return st
}

// TickPosition reports the tick cadence cursor: traced sessions now
// versus at the last applied tick. Exported for read-only surfaces
// (dashboard); the cadence itself stays inside ComputeTick/ApplyTick.
func TickPosition(root string) (sessions, atTick int) {
	st := loadTickState(root)
	sessions, _ = trace.UsageSessions(root)
	return sessions, st.SessionsAtTick
}

func saveTickState(root string, st tickState) error {
	if err := os.MkdirAll(EvolveDir(root), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(EvolveDir(root), "tick-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, tickStateFile(root))
}

// ComputeTick evaluates the tick cadence without writing anything:
// sessions traced past the last applied tick decide Due; findings
// mine the traces (repeated errors) and the admitted lessons
// (evidence rot). Deterministic from store contents alone.
func ComputeTick(root string, every int) (TickResult, error) {
	if every <= 0 {
		every = TickDefaultEvery
	}
	st := loadTickState(root)
	sessions, err := trace.UsageSessions(root)
	if err != nil {
		return TickResult{}, err
	}
	res := TickResult{Sessions: sessions, AtTick: st.SessionsAtTick, Every: every}
	if sessions-st.SessionsAtTick < every {
		return res, nil
	}
	res.Due = true
	for _, f := range repeatedErrorFindings(root) {
		res.Findings = append(res.Findings, f)
		if len(res.Findings) >= TickMaxFindings {
			return res, nil
		}
	}
	for _, f := range staleLessonFindings(root) {
		res.Findings = append(res.Findings, f)
		if len(res.Findings) >= TickMaxFindings {
			return res, nil
		}
	}
	return res, nil
}

// repeatedErrorFindings groups error-status trace entries by tool: a
// tool failing TickMinErrors times across TickMinErrorSessions
// sessions is a repo fact worth a micro-skill seed. Sorted by tool so
// the finding order is stable.
func repeatedErrorFindings(root string) []Finding {
	sessions, err := trace.Sessions(root)
	if err != nil {
		return nil
	}
	type acc struct {
		errors int
		sess   map[string]bool
	}
	byTool := map[string]*acc{}
	for _, s := range sessions {
		entries, err := trace.List(root, s)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Status != trace.StatusError {
				continue
			}
			a, ok := byTool[e.Tool]
			if !ok {
				a = &acc{sess: map[string]bool{}}
				byTool[e.Tool] = a
			}
			a.errors++
			a.sess[s] = true
		}
	}
	var tools []string
	for tool, a := range byTool {
		if a.errors >= TickMinErrors && len(a.sess) >= TickMinErrorSessions {
			tools = append(tools, tool)
		}
	}
	sort.Strings(tools)
	var out []Finding
	for _, tool := range tools {
		a := byTool[tool]
		var sess []string
		for s := range a.sess {
			sess = append(sess, s)
		}
		sort.Strings(sess)
		out = append(out, Finding{
			Kind:     FindingRepeatedErrors,
			Domain:   TickDomain,
			Cue:      fmt.Sprintf("tool %s fails repeatedly", tool),
			Lesson:   fmt.Sprintf("Tool %s failed %d times across %d sessions — check its preconditions (or the repo facts it assumes) before retrying it the same way.", tool, a.errors, len(sess)),
			Evidence: sess,
			Sessions: sess,
		})
	}
	return out
}

// staleLessonFindings reviews admitted lessons against their CITED
// task evidence (shared evidence.CitedRot semantics): a lesson only
// seeds a retirement review when a receipt it cites fails. Lessons
// without a task or citations cannot be checked and are skipped; an
// unreadable evidence store fails open (no findings) rather than
// proposing retirements from ignorance. Sorted by domain, then cue.
func staleLessonFindings(root string) []Finding {
	domains, err := library.Domains(root)
	if err != nil {
		return nil
	}
	var out []Finding
	for _, d := range domains {
		lessons, err := library.List(root, d)
		if err != nil {
			continue
		}
		for _, l := range lessons {
			if l.Task == "" || len(l.Evidence) == 0 {
				continue
			}
			rot, checked := evidence.CitedRot(root, l.Task, l.Evidence)
			if !checked || !rot {
				continue
			}
			files, _ := evidence.CitedFailures(root, l.Task, l.Evidence)
			out = append(out, Finding{
				Kind:     FindingStaleLesson,
				Domain:   d,
				Cue:      fmt.Sprintf("lesson %s evidence no longer verifies", l.ID),
				Lesson:   fmt.Sprintf("Admitted lesson %q (%s) cites evidence its task receipts no longer support — re-validate or retire it.", l.Cue, l.ID),
				Evidence: files,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].Cue < out[j].Cue
	})
	return out
}

// ApplyTick writes one computed tick: a candidate per finding (low
// confidence — the guardian validates) plus the advanced cadence
// cursor. A non-due result writes nothing and advances nothing.
func ApplyTick(root string, res TickResult, now time.Time) error {
	if !res.Due {
		return nil
	}
	for _, f := range res.Findings {
		if err := library.Propose(root, library.Lesson{
			Domain:     f.Domain,
			Cue:        f.Cue,
			Lesson:     f.Lesson,
			Evidence:   f.Evidence,
			Confidence: "low",
			CreatedAt:  now.UTC(),
		}); err != nil {
			return err
		}
		now = now.Add(time.Second)
	}
	return saveTickState(root, tickState{SessionsAtTick: res.Sessions})
}
