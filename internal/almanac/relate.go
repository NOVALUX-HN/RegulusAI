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
// Freshness is self-healing, not a refusal: a missing, stale (log
// changed since the last reindex), corrupt or header-mismatched
// retrieval.bin is rebuilt once via Reindex — a pure function of the
// log, byte-deterministic, cheap and zero-dep — and reported with
// rebuilt=true so the caller can say so. The vector channel stays honest:
// suggestions are always cosine similarities over a fresh index, never
// keyword hits dressed as similarities. An unknown id still refuses
// before any rebuild, so bad input never triggers a write; a failed
// rebuild keeps the reindex pointer instead of failing silently.
func SuggestRelated(root, id string) ([]Suggestion, bool, error) {
	all, err := All(root)
	if err != nil {
		return nil, false, err
	}
	var src *Observation
	for i := range all {
		if all[i].ID == id {
			src = &all[i]
			break
		}
	}
	if src == nil {
		return nil, false, fmt.Errorf("no observation %q", id)
	}
	entries, rebuilt, err := freshEntries(root)
	if err != nil {
		return nil, false, err
	}
	superseded, err := SupersededIDs(root)
	if err != nil {
		return nil, false, err
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
		return nil, rebuilt, nil
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
	return out, rebuilt, nil
}

// freshEntries returns the vector index when it pins the current log,
// rebuilding it once via Reindex otherwise. Reindex is a pure function
// of observations.jsonl (byte-identical rebuilds, cheap, zero-dep), so
// the repair is deterministic; a failed rebuild keeps the reindex
// pointer instead of failing silently.
func freshEntries(root string) ([]IndexEntry, bool, error) {
	if entries, hdr, err := LoadIndex(root); err == nil && shaPinned(root, hdr) {
		return entries, false, nil
	}
	// Missing, corrupt, header-mismatched or stale: rebuild once.
	if _, _, err := Reindex(root); err != nil {
		return nil, false, fmt.Errorf("suggestions require a fresh semantic index — run `regulus memory reindex` (%v)", err)
	}
	entries, hdr, err := LoadIndex(root)
	if err != nil {
		return nil, false, fmt.Errorf("suggestions require a fresh semantic index — run `regulus memory reindex` (%v)", err)
	}
	if !shaPinned(root, hdr) {
		return nil, false, fmt.Errorf("suggestions require a fresh semantic index — run `regulus memory reindex` (index is stale: the log changed since the last reindex)")
	}
	return entries, true, nil
}

// shaPinned reports whether hdr pins the current observation log.
func shaPinned(root string, hdr IndexHeader) bool {
	raw, err := os.ReadFile(File(root))
	if err != nil {
		return false
	}
	sum := sha256.Sum256(raw)
	return hdr.SourceSHAHex() == hex.EncodeToString(sum[:])
}
