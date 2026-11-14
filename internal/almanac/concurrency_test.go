package almanac

// NOV-29 concurrency and idempotency (S13): the store lock makes the
// check-then-append atomic. Seeds are fixed, no network, no clock
// dependence beyond the fixed base instant.

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

var concBase = time.Date(2026, 11, 29, 9, 0, 0, 0, time.UTC)

// TestConcurrentDedupRace pins the TOCTOU fix: N goroutines saving the
// same content through the atomic SaveChecked path serialize on the
// store lock and land exactly one line. Identical content is an
// idempotent no-op (not a DuplicateError), so every writer succeeds
// with the same id.
func TestConcurrentDedupRace(t *testing.T) {
	root := t.TempDir()
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	saved := make([]Observation, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, err := SaveChecked(root, &Observation{
				Type:    TypeDecision,
				Title:   "concurrent dedup race",
				Content: "identical payload for all writers — only one line may land",
			}, concBase)
			if err != nil {
				errs[i] = err
				return
			}
			saved[i] = s
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent save %d: %v", i, err)
		}
	}
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("N concurrent identical saves must land exactly 1 line, got %d", len(all))
	}
	for i := 1; i < n; i++ {
		if saved[i].ID != saved[0].ID {
			t.Fatalf("idempotent saves must return the same id: %q vs %q", saved[i].ID, saved[0].ID)
		}
	}
}

// concBundle writes a fixed two-observation bundle (plus one pin, one
// relation and one status line) and returns its directory.
func concBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	at := "2026-11-29T09:00:00Z"
	obs := `{"id":"obs_conc1","type":"decision","title":"conc one","content":"alpha content for concurrency","saved_at":"` + at + `"}` + "\n" +
		`{"id":"obs_conc2","type":"discovery","title":"conc two","content":"beta content for concurrency","saved_at":"` + at + `"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "almanac.jsonl"), []byte(obs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pins.jsonl"), []byte(`{"id":"obs_conc1","pinned":true,"at":"`+at+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "relations.jsonl"), []byte(`{"from":"obs_conc2","relation":"related","to":"obs_conc1","at":"`+at+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status.jsonl"), []byte(`{"id":"obs_conc1","from":"","to":"VERIFIED","by":"test","note":"bundle trail","at":"`+at+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func concSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range []string{File(root), pinsFile(root), relationsFile(root), StatusFile(root)} {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[p] = string(raw)
	}
	return out
}

// TestConcurrentIdenticalImport pins bundle atomicity: two concurrent
// identical bundle imports serialize under one lock — one lands the
// bundle, the other reports 0 imported — and a sequential re-import
// leaves every store byte-identical.
func TestConcurrentIdenticalImport(t *testing.T) {
	bundle := concBundle(t)
	root := t.TempDir()
	type res struct {
		imported, skipped int
		err               error
	}
	out := make([]res, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, skipped, err := Import(root, bundle)
			out[i] = res{got, skipped, err}
		}(i)
	}
	wg.Wait()
	for i, r := range out {
		if r.err != nil {
			t.Fatalf("concurrent import %d: %v", i, r.err)
		}
	}
	// One winner lands both observations, the loser lands nothing: the
	// single bundle lock makes the commit all-or-nothing.
	got0, got1 := out[0].imported, out[1].imported
	if !((got0 == 2 && got1 == 0) || (got0 == 0 && got1 == 2)) {
		t.Fatalf("concurrent identical imports must split 2/0, got %d/%d", got0, got1)
	}
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("concurrent bundle imports must hold exactly 2 observations, got %d", len(all))
	}
	before := concSnapshot(t, root)
	got, skipped, err := Import(root, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("sequential re-import must be a no-op, got %d imported", got)
	}
	if skipped != 2 {
		t.Fatalf("sequential re-import must skip both observations, got skipped=%d", skipped)
	}
	if after := concSnapshot(t, root); !reflect.DeepEqual(after, before) {
		t.Fatal("re-import must leave every store byte-identical")
	}
}

// TestRepeatSaveIdempotent pins content-hash idempotency: three
// identical SaveChecked calls (even with different timestamps) land one
// line and every call returns the existing observation.
func TestRepeatSaveIdempotent(t *testing.T) {
	root := t.TempDir()
	var ids []string
	for i := 0; i < 3; i++ {
		saved, err := SaveChecked(root, &Observation{
			Type:     TypeDecision,
			Title:    "repeat identical",
			Content:  "same payload three times",
			TopicKey: "conc-idem",
		}, concBase.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatalf("repeat save %d: %v", i, err)
		}
		ids = append(ids, saved.ID)
	}
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("3 identical saves must land 1 observation, got %d", len(all))
	}
	for _, id := range ids {
		if id != all[0].ID {
			t.Fatalf("repeat save returned %q, want existing %q", id, all[0].ID)
		}
	}
}
