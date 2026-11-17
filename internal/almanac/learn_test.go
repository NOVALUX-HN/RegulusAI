package almanac

// F12.6 cross-agent learning (NOV-101): typed learn knowledge with
// provenance, ACL-scoped retrieval, invalidation and metrics. RED:
// every behavior below fails before learn.go exists.

import (
	"strings"
	"testing"
	"time"
)

func learnSeed(t *testing.T, dir string, kind, title, content string, mut func(*Observation)) Observation {
	t.Helper()
	o := &Observation{
		Type: kind, Title: title, Content: content,
		Author: "agent-alpha", Scope: ScopeProject, Confidence: "medium",
		Provenance: &Provenance{Task: "t1"},
	}
	if kind == KindVerified {
		o.Evidence = []string{"go test ./internal/almanac/ passes"}
	}
	if mut != nil {
		mut(o)
	}
	saved, err := LearnSave(dir, o, t0)
	if err != nil {
		t.Fatalf("LearnSave %s: %v", title, err)
	}
	return saved
}

func TestLearnVerifiedRequiresEvidence(t *testing.T) {
	dir := t.TempDir()
	bad := &Observation{
		Type: KindVerified, Title: "fix", Content: "it works",
		Author: "agent-alpha", Scope: ScopeProject, Confidence: "high",
	}
	if _, err := LearnSave(dir, bad, t0); err == nil {
		t.Fatal("verified-solution without evidence must be refused")
	} else if !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("refusal must name evidence, got: %v", err)
	}
	good := &Observation{
		Type: KindVerified, Title: "fix", Content: "it works",
		Author: "agent-alpha", Scope: ScopeProject, Confidence: "high",
		Evidence: []string{"go test ./... passes"},
	}
	if _, err := LearnSave(dir, good, t0); err != nil {
		t.Fatalf("verified-solution with evidence must save: %v", err)
	}
}

func TestLearnKindValidation(t *testing.T) {
	dir := t.TempDir()
	base := func() *Observation {
		return &Observation{
			Type: KindPattern, Title: "p", Content: "c",
			Author: "agent-alpha", Scope: ScopeProject, Confidence: "medium",
		}
	}
	cases := []struct {
		name string
		mut  func(*Observation)
		want string
	}{
		{"scope required", func(o *Observation) { o.Scope = "" }, "scope"},
		{"bad scope", func(o *Observation) { o.Scope = "galaxy" }, "scope"},
		{"bad confidence", func(o *Observation) { o.Confidence = "certain" }, "confidence"},
		{"author required", func(o *Observation) { o.Author = "" }, "author"},
	}
	for _, tc := range cases {
		o := base()
		tc.mut(o)
		if _, err := LearnSave(dir, o, t0); err == nil {
			t.Fatalf("%s: must be refused", tc.name)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: error must name %q, got: %v", tc.name, tc.want, err)
		}
	}
	if _, err := LearnSave(dir, &Observation{Type: "discovery", Title: "x", Content: "y"}, t0); err == nil {
		t.Fatal("non-learn kinds must not enter through LearnSave")
	}
}

func TestLearnHypothesisNeverVerified(t *testing.T) {
	dir := t.TempDir()
	h := learnSeed(t, dir, KindHypothesis, "maybe pooling", "pooling may help", nil)
	for _, to := range []string{StatusVerified, StatusAccepted} {
		if err := RecordStatus(dir, h.ID, to, "reviewer", "looks right", t1); err == nil {
			t.Fatalf("hypothesis must never reach %s", to)
		} else if !strings.Contains(err.Error(), "hypothes") {
			t.Fatalf("refusal must name hypotheses, got: %v", err)
		}
	}
	if err := RecordStatus(dir, h.ID, StatusVerified, "reviewer", "x", t1); err == nil {
		t.Fatal("hypothesis with no evidence must still never verify")
	}
	h2 := learnSeed(t, dir, KindHypothesis, "maybe cache", "caching may help",
		func(o *Observation) { o.Evidence = []string{"a benchmark hint"} })
	if err := RecordStatus(dir, h2.ID, StatusAccepted, "reviewer", "with a hint", t1); err == nil {
		t.Fatal("hypothesis with a hint is still not a fact: ACCEPTED refused")
	}
}

func TestLearnVerificationRequiresEvidenceStatus(t *testing.T) {
	dir := t.TempDir()
	p := learnSeed(t, dir, KindPattern, "pool pattern", "pool everywhere", nil)
	if err := RecordStatus(dir, p.ID, StatusVerified, "reviewer", "trust me", t1); err == nil {
		t.Fatal("pattern without evidence must not verify")
	} else if !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("refusal must name evidence, got: %v", err)
	}
	e := learnSeed(t, dir, KindError, "nil map panic", "never write to nil maps",
		func(o *Observation) { o.Evidence = []string{"go test TestNilMap passes"} })
	if err := RecordStatus(dir, e.ID, StatusVerified, "reviewer", "repro passes", t1); err != nil {
		t.Fatalf("error with evidence must verify: %v", err)
	}
}

func TestLearnSearchVerifiedOnlyExcludesHypotheses(t *testing.T) {
	dir := t.TempDir()
	learnSeed(t, dir, KindHypothesis, "pooling hunch", "pooling may speed things", nil)
	v := learnSeed(t, dir, KindVerified, "pooling fix", "pooling speeds things", nil)
	if err := RecordStatus(dir, v.ID, StatusVerified, "reviewer", "bench passes", t1); err != nil {
		t.Fatalf("verify: %v", err)
	}
	viewer := LearnViewer{AgentID: "agent-alpha", Scopes: []string{MemoryReadScope}, Task: "t1"}
	hits, denied, err := LearnSearch(dir, LearnQuery{Terms: []string{"pooling"}, Viewer: viewer})
	if err != nil {
		t.Fatalf("LearnSearch: %v", err)
	}
	if denied != 0 {
		t.Fatalf("denied = %d, want 0", denied)
	}
	if len(hits) != 2 {
		t.Fatalf("default search shows both, got %d", len(hits))
	}
	for _, h := range hits {
		if h.Observation.Type == KindHypothesis && !h.Hypothesis {
			t.Fatal("hypothesis hit must carry the hypothesis label")
		}
		if h.Observation.Type == KindHypothesis && h.Verified {
			t.Fatal("hypothesis must never read verified")
		}
		if got := LearnLabel(h.Observation, h.Verified); h.Hypothesis && !strings.Contains(got, "no es un hecho") {
			t.Fatalf("hypothesis label must warn it is not a fact, got %q", got)
		}
	}
	got, _, err := LearnSearch(dir, LearnQuery{Terms: []string{"pooling"}, VerifiedOnly: true, Viewer: viewer})
	if err != nil {
		t.Fatalf("verified-only: %v", err)
	}
	if len(got) != 1 || got[0].Observation.ID != v.ID {
		t.Fatalf("verified-only must show exactly the verified fix, got %v", got)
	}
	if !got[0].Verified {
		t.Fatal("verified hit must carry the verified label")
	}
}

func TestLearnSearchRespectsACL(t *testing.T) {
	dir := t.TempDir()
	learnSeed(t, dir, KindPattern, "shared pattern", "visible to all", func(o *Observation) {
		o.Scope = ScopeShared
	})
	learnSeed(t, dir, KindPattern, "private pattern", "only for alpha", func(o *Observation) {
		o.Scope = ScopePrivate
	})
	learnSeed(t, dir, KindPattern, "project pattern", "needs scope", nil)
	learnSeed(t, dir, KindPattern, "task pattern", "needs same task", func(o *Observation) {
		o.Scope = ScopeTask
	})
	anon := LearnViewer{}
	hits, denied, err := LearnSearch(dir, LearnQuery{Viewer: anon})
	if err != nil {
		t.Fatalf("LearnSearch: %v", err)
	}
	if len(hits) != 1 || hits[0].Observation.Title != "shared pattern" {
		t.Fatalf("anonymous sees only shared, got %v", hits)
	}
	if denied != 3 {
		t.Fatalf("denied = %d, want 3", denied)
	}
	alpha := LearnViewer{AgentID: "agent-alpha", Scopes: []string{MemoryReadScope}, Task: "t1"}
	hits, denied, err = LearnSearch(dir, LearnQuery{Viewer: alpha})
	if err != nil {
		t.Fatalf("LearnSearch: %v", err)
	}
	if len(hits) != 4 || denied != 0 {
		t.Fatalf("author with scope sees all, got %d hits %d denied", len(hits), denied)
	}
	beta := LearnViewer{AgentID: "agent-beta", Scopes: []string{MemoryReadScope}, Task: "other"}
	hits, denied, err = LearnSearch(dir, LearnQuery{Viewer: beta})
	if err != nil {
		t.Fatalf("LearnSearch: %v", err)
	}
	titles := map[string]bool{}
	for _, h := range hits {
		titles[h.Observation.Title] = true
	}
	if !titles["shared pattern"] || !titles["project pattern"] {
		t.Fatalf("beta keeps shared+project, got %v", titles)
	}
	if titles["private pattern"] || titles["task pattern"] {
		t.Fatalf("beta must not see private/task, got %v", titles)
	}
	if denied != 2 {
		t.Fatalf("denied = %d, want 2", denied)
	}
}

func TestLearnInvalidateAndAudit(t *testing.T) {
	dir := t.TempDir()
	v := learnSeed(t, dir, KindVerified, "old fix", "used to work", nil)
	if err := RecordStatus(dir, v.ID, StatusVerified, "reviewer", "bench passes", t1); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := InvalidateKnowledge(dir, v.ID, "", "stale", t1.Add(time.Hour)); err == nil {
		t.Fatal("invalidation needs an actor")
	}
	if err := InvalidateKnowledge(dir, v.ID, "reviewer", "", t1.Add(time.Hour)); err == nil {
		t.Fatal("invalidation needs a reason")
	}
	if err := InvalidateKnowledge(dir, v.ID, "reviewer", "superseded by new runtime", t1.Add(time.Hour)); err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	st, err := EffectiveStatus(dir, v.ID)
	if err != nil || st != StatusRejected {
		t.Fatalf("status = %q, %v; want REJECTED", st, err)
	}
	hist, err := StatusHistory(dir, v.ID)
	if err != nil || len(hist) == 0 {
		t.Fatalf("audit trail must record the invalidation: %v", hist)
	}
	last := hist[len(hist)-1]
	if last.By != "reviewer" || !strings.Contains(last.Note, "superseded by new runtime") {
		t.Fatalf("audit must name actor and reason, got %+v", last)
	}
	viewer := LearnViewer{AgentID: "agent-alpha", Scopes: []string{MemoryReadScope}, Task: "t1"}
	hits, _, err := LearnSearch(dir, LearnQuery{Terms: []string{"old fix"}, Viewer: viewer})
	if err != nil {
		t.Fatalf("LearnSearch: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("invalidated knowledge hides by default, got %v", hits)
	}
	stale, _, err := LearnSearch(dir, LearnQuery{Terms: []string{"old fix"}, IncludeStale: true, Viewer: viewer})
	if err != nil {
		t.Fatalf("include-stale: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("include-stale resurfaces, got %v", stale)
	}
}

func TestLearnResolveContradiction(t *testing.T) {
	dir := t.TempDir()
	a := learnSeed(t, dir, KindPattern, "pool always", "pool every connection", nil)
	b := learnSeed(t, dir, KindPattern, "never pool", "pooling hurts here", nil)
	if err := Relate(dir, a.ID, RelationConflictsWith, b.ID, t1); err != nil {
		t.Fatalf("relate: %v", err)
	}
	if err := ResolveContradiction(dir, a.ID, b.ID, "", t1.Add(time.Hour)); err == nil {
		t.Fatal("resolution needs an actor")
	}
	if err := ResolveContradiction(dir, a.ID, "obs_missing", "reviewer", t1.Add(time.Hour)); err == nil {
		t.Fatal("resolution of unknown ids must fail")
	}
	if err := ResolveContradiction(dir, a.ID, b.ID, "reviewer", t1.Add(time.Hour)); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	viewer := LearnViewer{AgentID: "agent-alpha", Scopes: []string{MemoryReadScope}, Task: "t1"}
	hits, _, err := LearnSearch(dir, LearnQuery{Viewer: viewer})
	if err != nil {
		t.Fatalf("LearnSearch: %v", err)
	}
	ids := map[string]bool{}
	for _, h := range hits {
		ids[h.Observation.ID] = true
	}
	if !ids[a.ID] || ids[b.ID] {
		t.Fatalf("loser hides, winner stays: %v", ids)
	}
	rels, err := Relations(dir)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	found := false
	for _, r := range rels {
		if r.From == a.ID && r.To == b.ID && r.Relation == RelationSupersedes && r.By == "reviewer" {
			found = true
		}
	}
	if !found {
		t.Fatalf("resolution must record an attributed supersedes, got %+v", rels)
	}
}

func TestLearnDedup(t *testing.T) {
	dir := t.TempDir()
	first := learnSeed(t, dir, KindPattern, "pool pattern", "pool everywhere", func(o *Observation) {
		o.TopicKey = "pooling"
	})
	again, err := LearnSave(dir, &Observation{
		Type: KindPattern, Title: "pool pattern", Content: "pool everywhere",
		Author: "agent-alpha", Scope: ScopeProject, Confidence: "medium",
		TopicKey: "pooling",
	}, t0)
	if err != nil {
		t.Fatalf("identical re-save must be a no-op success: %v", err)
	}
	if again.ID != first.ID {
		t.Fatalf("identical re-save must return the same id, got %s vs %s", again.ID, first.ID)
	}
	all, err := All(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("identical re-save appends no line: %v %d", err, len(all))
	}
	rev, err := LearnSave(dir, &Observation{
		Type: KindPattern, Title: "pool pattern", Content: "pool everywhere, v2",
		Author: "agent-alpha", Scope: ScopeProject, Confidence: "medium",
		TopicKey: "pooling",
	}, t1)
	if err != nil {
		t.Fatalf("revision with topic key must land: %v", err)
	}
	if first.TopicKey != "pooling" || rev.TopicKey != "pooling" {
		t.Fatal("revision chain shares the topic key")
	}
	chain, err := History(dir, "pooling")
	if err != nil || len(chain) != 2 {
		t.Fatalf("revision chain: %v %d", err, len(chain))
	}
}

func TestLearnMetrics(t *testing.T) {
	dir := t.TempDir()
	m, err := ComputeLearnMetrics(dir, t1)
	if err != nil {
		t.Fatalf("empty metrics: %v", err)
	}
	if m.Total != 0 || m.Precision != 1 || m.UsefulRecall != 1 || m.ObsoleteRate != 0 || m.NoiseRate != 0 {
		t.Fatalf("empty store reads clean: %+v", m)
	}
	v1 := learnSeed(t, dir, KindVerified, "fix one", "pooling speeds reads", nil)
	v2 := learnSeed(t, dir, KindVerified, "fix two", "pooling speeds writes", nil)
	learnSeed(t, dir, KindHypothesis, "hunch", "pooling may speed deletes", nil)
	if err := RecordStatus(dir, v1.ID, StatusVerified, "reviewer", "bench", t1); err != nil {
		t.Fatalf("verify v1: %v", err)
	}
	if err := RecordStatus(dir, v2.ID, StatusVerified, "reviewer", "bench", t1); err != nil {
		t.Fatalf("verify v2: %v", err)
	}
	if err := InvalidateKnowledge(dir, v2.ID, "reviewer", "runtime changed", t1.Add(time.Hour)); err != nil {
		t.Fatalf("invalidate v2: %v", err)
	}
	m, err = ComputeLearnMetrics(dir, t1.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if m.Total != 3 || m.Verified != 1 || m.Hypotheses != 1 {
		t.Fatalf("counts: %+v", m)
	}
	if m.HypothesesVerified != 0 {
		t.Fatalf("no hypothesis may read verified: %+v", m)
	}
	if m.Obsolete != 1 {
		t.Fatalf("obsolete = %d, want 1", m.Obsolete)
	}
	// decided = 1 verified + 1 rejected => precision 1/2; verified total 2, active 1 => recall 1/2.
	if m.Precision != 0.5 {
		t.Fatalf("precision = %v, want 0.5", m.Precision)
	}
	if m.UsefulRecall != 0.5 {
		t.Fatalf("useful recall = %v, want 0.5", m.UsefulRecall)
	}
}

func TestLearnPruneImmunity(t *testing.T) {
	dir := t.TempDir()
	v := learnSeed(t, dir, KindVerified, "kept fix", "pooling speeds reads", nil)
	if err := RecordStatus(dir, v.ID, StatusVerified, "reviewer", "bench", t0); err != nil {
		t.Fatalf("verify: %v", err)
	}
	h := learnSeed(t, dir, KindHypothesis, "old hunch", "maybe pooling", nil)
	pruneAll := func(o Observation) bool { return false }
	removed, err := Prune(dir, pruneAll)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	for _, o := range removed {
		if o.ID == v.ID {
			t.Fatal("active verified knowledge is curated: prune must spare it like lessons")
		}
	}
	found := false
	for _, o := range removed {
		if o.ID == h.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("unverified hypotheses prune like ordinary content")
	}
}

func TestLearnSystemMapping(t *testing.T) {
	for typ, want := range map[string]string{
		KindVerified:   SystemSemantic,
		KindPattern:    SystemSemantic,
		KindError:      SystemEpisodic,
		KindWorkaround: SystemProcedural,
		KindHypothesis: SystemSemantic,
	} {
		if got := SystemOf(typ); got != want {
			t.Fatalf("SystemOf(%q) = %q, want %q", typ, got, want)
		}
	}
	if MemoryReadScope != "memory:read" {
		t.Fatalf("MemoryReadScope = %q, must mirror mcp.ScopeMemoryRead", MemoryReadScope)
	}
}
