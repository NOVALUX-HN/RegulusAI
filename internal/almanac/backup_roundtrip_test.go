package almanac

// NOV-30 (S15) bundle round-trip and snapshot tests: an export verifies,
// imports, and re-exports with a valid manifest at every stage, and
// exports racing concurrent saves never leave a mixed bundle — the
// store lock makes each export one atomic snapshot (a prefix of the
// append-only log). All roots are tmp dirs — never the real .regulus/.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBundleRoundTripVerifiesManifest(t *testing.T) {
	src := t.TempDir()
	a, b := seedBackup(t, src)
	if err := Relate(src, a.ID, RelationRelated, b.ID, backupBase.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := RecordStatus(src, a.ID, StatusVerified, "ana", "checked", backupBase.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(src); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(src, ".regulus-export")
	if err := VerifyBundle(bundle); err != nil {
		t.Fatalf("source bundle must verify: %v", err)
	}
	dst := t.TempDir()
	imported, skipped, err := Import(dst, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if imported != 2 || skipped != 0 {
		t.Fatalf("bundle import = %d/%d, want 2/0", imported, skipped)
	}
	if st, _ := EffectiveStatus(dst, a.ID); st != StatusVerified {
		t.Fatalf("the governed observation keeps its trail, got %q", st)
	}
	if pins, _ := PinnedIDs(dst); !pins[b.ID] {
		t.Fatalf("the pin must restore, got %v", pins)
	}
	// The restored tree re-exports a bundle that verifies too.
	if _, err := Export(dst); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBundle(filepath.Join(dst, ".regulus-export")); err != nil {
		t.Fatalf("re-exported bundle must verify: %v", err)
	}
}

func TestExportUnderConcurrentWrites(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "baseline", Content: "before the race"}, backupBase); err != nil {
		t.Fatal(err)
	}
	// Writers append distinct observations while the main goroutine
	// exports snapshots; the lock serializes them either way.
	const writers, saves = 4, 25
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < saves; i++ {
				_, err := Save(root, &Observation{
					Type:    TypeDiscovery,
					Title:   fmt.Sprintf("writer %d observation %d", w, i),
					Content: "concurrent payload for the export race",
				}, backupBase.Add(time.Duration(w*saves+i+1)*time.Millisecond))
				if err != nil {
					t.Errorf("concurrent save w%d/%d: %v", w, i, err)
					return
				}
			}
		}(w)
	}
	const exports = 8
	bundles := make([]string, 0, exports)
	for i := 0; i < exports; i++ {
		out := filepath.Join(t.TempDir(), fmt.Sprintf("bundle-%d", i))
		if _, err := ExportDomain(root, "", out); err != nil {
			t.Fatal(err)
		}
		bundles = append(bundles, out)
	}
	wg.Wait()
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1+writers*saves {
		t.Fatalf("concurrent saves must all land: got %d, want %d", len(all), 1+writers*saves)
	}
	// Every export is an atomic snapshot: its manifest matches its files
	// (no mixed bundle), and its log is a prefix of the append-only final
	// log — no torn read, even mid-race.
	final, err := os.ReadFile(File(root))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bundles {
		if err := VerifyBundle(b); err != nil {
			t.Fatalf("export raced by saves must still verify: %v", err)
		}
		snap, err := os.ReadFile(filepath.Join(b, "almanac.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(final), string(snap)) {
			t.Fatalf("bundle %s is not an atomic snapshot of the log", b)
		}
	}
}
