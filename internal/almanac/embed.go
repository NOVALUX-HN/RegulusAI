// Package almanac — hash embedder (docs/memory-v2.md §4): hashed
// char-ngram features → 384 dims, int8 quantized. Pure Go, no new
// go.mod dependencies. Deterministic: same input → same bytes, every
// process, every run. It gives topic-affinity and typo-tolerance; it is
// NOT a MiniLM-class paraphrase model.
package almanac

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"
)

// EmbedDim is the vector width (docs/memory-v2.md §4: 384 dims).
const EmbedDim = 384

// EmbedderID pins the encoder in the retrieval.bin header; a mismatch
// forces a rebuild, never a trusted read.
const EmbedderID = "hash-ngram-v1"

// Embedder is the pluggable encoder interface (docs/memory-v2.md §4,
// v2.1): v2.0 ships one implementation, hash-ngram-v1. A future ONNX
// MiniLM registers under its own id; the retrieval.bin header pins
// which encoder produced it ({embedder, dim, version}), and a mismatch
// forces a reindex — receipts pin the encoder via `reindex` stdout.
type Embedder interface {
	// ID names the encoder in the index header and receipts.
	ID() string
	// Embed returns the int8-quantized embedding of text.
	Embed(text string) [EmbedDim]int8
	// EmbedObservation embeds the retrieval text of one observation:
	// title + newline + content.
	EmbedObservation(o Observation) [EmbedDim]int8
}

// hashEmbedder is the v2.0 hashed char-ngram encoder: signed hash
// buckets, L2-normalized, scaled to int8. Pure Go, deterministic.
type hashEmbedder struct{}

// HashEmbedder is the v2.0 default encoder (EmbedderID).
func HashEmbedder() Embedder { return hashEmbedder{} }

func (hashEmbedder) ID() string { return EmbedderID }

func (hashEmbedder) Embed(text string) [EmbedDim]int8 { return Embed(text) }

func (hashEmbedder) EmbedObservation(o Observation) [EmbedDim]int8 { return EmbedObservation(o) }

// LookupEmbedder resolves a --embedder id; "" means the default. An
// unknown id refuses: only hash-ngram-v1 ships (ONNX MiniLM deferred
// per docs/memory-v2.md §4 — no onnx dependency, no model binary).
func LookupEmbedder(id string) (Embedder, error) {
	if id == "" || id == EmbedderID {
		return hashEmbedder{}, nil
	}
	return nil, fmt.Errorf("unknown embedder %q (only %q ships — ONNX MiniLM deferred per docs/memory-v2.md §4)", id, EmbedderID)
}

// Embed returns the int8-quantized char-ngram embedding of text:
// signed hash buckets accumulated, L2-normalized, scaled by 127.
// Empty text embeds to the zero vector.
func Embed(text string) [EmbedDim]int8 {
	var acc [EmbedDim]float64
	accumulate(&acc, strings.ToLower(text))
	return quantize(&acc)
}

// EmbedObservation embeds the retrieval text of one observation:
// title + newline + content.
func EmbedObservation(o Observation) [EmbedDim]int8 {
	return Embed(o.Title + "\n" + o.Content)
}

// accumulate adds signed hash-bucket hits for every 3..5-gram.
func accumulate(acc *[EmbedDim]float64, text string) {
	r := []rune(text)
	if len(r) == 0 {
		return
	}
	if len(r) < 3 {
		idx, sign := bucket(text)
		acc[idx] += sign
		return
	}
	for _, n := range []int{3, 4, 5} {
		addNgrams(acc, r, n)
	}
}

// addNgrams hashes every n-char window into its signed bucket.
func addNgrams(acc *[EmbedDim]float64, r []rune, n int) {
	if len(r) < n {
		return
	}
	for i := 0; i+n <= len(r); i++ {
		idx, sign := bucket(string(r[i : i+n]))
		acc[idx] += sign
	}
}

// bucket maps one n-gram to its signed bucket: index +1/-1 by the top
// hash bit (the hashing trick, deterministic FNV-1a).
func bucket(s string) (int, float64) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	sum := h.Sum64()
	idx := int(sum % EmbedDim)
	if sum>>63 == 0 {
		return idx, 1
	}
	return idx, -1
}

// quantize L2-normalizes then scales to int8 ([-127, 127]).
func quantize(acc *[EmbedDim]float64) [EmbedDim]int8 {
	var out [EmbedDim]int8
	norm := 0.0
	for _, v := range acc {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return out
	}
	for i, v := range acc {
		q := int(math.Round(v / norm * 127))
		out[i] = clampInt8(q)
	}
	return out
}

// clampInt8 keeps the quantized value in the int8 range.
func clampInt8(q int) int8 {
	if q > 127 {
		return 127
	}
	if q < -127 {
		return -127
	}
	return int8(q)
}
