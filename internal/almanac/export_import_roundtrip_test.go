package almanac

// M3 (S3) export/import round-trip: the bundle carries pins, relations
// and status beside the observations, and importing a bundle directory
// into a clean tree restores everything idempotently. All roots are tmp
// dirs — never the real .regulus/.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// seedRoundTrip builds a source Almanac with observations + one pin +
// one relation + one governance transition, and returns the ids.
func seedRoundTrip(t *testing.T, root string) (a, b, c Observation) {
	t.Helper()
	var err error
	a, err = Save(root, &Observation{Type: TypeDecision, Title: "chose sqlite", Content: "simpler ops for v1"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	b, err = Save(root, &Observation{Type: TypeDiscovery, Title: "pool leak found", Content: "postgres pool under load"}, t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	c, err = Save(root, &Observation{
		Type: TypeLesson, Title: "cache tuning", Content: "retrieval cache sizing", Domain: "go",
		Lesson: &Lesson{Cue: "when retrieval slows", Lesson: "size the cache first", Evidence: []string{"r.json"}, Confidence: "high"},
	}, t0.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := Pin(root, b.ID, true, t0.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, c.ID, RelationRelated, a.ID, t0.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := RecordStatus(root, a.ID, StatusVerified, "ana", "checked vs r.json", t0.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return a, b, c
}

func TestExportBundleCarriesPinsRelationsStatus(t *testing.T) {
	root := t.TempDir()
	seedRoundTrip(t, root)
	paths, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 5 {
		t.Fatalf("want the jsonl + INDEX + pins + relations + status, got %v", paths)
	}
	// The extra bundle files are verbatim copies of their stores,
	// re-importable as-is.
	for _, tc := range []struct {
		store string
		name  string
	}{
		{pinsFile(root), "pins.jsonl"},
		{relationsFile(root), "relations.jsonl"},
		{StatusFile(root), "status.jsonl"},
	} {
		want, err := os.ReadFile(tc.store)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(root, ".regulus-export", tc.name))
		if err != nil {
			t.Fatalf("bundle must carry %s: %v", tc.name, err)
		}
		if string(got) != string(want) {
			t.Fatalf("%s must mirror its store, got %q want %q", tc.name, got, want)
		}
	}
}

func TestExportEmptyStoresWriteEmptyFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "lone", Content: "c"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(root); err != nil {
		t.Fatal(err)
	}
	// No pins, relations or status recorded: the bundle keeps its shape
	// with empty files instead of omitting them.
	for _, name := range []string{"pins.jsonl", "relations.jsonl", "status.jsonl"} {
		raw, err := os.ReadFile(filepath.Join(root, ".regulus-export", name))
		if err != nil {
			t.Fatalf("bundle must keep %s even when empty: %v", name, err)
		}
		if len(raw) != 0 {
			t.Fatalf("%s must be empty, got %q", name, raw)
		}
	}
}

func TestBundleRoundTripRestoresEverything(t *testing.T) {
	src := t.TempDir()
	a, b, c := seedRoundTrip(t, src)
	if _, _, err := Reindex(src); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(src); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	imported, skipped, err := Import(dst, filepath.Join(src, ".regulus-export"))
	if err != nil {
		t.Fatal(err)
	}
	if imported != 3 || skipped != 0 {
		t.Fatalf("clean-tree bundle import = %d/%d, want 3/0", imported, skipped)
	}
	all, err := All(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].ID != a.ID || all[1].ID != b.ID || all[2].ID != c.ID {
		t.Fatalf("observations must restore by id in order, got %+v", all)
	}
	// Imports stay import-sourced, keeping their origin.
	for _, o := range all {
		if o.Source != SourceImport {
			t.Fatalf("%s: source = %q, want import", o.ID, o.Source)
		}
	}
	pins, err := PinnedIDs(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !pins[b.ID] || len(pins) != 1 {
		t.Fatalf("pins must restore, got %v", pins)
	}
	wantRels, _ := Relations(src)
	gotRels, err := Relations(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotRels, wantRels) {
		t.Fatalf("relations must restore verbatim, got %+v want %+v", gotRels, wantRels)
	}
	// Governance restores: the verified observation keeps its trail,
	// and ids the bundle status does not govern enter PROPOSED.
	if st, _ := EffectiveStatus(dst, a.ID); st != StatusVerified {
		t.Fatalf("verified observation must stay VERIFIED, got %q", st)
	}
	if hist, _ := StatusHistory(dst, a.ID); len(hist) != 1 || hist[0].By != "ana" {
		t.Fatalf("status trail must restore, got %+v", hist)
	}
	for _, id := range []string{b.ID, c.ID} {
		if st, _ := EffectiveStatus(dst, id); st != StatusProposed {
			t.Fatalf("%s must enter PROPOSED, got %q", id, st)
		}
	}
	// The restored log reindexes fresh (the CLI auto-reindex after
	// import does exactly this Reindex — M1, untouched here).
	if _, _, err := Reindex(dst); err != nil {
		t.Fatal(err)
	}
	if state, err := IndexState(dst); err != nil || state != "fresh" {
		t.Fatalf("IndexState after reindex = %q/%v, want fresh", state, err)
	}
	hits, degraded, err := HybridSearch(dst, Query{Terms: []string{"sqlite"}})
	if err != nil || degraded || len(hits) == 0 {
		t.Fatalf("hybrid search on the restored index = %+v/%v/%v", hits, degraded, err)
	}
}

func TestBundleImportIsIdempotent(t *testing.T) {
	src := t.TempDir()
	seedRoundTrip(t, src)
	if _, err := Export(src); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(src, ".regulus-export")
	dst := t.TempDir()
	snapshot := func() map[string]string {
		t.Helper()
		out := map[string]string{}
		for _, p := range []string{File(dst), pinsFile(dst), relationsFile(dst), StatusFile(dst)} {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			out[p] = string(raw)
		}
		return out
	}
	got, skipped, err := Import(dst, bundle)
	if err != nil || got != 3 || skipped != 0 {
		t.Fatalf("first import = %d/%d (%v), want 3/0", got, skipped, err)
	}
	before := snapshot()
	got, skipped, err = Import(dst, bundle)
	if err != nil || got != 0 || skipped != 3 {
		t.Fatalf("re-import must be a no-op, got %d/%d (%v)", got, skipped, err)
	}
	if after := snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatal("re-import must leave every store byte-identical")
	}
}

func TestBundleImportSkipsOrphans(t *testing.T) {
	dir := t.TempDir()
	obs := `{"id":"obs_orphan1","type":"decision","title":"kept","content":"c","saved_at":"2026-10-26T08:00:00Z"}` + "\n"
	pins := `{"id":"obs_ghost","pinned":true,"at":"2026-10-26T09:00:00Z"}` + "\n"
	rels := `{"from":"obs_orphan1","relation":"related","to":"obs_ghost","at":"2026-10-26T09:00:00Z"}` + "\n"
	status := `{"id":"obs_ghost","from":"","to":"VERIFIED","by":"ana","note":"x","at":"2026-10-26T09:00:00Z"}` + "\n"
	for name, body := range map[string]string{
		"almanac.jsonl": obs, "pins.jsonl": pins, "relations.jsonl": rels, "status.jsonl": status,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dst := t.TempDir()
	got, skipped, err := Import(dst, dir)
	if err != nil {
		t.Fatalf("orphan events must skip, not fail: %v", err)
	}
	if got != 1 || skipped != 0 {
		t.Fatalf("import = %d/%d, want 1/0", got, skipped)
	}
	if pins, _ := PinnedIDs(dst); len(pins) != 0 {
		t.Fatalf("orphan pins must not land, got %v", pins)
	}
	if rels, _ := Relations(dst); len(rels) != 0 {
		t.Fatalf("orphan relations must not land, got %+v", rels)
	}
}

func TestBundleImportIsAtomic(t *testing.T) {
	dir := t.TempDir()
	obs := `{"id":"obs_atom1","type":"decision","title":"kept","content":"c","saved_at":"2026-10-26T08:00:00Z"}` + "\n"
	bad := `{"from":"obs_atom1","relation":"kinda","to":"obs_atom1","at":"2026-10-26T09:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "almanac.jsonl"), []byte(obs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "relations.jsonl"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if _, _, err := Import(dst, dir); err == nil || !strings.Contains(err.Error(), "relations.jsonl line 1") {
		t.Fatalf("invalid bundle line must abort naming file+line, got %v", err)
	}
	if all, _ := All(dst); len(all) != 0 {
		t.Fatal("an aborted bundle import must write no observations")
	}
	for _, p := range []string{pinsFile(dst), relationsFile(dst), StatusFile(dst)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("an aborted bundle import must not create %s", p)
		}
	}
}

func TestBundleDirWithoutExtrasImportsLikeFile(t *testing.T) {
	// Old bundles carry no pins/relations/status files: a directory with
	// only almanac.jsonl imports the observations and nothing else.
	dir := t.TempDir()
	obs := `{"id":"obs_compat1","type":"discovery","title":"old","content":"c","saved_at":"2026-10-26T08:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "almanac.jsonl"), []byte(obs), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	got, skipped, err := Import(dst, dir)
	if err != nil || got != 1 || skipped != 0 {
		t.Fatalf("compat import = %d/%d (%v), want 1/0", got, skipped, err)
	}
	if st, _ := EffectiveStatus(dst, "obs_compat1"); st != StatusProposed {
		t.Fatalf("compat import must mark PROPOSED, got %q", st)
	}
}

func TestBundleImportRejectsBadStoreLines(t *testing.T) {
	obs := `{"id":"obs_bad1","type":"decision","title":"kept","content":"c","saved_at":"2026-10-26T08:00:00Z"}` + "\n"
	cases := map[string]map[string]string{
		"malformed pins":       {"pins.jsonl": "{not json}\n"},
		"pin without id":       {"pins.jsonl": `{"pinned":true,"at":"2026-10-26T09:00:00Z"}` + "\n"},
		"unknown relation":     {"relations.jsonl": `{"from":"obs_bad1","relation":"kinda","to":"obs_bad1","at":"2026-10-26T09:00:00Z"}` + "\n"},
		"self relation":        {"relations.jsonl": `{"from":"obs_bad1","relation":"related","to":"obs_bad1","at":"2026-10-26T09:00:00Z"}` + "\n"},
		"relation no endpoint": {"relations.jsonl": `{"from":"obs_bad1","relation":"related","to":"","at":"2026-10-26T09:00:00Z"}` + "\n"},
		"unknown status":       {"status.jsonl": `{"id":"obs_bad1","to":"ASCENDED","by":"ana","at":"2026-10-26T09:00:00Z"}` + "\n"},
		"status without id":    {"status.jsonl": `{"to":"VERIFIED","by":"ana","at":"2026-10-26T09:00:00Z"}` + "\n"},
		"malformed status":     {"status.jsonl": "{not json}\n"},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "almanac.jsonl"), []byte(obs), 0o644); err != nil {
				t.Fatal(err)
			}
			var wantFile string
			for f, body := range files {
				wantFile = f
				if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			dst := t.TempDir()
			_, _, err := Import(dst, dir)
			if err == nil || !strings.Contains(err.Error(), wantFile+" line 1") {
				t.Fatalf("must abort naming %s line 1, got %v", wantFile, err)
			}
			if all, _ := All(dst); len(all) != 0 {
				t.Fatal("an aborted bundle import must write nothing")
			}
		})
	}
	// A directory without almanac.jsonl is not a bundle.
	if _, _, err := Import(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("a bundle without almanac.jsonl must fail")
	}
}
