package cli

// M1 (S1) auto-reindex: `memory note` / `save` / `import` / `prune
// --yes` leave the derived retrieval.bin fresh without a manual
// `memory reindex`, so `memory doctor` reports fresh and `memory
// search` runs hybrid with no stale warning. All roots are tmp dirs
// (t.TempDir + t.Chdir) — never the real .regulus/.

import (
	"os"
	"strings"
	"testing"

	"regulus-ai/internal/almanac"
)

func TestMemoryAutoReindexAfterSaveFresh(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "pooling everywhere"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	if state, err := almanac.IndexState("."); err != nil || state != "fresh" {
		t.Fatalf("after CLI save: IndexState = %q, err = %v; want fresh, nil", state, err)
	}
	cD, outD, _ := newCLI(nil)
	if code := cD.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("doctor: exit = %d", code)
	}
	if !strings.Contains(outD.String(), "index:      fresh") {
		t.Fatalf("doctor must report fresh after save, got:\n%s", outD.String())
	}
	// Hybrid search on the fresh index: hits, no stale warning.
	cS, outS, errS := newCLI(nil)
	if code := cS.Run([]string{"memory", "search", "postgres"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	if !strings.Contains(outS.String(), "chose postgres") {
		t.Fatalf("search must hit, got %q", outS.String())
	}
	if strings.Contains(errS.String(), "warning: semantic index") {
		t.Fatalf("fresh search must not warn, got %q", errS.String())
	}
	hits, degraded, err := almanac.HybridSearch(".", almanac.Query{Terms: []string{"postgres"}})
	if err != nil {
		t.Fatal(err)
	}
	if degraded {
		t.Fatal("HybridSearch after CLI save must not degrade")
	}
	if len(hits) == 0 {
		t.Fatal("HybridSearch after CLI save must hit")
	}
}

func TestMemoryAutoReindexAfterNoteFresh(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "note", "session checkpoint alpha"}); code != 0 {
		t.Fatalf("note: exit = %d", code)
	}
	if state, err := almanac.IndexState("."); err != nil || state != "fresh" {
		t.Fatalf("after CLI note: IndexState = %q, err = %v; want fresh, nil", state, err)
	}
	cD, outD, _ := newCLI(nil)
	if code := cD.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("doctor: exit = %d", code)
	}
	if !strings.Contains(outD.String(), "index:      fresh") {
		t.Fatalf("doctor must report fresh after note, got:\n%s", outD.String())
	}
	cS, _, errS := newCLI(nil)
	if code := cS.Run([]string{"memory", "search", "checkpoint"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	if strings.Contains(errS.String(), "warning: semantic index") {
		t.Fatalf("fresh search must not warn, got %q", errS.String())
	}
}

func TestMemoryAutoReindexAfterImportFresh(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("bundle.jsonl", []byte(
		"{\"id\":\"obs_imp1\",\"type\":\"decision\",\"title\":\"chose sqlite\",\"content\":\"simpler ops\",\"saved_at\":\"2026-10-26T08:00:00Z\"}\n"+
			"{\"id\":\"obs_imp2\",\"type\":\"discovery\",\"title\":\"pool leak\",\"content\":\"found under load\",\"saved_at\":\"2026-10-26T09:00:00Z\"}\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "import", "bundle.jsonl"}); code != 0 {
		t.Fatalf("import: exit = %d", code)
	}
	if state, err := almanac.IndexState("."); err != nil || state != "fresh" {
		t.Fatalf("after CLI import: IndexState = %q, err = %v; want fresh, nil", state, err)
	}
	cD, outD, _ := newCLI(nil)
	if code := cD.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("doctor: exit = %d", code)
	}
	if !strings.Contains(outD.String(), "index:      fresh") {
		t.Fatalf("doctor must report fresh after import, got:\n%s", outD.String())
	}
	cS, _, errS := newCLI(nil)
	if code := cS.Run([]string{"memory", "search", "sqlite"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	if strings.Contains(errS.String(), "warning: semantic index") {
		t.Fatalf("fresh search must not warn, got %q", errS.String())
	}
}

func TestMemoryAutoReindexAfterPruneFresh(t *testing.T) {
	t.Chdir(t.TempDir())
	c1, _, _ := newCLI(nil)
	if code := c1.Run([]string{"memory", "save", "--type", "decision", "--title", "keep choice", "stable decision"}); code != 0 {
		t.Fatalf("save decision: exit = %d", code)
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "save", "--type", "discovery", "--title", "drop discovery", "transient finding"}); code != 0 {
		t.Fatalf("save discovery: exit = %d", code)
	}
	// Prune only the discovery type; the decision stays so the log is
	// non-empty and the index must be fresh (not empty, not stale).
	cP, _, _ := newCLI(nil)
	if code := cP.Run([]string{"memory", "prune", "--older-than-days", "0", "--type", "discovery", "--yes"}); code != 0 {
		t.Fatalf("prune --yes: exit = %d", code)
	}
	all, err := almanac.All(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Type != almanac.TypeDecision {
		t.Fatalf("prune must leave exactly the decision, got %+v", all)
	}
	if state, err := almanac.IndexState("."); err != nil || state != "fresh" {
		t.Fatalf("after CLI prune --yes: IndexState = %q, err = %v; want fresh, nil", state, err)
	}
	cD, outD, _ := newCLI(nil)
	if code := cD.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("doctor: exit = %d", code)
	}
	if !strings.Contains(outD.String(), "index:      fresh") {
		t.Fatalf("doctor must report fresh after prune, got:\n%s", outD.String())
	}
	cS, _, errS := newCLI(nil)
	if code := cS.Run([]string{"memory", "search", "stable"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	if strings.Contains(errS.String(), "warning: semantic index") {
		t.Fatalf("fresh search must not warn, got %q", errS.String())
	}
}

func TestMemoryPruneDryRunWritesNoIndex(t *testing.T) {
	t.Chdir(t.TempDir())
	// Seed past the CLI auto-reindex, then stale the index with a
	// direct library append (bypasses the CLI refresh).
	c1, _, _ := newCLI(nil)
	if code := c1.Run([]string{"memory", "save", "--type", "decision", "--title", "keep choice", "stable"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	// A dry run must not touch the index either way: remove the index
	// first so any write would be visible.
	if err := os.Remove(almanac.IndexPath(".")); err != nil {
		t.Fatal(err)
	}
	cP, _, _ := newCLI(nil)
	if code := cP.Run([]string{"memory", "prune", "--older-than-days", "0"}); code != 0 {
		t.Fatalf("prune dry run: exit = %d", code)
	}
	if _, err := os.Stat(almanac.IndexPath(".")); !os.IsNotExist(err) {
		t.Fatal("dry run must not write retrieval.bin")
	}
}
