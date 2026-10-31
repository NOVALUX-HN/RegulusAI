package cli

import (
	"os"
	"strings"
	"testing"

	"regulus-ai/internal/almanac"
)

func lastObservation(t *testing.T, root string) almanac.Observation {
	t.Helper()
	all, err := almanac.All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("want at least one observation")
	}
	return all[len(all)-1]
}

func TestMemorySaveSourceOriginExpiry(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	c, _, _ := newCLI(nil)
	args := []string{"memory", "save", "--type", "discovery", "--title", "t", "--source", "human", "--expires", "30d", "content here"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	o := lastObservation(t, ".")
	if o.Source != "human" {
		t.Fatalf("source must persist, got %q", o.Source)
	}
	if o.Origin == "" {
		t.Fatal("origin must be recorded automatically")
	}
	if o.ExpiresAt == nil {
		t.Fatal("expiry must persist")
	}
}

func TestMemorySaveRejectsBadSourceAndExpiry(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "discovery", "--title", "t", "--source", "oracle", "x"}); code == 0 {
		t.Fatal("unknown source must be refused")
	} else if !strings.Contains(errb.String(), "source") {
		t.Fatalf("stderr must name source, got %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"memory", "save", "--type", "discovery", "--title", "t", "--expires", "soon", "x"}); code == 0 {
		t.Fatal("bad expiry must be refused")
	} else if !strings.Contains(errb2.String(), "expires") {
		t.Fatalf("stderr must name expires, got %q", errb2.String())
	}
	c3, _, _ := newCLI(nil)
	if code := c3.Run([]string{"memory", "save", "--type", "discovery", "--title", "t", "--source", "import", "x"}); code == 0 {
		t.Fatal("source import is reserved for the import path")
	}
}

func TestMemoryImportMarksProvenance(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("bundle.jsonl", []byte("{\"id\":\"obs_1\",\"type\":\"discovery\",\"title\":\"t\",\"content\":\"c\",\"saved_at\":\"2026-01-01T00:00:00Z\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "import", "bundle.jsonl"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	o := lastObservation(t, ".")
	if o.Source != almanac.SourceImport {
		t.Fatalf("imported knowledge must be marked import, got %q", o.Source)
	}
	if o.Origin == "" {
		t.Fatal("import must record an origin (unknown-origin when the bundle lacks one)")
	}
	// Re-import skips by id, never duplicates.
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "import", "bundle.jsonl"}); code != 0 {
		t.Fatalf("re-import exit = %d, want 0", code)
	}
	all, _ := almanac.All(".")
	if len(all) != 1 {
		t.Fatalf("re-import must not duplicate, got %d", len(all))
	}
}
