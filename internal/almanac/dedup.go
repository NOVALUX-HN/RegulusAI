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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// DedupThreshold is the save-time near-duplicate bar: a candidate whose
// best cosine against the live log exceeds it is refused.
const DedupThreshold = 0.92

// DuplicateError is the save-time refusal: the candidate duplicates ID
// at Similarity, and the guidance points at the topic-key escape hatch.
type DuplicateError struct {
	ID         string
	Similarity float64
}

// Error names the near-duplicate id and hints at setting a topic key to
// link the save as a revision.
func (e *DuplicateError) Error() string {
	return fmt.Sprintf("refusing near-duplicate save (matches %s at similarity %.2f, threshold %.2f) — set a topic key to link it as a revision", e.ID, e.Similarity, DedupThreshold)
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
	bestID, best := "", 0.0
	for _, o := range all {
		if superseded[o.ID] {
			continue
		}
		if sim := cosineVec(query, vecs[o.ID]); sim > best {
			best, bestID = sim, o.ID
		}
	}
	if best > DedupThreshold {
		return &DuplicateError{ID: bestID, Similarity: best}
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
	raw, err := os.ReadFile(File(root))
	if err != nil {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	if hdr.SourceSHAHex() != hex.EncodeToString(sum[:]) {
		return nil, false
	}
	vecs := make(map[string][EmbedDim]int8, len(entries))
	for _, e := range entries {
		vecs[e.ID] = e.Vec
	}
	return vecs, true
}
