// Knowledge corroboration, derived confidence, freshness and contextual
// coexistence (F18.2 / NOV-143).
//
// The F18.1 audit (docs/knowledge-model-audit.md) found that promotion rested on
// fixed rules only: three failing sessions for a mined fact, one passing receipt
// for a reflection, evidence required on a verified solution. Confidence was
// whatever the caller wrote, so a claim seen once and a claim seen twenty times
// looked identical. Expiry could discard a record but nothing could renew one,
// and a contradiction had to produce a winner because there was no way to say
// "true here, false there".
//
// This file adds the three missing properties to the existing record. It adds no
// store: everything lives on the Observation the Almanac already persists.
package almanac

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Confidences. These mirror library.Confidence* as literals so the Almanac
// stays dependency-free, the same way MemoryReadScope mirrors the MCP scope; a
// test pins the equality so a rename breaks loudly instead of drifting.
const (
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)

// Corroboration bars. Two independent sources make a claim more than an
// anecdote; four make it strong. One source never reaches high — that is the
// parent's rule that a single run is not a universal rule.
const (
	CorroborationsForMedium = 2
	CorroborationsForHigh   = 4
)

// Corroborate records one independent source that saw the same claim.
//
// Idempotent per source: corroborating twice from the same place adds nothing,
// because repetition by the same actor is not independent evidence. An author
// cannot corroborate their own claim. An empty source is refused: a
// corroboration nobody signed is not evidence.
func Corroborate(o *Observation, source string) error {
	if o == nil {
		return errors.New("almanac: corroborate needs an observation")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return errors.New("almanac: corroborate needs a source (independent actor or host)")
	}
	if strings.TrimSpace(o.Author) != "" && strings.EqualFold(source, strings.TrimSpace(o.Author)) {
		return fmt.Errorf("almanac: %q cannot corroborate its own claim", source)
	}
	for _, existing := range o.Corroborations {
		if strings.EqualFold(strings.TrimSpace(existing), source) {
			return nil
		}
	}
	o.Corroborations = append(o.Corroborations, source)
	return nil
}

// independentCorroborations counts distinct, non-self sources.
func independentCorroborations(o Observation) int {
	seen := map[string]bool{}
	for _, source := range o.Corroborations {
		key := strings.ToLower(strings.TrimSpace(source))
		if key == "" {
			continue
		}
		if strings.EqualFold(key, strings.TrimSpace(o.Author)) {
			continue
		}
		seen[key] = true
	}
	return len(seen)
}

// assignedConfidence is the confidence the author claimed. For a lesson it is
// the lesson's own field, which is where lesson confidence actually lives;
// otherwise it is the observation's.
func assignedConfidence(o Observation) string {
	if o.Lesson != nil {
		if v := strings.ToLower(strings.TrimSpace(o.Lesson.Confidence)); v != "" {
			return v
		}
	}
	return strings.ToLower(strings.TrimSpace(o.Confidence))
}

// DerivedConfidence returns the confidence the evidence supports.
//
// It never returns less than the assigned value: an author may know something a
// count does not, and lowering it here would discard that silently. It never
// promotes on a single source, which is what keeps one run from becoming a
// universal rule.
func DerivedConfidence(o Observation) string {
	assigned := assignedConfidence(o)
	if assigned != ConfidenceLow && assigned != ConfidenceMedium && assigned != ConfidenceHigh {
		assigned = ConfidenceLow
	}
	count := independentCorroborations(o)
	switch {
	case count >= CorroborationsForHigh:
		return ConfidenceHigh
	case count >= CorroborationsForMedium && assigned != ConfidenceHigh:
		return ConfidenceMedium
	}
	return assigned
}

// Revalidate marks the claim as checked again by an actor and renews its
// freshness, instead of letting a still-true claim expire and be discarded.
//
// A revalidation needs an actor and a positive TTL: an unsigned renewal and an
// indefinite one are both worthless as evidence.
func Revalidate(o *Observation, by string, now time.Time, ttl time.Duration) error {
	if o == nil {
		return errors.New("almanac: revalidate needs an observation")
	}
	by = strings.TrimSpace(by)
	if by == "" {
		return errors.New("almanac: revalidate needs an actor")
	}
	if ttl <= 0 {
		return fmt.Errorf("almanac: revalidate needs a positive ttl, got %s", ttl)
	}
	if now.IsZero() {
		return errors.New("almanac: revalidate needs a timestamp")
	}
	stamp := now.UTC()
	expires := stamp.Add(ttl)
	o.RevalidatedAt = &stamp
	o.ExpiresAt = &expires
	return nil
}

// observationsByID reads the requested records from the log. Missing ids are
// simply absent from the map; the caller decides whether that is an error.
func observationsByID(root string, ids ...string) (map[string]Observation, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Observation, len(ids))
	for _, o := range all {
		if want[o.ID] {
			out[o.ID] = o
		}
	}
	return out, nil
}

// CompatibleContexts reports whether two claims can both be true.
//
// Two claims coexist when they were established in different hosts or different
// environments: the same lesson can be correct on Linux and wrong on Windows,
// and forcing a winner there destroys knowledge. When neither differs they
// describe the same context, so they genuinely conflict and resolution is
// required.
func CompatibleContexts(a, b Observation) bool {
	if ah, bh := strings.TrimSpace(a.Host), strings.TrimSpace(b.Host); ah != "" && bh != "" && !strings.EqualFold(ah, bh) {
		return true
	}
	if ae, be := strings.TrimSpace(a.Environment), strings.TrimSpace(b.Environment); ae != "" && be != "" && !strings.EqualFold(ae, be) {
		return true
	}
	return false
}
