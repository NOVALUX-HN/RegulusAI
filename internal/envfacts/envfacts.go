// Package envfacts mines repeated failures into domain facts without
// any model call (macizo-memory M4): same gate failing — or same tool
// erroring — in MinSessions distinct sessions becomes a domain-lesson
// candidate, and a candidate whose last ContradictionWindow executions
// run clean is discarded as contradicted. Every decision is a pure
// function of the receipts and traces on disk: re-running the miner on
// the same tree yields the same plan.
package envfacts

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Failure kinds.
const (
	KindGate = "gate"
	KindTool = "tool"
)

// Mining thresholds (M4): a failure repeating in MinSessions distinct
// sessions is a fact candidate; a candidate whose last
// ContradictionWindow executions contain zero failures stopped
// repeating and is discarded.
const (
	MinSessions         = 3
	ContradictionWindow = 3
)

// MaxEvidence caps the cited paths per fact: enough to audit, few
// enough to stay readable.
const MaxEvidence = 5

// TopicPrefix namespaces mined-fact revision chains so re-mining the
// same key appends instead of duplicating.
const TopicPrefix = "envfact/"

// MinerBy names the actor on discard notes.
const MinerBy = "envfacts-miner"

// LessonDimension is the harness² dimension of mined facts: they say
// how the agent's environment behaves, not what a practitioner knows.
const LessonDimension = "inv"

// Key identifies one minable failure series: what failed, where it was
// observed (the chart domain of the failing task).
type Key struct {
	Kind   string
	Name   string
	Domain string
}

// slugSegment sanitizes one topic path segment: only [A-Za-z0-9_.-]
// survive, anything else becomes "-", runs collapse to one, outer
// dashes trim, empty falls back to "unknown" so every Topic stays a
// safe slash-separated path even with typos or odd chart domains.
func slugSegment(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-' {
			if r == '-' {
				if dash {
					continue
				}
				dash = true
			} else {
				dash = false
			}
			b.WriteRune(r)
			continue
		}
		if dash {
			continue
		}
		b.WriteByte('-')
		dash = true
	}
	out := strings.Trim(b.String(), "-")
	if strings.TrimSpace(out) == "" {
		return "unknown"
	}
	return out
}

// Topic returns the revision-chain topic of the key's fact.
func (k Key) Topic() string {
	return TopicPrefix + slugSegment(k.Domain) + "/" + slugSegment(k.Kind) + "/" + slugSegment(k.Name)
}

// Execution is one chronological observation of a key: a gate run
// (receipt) or a session using the tool (trace). Session names the
// distinct context (run id, receipt path, trace session); Failed says
// it failed there; Evidence cites the file backing the claim.
type Execution struct {
	At       time.Time
	Session  string
	Failed   bool
	Evidence string
}

// Series is the ordered history of one key.
type Series struct {
	Key   Key
	Execs []Execution
}

// FailingSessions counts the distinct sessions with a failure.
func FailingSessions(s Series) int {
	seen := map[string]bool{}
	for _, e := range s.Execs {
		if e.Failed {
			seen[e.Session] = true
		}
	}
	return len(seen)
}

// IsCandidate reports whether the failure repeated enough to mine.
func IsCandidate(s Series) bool {
	return FailingSessions(s) >= MinSessions
}

// IsContradicted reports whether the failure stopped repeating: the
// last ContradictionWindow executions containing the key run clean.
// A single later success does not contradict a repeating failure; a
// sustained clean window does (success where systematic failure was,
// or the fact simply stops repeating).
func IsContradicted(s Series) bool {
	n := len(s.Execs)
	if n == 0 {
		return false
	}
	w := ContradictionWindow
	if n < w {
		w = n
	}
	for _, e := range s.Execs[n-w:] {
		if e.Failed {
			return false
		}
	}
	return true
}

// EvidenceFor returns the sorted distinct evidence paths of the failed
// executions, capped at MaxEvidence.
func EvidenceFor(s Series) []string {
	seen := map[string]bool{}
	for _, e := range s.Execs {
		if e.Failed && strings.TrimSpace(e.Evidence) != "" {
			seen[e.Evidence] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	if len(out) > MaxEvidence {
		out = out[:MaxEvidence]
	}
	return out
}

// CueFor renders the deterministic fact cue (zero model).
func CueFor(k Key, failing int) string {
	unit := "executions"
	if k.Kind == KindTool {
		unit = "sessions"
	}
	return fmt.Sprintf("%s %s fails repeatedly in domain %s (%d %s)", k.Kind, k.Name, k.Domain, failing, unit)
}

// LessonFor renders the deterministic fact lesson (zero model): what
// repeated, where the evidence is, what to do on recurrence.
func LessonFor(k Key, failing int, evidence []string) string {
	unit := "executions"
	if k.Kind == KindTool {
		unit = "sessions"
	}
	return fmt.Sprintf("%s %s failed in %d %s in domain %s; evidence: %s. When %s fails again, compare against that evidence and check the failure repeats before treating it as environmental.",
		k.Kind, k.Name, failing, unit, k.Domain, strings.Join(evidence, ", "), k.Name)
}

// domainRe mirrors the plugin's Domain: line parser
// (.opencode/lib/domain.ts findDomainInChart).
var domainRe = regexp.MustCompile(`(?m)^Domain:\s*([A-Za-z0-9_][\w.\-]*)`)

// chartDomain reads one chart's Domain line ("", when the chart is
// missing or declares none).
func chartDomain(root, chart string, cache map[string]string) string {
	if d, ok := cache[chart]; ok {
		return d
	}
	d := ""
	if raw, err := os.ReadFile(filepath.Join(root, "charts", chart+".md")); err == nil {
		if m := domainRe.FindSubmatch(raw); m != nil {
			d = string(m[1])
		}
	}
	cache[chart] = d
	return d
}

// DomainOfTask resolves the domain of a task id via its chart prefix
// ("macizo-memory/M3" reads charts/macizo-memory.md). Tasks without a
// chart or without a Domain line have no domain and cannot form domain
// facts.
func DomainOfTask(root, task string, cache map[string]string) string {
	chart := task
	if i := strings.Index(chart, "/"); i >= 0 {
		chart = chart[:i]
	}
	if strings.TrimSpace(chart) == "" {
		return ""
	}
	return chartDomain(root, chart, cache)
}
