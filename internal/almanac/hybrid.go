// HybridSearch fuses the lexical channel (existing Search) with a vector
// channel (cosine over retrieval.bin, top-N) via RRF (k=60), per-type
// recency decay and domain-scope-first ordering. Fresh index only;
// missing/stale/corrupt degrades to lexical with degraded=true.
package almanac

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// RRFK is the reciprocal-rank-fusion constant (docs/memory-v2.md §5).
const RRFK = 60

// vectorTopN bounds the vector channel; a larger Limit raises the bound
// so high-limit hybrid still recalls.
const vectorTopN = 50

// Recency half-lives in days: notes decay fast, decisions slower,
// admitted lessons never decay.
const (
	noteHalfLifeDays     = 14.0
	decisionHalfLifeDays = 60.0
)

// HybridSearch runs hybrid retrieval when retrieval.bin is fresh
// (LoadIndex ok, sha matches the log); otherwise it returns pure lexical
// Search with degraded=true. The vector channel reuses the same bucket
// decider (keepsBuckets) as lexical — one decider, no double-hiding;
// terms rank vectors instead of filtering them.
func HybridSearch(root string, q Query) ([]Observation, bool, error) {
	return HybridSearchWith(root, q, "")
}

// HybridSearchWith runs hybrid retrieval with the named embedder (""
// means the default hash-ngram-v1): the query vector comes from that
// encoder and the index must pin the same one, or the call degrades to
// lexical (header mismatch) / refuses (unknown embedder id).
func HybridSearchWith(root string, q Query, embedderID string) ([]Observation, bool, error) {
	emb, err := LookupEmbedder(embedderID)
	if err != nil {
		return nil, false, err
	}
	return hybridAt(root, q, emb, time.Now().UTC())
}

// hybridAt is HybridSearchWith with an injectable now (decay clock).
func hybridAt(root string, q Query, emb Embedder, now time.Time) ([]Observation, bool, error) {
	entries, hdr, err := LoadIndexWith(root, emb.ID())
	if err != nil {
		return degradedLexical(root, q)
	}
	raw, err := os.ReadFile(File(root))
	if err != nil {
		return degradedLexical(root, q)
	}
	sum := sha256.Sum256(raw)
	if hdr.SourceSHAHex() != hex.EncodeToString(sum[:]) {
		return degradedLexical(root, q)
	}
	ranked, err := hybridFresh(root, q, entries, emb, now)
	if err != nil {
		return nil, false, err
	}
	return ranked, false, nil
}

// degradedLexical is the stale/missing/corrupt path: pure lexical.
func degradedLexical(root string, q Query) ([]Observation, bool, error) {
	hits, err := Search(root, q)
	if err != nil {
		return nil, false, err
	}
	return hits, true, nil
}

// hybridCorpus is the once-per-HybridSearch shared read: the parsed log
// plus pins and the supersede-hidden set. Loaded once in hybridFresh and
// shared by the lexical-rank and vector-rank paths (and by both domain
// scopes) so one HybridSearch parses the JSONL once, not twice, and reads
// pins/relations once.
type hybridCorpus struct {
	all    []Observation
	pins   map[string]bool
	hidden map[string]bool // full supersede-hidden set; nil when no relations (matches SupersededIDs)
}

// loadHybridCorpus reads the log, pins and relations once. The hidden set
// mirrors SupersededIDs exactly (nil with no relations, otherwise the
// superseder-exists-guarded set) but reuses the already-loaded log
// instead of parsing it a second time inside SupersededIDs.
func loadHybridCorpus(root string) (hybridCorpus, error) {
	all, err := All(root)
	if err != nil {
		return hybridCorpus{}, err
	}
	pins, err := PinnedIDs(root)
	if err != nil {
		return hybridCorpus{}, err
	}
	rels, err := Relations(root)
	if err != nil {
		return hybridCorpus{}, err
	}
	var hidden map[string]bool
	if len(rels) > 0 {
		exists := make(map[string]bool, len(all))
		for _, o := range all {
			exists[o.ID] = true
		}
		hidden = make(map[string]bool)
		for _, r := range rels {
			if r.Relation == RelationSupersedes && exists[r.From] && exists[r.To] {
				hidden[r.To] = true
			}
		}
	}
	return hybridCorpus{all: all, pins: pins, hidden: hidden}, nil
}

// supersededFor selects the hidden set for one query scope: empty when
// IncludeSuperseded (matches Search), otherwise the shared hidden set
// (nil with no relations, matching SupersededIDs).
func (c hybridCorpus) supersededFor(q Query) map[string]bool {
	if q.IncludeSuperseded {
		return map[string]bool{}
	}
	return c.hidden
}

// hybridFresh fuses both channels; domain queries rank scoped hits before
// the global fallback, deduped by ID, then Latest/Limit apply. The corpus
// loads once here and is shared by every rank path below.
func hybridFresh(root string, q Query, entries []IndexEntry, emb Embedder, now time.Time) ([]Observation, error) {
	corpus, err := loadHybridCorpus(root)
	if err != nil {
		return nil, err
	}
	if q.Domain != "" {
		scoped := fusedRankedWith(corpus, q, entries, emb, now)
		globalQ := q
		globalQ.Domain = ""
		global := fusedRankedWith(corpus, globalQ, entries, emb, now)
		return applyLatestLimit(mergeDomainFirst(scoped, global), q), nil
	}
	ranked := fusedRankedWith(corpus, q, entries, emb, now)
	return applyLatestLimit(ranked, q), nil
}

// mergeDomainFirst concatenates scoped hits before unseen global hits.
func mergeDomainFirst(scoped, global []Observation) []Observation {
	seen := map[string]bool{}
	out := make([]Observation, 0, len(scoped)+len(global))
	for _, o := range scoped {
		if !seen[o.ID] {
			seen[o.ID] = true
			out = append(out, o)
		}
	}
	for _, o := range global {
		if !seen[o.ID] {
			seen[o.ID] = true
			out = append(out, o)
		}
	}
	return out
}

// applyLatestLimit keeps the first hit per topic (fused order wins) then
// truncates to Limit.
func applyLatestLimit(ranked []Observation, q Query) []Observation {
	if q.Latest {
		seen := map[string]bool{}
		kept := ranked[:0]
		for _, o := range ranked {
			if o.TopicKey == "" {
				kept = append(kept, o)
				continue
			}
			if seen[o.TopicKey] {
				continue
			}
			seen[o.TopicKey] = true
			kept = append(kept, o)
		}
		ranked = kept
	}
	if q.Limit > 0 && len(ranked) > q.Limit {
		ranked = ranked[:q.Limit]
	}
	return ranked
}

// scoredObs carries one fused candidate.
type scoredObs struct {
	obs   Observation
	score float64
}

// fusedRankedWith returns the fused order for one domain scope, without
// Latest/Limit: pinned first, fused score, then confidence/recency/id.
// Pure function over the shared corpus — no I/O.
func fusedRankedWith(corpus hybridCorpus, q Query, entries []IndexEntry, emb Embedder, now time.Time) []Observation {
	lexRank := lexicalRanksWith(corpus, q)
	superseded := corpus.supersededFor(q)
	vecRank, byID := vectorRanks(q, corpus.all, entries, superseded, emb)
	scored := scoreCandidates(byID, lexRank, vecRank, now)
	sortScored(corpus.pins, scored)
	out := make([]Observation, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.obs)
	}
	return out
}

// lexicalRanksWith ranks the shared log with the exact Search order
// (same matcher.keeps buckets+terms, same sortHits pin/confidence/
// recency/id) untruncated for RRF ranks — no second log parse, no Search call.
func lexicalRanksWith(corpus hybridCorpus, q Query) map[string]int {
	qLex := q
	qLex.Limit = 0
	qLex.Latest = false
	matcher := queryMatcher{q: qLex, superseded: corpus.supersededFor(qLex)}
	var hits []Observation
	for _, o := range corpus.all {
		if !matcher.keeps(o) {
			continue
		}
		hits = append(hits, o)
	}
	sortHits(corpus.pins, hits)
	ranks := make(map[string]int, len(hits))
	for i, h := range hits {
		ranks[h.ID] = i + 1
	}
	return ranks
}

// vectorRanks scans the index for bucket-passing candidates above zero
// cosine, returning 1-indexed ranks plus the observation index.
func vectorRanks(q Query, all []Observation, entries []IndexEntry, superseded map[string]bool, emb Embedder) (map[string]int, map[string]Observation) {
	byID := make(map[string]Observation, len(all))
	for _, o := range all {
		byID[o.ID] = o
	}
	vecByID := make(map[string][EmbedDim]int8, len(entries))
	for _, e := range entries {
		vecByID[e.ID] = e.Vec
	}
	matcher := queryMatcher{q: q, superseded: superseded}
	queryVec := emb.Embed(strings.Join(q.Terms, " "))
	if isZeroVec(queryVec) {
		return map[string]int{}, byID
	}
	type cand struct {
		id  string
		sim float64
	}
	var cands []cand
	for _, o := range all {
		if !matcher.keepsBuckets(o) {
			continue
		}
		v, ok := vecByID[o.ID]
		if !ok {
			continue
		}
		if sim := cosineVec(queryVec, v); sim > 0 {
			cands = append(cands, cand{id: o.ID, sim: sim})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].sim != cands[j].sim {
			return cands[i].sim > cands[j].sim
		}
		return cands[i].id > cands[j].id
	})
	top := vectorTopN
	if q.Limit > top {
		top = q.Limit
	}
	if len(cands) > top {
		cands = cands[:top]
	}
	ranks := make(map[string]int, len(cands))
	for i, c := range cands {
		ranks[c.id] = i + 1
	}
	return ranks, byID
}

// scoreCandidates unions both channels with RRF × decay.
func scoreCandidates(byID map[string]Observation, lexRank, vecRank map[string]int, now time.Time) []scoredObs {
	ids := map[string]bool{}
	for id := range lexRank {
		ids[id] = true
	}
	for id := range vecRank {
		ids[id] = true
	}
	out := make([]scoredObs, 0, len(ids))
	for id := range ids {
		o, ok := byID[id]
		if !ok {
			continue
		}
		rrf := 0.0
		if r, ok := lexRank[id]; ok {
			rrf += 1.0 / (RRFK + float64(r))
		}
		if r, ok := vecRank[id]; ok {
			rrf += 1.0 / (RRFK + float64(r))
		}
		out = append(out, scoredObs{obs: o, score: rrf * decayFactor(o.Type, now.Sub(o.SavedAt))})
	}
	return out
}

// sortScored orders pinned first, fused score, then the existing
// confidence/recency/id tie-break.
func sortScored(pins map[string]bool, scored []scoredObs) {
	sort.Slice(scored, func(i, j int) bool {
		if pins[scored[i].obs.ID] != pins[scored[j].obs.ID] {
			return pins[scored[i].obs.ID]
		}
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		li, lj := scored[i].obs.Lesson != nil, scored[j].obs.Lesson != nil
		if li && lj && confidenceRank(scored[i].obs) != confidenceRank(scored[j].obs) {
			return confidenceRank(scored[i].obs) < confidenceRank(scored[j].obs)
		}
		if !scored[i].obs.SavedAt.Equal(scored[j].obs.SavedAt) {
			return scored[i].obs.SavedAt.After(scored[j].obs.SavedAt)
		}
		return scored[i].obs.ID > scored[j].obs.ID
	})
}

// decayFactor scales RRF by age: notes 14d half-life, decisions 60d,
// admitted lessons never decay.
func decayFactor(obsType string, age time.Duration) float64 {
	if obsType == TypeLesson {
		return 1
	}
	days := age.Hours() / 24
	if days < 0 {
		days = 0
	}
	half := noteHalfLifeDays
	switch obsType {
	case TypeDecision, TypePreference:
		half = decisionHalfLifeDays
	}
	return math.Pow(0.5, days/half)
}

// cosineVec is the cosine over int8-quantized vectors; zero on zero-norm.
func cosineVec(a, b [EmbedDim]int8) float64 {
	var dot, na, nb float64
	for i := 0; i < EmbedDim; i++ {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// isZeroVec reports an empty query embedding (no vector channel).
func isZeroVec(v [EmbedDim]int8) bool {
	for _, b := range v {
		if b != 0 {
			return false
		}
	}
	return true
}
