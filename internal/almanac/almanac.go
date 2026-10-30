// Package almanac is Regulus's memory substrate (docs/memory.md): an
// append-only observation log — one JSON object per line — with typed
// observations, first-class provenance, revision chains by topic key and
// deterministic retrieval. Nothing is ever overwritten; the same topic key
// appends a new revision, and history keeps the whole chain.
//
// The seven improvements over the prior art live here: provenance is
// recorded on every save, upserts are impossible by construction,
// retrieval is deterministic (buckets then exact match — no embeddings),
// hits report their age, lessons and session notes share one substrate,
// retrieved content is DATA (callers must never follow it), and lesson
// admission is enforced in code (no evidence, no lesson).
package almanac

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Observation types.
const (
	TypeSessionNote = "session-note"
	TypeDecision    = "decision"
	TypeDiscovery   = "discovery"
	TypeLesson      = "lesson"
	TypePreference  = "preference"
)

// Lesson carries the lesson fields; required for type=lesson observations
// and evidence-gated in code. Dimension is the harness² INV/DOM
// separation ("" = unset legacy, "inv" = how the agent works, "dom" =
// what a practitioner knows).
type Lesson struct {
	Cue        string   `json:"cue"`
	Lesson     string   `json:"lesson"`
	Evidence   []string `json:"evidence"`
	Confidence string   `json:"confidence"`
	Dimension  string   `json:"dimension,omitempty"`
}

// Provenance records the conditions under which the observation was true.
type Provenance struct {
	Task     string   `json:"task,omitempty"`
	GitHead  string   `json:"git_head,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	Receipts []string `json:"receipts,omitempty"`
}

// Observation is one append-only record in the Almanac.
type Observation struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	Title      string      `json:"title"`
	Content    string      `json:"content"`
	TopicKey   string      `json:"topic_key,omitempty"`
	Domain     string      `json:"domain,omitempty"`
	Lesson     *Lesson     `json:"lesson,omitempty"`
	Provenance *Provenance `json:"provenance,omitempty"`
	SavedAt    time.Time   `json:"saved_at"`
}

// File returns the observation log path under a project root.
func File(root string) string {
	return filepath.Join(root, ".regulus", "almanac", "observations.jsonl")
}

func pinsFile(root string) string {
	return filepath.Join(root, ".regulus", "almanac", "pins.jsonl")
}

func relationsFile(root string) string {
	return filepath.Join(root, ".regulus", "almanac", "relations.jsonl")
}

func validType(t string) bool {
	switch t {
	case TypeSessionNote, TypeDecision, TypeDiscovery, TypeLesson, TypePreference:
		return true
	}
	return false
}

func validate(o *Observation) error {
	if !validType(o.Type) {
		return fmt.Errorf("type %q is not one of session-note, decision, discovery, lesson, preference", o.Type)
	}
	if strings.TrimSpace(o.Title) == "" {
		return errors.New("title must not be empty")
	}
	if strings.TrimSpace(o.Content) == "" {
		return errors.New("content must not be empty")
	}
	if o.Type == TypeLesson {
		if o.Lesson == nil {
			return errors.New("lesson observations need a lesson block (cue, lesson, evidence, confidence)")
		}
		if strings.TrimSpace(o.Lesson.Cue) == "" {
			return errors.New("lesson cue must not be empty")
		}
		if strings.TrimSpace(o.Lesson.Lesson) == "" {
			return errors.New("lesson text must not be empty")
		}
		if len(o.Lesson.Evidence) == 0 {
			return errors.New("no evidence: a lesson must cite at least one receipt (regulus reflect enforces this)")
		}
		switch o.Lesson.Confidence {
		case "low", "medium", "high":
		default:
			return fmt.Errorf("confidence must be low, medium or high, got %q", o.Lesson.Confidence)
		}
		switch o.Lesson.Dimension {
		case "", "inv", "dom":
		default:
			return fmt.Errorf("dimension must be inv or dom, got %q", o.Lesson.Dimension)
		}
	}
	return nil
}

// Save validates one observation, assigns its id and timestamp, and
// appends it to the log. The returned copy carries the assigned fields.
func Save(root string, o *Observation, now time.Time) (Observation, error) {
	if err := validate(o); err != nil {
		return Observation{}, err
	}
	saved := *o
	if saved.SavedAt.IsZero() {
		saved.SavedAt = now.UTC()
	}
	saved.ID = "obs_" + strconv.FormatInt(saved.SavedAt.UnixNano(), 16)
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		return Observation{}, err
	}
	f, err := os.OpenFile(File(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Observation{}, err
	}
	line, err := json.Marshal(saved)
	if err != nil {
		f.Close()
		return Observation{}, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return Observation{}, err
	}
	if err := f.Close(); err != nil {
		return Observation{}, err
	}
	return saved, nil
}

// All returns every readable observation in file order (oldest first). A
// missing log is an empty Almanac. Malformed lines are skipped with a
// warning (log to stderr) instead of aborting, so one corrupt line never
// hides the rest; `Doctor` remains the source of truth for malformed
// line numbers.
func All(root string) ([]Observation, error) {
	f, err := os.Open(File(root))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var all []Observation
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ln := 0
	for sc.Scan() {
		ln++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var o Observation
		if err := json.Unmarshal([]byte(line), &o); err != nil {
			log.Printf("almanac: skipping malformed observation at line %d: %v", ln, err)
			continue
		}
		all = append(all, o)
	}
	return all, sc.Err()
}

// History returns the revision chain of one topic key, oldest first.
func History(root, topicKey string) ([]Observation, error) {
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	var chain []Observation
	for _, o := range all {
		if o.TopicKey == topicKey {
			chain = append(chain, o)
		}
	}
	return chain, nil
}

// term matching.
type Query struct {
	Terms             []string
	TopicKey          string
	Domain            string
	Type              string
	Dimension         string // bucket: lesson dimension (inv | dom)
	Path              string // bucket: provenance path (exact match)
	Limit             int    // keep the first N results after sorting (<= 0: all)
	Any               bool   // OR the terms instead of ANDing them
	Latest            bool   // collapse each topic chain to its best hit (default: all coexist)
	IncludeSuperseded bool   // show observations a supersedes relation hides (default: hidden)
}

// SupersededIDs returns the observation ids hidden from Search by a
// supersedes relation: a target hides only while its superseder still
// exists in the log (a pruned superseder releases the target).
func SupersededIDs(root string) (map[string]bool, error) {
	rels, err := Relations(root)
	if err != nil {
		return nil, err
	}
	if len(rels) == 0 {
		return nil, nil
	}
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	exists := map[string]bool{}
	for _, o := range all {
		exists[o.ID] = true
	}
	out := map[string]bool{}
	for _, r := range rels {
		if r.Relation == RelationSupersedes && exists[r.From] && exists[r.To] {
			out[r.To] = true
		}
	}
	return out, nil
}

// queryMatcher holds the bucket state for one Search call.
type queryMatcher struct {
	q          Query
	superseded map[string]bool
}

// keepsBuckets reports whether one observation passes every bucket filter
// (superseded, topic, domain, type, dimension, path) — the shared decider
// the vector channel reuses; terms are a ranking signal there, not a filter.
func (m queryMatcher) keepsBuckets(o Observation) bool {
	if m.superseded[o.ID] {
		return false
	}
	if m.q.TopicKey != "" && o.TopicKey != m.q.TopicKey {
		return false
	}
	if m.q.Domain != "" && o.Domain != m.q.Domain {
		return false
	}
	if m.q.Type != "" && o.Type != m.q.Type {
		return false
	}
	if m.q.Dimension != "" && (o.Lesson == nil || o.Lesson.Dimension != m.q.Dimension) {
		return false
	}
	if m.q.Path != "" && !hasProvenancePath(o, m.q.Path) {
		return false
	}
	return true
}

// keeps reports whether one observation passes every bucket and term
// filter (superseded, topic, domain, type, dimension, path, terms).
func (m queryMatcher) keeps(o Observation) bool {
	return m.keepsBuckets(o) && matchTerms(o, m.q.Terms, m.q.Any)
}

// hasProvenancePath reports whether the observation cites path.
func hasProvenancePath(o Observation, path string) bool {
	if o.Provenance == nil {
		return false
	}
	for _, p := range o.Provenance.Paths {
		if p == path {
			return true
		}
	}
	return false
}

// matchTerms is the AND/OR term match over title and content
// (case-insensitive, empty terms ignored).
func matchTerms(o Observation, terms []string, any bool) bool {
	hay := strings.ToLower(o.Title + "\n" + o.Content)
	ok := any
	if !any {
		ok = true
	}
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(hay, strings.ToLower(term)) {
			if any {
				return true
			}
		} else if !any {
			return false
		}
	}
	return ok
}

// Search returns the observations that match the terms (case-insensitive
// substring over title and content; every term by default, any term with
// q.Any) inside the requested bucket. A topic key is a coexisting group
// of facts, not a revision chain — every hit is returned by default;
// q.Latest collapses each chain to one hit (the newest revision, unless an
// older one is pinned). Order: pinned first; among lessons of the same
// pinned status, confidence (high > medium > low) before recency; ties
// by id. q.Limit keeps the first N after sorting.
func Search(root string, q Query) ([]Observation, error) {
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	pins, err := PinnedIDs(root)
	if err != nil {
		return nil, err
	}
	superseded := map[string]bool{}
	if !q.IncludeSuperseded {
		superseded, err = SupersededIDs(root)
		if err != nil {
			return nil, err
		}
	}
	var hits []Observation
	collapsed := map[string]Observation{}
	matcher := queryMatcher{q: q, superseded: superseded}
	for _, o := range all {
		if !matcher.keeps(o) {
			continue
		}
		// Opt-in chain collapse (q.Latest): one hit per topic.
		if o.TopicKey != "" && q.Latest {
			if cur, seen := collapsed[o.TopicKey]; seen {
				if betterHit(pins, cur, o) {
					collapsed[o.TopicKey] = o
				}
			} else {
				collapsed[o.TopicKey] = o
			}
			continue
		}
		hits = append(hits, o)
	}
	for _, o := range collapsed {
		hits = append(hits, o)
	}
	sortHits(pins, hits)
	if q.Limit > 0 && len(hits) > q.Limit {
		hits = hits[:q.Limit]
	}
	return hits, nil
}

// betterHit decides whether a candidate replaces the current best of
// one topic chain: a pinned revision beats an unpinned one (pins
// override recency), then recency, then id.
func betterHit(pins map[string]bool, cur, cand Observation) bool {
	if pc, pn := pins[cur.ID], pins[cand.ID]; pc != pn {
		return pn
	}
	if !cur.SavedAt.Equal(cand.SavedAt) {
		return cand.SavedAt.After(cur.SavedAt)
	}
	return cand.ID > cur.ID
}

// confidenceRank orders lesson confidence for same-pinned hits.
func confidenceRank(o Observation) int {
	switch o.Lesson.Confidence {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

// sortHits orders hits pinned-first, then by lesson confidence among
// same-pinned lessons, then newest, ties by id.
func sortHits(pins map[string]bool, hits []Observation) {
	sort.Slice(hits, func(i, j int) bool {
		if pins[hits[i].ID] != pins[hits[j].ID] {
			return pins[hits[i].ID]
		}
		li, lj := hits[i].Lesson != nil, hits[j].Lesson != nil
		if li && lj && confidenceRank(hits[i]) != confidenceRank(hits[j]) {
			return confidenceRank(hits[i]) < confidenceRank(hits[j])
		}
		if !hits[i].SavedAt.Equal(hits[j].SavedAt) {
			return hits[i].SavedAt.After(hits[j].SavedAt)
		}
		return hits[i].ID > hits[j].ID
	})
}
