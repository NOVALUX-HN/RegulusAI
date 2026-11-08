package cli

import (
	"os"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
)

func retrievalSeed(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	mustSave := func(typ, title, content string, extra ...string) {
		t.Helper()
		args := []string{"memory", "save", "--type", typ, "--title", title}
		args = append(args, extra...)
		args = append(args, content)
		c, _, _ := newCLI(nil)
		if code := c.Run(args); code != 0 {
			t.Fatalf("save %s: exit = %d", title, code)
		}
	}
	mustSave("discovery", "postgres pooling", "pooling everywhere")
	mustSave("session-note", "standup", "pooling discussed")
	mustSave("discovery", "old news", "pooling retired", "--expires", "1ns")
}

func TestMemorySearchSystemFilter(t *testing.T) {
	retrievalSeed(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "--system", "semantic", "pooling"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "postgres pooling") || strings.Contains(out.String(), "standup") {
		t.Fatalf("system filter must isolate, got:\n%s", out.String())
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "--system", "vibes", "x"}); code != 2 {
		t.Fatalf("bad system must be usage error, got %d (%q)", code, errb.String())
	}
}

func TestMemorySearchStaleHiddenByDefault(t *testing.T) {
	retrievalSeed(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "--lexical", "retired"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out.String(), "old news") {
		t.Fatalf("expired must hide by default, got:\n%s", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "--lexical", "--include-stale", "retired"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "old news") {
		t.Fatalf("include-stale must resurface, got:\n%s", out2.String())
	}
}

func TestMemorySearchHybridHonorsStale(t *testing.T) {
	retrievalSeed(t)
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex exit = %d", code)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "retired"}); code != 0 {
		t.Fatalf("exit = %d (%s)", code, errb.String())
	}
	if strings.Contains(out.String(), "old news") {
		t.Fatalf("hybrid must exclude expired too, got:\n%s", out.String())
	}
}

func TestMemoryContextScoped(t *testing.T) {
	retrievalSeed(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--system", "semantic", "--budget", "10"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	s := out.String()
	if !strings.Contains(s, "scoped: system=semantic") {
		t.Fatalf("scope must be announced, got:\n%s", s)
	}
	if strings.Contains(s, "standup") {
		t.Fatalf("system scope must filter notes, got:\n%s", s)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "context", "--budget", "1"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "narrow with") && !strings.Contains(out2.String(), "showing latest 1 of") {
		t.Fatalf("budget must cap with a pointer, got:\n%s", out2.String())
	}
}

func TestMemoryRetrievalCarriesTrustBanner(t *testing.T) {
	retrievalSeed(t)
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "--lexical", "pooling"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errb.String(), "DATA") {
		t.Fatalf("search must frame content as untrusted DATA, stderr = %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"memory", "show"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errb2.String(), "DATA") {
		t.Fatalf("show must frame content as untrusted DATA, stderr = %q", errb2.String())
	}
}

func TestMemoryContextInvalidSystem(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--system", "nope"}); code != 2 {
		t.Fatalf("exit = %d, want 2 (%q)", code, errb.String())
	}
}

// TestMemorySearchStalenessProbesCached (T3 S2-S3): N hits sharing one
// git head must cost 1 rev-list probe (process-life cache), and
// --no-staleness must cost zero probes on search and context while
// keeping the default annotated.
func TestMemorySearchStalenessProbesCached(t *testing.T) {
	resetStalenessCache()
	t.Cleanup(resetStalenessCache)
	t.Chdir(t.TempDir())
	const head = "ca1ab1e5"
	for i, title := range []string{"cacheme alpha", "cacheme beta", "cacheme gamma"} {
		if _, err := almanac.Save(".", &almanac.Observation{
			Type: almanac.TypeDecision, Title: title, Content: "cacheme content",
			Provenance: &almanac.Provenance{GitHead: head},
		}, time.Unix(1700000000+int64(i), 0).UTC()); err != nil {
			t.Fatal(err)
		}
	}
	countRevList := func(r *fakeGateRunner) int {
		n := 0
		for _, call := range r.calls {
			if strings.HasPrefix(call, "git rev-list") {
				n++
			}
		}
		return n
	}
	// Default stays annotated: 3 hits, 1 shared head = 1 probe.
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-list --count " + head + "..HEAD": {stdout: "5\n"},
	}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"memory", "search", "--lexical", "cacheme"}); code != 0 {
		t.Fatalf("search exit = %d", code)
	}
	if !strings.Contains(out.String(), "5 commits behind") {
		t.Fatalf("default search must stay annotated, got:\n%s", out.String())
	}
	if got := countRevList(r); got != 1 {
		t.Fatalf("3 hits sharing one head = %d rev-list probes, want 1", got)
	}
	// --no-staleness: zero probes, no annotation.
	r2 := &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-list --count " + head + "..HEAD": {stdout: "5\n"},
	}}
	c2, out2, _ := newCLI(nil)
	c2.Runner = r2
	if code := c2.Run([]string{"memory", "search", "--lexical", "--no-staleness", "cacheme"}); code != 0 {
		t.Fatalf("search --no-staleness exit = %d", code)
	}
	if strings.Contains(out2.String(), "commits behind") {
		t.Fatalf("--no-staleness must skip annotations, got:\n%s", out2.String())
	}
	if got := countRevList(r2); got != 0 {
		t.Fatalf("--no-staleness = %d rev-list probes, want 0", got)
	}
	// --no-staleness on context: accepted, zero probes.
	r3 := &fakeGateRunner{}
	c3, _, _ := newCLI(nil)
	c3.Runner = r3
	if code := c3.Run([]string{"memory", "context", "--no-staleness"}); code != 0 {
		t.Fatalf("context --no-staleness exit = %d, want 0", code)
	}
	if got := countRevList(r3); got != 0 {
		t.Fatalf("context --no-staleness = %d rev-list probes, want 0", got)
	}
}

// TestMemoryDeferReindexBurstStaysStale (T4 S2-S3): a burst of N writes
// under REGULUS_DEFER_REINDEX=1 must perform zero synchronous reindexes
// (retrieval.bin bytes untouched, IndexState stale) while search still
// answers lexically with the stale-index warning. An explicit reindex
// then restores fresh in one shot.
func TestMemoryDeferReindexBurstStaysStale(t *testing.T) {
	t.Chdir(t.TempDir())
	c0, _, _ := newCLI(nil)
	if code := c0.Run([]string{"memory", "save", "--type", "decision", "--title", "base choice", "pooling baseline"}); code != 0 {
		t.Fatalf("seed save: exit = %d", code)
	}
	if state, err := almanac.IndexState("."); err != nil || state != "fresh" {
		t.Fatalf("seed must leave fresh index, state = %q, err = %v", state, err)
	}
	before, err := os.ReadFile(almanac.IndexPath("."))
	if err != nil {
		t.Fatalf("read index before burst: %v", err)
	}
	t.Setenv("REGULUS_DEFER_REINDEX", "1")
	burst := []string{"burst alpha", "burst beta", "burst gamma", "burst delta", "burst epsilon"}
	for i, title := range burst {
		c, _, _ := newCLI(nil)
		var code int
		if i%2 == 0 {
			code = c.Run([]string{"memory", "save", "--type", "discovery", "--title", title, "pooling " + title})
		} else {
			code = c.Run([]string{"memory", "note", "pooling " + title})
		}
		if code != 0 {
			t.Fatalf("burst write %q: exit = %d", title, code)
		}
	}
	after, err := os.ReadFile(almanac.IndexPath("."))
	if err != nil {
		t.Fatalf("read index after burst: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("deferred burst must perform zero synchronous reindexes (retrieval.bin untouched)")
	}
	if state, err := almanac.IndexState("."); err != nil || state != "stale" {
		t.Fatalf("deferred burst must leave index stale, state = %q, err = %v", state, err)
	}
	cS, outS, errS := newCLI(nil)
	if code := cS.Run([]string{"memory", "search", "burst"}); code != 0 {
		t.Fatalf("deferred search: exit = %d (%s)", code, errS.String())
	}
	if !strings.Contains(outS.String(), "burst") {
		t.Fatalf("deferred search must still answer lexically, got:\n%s", outS.String())
	}
	if !strings.Contains(errS.String(), "warning: semantic index stale or missing") {
		t.Fatalf("deferred search must warn stale (honest lexical), stderr = %q", errS.String())
	}
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("explicit reindex: exit = %d", code)
	}
	if state, err := almanac.IndexState("."); err != nil || state != "fresh" {
		t.Fatalf("explicit reindex must restore fresh, state = %q, err = %v", state, err)
	}
}
