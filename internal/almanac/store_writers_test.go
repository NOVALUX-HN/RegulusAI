package almanac

// M2 phase 3: lockfile portable en los escritores del WAL + chequeo de
// identidad acotado por el índice.

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// holdLock acquires the store lock for the contention tests (the caller
// releases).
func holdLock(t *testing.T, root string) func() {
	t.Helper()
	release, err := acquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

func TestWritersRefuseUnderContention(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "seed", Content: "s"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	// Fail fast while the lock is held: no 5s waits in this test.
	oldBudget := StoreLockWait
	StoreLockWait = 0
	defer func() { StoreLockWait = oldBudget }()
	release := holdLock(t, root)
	defer release()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "x", Content: "y"}, t1); !errors.Is(err, ErrStoreLocked) {
		t.Fatalf("Save under contention must refuse ErrStoreLocked, got %v", err)
	}
	if _, _, err := Reindex(root); !errors.Is(err, ErrStoreLocked) {
		t.Fatalf("Reindex under contention must refuse ErrStoreLocked, got %v", err)
	}
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPrune(root, []Observation{all[0]}); !errors.Is(err, ErrStoreLocked) {
		t.Fatalf("ApplyPrune under contention must refuse ErrStoreLocked, got %v", err)
	}
	bundle := t.TempDir()
	fresh := Observation{ID: "obs_import1", Type: TypeDiscovery, Title: "imp", Content: "c", SavedAt: t1}
	line := fmt.Sprintf(`{"id":%q,"type":%q,"title":%q,"content":%q,"saved_at":%q}`, fresh.ID, fresh.Type, fresh.Title, fresh.Content, fresh.SavedAt.Format(time.RFC3339Nano))
	if err := os.WriteFile(bundle+"/almanac.jsonl", []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Import(root, bundle+"/almanac.jsonl"); !errors.Is(err, ErrStoreLocked) {
		t.Fatalf("Import under contention must refuse ErrStoreLocked, got %v", err)
	}
	// Released: every writer lands again.
	release()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "after", Content: "y"}, t2); err != nil {
		t.Fatalf("Save after release must land: %v", err)
	}
}

func TestSaveCollisionAdvancesWithFreshIndex(t *testing.T) {
	root := t.TempDir()
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first, err := Save(root, &Observation{Type: TypeDiscovery, Title: "a", Content: "c"}, fixed)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	// Same instant, fresh index: the bounded idTaken path must still
	// advance identity past the collision.
	second, err := Save(root, &Observation{Type: TypeDiscovery, Title: "b", Content: "c2"}, fixed)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("same-instant saves must not share identity, got %q twice", first.ID)
	}
	if !second.SavedAt.After(first.SavedAt) {
		t.Fatal("collision advance must move the timestamp forward (indexed path)")
	}
}

func TestConcurrentSavesSerialize(t *testing.T) {
	root := t.TempDir()
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			saved, err := Save(root, &Observation{Type: TypeDecision, Title: fmt.Sprintf("w%d", i), Content: "race"}, t0)
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = saved.ID
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent save %d: %v", i, err)
		}
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			t.Fatalf("concurrent saves must mint distinct ids, got %v", ids)
		}
		seen[id] = true
	}
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n {
		t.Fatalf("log holds %d, want %d", len(all), n)
	}
	if _, err := os.Stat(LockPath(root)); !os.IsNotExist(err) {
		t.Fatal("no lockfile may survive the writers")
	}
}
