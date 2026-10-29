package cli

// T4 (S9) `memory relate --suggest <id>`: vector candidates print top-k
// above the floor, one per line `id cosine title`; nothing is written.
// Missing/stale index refuses on stderr; the plain relate path is
// untouched; usage mentions --suggest.

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
)

func relateSuggestSeed(t *testing.T, o *almanac.Observation, at time.Time) almanac.Observation {
	t.Helper()
	// Seed through almanac.Save directly: the CLI save boundary enforces
	// save-time dedup, which would refuse the deliberate paraphrases this
	// suite suggests among.
	saved, err := almanac.Save(".", o, at)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func TestMemoryRelateSuggestRanksParaphrase(t *testing.T) {
	t.Chdir(t.TempDir())
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := relateSuggestSeed(t, &almanac.Observation{
		Type:    almanac.TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	para := relateSuggestSeed(t, &almanac.Observation{
		Type:    almanac.TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base.Add(time.Hour))
	relateSuggestSeed(t, &almanac.Observation{
		Type:    almanac.TypeDecision,
		Title:   "chose bun runtime for the plugin host",
		Content: "cold start under fifty milliseconds keeps the picker responsive",
	}, base.Add(2*time.Hour))
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "relate", "--suggest", src.ID}); code != 0 {
		t.Fatalf("suggest: exit = %d, stderr = %q", code, errb.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("suggest must print candidates, got %q", out.String())
	}
	fields := strings.SplitN(lines[0], " ", 3)
	if len(fields) != 3 {
		t.Fatalf("first line must be `id cosine title`, got %q", lines[0])
	}
	if fields[0] != para.ID {
		t.Fatalf("paraphrase must rank first: got %q, want %q (all: %q)", fields[0], para.ID, out.String())
	}
	sim, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		t.Fatalf("cosine must parse: %q (%v)", fields[1], err)
	}
	if sim < 0.5 {
		t.Fatalf("printed cosine must clear the 0.5 floor: got %v", sim)
	}
	if fields[2] != para.Title {
		t.Fatalf("line must end with the title: got %q", lines[0])
	}
	for _, ln := range lines {
		if strings.HasPrefix(ln, src.ID+" ") {
			t.Fatalf("suggestion must exclude self: %q", out.String())
		}
	}
	// Suggestion writes nothing.
	rels, err := almanac.Relations(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 0 {
		t.Fatalf("suggest must write no relation: %+v", rels)
	}
}

func TestMemoryRelateSuggestExcludesSuperseded(t *testing.T) {
	t.Chdir(t.TempDir())
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := relateSuggestSeed(t, &almanac.Observation{
		Type:    almanac.TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	live := relateSuggestSeed(t, &almanac.Observation{
		Type:    almanac.TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base.Add(time.Hour))
	hidden := relateSuggestSeed(t, &almanac.Observation{
		Type:    almanac.TypeDecision,
		Title:   "chose postgres conenction pooling",
		Content: "pooling keeps p95 latnecy flat under load; sized the pool to twice the worker count",
	}, base.Add(2*time.Hour))
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	cH, _, _ := newCLI(nil)
	if code := cH.Run([]string{"memory", "relate", "--id", live.ID, "--relation", "supersedes", "--to", hidden.ID}); code != 0 {
		t.Fatalf("relate: exit = %d", code)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "relate", "--suggest", src.ID}); code != 0 {
		t.Fatalf("suggest: exit = %d, stderr = %q", code, errb.String())
	}
	if strings.Contains(out.String(), hidden.ID) {
		t.Fatalf("superseded target must be excluded: %q", out.String())
	}
	if !strings.Contains(out.String(), live.ID) {
		t.Fatalf("live paraphrase must be suggested: %q", out.String())
	}
}

func TestMemoryRelateSuggestFloorFiltersUnrelated(t *testing.T) {
	t.Chdir(t.TempDir())
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := relateSuggestSeed(t, &almanac.Observation{
		Type:    almanac.TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	relateSuggestSeed(t, &almanac.Observation{
		Type:    almanac.TypeDecision,
		Title:   "chose bun runtime for the plugin host",
		Content: "cold start under fifty milliseconds keeps the picker responsive",
	}, base.Add(time.Hour))
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "relate", "--suggest", src.ID}); code != 0 {
		t.Fatalf("suggest: exit = %d, stderr = %q", code, errb.String())
	}
	if strings.Contains(out.String(), "obs_") {
		t.Fatalf("floor must filter unrelated observations: %q", out.String())
	}
}

func TestMemoryRelateSuggestMissingIndexRefuses(t *testing.T) {
	t.Chdir(t.TempDir())
	src := relateSuggestSeed(t, &almanac.Observation{
		Type: almanac.TypeDecision, Title: "chose postgres", Content: "pooling everywhere",
	}, time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC))
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "relate", "--suggest", src.ID}); code == 0 {
		t.Fatal("missing index must refuse with a non-zero exit")
	}
	if !strings.Contains(strings.ToLower(errb.String()), "reindex") {
		t.Fatalf("missing-index refusal must point at reindex, got %q", errb.String())
	}
}

func TestMemoryRelateSuggestStaleIndexRefuses(t *testing.T) {
	t.Chdir(t.TempDir())
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := relateSuggestSeed(t, &almanac.Observation{
		Type: almanac.TypeDecision, Title: "chose postgres", Content: "pooling everywhere",
	}, base)
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	relateSuggestSeed(t, &almanac.Observation{
		Type: almanac.TypeDecision, Title: "second choice", Content: "more content",
	}, base.Add(time.Hour))
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "relate", "--suggest", src.ID}); code == 0 {
		t.Fatal("stale index must refuse with a non-zero exit")
	}
	if !strings.Contains(strings.ToLower(errb.String()), "reindex") {
		t.Fatalf("stale-index refusal must point at reindex, got %q", errb.String())
	}
}

func TestMemoryRelatePlainPathUntouched(t *testing.T) {
	t.Chdir(t.TempDir())
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	a := relateSuggestSeed(t, &almanac.Observation{Type: almanac.TypeDecision, Title: "v1", Content: "x"}, base)
	b := relateSuggestSeed(t, &almanac.Observation{Type: almanac.TypeDecision, Title: "v2", Content: "y"}, base.Add(time.Hour))
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "relate", "--id", a.ID, "--relation", "related", "--to", b.ID}); code != 0 {
		t.Fatalf("plain relate: exit = %d", code)
	}
	if !strings.Contains(out.String(), "recorded:") {
		t.Fatalf("plain relate output = %q", out.String())
	}
	rels, err := almanac.Relations(".")
	if err != nil || len(rels) != 1 {
		t.Fatalf("plain relate must still record: %+v (%v)", rels, err)
	}
}

func TestMemoryRelateUsageMentionsSuggest(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "relate"}); code != 2 {
		t.Fatalf("bare relate: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--suggest") {
		t.Fatalf("relate usage must mention --suggest, got %q", errb.String())
	}
}
