package almanac

// M1 (S2) pluggable-embedder gate: the retrieval.bin header already
// carries {embedder, dim, version} — this pins the triple, the default
// registry, the unknown-id refusal, and mismatch-forces-reindex without
// any onnx dependency or model binary (TestZeroDepsGoMod stays green).

import (
	"crypto/sha256"
	"errors"
	"os"
	"testing"
)

func TestLookupEmbedderDefaultsToHash(t *testing.T) {
	for _, id := range []string{"", EmbedderID} {
		emb, err := LookupEmbedder(id)
		if err != nil {
			t.Fatalf("LookupEmbedder(%q) must resolve: %v", id, err)
		}
		if emb.ID() != EmbedderID {
			t.Fatalf("LookupEmbedder(%q).ID() = %q, want %q", id, emb.ID(), EmbedderID)
		}
		if emb.Embed("hello world") != Embed("hello world") {
			t.Fatalf("LookupEmbedder(%q) must embed exactly like the hash encoder", id)
		}
	}
}

func TestLookupEmbedderUnknownRefuses(t *testing.T) {
	if _, err := LookupEmbedder("onnx-minilm-v1"); err == nil {
		t.Fatal("unknown embedder must refuse (only hash-ngram-v1 ships — ONNX deferred)")
	}
}

func TestReindexWithUnknownRefusesBeforeWrite(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReindexWith(root, "onnx-minilm-v1"); err == nil {
		t.Fatal("ReindexWith an unknown embedder must refuse")
	}
	if _, err := os.Stat(IndexPath(root)); !os.IsNotExist(err) {
		t.Fatal("refused reindex must not write retrieval.bin")
	}
}

func TestHeaderPinsEmbedderDimVersion(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReindexWith(root, ""); err != nil {
		t.Fatal(err)
	}
	_, hdr, err := LoadIndexWith(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Embedder != EmbedderID || hdr.Dim != EmbedDim || hdr.Version != IndexVersion {
		t.Fatalf("header must pin {embedder,dim,version} = {%q,%d,%d}, got %+v", EmbedderID, EmbedDim, IndexVersion, hdr)
	}
}

// TestHeaderMismatchForcesReindex builds an index with a foreign encoder
// name through the same header writer and proves the default load
// refuses it with ErrHeaderMismatch — rebuild, never read. Loading it
// under the foreign name refuses at the registry instead (unknown
// encoder, ONNX deferred).
func TestHeaderMismatchForcesReindex(t *testing.T) {
	root := t.TempDir()
	saved, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	// Sanity: the shipped index loads under the default encoder.
	if _, _, err := LoadIndexWith(root, ""); err != nil {
		t.Fatalf("default index must load under the default encoder: %v", err)
	}
	// Forge a foreign-encoder index with the real writer: same vector
	// bytes, header naming another encoder.
	emb := HashEmbedder()
	rawLog, err := os.ReadFile(File(root))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(rawLog)
	buf := newIndexBuffer("probe-encoder-v9", 1, sum)
	writeEntry(buf, Observation{ID: saved.ID, Title: "a", Content: "b"}, emb)
	if err := os.WriteFile(IndexPath(root), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadIndexWith(root, ""); !errors.Is(err, ErrHeaderMismatch) {
		t.Fatalf("foreign-encoder header must refuse with ErrHeaderMismatch, got %v", err)
	}
	if _, _, err := LoadIndexWith(root, "probe-encoder-v9"); err == nil {
		t.Fatal("foreign encoder name must refuse at the registry (unknown embedder)")
	}
	// Repair is one reindex with the default encoder.
	if _, _, err := ReindexWith(root, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadIndex(root); err != nil {
		t.Fatalf("reindex with the default encoder must heal the index: %v", err)
	}
	if state, err := IndexState(root); err != nil || state != "fresh" {
		t.Fatalf("reindexed: state = %q, err = %v; want fresh, nil", state, err)
	}
}

// TestHybridSearchWithUnknownRefuses pins the usage boundary: an
// unknown --embedder is a caller error, never a silent lexical fallback.
func TestHybridSearchWithUnknownRefuses(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "a", Content: "b"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := HybridSearchWith(root, Query{Terms: []string{"a"}}, "onnx-minilm-v1"); err == nil {
		t.Fatal("HybridSearchWith an unknown embedder must refuse, not degrade")
	}
	// The explicit default behaves exactly like the implicit one.
	want, wantDegraded, err := HybridSearch(root, Query{Terms: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	got, gotDegraded, err := HybridSearchWith(root, Query{Terms: []string{"a"}}, EmbedderID)
	if err != nil {
		t.Fatal(err)
	}
	if wantDegraded != gotDegraded || len(want) != len(got) {
		t.Fatalf("explicit default embedder must match the implicit path: %v vs %v", want, got)
	}
	for i := range want {
		if want[i].ID != got[i].ID {
			t.Fatalf("explicit default embedder must match the implicit path: %v vs %v", want, got)
		}
	}
}
