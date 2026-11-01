package envfacts

import (
	"fmt"
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

// liveFactIDs returns the mined lesson revisions of a topic that still
// surface in default retrieval (not invalidated, superseded or stale).
func liveFactIDs(root string, topic string) ([]string, error) {
	chain, err := almanac.History(root, topic)
	if err != nil {
		return nil, err
	}
	inv, err := almanac.InvalidatedIDs(root)
	if err != nil {
		return nil, err
	}
	sup, err := almanac.SupersededIDs(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, o := range chain {
		if o.Type != almanac.TypeLesson {
			continue
		}
		if !strings.HasPrefix(o.TopicKey, TopicPrefix) {
			continue
		}
		if inv[o.ID] || sup[o.ID] {
			continue
		}
		st, err := almanac.EffectiveStatus(root, o.ID)
		if err != nil {
			return nil, err
		}
		if st == almanac.StatusRejected {
			continue
		}
		out = append(out, o.ID)
	}
	return out, nil
}

// Plan decides every series without writing: mine uncontradicted
// candidates with no live fact, discard contradicted candidates with
// one, skip everything else with the reason.
func Plan(root string, series []Series) ([]Decision, error) {
	out := make([]Decision, 0, len(series))
	for _, s := range series {
		live, err := liveFactIDs(root, s.Key.Topic())
		if err != nil {
			return nil, err
		}
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
// --include-stale. Skips write nothing.
func Apply(root string, decisions []Decision, now time.Time) ([]Result, error) {
	out := make([]Result, 0, len(decisions))
	for _, d := range decisions {
		switch d.Action {
		case ActionMine:
			id, err := mineFact(root, d.Series, now)
			if err != nil {
				return nil, err
			}
			out = append(out, Result{Decision: d, FactID: id})
		case ActionDiscard:
			note, err := discardFact(root, d, now)
			if err != nil {
				return nil, err
			}
			out = append(out, Result{Decision: d, NoteID: note})
		default:
			out = append(out, Result{Decision: d})
		}
	}
	return out, nil
}

// mineFact saves the key's fact as a domain lesson (the existing
// per-domain injection `memory search --type lesson --domain` picks it
// up with no plugin change). Lessons need evidence in code; the failing
// receipts or trace files are cited.
func mineFact(root string, s Series, now time.Time) (string, error) {
	failing := FailingSessions(s)
	evidence := EvidenceFor(s)
	saved, err := almanac.Save(root, &almanac.Observation{
		Type: almanac.TypeLesson, Title: CueFor(s.Key, failing), Content: LessonFor(s.Key, failing, evidence),
		TopicKey: s.Key.Topic(), Domain: s.Key.Domain,
		Lesson: &almanac.Lesson{
			Cue: CueFor(s.Key, failing), Lesson: LessonFor(s.Key, failing, evidence),
			Evidence: evidence, Confidence: ConfidenceFor(failing), Dimension: LessonDimension,
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
func discardFact(root string, d Decision, now time.Time) (string, error) {
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
	for _, id := range d.LiveFactIDs {
		if err := almanac.Relate(root, note.ID, almanac.RelationInvalidatedBy, id, now); err != nil {
			return "", err
		}
	}
	return note.ID, nil
}
