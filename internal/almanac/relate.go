package almanac

// Relation suggestions (docs/memory-v2.md §5, T4): `memory relate
// --suggest <id>` prints the top-k vector-channel neighbours of one
// observation — cosine over retrieval.bin, same bucket decider as the
// hybrid channel (superseded excluded, self excluded), above a floor.
// Suggestion only: no relation is written, the human/agent confirms with
// a plain relate.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
)

// SuggestTopK caps the suggestion list (k=5).
const SuggestTopK = 5

// SuggestFloor is the candidate bar: similarities at or above it are
// leads worth showing. It sits below the DedupThreshold (0.92) on
// purpose — candidates are leads for a human/agent to confirm, not
// near-duplicates to refuse.
const SuggestFloor = 0.5

// Suggestion is one relation candidate: the neighbour id, its cosine
// against the source, and its title for the one-per-line output.
type Suggestion struct {
	ID     string
	Cosine float64
	Title  string
}

// SuggestRelated returns the top-k observations most similar to id by
// vector cosine, hottest first. It reuses the embed (EmbedObservation),
// index (LoadIndex) and cosine (cosineVec) helpers plus the hybrid
// bucket decider (queryMatcher.keepsBuckets with an empty query, so the
// only bucket rule is supersede-hiding) and the CheckDuplicate threshold
// thinking (a floor far below the dedup bar).
//
// Freshness is strict: unlike HybridSearch, which degrades to lexical
// with a warning, suggestions ARE the vector channel — there is no
// lexical fallback that could silently present keyword hits as vector
// similarities. A missing, stale (log changed since the reindex),
// corrupt or header-mismatched retrieval.bin refuses with a reindex
// pointer instead of lying.
func SuggestRelated(root, id string) ([]Suggestion, error) {
	all, err := All(root)
	if err != nil {
		return nil, err
	}
	var src *Observation
	for i := range all {
		if all[i].ID == id {
			src = &all[i]
			break
		}
	}
	if src == nil {
		return nil, fmt.Errorf("no observation %q", id)
	}
	entries, hdr, err := LoadIndex(root)
	if err != nil {
		return nil, fmt.Errorf("suggestions require a fresh semantic index — run `regulus memory reindex` (%v)", err)
	}
	raw, err := os.ReadFile(File(root))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if hdr.SourceSHAHex() != hex.EncodeToString(sum[:]) {
		return nil, fmt.Errorf("suggestions require a fresh semantic index — run `regulus memory reindex` (index is stale: the log changed since the last reindex)")
	}
	superseded, err := SupersededIDs(root)
	if err != nil {
		return nil, err
	}
	vecByID := make(map[string][EmbedDim]int8, len(entries))
	for _, e := range entries {
		vecByID[e.ID] = e.Vec
	}
	query, ok := vecByID[id]
	if !ok {
		query = EmbedObservation(*src)
	}
	if isZeroVec(query) {
		return nil, nil
	}
	byID := make(map[string]Observation, len(all))
	for _, o := range all {
		byID[o.ID] = o
	}
	// Same bucket decider as the hybrid vector channel: an empty query
	// keeps every bucket open, so only supersede-hiding applies here.
	matcher := queryMatcher{superseded: superseded}
	type cand struct {
		id  string
		sim float64
	}
	var cands []cand
	for _, o := range all {
		if o.ID == id {
			continue
		}
		if !matcher.keepsBuckets(o) {
			continue
		}
		v, ok := vecByID[o.ID]
		if !ok {
			continue
		}
		if sim := cosineVec(query, v); sim >= SuggestFloor {
			cands = append(cands, cand{id: o.ID, sim: sim})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].sim != cands[j].sim {
			return cands[i].sim > cands[j].sim
		}
		return cands[i].id > cands[j].id
	})
	if len(cands) > SuggestTopK {
		cands = cands[:SuggestTopK]
	}
	out := make([]Suggestion, 0, len(cands))
	for _, c := range cands {
		out = append(out, Suggestion{ID: c.id, Cosine: c.sim, Title: byID[c.id].Title})
	}
	return out, nil
}
