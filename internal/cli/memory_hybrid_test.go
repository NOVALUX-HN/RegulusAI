package cli

import (
	"strings"
	"testing"
)

func TestMemorySearchStaleWarnsLexicalOnly(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "pooling everywhere"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	// Append after the reindex so the index goes stale.
	cS, _, _ := newCLI(nil)
	if code := cS.Run([]string{"memory", "save", "--type", "decision", "--title", "second choice", "more content"}); code != 0 {
		t.Fatalf("save 2: exit = %d", code)
	}
	c2, out, errb := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "postgres"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	if !strings.Contains(errb.String(), "warning: semantic index stale or missing") {
		t.Fatalf("stale index must warn on stderr, got %q", errb.String())
	}
	if !strings.Contains(out.String(), "chose postgres") {
		t.Fatalf("degraded search must still hit lexically, got %q", out.String())
	}
	// Stdout stays stable: identical to the --lexical path.
	c3, outLex, errbLex := newCLI(nil)
	if code := c3.Run([]string{"memory", "search", "--lexical", "postgres"}); code != 0 {
		t.Fatalf("lexical search: exit = %d", code)
	}
	if out.String() != outLex.String() {
		t.Fatalf("degraded stdout must match --lexical stdout:\ndegraded: %q\nlexical:  %q", out.String(), outLex.String())
	}
	if strings.Contains(errbLex.String(), "warning: semantic index") {
		t.Fatalf("--lexical must not warn, got %q", errbLex.String())
	}
}

func TestMemorySearchLexicalFlagPath(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "pooling everywhere"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex: exit = %d", code)
	}
	// Fresh index + --lexical forces the old path with no warning.
	c2, out, errb := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "--lexical", "postgres"}); code != 0 {
		t.Fatalf("search --lexical: exit = %d", code)
	}
	if !strings.Contains(out.String(), "chose postgres") {
		t.Fatalf("lexical search must hit, got %q", out.String())
	}
	if strings.Contains(errb.String(), "warning: semantic index") {
		t.Fatalf("fresh --lexical must not warn, got %q", errb.String())
	}
	// Fresh index without the flag uses hybrid and stays silent.
	c3, outHyb, errbHyb := newCLI(nil)
	if code := c3.Run([]string{"memory", "search", "postgres"}); code != 0 {
		t.Fatalf("hybrid search: exit = %d", code)
	}
	if !strings.Contains(outHyb.String(), "chose postgres") {
		t.Fatalf("hybrid search must hit, got %q", outHyb.String())
	}
	if strings.Contains(errbHyb.String(), "warning: semantic index") {
		t.Fatalf("fresh hybrid must not warn, got %q", errbHyb.String())
	}
}
