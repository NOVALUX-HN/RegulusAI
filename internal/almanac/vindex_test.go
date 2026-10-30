package almanac

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReindexByteIdenticalTwice(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "chose postgres", Content: "pooling everywhere"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, &Observation{Type: TypeDiscovery, Title: "pool leak", Content: "found under load"}, t1); err != nil {
		t.Fatal(err)
	}
	count1, sha1, err := Reindex(root)
	if err != nil {
		t.Fatal(err)
	}
	if count1 != 2 {
		t.Fatalf("count = %d, want 2", count1)
	}
	if len(sha1) != 64 {
		t.Fatalf("sha256 hex = %q, want 64 hex chars", sha1)
	}
	first, err := os.ReadFile(IndexPath(root))
	if err != nil {
		t.Fatal(err)
	}
	count2, sha2, err := Reindex(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(IndexPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if count1 != count2 || sha1 != sha2 {
		t.Fatalf("reindex must be a pure function: (%d,%s) vs (%d,%s)", count1, sha1, count2, sha2)
	}
	if string(first) != string(second) {
		t.Fatal("reindex twice on the same log must be byte-identical")
	}
}

func TestReindexSHAIsPinnedToSource(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0); err != nil {
		t.Fatal(err)
	}
	_, sha, err := Reindex(root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(File(root))
	if err != nil {
		t.Fatal(err)
	}
	want := sha256Hex(raw)
	if sha != want {
		t.Fatalf("sha = %s, want sha256 of observations.jsonl %s", sha, want)
	}
	_, hdr, err := LoadIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.SourceSHAHex() != want {
		t.Fatalf("header sha = %s, want %s", hdr.SourceSHAHex(), want)
	}
	if hdr.Count != 1 || hdr.Dim != EmbedDim || hdr.Embedder != EmbedderID {
		t.Fatalf("header = %+v, want count=1 dim=%d embedder=%q", hdr, EmbedDim, EmbedderID)
	}
}

func TestReindexHeaderMismatchNeverTrusted(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(IndexPath(root))
	if err != nil {
		t.Fatal(err)
	}
	corrupt := func(mut func([]byte)) {
		t.Helper()
		bad := make([]byte, len(raw))
		copy(bad, raw)
		mut(bad)
		if err := os.WriteFile(IndexPath(root), bad, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadIndex(root); !errors.Is(err, ErrHeaderMismatch) {
			t.Fatalf("header mismatch must return ErrHeaderMismatch, got %v", err)
		}
		if err := os.WriteFile(IndexPath(root), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	corrupt(func(b []byte) { b[0] ^= 0xff })  // magic
	corrupt(func(b []byte) { b[4] ^= 0xff })  // version
	corrupt(func(b []byte) { b[10] ^= 0xff }) // embedder bytes
	corrupt(func(b []byte) { b[23] ^= 0xff }) // dim
	// The last-byte flip above must NOT be a header mismatch (vectors
	// are data, not trust); it must load fine.
	bad := make([]byte, len(raw))
	copy(bad, raw)
	bad[len(bad)-1] ^= 0xff
	if err := os.WriteFile(IndexPath(root), bad, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadIndex(root); errors.Is(err, ErrHeaderMismatch) {
		t.Fatal("vector bytes must not trigger ErrHeaderMismatch")
	}
	if _, _, err := LoadIndex(root); err != nil {
		t.Fatalf("flipped vector byte must still load (data, not header): %v", err)
	}
}

func TestReindexEmptyLogRefused(t *testing.T) {
	if _, _, err := Reindex(t.TempDir()); !errors.Is(err, ErrNoLog) {
		t.Fatalf("missing log must return ErrNoLog, got %v", err)
	}
	// An existing but empty log is also "no log yet".
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(File(root), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); !errors.Is(err, ErrNoLog) {
		t.Fatalf("empty log must return ErrNoLog, got %v", err)
	}
}

// TestIndexStateCycle: H3 (S5) — the freshness token behind `status`
// and `memory doctor`: empty before any save, missing before the first
// reindex, fresh right after, stale after any save past the reindex.
func TestIndexStateCycle(t *testing.T) {
	root := t.TempDir()
	if state, err := IndexState(root); err != nil || state != "empty" {
		t.Fatalf("no log: state = %q, err = %v; want empty, nil", state, err)
	}
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0); err != nil {
		t.Fatal(err)
	}
	if state, err := IndexState(root); err != nil || state != "missing" {
		t.Fatalf("unindexed log: state = %q, err = %v; want missing, nil", state, err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	if state, err := IndexState(root); err != nil || state != "fresh" {
		t.Fatalf("just reindexed: state = %q, err = %v; want fresh, nil", state, err)
	}
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "c", Content: "d"}, t1); err != nil {
		t.Fatal(err)
	}
	if state, err := IndexState(root); err != nil || state != "stale" {
		t.Fatalf("save past reindex: state = %q, err = %v; want stale, nil", state, err)
	}
	// A corrupt index refuses trust and reads as stale: same repair.
	raw, err := os.ReadFile(IndexPath(root))
	if err != nil {
		t.Fatal(err)
	}
	bad := make([]byte, len(raw))
	copy(bad, raw)
	bad[0] ^= 0xff // magic
	if err := os.WriteFile(IndexPath(root), bad, 0o644); err != nil {
		t.Fatal(err)
	}
	if state, err := IndexState(root); err != nil || state != "stale" {
		t.Fatalf("corrupt index: state = %q, err = %v; want stale, nil", state, err)
	}
}
