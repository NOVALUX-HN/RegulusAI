package almanac

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// TestStorePathIsRooted is the tripwire the F18 review asked for.
//
// The earlier cross-project fixture documented behaviour but could not fail:
// separation came from the per-project store root, so there was no single guard
// to remove. This test pins the property that makes that separation real — the
// store path is derived from the root — so a change that hardcodes the path, or
// reads a global one, fails here immediately.
func TestStorePathIsRooted(t *testing.T) {
	projectA, projectB := t.TempDir(), t.TempDir()

	if File(projectA) == File(projectB) {
		t.Fatalf("two distinct roots share one store path: %q", File(projectA))
	}
	if !strings.HasPrefix(File(projectA), projectA) {
		t.Fatalf("the store path must live under its own root: %q is not under %q", File(projectA), projectA)
	}
	if !strings.HasPrefix(File(projectB), projectB) {
		t.Fatalf("the store path must live under its own root: %q is not under %q", File(projectB), projectB)
	}

	// Writing in A creates a store under A, and nothing under B.
	if _, err := Save(projectA, &Observation{Type: "discovery", Title: "only A", Content: "c", Author: "a1"}, now()); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(File(projectA)); err != nil {
		t.Fatalf("writing must create the store under its own root: %v", err)
	}
	if _, err := os.Stat(File(projectB)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("writing in A must not create a store under B: %v", err)
	}

	// Reading B returns nothing and does not create a file there either: an
	// empty project stays empty instead of materialising someone else's store.
	hits, err := Search(projectB, Query{})
	if err != nil {
		t.Fatalf("search B: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("project B read %d record(s) from project A", len(hits))
	}
	if _, err := os.Stat(File(projectB)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("reading an empty project must not create a store: %v", err)
	}
}

// TestAllOnEmptyRootDoesNotInventAStore pins the read path for the same reason:
// listing an unknown project must be a bounded empty answer, not a side effect
// that could collide with another project's store.
func TestAllOnEmptyRootDoesNotInventAStore(t *testing.T) {
	empty := t.TempDir()
	all, err := All(empty)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("All on an empty root returned %d record(s)", len(all))
	}
	if _, err := os.Stat(File(empty)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("All must not create a store: %v", err)
	}
}
