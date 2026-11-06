package almanac

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCachedSourceSHAMatchesDirectAndInvalidates(t *testing.T) {
	root := t.TempDir()
	clearSHACache()
	if _, err := Save(root, &Observation{Type: TypeSessionNote, Title: "c1", Content: "body one"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	first, err := cachedSourceSHA(root)
	if err != nil || first == "" {
		t.Fatalf("first hash failed: %q %v", first, err)
	}
	second, err := cachedSourceSHA(root)
	if err != nil || second != first {
		t.Fatalf("repeat check must hit cache with identical sha, got %q vs %q (%v)", second, first, err)
	}
	if _, err := Save(root, &Observation{Type: TypeSessionNote, Title: "c2", Content: "body two"}, time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	third, err := cachedSourceSHA(root)
	if err != nil || third == first {
		t.Fatalf("appended log must change sha, got %q vs %q (%v)", third, first, err)
	}
}

func TestCachedSourceSHAMissingLog(t *testing.T) {
	root := t.TempDir()
	clearSHACache()
	if _, err := cachedSourceSHA(root); err == nil {
		t.Fatal("missing log must error, not return empty sha")
	}
	_ = filepath.Join(root, "unused")
	_ = os.MkdirAll(root, 0o755)
}
