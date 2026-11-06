package almanac

// Save-time dedup (docs/memory-v2.md §5, T3): one decider shared by the
// CLI save/note path and tests. A save carrying an explicit topic key
// that matches an existing chain appends as a revision (declared
// grouping wins, untouched behavior). Otherwise the candidate's cosine
// against every non-superseded observation decides: above the bar the
// save is refused with the near-duplicate id and a hint to set a topic
// key; below it the save lands normally. Superseded observations stay
// out of the comparison (one decider, no double-hiding — the same
// SupersededIDs the search bucket uses).
//
// Index freshness: a fresh on-disk retrieval.bin supplies the existing
// vectors; a missing or stale index falls back to embedding on the fly.
// Dedup never blocks a save for index reasons — staleness only changes
// where the vectors come from.
//
// Threshold provenance (S2 eval fixture in dedup_test.go): the 0.92 bar
// sits between the nearest below-threshold sim (unrelated/pool-vs-runtime
// at 0.2565) and the farthest above-threshold sim (typo/pooling at
// 0.9350). Margins are re-measured by
// TestDedupThresholdMatchesFixture; the numbers above are the observed
// values at commit time.

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// DedupThreshold is the save-time near-duplicate bar: a candidate whose
// best cosine against the live log exceeds it is refused.
const DedupThreshold = 0.92

// DedupThresholdEnv names the optional override for the save-time
// near-duplicate bar. Empty or invalid values fall back to
// DedupThreshold (with a stderr warning); valid values in [0,1] win.
const DedupThresholdEnv = "REGULUS_DEDUP_THRESHOLD"

// DedupThresholdFromEnv resolves the effective save-time bar: the
// REGULUS_DEDUP_THRESHOLD override when it parses to a number in
// [0,1], DedupThreshold otherwise. The second return reports whether
// the env override applied. Invalid non-empty values (unparsable,
// NaN/Inf, out of [0,1]) warn on stderr and fall back to the default;
// resolving the threshold never fails a save.
func DedupThresholdFromEnv() (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(DedupThresholdEnv))
	if raw == "" {
		return DedupThreshold, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
		fmt.Fprintf(os.Stderr, "warning: invalid %s %q — want a number in [0,1]; using default %.2f\n", DedupThresholdEnv, raw, DedupThreshold)
		return DedupThreshold, false
	}
	return v, true
}

// DuplicateError is the save-time refusal: the candidate duplicates ID
// at Similarity, and the guidance points at the topic-key escape hatch.
type DuplicateError struct {
	ID         string
	Similarity float64
	// Threshold is the effective bar that refused the save (default
	// DedupThreshold unless a valid env override applied). Zero with
	// FromEnv unset means a pre-threshold literal — Error renders the
	// default bar.
	Threshold float64
	// FromEnv reports the bar came from REGULUS_DEDUP_THRESHOLD, so
	// the message can cite the env source.
	FromEnv bool
}

// Error names the near-duplicate id and hints at setting a topic key to
// link the save as a revision.
func (e *DuplicateError) Error() string {
	th := e.Threshold
	if th == 0 && !e.FromEnv {
		th = DedupThreshold
	}
	if e.FromEnv {
		return fmt.Sprintf("refusing near-duplicate save (matches %s at similarity %.2f, threshold %.2f from %s) — set a topic key to link it as a revision", e.ID, e.Similarity, th, DedupThresholdEnv)
	}
	return fmt.Sprintf("refusing near-duplicate save (matches %s at similarity %.2f, threshold %.2f) — set a topic key to link it as a revision", e.ID, e.Similarity, th)
}

// CheckDuplicate decides one save: nil lets it through, *DuplicateError
// refuses it. It never fails for index reasons (missing/stale/corrupt
// retrieval.bin falls back to on-the-fly vectors); only log or relation
// read errors propagate.
func CheckDuplicate(root string, candidate *Observation) error {
	all, err := All(root)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return nil
	}
	if candidate.TopicKey != "" {
		for _, o := range all {
			if o.TopicKey == candidate.TopicKey {
				return nil
			}
		}
	}
	superseded, err := SupersededIDs(root)
	if err != nil {
		return err
	}
	vecs := liveVectors(root, all)
	query := EmbedObservation(*candidate)
	if isZeroVec(query) {
		return nil
	}
	threshold, fromEnv := DedupThresholdFromEnv()
	bestID, best := "", 0.0
	for _, o := range all {
		if superseded[o.ID] {
			continue
		}
		if sim := cosineVec(query, vecs[o.ID]); sim > best {
			best, bestID = sim, o.ID
		}
	}
	if best > threshold {
		return &DuplicateError{ID: bestID, Similarity: best, Threshold: threshold, FromEnv: fromEnv}
	}
	return nil
}

// liveVectors returns the comparison vectors for the log: the fresh
// on-disk index when its pinned sha still matches the log, on-the-fly
// embeddings otherwise.
func liveVectors(root string, all []Observation) map[string][EmbedDim]int8 {
	if vecs, ok := freshIndexVectors(root); ok {
		for _, o := range all {
			if _, seen := vecs[o.ID]; !seen {
				vecs[o.ID] = EmbedObservation(o)
			}
		}
		return vecs
	}
	vecs := make(map[string][EmbedDim]int8, len(all))
	for _, o := range all {
		vecs[o.ID] = EmbedObservation(o)
	}
	return vecs
}

// freshIndexVectors loads retrieval.bin only when it is fresh (its
// pinned source sha matches the current log); any miss, staleness or
// corruption reports false so the caller embeds on the fly.
func freshIndexVectors(root string) (map[string][EmbedDim]int8, bool) {
	entries, hdr, err := LoadIndex(root)
	if err != nil {
		return nil, false
	}
	shaHex, err := cachedSourceSHA(root)
	if err != nil {
		return nil, false
	}
	if hdr.SourceSHAHex() != shaHex {
		return nil, false
	}
	vecs := make(map[string][EmbedDim]int8, len(entries))
	for _, e := range entries {
		vecs[e.ID] = e.Vec
	}
	return vecs, true
}
