package envfacts

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
)

// Actions the miner takes per series.
const (
	ActionMine    = "mine"
	ActionDiscard = "discard"
	ActionSkip    = "skip"
)

// Decision is the miner's verdict for one series: what to do and why.
// LiveFactIDs names the live mined revisions a discard retires.
type Decision struct {
	Series      Series
	Action      string
	Reason      string
	LiveFactIDs []string
}

// liveCache holds one full log load (H4): every observation grouped by
// topic plus the retired-id and status maps, so Plan scans the log once
// instead of paying N+1 full scans (History + InvalidatedIDs +
// SupersededIDs + EffectiveStatus per series).
type liveCache struct {
	byTopic map[string][]almanac.Observation
	byID    map[string]almanac.Observation
	inv     map[string]bool
	sup     map[string]bool
	status  map[string]string
}

func newLiveCache() *liveCache {
	return &liveCache{
		byTopic: map[string][]almanac.Observation{},
		byID:    map[string]almanac.Observation{},
		inv:     map[string]bool{},
		sup:     map[string]bool{},
		status:  map[string]string{},
	}
}

func indexObservations(c *liveCache, all []almanac.Observation) {
	for _, o := range all {
		c.byTopic[o.TopicKey] = append(c.byTopic[o.TopicKey], o)
		c.byID[o.ID] = o
	}
}

func applyRelationsToCache(c *liveCache, rels []almanac.Relation) {
	for _, r := range rels {
		if _, ok := c.byID[r.From]; !ok {
			continue
		}
		if _, ok := c.byID[r.To]; !ok {
			continue
		}
		if r.Relation == almanac.RelationInvalidatedBy {
			c.inv[r.To] = true
		} else if r.Relation == almanac.RelationSupersedes {
			c.sup[r.To] = true
		}
	}
}

func loadStatusIntoCache(c *liveCache, root string) {
	data, err := os.ReadFile(almanac.StatusFile(root))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if id, to, ok := parseStatusLine(line); ok {
			c.status[id] = to
		}
	}
}

func parseStatusLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", "", false
	}
	var ev almanac.StatusEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return "", "", false
	}
	return ev.ID, ev.To, true
}

func loadLiveCache(root string) (*liveCache, error) {
	all, err := almanac.All(root)
	if err != nil {
		return nil, err
	}
	rels, err := almanac.Relations(root)
	if err != nil {
		return nil, err
	}
	c := newLiveCache()
	indexObservations(c, all)
	applyRelationsToCache(c, rels)
	loadStatusIntoCache(c, root)
	return c, nil
}

func (c *liveCache) effective(id string) string {
	if st, ok := c.status[id]; ok && st != "" {
		return st
	}
	if c.sup[id] {
		return almanac.StatusSuperseded
	}
	o, ok := c.byID[id]
	if !ok {
		return ""
	}
	if o.Type == almanac.TypeLesson {
		return almanac.StatusAccepted
	}
	if o.Source == almanac.SourceImport {
		return almanac.StatusProposed
	}
	return almanac.StatusObserved
}

func (c *liveCache) live(topic string) []string {
	var out []string
	for _, o := range c.byTopic[topic] {
		if o.Type != almanac.TypeLesson {
			continue
		}
		if !strings.HasPrefix(o.TopicKey, TopicPrefix) {
			continue
		}
		if c.inv[o.ID] || c.sup[o.ID] {
			continue
		}
		if c.effective(o.ID) == almanac.StatusRejected {
			continue
		}
		out = append(out, o.ID)
	}
	return out
}

// liveFactIDs returns the mined lesson revisions of a topic that still
// surface in default retrieval (not invalidated, superseded or stale).
func liveFactIDs(root string, topic string) ([]string, error) {
	c, err := loadLiveCache(root)
	if err != nil {
		return nil, err
	}
	return c.live(topic), nil
}

// Plan decides every series without writing: mine uncontradicted
// candidates with no live fact, discard contradicted candidates with
// one, skip everything else with the reason.
func Plan(root string, series []Series) ([]Decision, error) {
	c, err := loadLiveCache(root)
	if err != nil {
		return nil, err
	}
	out := make([]Decision, 0, len(series))
	for _, s := range series {
		live := c.live(s.Key.Topic())
		failing := FailingSessions(s)
		switch {
		case !IsCandidate(s):
			out = append(out, Decision{Series: s, Action: ActionSkip,
				Reason: fmt.Sprintf("needs %d failing sessions, has %d", MinSessions, failing)})
		case IsContradicted(s):
			if len(live) == 0 {
				out = append(out, Decision{Series: s, Action: ActionSkip,
					Reason: fmt.Sprintf("contradicted (last %d executions clean) and never mined", ContradictionWindow)})
			} else {
				out = append(out, Decision{Series: s, Action: ActionDiscard, LiveFactIDs: live,
					Reason: fmt.Sprintf("contradicted (last %d executions clean)", ContradictionWindow)})
			}
		case len(live) > 0:
			out = append(out, Decision{Series: s, Action: ActionSkip, LiveFactIDs: live,
				Reason: fmt.Sprintf("already mined as %s", strings.Join(live, ", "))})
		default:
			out = append(out, Decision{Series: s, Action: ActionMine})
		}
	}
	return out, nil
}

// Result records what applying one decision wrote.
type Result struct {
	Decision Decision
	FactID   string
	NoteID   string
}

// Apply executes mine and discard decisions. Mining saves one lesson
// observation on the key's topic (a re-mined key appends a new
// revision, never overwriting); discarding saves one session-note with
// the contradicting evidence and retires every live revision through
// invalidated_by, so discarded facts hide from default retrieval
// (including the per-domain lesson injection) and resurface with
// --include-stale. Skips write nothing. H3: Apply never aborts on the
// first failure — every decision runs, successes are returned, and the
// error aggregates each failure per-decision
// ("apply: m/n failed: <action key: err>; ..."), so callers exit
// non-zero with the full list while keeping the partial progress.
func Apply(root string, decisions []Decision, now time.Time) ([]Result, error) {
	out := make([]Result, 0, len(decisions))
	var failed []string
	for _, d := range decisions {
		switch d.Action {
		case ActionMine:
			id, err := mineFact(root, d.Series, now)
			if err != nil {
				failed = append(failed, fmt.Sprintf("mine %s: %v", d.Series.Key.Topic(), err))
				continue
			}
			out = append(out, Result{Decision: d, FactID: id})
		case ActionDiscard:
			note, err := discardFact(root, d, now)
			if err != nil {
				failed = append(failed, fmt.Sprintf("discard %s: %v", d.Series.Key.Topic(), err))
				continue
			}
			out = append(out, Result{Decision: d, NoteID: note})
		default:
			out = append(out, Result{Decision: d})
		}
	}
	if len(failed) > 0 {
		return out, fmt.Errorf("apply: %d/%d failed: %s", len(failed), len(decisions), strings.Join(failed, "; "))
	}
	return out, nil
}

// mineFact saves the key's fact as a domain lesson (the existing
// per-domain injection `memory search --type lesson --domain` picks it
// up with no plugin change). Lessons need evidence in code; the failing
// receipts or trace files are cited. Confidence is fixed at medium:
// the count already gates mining (MinSessions) and the lesson text
// cites the failing-session count, so no second threshold adds signal.
func mineFact(root string, s Series, now time.Time) (string, error) {
	failing := FailingSessions(s)
	evidence := EvidenceFor(s)
	saved, err := almanac.Save(root, &almanac.Observation{
		Type: almanac.TypeLesson, Title: CueFor(s.Key, failing), Content: LessonFor(s.Key, failing, evidence),
		TopicKey: s.Key.Topic(), Domain: s.Key.Domain,
		Lesson: &almanac.Lesson{
			Cue: CueFor(s.Key, failing), Lesson: LessonFor(s.Key, failing, evidence),
			Evidence: evidence, Confidence: "medium", Dimension: LessonDimension,
		},
		Provenance: &almanac.Provenance{Task: MinerBy, Receipts: evidence},
		Source:     almanac.SourceCore, Origin: almanac.ProjectName(root),
	}, now)
	if err != nil {
		return "", err
	}
	return saved.ID, nil
}

// discardFact records the contradiction and retires the live revisions.
// H3 idempotency: it reloads the topic's live facts first; when none
// remain (the discard already ran, or a retry lands after a completed
// discard) it writes nothing and returns "" with no error — a no-op
// the caller reports as a skip — so a re-applied discard never mints a
// double note. Otherwise only decision IDs still un-invalidated are
// retired (already-retired IDs are skipped, never re-linked); a decision
// carrying no IDs retires the current live set, and a decision whose IDs
// are all already retired is likewise a no-op skip without overreaching
// into newer live facts (those need a fresh Plan).
func discardFact(root string, d Decision, now time.Time) (string, error) {
	c, err := loadLiveCache(root)
	if err != nil {
		return "", err
	}
	live := c.live(d.Series.Key.Topic())
	if len(live) == 0 {
		return "", nil
	}
	targets := live
	if len(d.LiveFactIDs) > 0 {
		targets = nil
		for _, id := range d.LiveFactIDs {
			if !c.inv[id] {
				targets = append(targets, id)
			}
		}
		if len(targets) == 0 {
			return "", nil
		}
	}
	evidence := EvidenceFor(d.Series)
	note, err := almanac.Save(root, &almanac.Observation{
		Type: almanac.TypeSessionNote,
		Title: fmt.Sprintf("envfact contradicted: %s %s (%s)",
			d.Series.Key.Kind, d.Series.Key.Name, d.Series.Key.Domain),
		Content: fmt.Sprintf("discarded %s: %s; contradicting window evidence: %s",
			d.Series.Key.Topic(), d.Reason, strings.Join(evidence, ", ")),
		TopicKey:   d.Series.Key.Topic(),
		Provenance: &almanac.Provenance{Task: MinerBy, Receipts: evidence},
		Source:     almanac.SourceCore, Origin: almanac.ProjectName(root),
	}, now)
	if err != nil {
		return "", err
	}
	for _, id := range targets {
		if err := almanac.Relate(root, note.ID, almanac.RelationInvalidatedBy, id, now); err != nil {
			return "", err
		}
	}
	return note.ID, nil
}
