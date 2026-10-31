package cli

// M3 (S3) bundle round-trip through the CLI: export writes the full
// bundle, wiping .regulus simulates a clean tree, and importing the
// bundle directory restores observations + pins + relations + status
// with a fresh index (M1 auto-reindex, untouched here). One tmp root,
// never the real .regulus/.

import (
	"os"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
)

func TestMemoryImportBundleDirRoundTripFresh(t *testing.T) {
	t.Chdir(t.TempDir())
	c1, _, _ := newCLI(nil)
	if code := c1.Run([]string{"memory", "save", "--type", "decision", "--title", "chose sqlite cache", "simpler ops for v1"}); code != 0 {
		t.Fatalf("save 1: exit = %d", code)
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "save", "--type", "discovery", "--title", "pool leak under load", "postgres pool saturates"}); code != 0 {
		t.Fatalf("save 2: exit = %d", code)
	}
	all, err := almanac.All(".")
	if err != nil || len(all) != 2 {
		t.Fatalf("seed = %+v (%v)", all, err)
	}
	first, second := all[0].ID, all[1].ID
	if err := almanac.Pin(".", second, true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := almanac.Relate(".", first, almanac.RelationRelated, second, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := almanac.RecordStatus(".", first, almanac.StatusVerified, "ana", "checked", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	cE, outE, _ := newCLI(nil)
	if code := cE.Run([]string{"memory", "export"}); code != 0 {
		t.Fatalf("export: exit = %d", code)
	}
	for _, want := range []string{"almanac.jsonl", "INDEX.md", "pins.jsonl", "relations.jsonl", "status.jsonl"} {
		if !strings.Contains(outE.String(), want) {
			t.Fatalf("export must list %s, got:\n%s", want, outE.String())
		}
	}
	// A clean tree keeps only the bundle: the Cloud copy step.
	if err := os.RemoveAll(".regulus"); err != nil {
		t.Fatal(err)
	}
	cI, outI, _ := newCLI(nil)
	if code := cI.Run([]string{"memory", "import", ".regulus-export"}); code != 0 {
		t.Fatalf("import dir: exit = %d", code)
	}
	if !strings.Contains(outI.String(), "imported 2, skipped 0") {
		t.Fatalf("import output = %q", outI.String())
	}
	got, err := almanac.All(".")
	if err != nil || len(got) != 2 {
		t.Fatalf("restored = %+v (%v)", got, err)
	}
	if pins, _ := almanac.PinnedIDs("."); !pins[second] {
		t.Fatalf("pin must restore, got %v", pins)
	}
	if rels, _ := almanac.Relations("."); len(rels) != 1 || rels[0].From != first {
		t.Fatalf("relation must restore, got %+v", rels)
	}
	if st, _ := almanac.EffectiveStatus(".", first); st != almanac.StatusVerified {
		t.Fatalf("status must restore VERIFIED, got %q", st)
	}
	// M1: the CLI import reindexed, so doctor is fresh with no manual step.
	if state, err := almanac.IndexState("."); err != nil || state != "fresh" {
		t.Fatalf("after CLI bundle import: IndexState = %q/%v, want fresh", state, err)
	}
	cD, outD, _ := newCLI(nil)
	if code := cD.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("doctor: exit = %d", code)
	}
	if !strings.Contains(outD.String(), "index:      fresh") {
		t.Fatalf("doctor must report fresh, got:\n%s", outD.String())
	}
	cS, outS, errS := newCLI(nil)
	if code := cS.Run([]string{"memory", "search", "sqlite"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	if !strings.Contains(outS.String(), "chose sqlite cache") {
		t.Fatalf("search must hit, got %q", outS.String())
	}
	if strings.Contains(errS.String(), "warning: semantic index") {
		t.Fatalf("fresh search must not warn, got %q", errS.String())
	}
	// Re-importing the bundle is a no-op.
	cR, outR, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "import", ".regulus-export"}); code != 0 {
		t.Fatalf("re-import: exit = %d", code)
	}
	if !strings.Contains(outR.String(), "imported 0, skipped 2") {
		t.Fatalf("re-import output = %q", outR.String())
	}
}
