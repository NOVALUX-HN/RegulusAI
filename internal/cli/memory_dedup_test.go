package cli

// T3 (S2+S5) save-time dedup at the CLI: refusals exit non-zero with the
// near-duplicate id on stderr and leave the log untouched; clean saves,
// topic-key revisions and superseded-hiding compositions still append.

import (
	"strings"
	"testing"

	"regulus-ai/internal/almanac"
)

func TestMemorySaveRefusesParaphrase(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision",
		"--title", "chose postgres connection pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"}); code != 0 {
		t.Fatalf("seed save: exit = %d", code)
	}
	seeded, err := almanac.All(".")
	if err != nil || len(seeded) != 1 {
		t.Fatalf("seed = %+v (%v)", seeded, err)
	}
	c2, _, errb := newCLI(nil)
	code := c2.Run([]string{"memory", "save", "--type", "decision",
		"--title", "picked postgres connection pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"})
	if code == 0 {
		t.Fatal("paraphrase save must be refused with a non-zero exit")
	}
	if !strings.Contains(errb.String(), seeded[0].ID) {
		t.Fatalf("refusal must name the near-duplicate id %s, got %q", seeded[0].ID, errb.String())
	}
	if !strings.Contains(errb.String(), "topic key") {
		t.Fatalf("refusal must hint at setting a topic key, got %q", errb.String())
	}
	after, err := almanac.All(".")
	if err != nil || len(after) != 1 {
		t.Fatalf("refused save must leave stored data untouched: %+v (%v)", after, err)
	}
}

func TestMemorySaveRefusesTypo(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision",
		"--title", "cache the retrieval index in memory",
		"sizing the cache to the working set keeps p95 under fifty milliseconds"}); code != 0 {
		t.Fatalf("seed save: exit = %d", code)
	}
	seeded, err := almanac.All(".")
	if err != nil || len(seeded) != 1 {
		t.Fatalf("seed = %+v (%v)", seeded, err)
	}
	c2, _, errb := newCLI(nil)
	code := c2.Run([]string{"memory", "save", "--type", "decision",
		"--title", "cache the retrival index in memory",
		"sizing the cache to the working set keeps p95 under fifty miliseconds"})
	if code == 0 {
		t.Fatal("typo save must be refused with a non-zero exit")
	}
	if !strings.Contains(errb.String(), seeded[0].ID) {
		t.Fatalf("refusal must name the near-duplicate id %s, got %q", seeded[0].ID, errb.String())
	}
	after, err := almanac.All(".")
	if err != nil || len(after) != 1 {
		t.Fatalf("refused save must leave stored data untouched: %+v (%v)", after, err)
	}
}

func TestMemorySaveUnrelatedSavesCleanly(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision",
		"--title", "chose postgres connection pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"}); code != 0 {
		t.Fatalf("save 1: exit = %d", code)
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "save", "--type", "decision",
		"--title", "chose bun runtime for the plugin host",
		"cold start under fifty milliseconds keeps the picker responsive"}); code != 0 {
		t.Fatalf("unrelated save 2: exit = %d", code)
	}
	all, err := almanac.All(".")
	if err != nil || len(all) != 2 {
		t.Fatalf("both unrelated saves must persist: %+v (%v)", all, err)
	}
}

func TestMemorySaveTopicKeyBypassAppends(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision",
		"--title", "chose postgres connection pooling", "--topic-key", "pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"}); code != 0 {
		t.Fatalf("seed save: exit = %d", code)
	}
	// Declared grouping wins: same chain appends as a revision.
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"memory", "save", "--type", "decision",
		"--title", "picked postgres connection pooling", "--topic-key", "pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"}); code != 0 {
		t.Fatalf("same topic-key revision must append: exit = %d, stderr = %q", code, errb.String())
	}
	chain, err := almanac.History(".", "pooling")
	if err != nil || len(chain) != 2 {
		t.Fatalf("chain must hold both revisions: %+v (%v)", chain, err)
	}
}

func TestMemorySaveSkipsSuperseded(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision",
		"--title", "chose postgres connection pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"}); code != 0 {
		t.Fatalf("seed save: exit = %d", code)
	}
	cL, _, _ := newCLI(nil)
	if code := cL.Run([]string{"memory", "save", "--type", "decision",
		"--title", "chose bun runtime for the plugin host",
		"cold start under fifty milliseconds keeps the picker responsive"}); code != 0 {
		t.Fatalf("live save: exit = %d", code)
	}
	all, err := almanac.All(".")
	if err != nil || len(all) != 2 {
		t.Fatalf("setup = %+v (%v)", all, err)
	}
	oldID, liveID := all[0].ID, all[1].ID
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "relate", "--id", liveID, "--relation", "supersedes", "--to", oldID}); code != 0 {
		t.Fatalf("relate: exit = %d", code)
	}
	// Paraphrase of the superseded observation saves cleanly.
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"memory", "save", "--type", "decision",
		"--title", "picked postgres connection pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"}); code != 0 {
		t.Fatalf("superseded paraphrase must save: exit = %d, stderr = %q", code, errb.String())
	}
	after, err := almanac.All(".")
	if err != nil || len(after) != 3 {
		t.Fatalf("log must hold all three: %+v (%v)", after, err)
	}
}

func TestMemorySaveRefusesWithStaleIndex(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision",
		"--title", "chose postgres connection pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"}); code != 0 {
		t.Fatalf("seed save: exit = %d", code)
	}
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	// Append past the index so it goes stale; the next save must still
	// refuse on the fly instead of trusting the stale artifact.
	cS, _, _ := newCLI(nil)
	if code := cS.Run([]string{"memory", "save", "--type", "decision",
		"--title", "chose bun runtime for the plugin host",
		"cold start under fifty milliseconds keeps the picker responsive"}); code != 0 {
		t.Fatalf("unrelated save: exit = %d", code)
	}
	c2, _, errb := newCLI(nil)
	code := c2.Run([]string{"memory", "save", "--type", "decision",
		"--title", "picked postgres connection pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"})
	if code == 0 {
		t.Fatal("stale index must not disable dedup: paraphrase must still be refused")
	}
	if !strings.Contains(errb.String(), "topic key") {
		t.Fatalf("stale-index refusal must carry the guidance, got %q", errb.String())
	}
	after, err := almanac.All(".")
	if err != nil || len(after) != 2 {
		t.Fatalf("refused save must leave stored data untouched: %+v (%v)", after, err)
	}
}
