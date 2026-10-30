package almanac

// M1 (S1) miss-rate eval: paraphrase vs typo, vector top-3 recall vs
// lexical-miss rate, on the dedup-fixture vocabulary.
//
// Corpus: the distinct a-side texts of TestDedupThresholdMatchesFixture
// (deduped by text — the fixture repeats two a-sides verbatim) plus the
// unrelated b-sides as distractors. Queries: the b-sides of the four dup
// pairs, tokenized into AND terms (the lexical default). Metrics:
//
//   - lexical-miss: the correct doc is absent from Search hits — the
//     novel word (paraphrase) or the typo'd word matches nothing.
//   - vector top-3 recall: the correct doc ranks ≤ 3 by pure cosine
//     (the vector channel), and HybridSearch (fresh index) surfaces it
//     in its top 3 end to end.
//
// The numbers pin the v2.1 ONNX decision (docs/memory-v2.md §4): hash
// covers near-paraphrase + typo, genuine rewording scores in the
// unrelated band (TestMissRateTrueParaphraseGap).

import (
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"
)

// missDoc is one corpus entry keyed for the eval.
type missDoc struct {
	key     string
	title   string
	content string
}

// missCorpus are the distinct fixture a-sides plus distractors.
var missCorpus = []missDoc{
	{key: "pooling", title: "chose postgres connection pooling", content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count"},
	{key: "leak", title: "postgres pool leaks idle connections", content: "the harvester stalls under load and idle connections never return; restart recovers the pool"},
	{key: "cache", title: "cache the retrieval index in memory", content: "sizing the cache to the working set keeps p95 under fifty milliseconds"},
	{key: "runtime", title: "chose bun runtime for the plugin host", content: "cold start under fifty milliseconds keeps the picker responsive"},
	{key: "trace", title: "trace sessions prune to the call-order tail", content: "keeping the last two calls per second keeps the debug trail short"},
}

// missQuery is one eval query: a fixture dup pair b-side plus its target.
type missQuery struct {
	name             string
	kind             string // paraphrase | typo
	bTitle, bContent string
	target           string
}

// missQueries are the four dup pairs of the dedup fixture.
var missQueries = []missQuery{
	{
		name: "paraphrase/postgres-pool", kind: "paraphrase",
		bTitle:   "picked postgres connection pooling",
		bContent: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		target:   "pooling",
	},
	{
		name: "paraphrase/pool-leak", kind: "paraphrase",
		bTitle:   "postgres pool leaking idle connections",
		bContent: "the harvester stalls under load and idle connections never return; a restart recovers the pool",
		target:   "leak",
	},
	{
		name: "typo/pooling", kind: "typo",
		bTitle:   "chose postgres conenction pooling",
		bContent: "pooling keeps p95 latnecy flat under load; sized the pool to twice the worker count",
		target:   "pooling",
	},
	{
		name: "typo/cache-sizing", kind: "typo",
		bTitle:   "cache the retrival index in memory",
		bContent: "sizing the cache to the working set keeps p95 under fifty miliseconds",
		target:   "cache",
	},
}

// missTerms tokenizes a query side the way the eval feeds Search: lower
// fields, punctuation stripped (matchTerms is substring-per-term, so a
// typo'd or novel word matches nothing — the lexical miss).
func missTerms(title, content string) []string {
	var out []string
	for _, f := range strings.Fields(strings.ToLower(title + " " + content)) {
		w := strings.TrimFunc(f, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// missSeedCorpus saves the eval corpus, returning ids by key.
func missSeedCorpus(t *testing.T, root string) map[string]string {
	t.Helper()
	ids := map[string]string{}
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	for i, d := range missCorpus {
		saved, err := Save(root, &Observation{Type: TypeDecision, Title: d.title, Content: d.content}, base.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		ids[d.key] = saved.ID
	}
	return ids
}

// vectorRankOf ranks the whole log by cosine against the query text,
// returning the 1-indexed rank of id (0 when absent).
func vectorRankOf(t *testing.T, root, query, id string) int {
	t.Helper()
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	qv := Embed(query)
	type cand struct {
		cid string
		sim float64
	}
	var cands []cand
	for _, o := range all {
		cands = append(cands, cand{o.ID, cosineVec(qv, EmbedObservation(o))})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].sim > cands[j].sim })
	for i, c := range cands {
		if c.cid == id {
			return i + 1
		}
	}
	return 0
}

// TestMissRateParaphraseVsTypo is the S1 gate: on the fixture vocabulary
// the lexical channel misses every dup query (AND terms carry the novel
// or typo'd word) while the vector channel recalls every one in its
// top 3 — split paraphrase vs typo, numbers logged for the v2.1 call.
func TestMissRateParaphraseVsTypo(t *testing.T) {
	root := t.TempDir()
	ids := missSeedCorpus(t, root)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	paraHit, paraTotal := 0, 0
	typoHit, typoTotal := 0, 0
	lexMiss := 0
	for _, mq := range missQueries {
		terms := missTerms(mq.bTitle, mq.bContent)
		lex, err := Search(root, Query{Terms: terms})
		if err != nil {
			t.Fatal(err)
		}
		lexHit := false
		for _, o := range lex {
			if o.ID == ids[mq.target] {
				lexHit = true
			}
		}
		if !lexHit {
			lexMiss++
		}
		sim := cosineVec(Embed(mq.bTitle+"\n"+mq.bContent), Embed(missTargetText(mq.target)))
		rank := vectorRankOf(t, root, mq.bTitle+"\n"+mq.bContent, ids[mq.target])
		recalled := rank >= 1 && rank <= 3
		hyb, degraded, err := HybridSearchWith(root, Query{Terms: terms, Limit: 3}, "")
		if err != nil {
			t.Fatal(err)
		}
		if degraded {
			t.Fatalf("query %q: fresh index must not degrade", mq.name)
		}
		hybTop3 := false
		for _, o := range hyb {
			if o.ID == ids[mq.target] {
				hybTop3 = true
			}
		}
		if !recalled {
			t.Errorf("query %q (%s): sim=%.4f vector rank=%d, want top-3 recall", mq.name, mq.kind, sim, rank)
		}
		if !hybTop3 {
			t.Errorf("query %q (%s): hybrid top-3 misses the target (lexHit=%v vecRank=%d)", mq.name, mq.kind, lexHit, rank)
		}
		if lexHit {
			t.Errorf("query %q (%s): lexical AND query must miss the target (the novel/typo word matches nothing)", mq.name, mq.kind)
		}
		switch mq.kind {
		case "paraphrase":
			paraTotal++
			if recalled {
				paraHit++
			}
		case "typo":
			typoTotal++
			if recalled {
				typoHit++
			}
		}
		t.Logf("miss %-26s kind=%-10s sim=%.4f lexHit=%v vecRank=%d hybridTop3=%v", mq.name, mq.kind, sim, lexHit, rank, hybTop3)
	}
	t.Logf("miss-rate: paraphrase vector top-3 recall %d/%d, typo vector top-3 recall %d/%d, lexical-miss %d/%d",
		paraHit, paraTotal, typoHit, typoTotal, lexMiss, len(missQueries))
	if paraHit != paraTotal || typoHit != typoTotal {
		t.Fatalf("vector top-3 recall must be 100%% in both bands: paraphrase %d/%d typo %d/%d", paraHit, paraTotal, typoHit, typoTotal)
	}
	if lexMiss != len(missQueries) {
		t.Fatalf("lexical-miss rate must be 100%% on the fixture queries: %d/%d", lexMiss, len(missQueries))
	}
}

// missTargetText returns the retrieval text of one corpus key.
func missTargetText(key string) string {
	for _, d := range missCorpus {
		if d.key == key {
			return d.title + "\n" + d.content
		}
	}
	return ""
}

// TestMissRateTrueParaphraseGap pins the ONNX-defer evidence: a genuine
// rewording (no shared content words beyond stopwords) scores in the
// unrelated band — the hash embedder is blind to it, and the dedup bar
// stays far above. v2.1 ONNX waits for a real paraphrase eval set; this
// test documents the gap without adding one word of onnx plumbing.
func TestMissRateTrueParaphraseGap(t *testing.T) {
	root := t.TempDir()
	missSeedCorpus(t, root)
	best, bestKey := 0.0, ""
	for _, d := range missCorpus {
		sim := cosineVec(
			Embed("went with pgbouncer style multiplexing\nmultiplexing keeps tail latency flat when busy"),
			Embed(d.title+"\n"+d.content),
		)
		t.Logf("gap candidate %-8s sim=%.4f", d.key, sim)
		if sim > best {
			best, bestKey = sim, d.key
		}
	}
	t.Logf("gap: true-rewording best sim=%.4f (%s) vs unrelated band 0.1193–0.2565, dedup bar %.2f", best, bestKey, DedupThreshold)
	if best >= SuggestFloor {
		t.Fatalf("true rewording must stay below the suggestion floor %.2f (hash is blind to it): best=%.4f (%s)", SuggestFloor, best, bestKey)
	}
}
