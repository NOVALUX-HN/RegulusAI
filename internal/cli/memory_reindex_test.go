package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/almanac"
)

func TestMemoryReindexPrintsCountAndSHA(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "pooling everywhere"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	c2, out, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	if !strings.Contains(out.String(), "1 observation") {
		t.Fatalf("reindex must print the count, got %q", out.String())
	}
	if len(reindexSHA(t, out.String())) != 64 {
		t.Fatalf("reindex must print the 64-char sha256, got %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(".regulus", "almanac", "retrieval.bin")); err != nil {
		t.Fatalf("reindex must write retrieval.bin: %v", err)
	}
	// The printed sha must pin the JSONL source.
	raw, err := os.ReadFile(filepath.Join(".regulus", "almanac", "observations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	_ = raw
	entries, hdr, err := almanac.LoadIndex(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || hdr.Count != 1 {
		t.Fatalf("index = %d entries, hdr %+v", len(entries), hdr)
	}
	if !strings.Contains(out.String(), hdr.SourceSHAHex()) {
		t.Fatalf("stdout sha must match the header sha %s, got %q", hdr.SourceSHAHex(), out.String())
	}
}

func TestMemoryReindexByteIdentical(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "a", "b"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	c1, _, _ := newCLI(nil)
	if code := c1.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex 1: exit = %d", code)
	}
	first, err := os.ReadFile(filepath.Join(".regulus", "almanac", "retrieval.bin"))
	if err != nil {
		t.Fatal(err)
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex 2: exit = %d", code)
	}
	second, err := os.ReadFile(filepath.Join(".regulus", "almanac", "retrieval.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("reindex twice must be byte-identical")
	}
}

func TestMemoryReindexEmptyRefusal(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "reindex"}); code != 1 {
		t.Fatalf("empty log: exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "no observations") && !strings.Contains(errb.String(), "no log") {
		t.Fatalf("empty log must be refused clearly, got %q", errb.String())
	}
	if _, err := os.Stat(filepath.Join(".regulus", "almanac", "retrieval.bin")); !os.IsNotExist(err) {
		t.Fatal("refused reindex must not write retrieval.bin")
	}
}

func TestMemoryReindexUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"memory", "reindex", "extra"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus memory reindex") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func reindexSHA(t *testing.T, out string) string {
	t.Helper()
	for _, field := range strings.Fields(out) {
		if len(field) == 64 && isHex(field) {
			return field
		}
	}
	return ""
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
