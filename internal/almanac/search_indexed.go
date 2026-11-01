package almanac

// Bounded reads (M2): when retrieval.bin v2 is fresh, Search, History
// and the hybrid corpus load read only the candidate byte windows
// (offset seeks + exact verify) instead of full-scanning the JSONL.
// Buckets the record pins (topic/domain/type/lesson-dimension, plus
// system derived from type) prefilter in memory; every other bucket
// (path, superseded, stale, expiry) and the terms verify on the lines
// actually read. Any skew (log rewritten without reindex, torn read)
// falls back to the legacy full-scan path — conducta idéntica by
// construction (the ranking code is shared, candidates iterate in file
// order) and by parity tests. Trigrams/postings deliberately NOT built:
// offsets + verify suffice at this scale (ADR M1).

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"time"
)

// freshEntriesIfPinned returns the v2 records only when the index is
// fresh (loads clean AND pins the current log). Anything else —
// missing, corrupt, header-mismatched, stale — reports false and the
// caller runs the legacy path.
func freshEntriesIfPinned(root string) ([]IndexEntry, bool) {
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
	return entries, true
}

// coverRecords prefilters records by the index-coverable buckets of one
// query: topic, domain, type, lesson-dimension and system (derived from
// type via SystemOf, the same decider keepsBuckets uses). Terms, path,
// superseded, stale and expiry are NOT coverable — they verify
// post-read. The surviving set is a superset of keeps-passing: an empty
// cover means an empty legacy result, never a missed hit.
func coverRecords(entries []IndexEntry, q Query) []IndexEntry {
	out := make([]IndexEntry, 0, len(entries))
	for _, e := range entries {
		if q.TopicKey != "" && e.TopicKey != q.TopicKey {
			continue
		}
		if q.Domain != "" && e.Domain != q.Domain {
			continue
		}
		if q.Type != "" && e.Type != q.Type {
			continue
		}
		if q.Dimension != "" && e.Dimension != q.Dimension {
			continue
		}
		if q.System != "" && SystemOf(e.Type) != q.System {
			continue
		}
		out = append(out, e)
	}
	return out
}

// byFileOrder sorts candidate records into log order so the shared
// ranking loop iterates exactly like the legacy full scan.
func byFileOrder(recs []IndexEntry) {
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].Offset != recs[j].Offset {
			return recs[i].Offset < recs[j].Offset
		}
		return recs[i].ID < recs[j].ID
	})
}

// searchIndexed serves one Search from the offset table when fresh,
// reporting the lines parsed. served=false means "run the legacy path"
// (stale/missing index or any skew mid-read — never an error: the
// legacy path decides errors).
func searchIndexed(root string, q Query) (hits []Observation, lines int, served bool) {
	entries, ok := freshEntriesIfPinned(root)
	if !ok {
		return nil, 0, false
	}
	cands := coverRecords(entries, q)
	byFileOrder(cands)
	all, err := readByOffsets(root, cands)
	if err != nil {
		return nil, 0, false
	}
	ranked, err := rankFiltered(all, root, q)
	if err != nil {
		return nil, 0, false
	}
	return ranked, len(cands), true
}

// historyIndexed serves one topic chain from the offset table when
// fresh, in file order (oldest first) like the legacy path.
func historyIndexed(root, topicKey string) ([]Observation, bool) {
	entries, ok := freshEntriesIfPinned(root)
	if !ok {
		return nil, false
	}
	cands := coverRecords(entries, Query{TopicKey: topicKey})
	byFileOrder(cands)
	chain, err := readByOffsets(root, cands)
	if err != nil {
		return nil, false
	}
	return chain, true
}

// indexedCorpusFor builds the hybrid corpus from offset reads when the
// caller already holds a fresh index: the union of the scoped and
// global coverable sets (domain queries rank scoped-before-global from
// the same corpus), read once in file order. served=false falls back to
// loadHybridCorpus. Pins/relations/status load exactly like the legacy
// path (same files, same errors); hidden/stale derive from the index id
// set instead of a second full log parse — the same sets (parity-tested
// via scanLog↔All), without the second scan.
func indexedCorpusFor(root string, q Query, entries []IndexEntry, now time.Time) (hybridCorpus, bool, error) {
	pins, err := PinnedIDs(root)
	if err != nil {
		return hybridCorpus{}, false, err
	}
	rels, err := Relations(root)
	if err != nil {
		return hybridCorpus{}, false, err
	}
	ends, err := statusEnds(root)
	if err != nil {
		return hybridCorpus{}, false, err
	}
	stale := map[string]bool{}
	var hidden map[string]bool
	if len(rels) > 0 {
		exists := make(map[string]bool, len(entries))
		for _, e := range entries {
			exists[e.ID] = true
		}
		hidden = make(map[string]bool)
		for _, r := range rels {
			if !exists[r.From] || !exists[r.To] {
				continue
			}
			switch r.Relation {
			case RelationSupersedes:
				hidden[r.To] = true
			case RelationInvalidatedBy:
				stale[r.To] = true
			}
		}
	}
	union := coverUnion(entries, q)
	byFileOrder(union)
	all, err := readByOffsets(root, union)
	if err != nil {
		return hybridCorpus{}, false, nil
	}
	for _, o := range all {
		if ends[o.ID] == StatusRejected {
			stale[o.ID] = true
		}
		if o.ExpiresAt != nil && !now.Before(*o.ExpiresAt) {
			stale[o.ID] = true
		}
	}
	if len(stale) == 0 {
		stale = nil
	}
	return hybridCorpus{all: all, pins: pins, hidden: hidden, stale: stale}, true, nil
}

// coverUnion is the scoped ∪ global coverable set, deduped by id: the
// domain path ranks scoped-before-global from one shared read.
func coverUnion(entries []IndexEntry, q Query) []IndexEntry {
	seen := map[string]bool{}
	var union []IndexEntry
	for _, scope := range []Query{q, withoutDomain(q)} {
		for _, e := range coverRecords(entries, scope) {
			if !seen[e.ID] {
				seen[e.ID] = true
				union = append(union, e)
			}
		}
	}
	return union
}

// withoutDomain clears the domain scope (the global fallback).
func withoutDomain(q Query) Query {
	q.Domain = ""
	return q
}

// SearchWithStats serves Search and reports how it read: lines is the
// JSONL lines parsed (candidates on the indexed path, the whole log on
// legacy), indexed tells which path served. The tripwire for "lecturas
// acotadas": bucket-scoped queries on a big log must read << total.
func SearchWithStats(root string, q Query) (hits []Observation, lines int, indexed bool, err error) {
	if h, n, served := searchIndexed(root, q); served {
		return h, n, true, nil
	}
	all, err := All(root)
	if err != nil {
		return nil, 0, false, err
	}
	ranked, err := rankFiltered(all, root, q)
	if err != nil {
		return nil, 0, false, err
	}
	return ranked, len(all), false, nil
}
