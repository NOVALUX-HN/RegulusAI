package cli

// M1 (S2) CLI surface for the pluggable embedder: `reindex` and `search`
// accept --embedder (default hash-ngram-v1), pin the encoder in stdout
// so evidence receipts name it, and refuse unknown ids with usage.

import (
	"strings"
	"testing"

	"regulus-ai/internal/almanac"
)

func TestMemoryReindexPinsEmbedder(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "pooling everywhere"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	c2, out, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	if !strings.Contains(out.String(), "embedder: "+almanac.EmbedderID) {
		t.Fatalf("reindex must pin the encoder in stdout (receipts), got %q", out.String())
	}
	_, hdr, err := almanac.LoadIndex(".")
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Embedder != almanac.EmbedderID || hdr.Dim != almanac.EmbedDim {
		t.Fatalf("header must pin the default encoder, got %+v", hdr)
	}
	// The explicit default behaves like the implicit one.
	c3, outExp, _ := newCLI(nil)
	if code := c3.Run([]string{"memory", "reindex", "--embedder", almanac.EmbedderID}); code != 0 {
		t.Fatalf("reindex --embedder default: exit = %d", code)
	}
	if !strings.Contains(outExp.String(), "embedder: "+almanac.EmbedderID) {
		t.Fatalf("explicit --embedder must pin the encoder too, got %q", outExp.String())
	}
}

func TestMemoryReindexUnknownEmbedderRefuses(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "reindex", "--embedder", "onnx-minilm-v1"}); code != 2 {
		t.Fatalf("unknown embedder: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown embedder") {
		t.Fatalf("unknown embedder must be refused clearly, got %q", errb.String())
	}
}

func TestMemorySearchUnknownEmbedderRefuses(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "pooling everywhere"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	c3, _, errb := newCLI(nil)
	if code := c3.Run([]string{"memory", "search", "--embedder", "onnx-minilm-v1", "postgres"}); code != 2 {
		t.Fatalf("unknown embedder: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unknown embedder") {
		t.Fatalf("unknown embedder must be refused clearly, got %q", errb.String())
	}
}

func TestMemorySearchExplicitEmbedderHits(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "pooling everywhere"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	c2, out, errb := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "--embedder", almanac.EmbedderID, "postgres"}); code != 0 {
		t.Fatalf("search --embedder default: exit = %d", code)
	}
	if !strings.Contains(out.String(), "chose postgres") {
		t.Fatalf("explicit-embedder search must hit, got %q", out.String())
	}
	if strings.Contains(errb.String(), "warning: semantic index") {
		t.Fatalf("matching embedder must not degrade, got %q", errb.String())
	}
}
