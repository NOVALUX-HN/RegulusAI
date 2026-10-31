package cli

import (
	"strings"
	"testing"
)

func retrievalSeed(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	mustSave := func(typ, title, content string, extra ...string) {
		t.Helper()
		args := []string{"memory", "save", "--type", typ, "--title", title}
		args = append(args, extra...)
		args = append(args, content)
		c, _, _ := newCLI(nil)
		if code := c.Run(args); code != 0 {
			t.Fatalf("save %s: exit = %d", title, code)
		}
	}
	mustSave("discovery", "postgres pooling", "pooling everywhere")
	mustSave("session-note", "standup", "pooling discussed")
	mustSave("discovery", "old news", "pooling retired", "--expires", "1ns")
}

func TestMemorySearchSystemFilter(t *testing.T) {
	retrievalSeed(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "--system", "semantic", "pooling"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "postgres pooling") || strings.Contains(out.String(), "standup") {
		t.Fatalf("system filter must isolate, got:\n%s", out.String())
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "--system", "vibes", "x"}); code != 2 {
		t.Fatalf("bad system must be usage error, got %d (%q)", code, errb.String())
	}
}

func TestMemorySearchStaleHiddenByDefault(t *testing.T) {
	retrievalSeed(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "--lexical", "retired"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out.String(), "old news") {
		t.Fatalf("expired must hide by default, got:\n%s", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "--lexical", "--include-stale", "retired"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "old news") {
		t.Fatalf("include-stale must resurface, got:\n%s", out2.String())
	}
}

func TestMemorySearchHybridHonorsStale(t *testing.T) {
	retrievalSeed(t)
	cR, _, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "reindex"}); code != 0 {
		t.Fatalf("reindex exit = %d", code)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "retired"}); code != 0 {
		t.Fatalf("exit = %d (%s)", code, errb.String())
	}
	if strings.Contains(out.String(), "old news") {
		t.Fatalf("hybrid must exclude expired too, got:\n%s", out.String())
	}
}

func TestMemoryContextScoped(t *testing.T) {
	retrievalSeed(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--system", "semantic", "--budget", "10"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	s := out.String()
	if !strings.Contains(s, "scoped: system=semantic") {
		t.Fatalf("scope must be announced, got:\n%s", s)
	}
	if strings.Contains(s, "standup") {
		t.Fatalf("system scope must filter notes, got:\n%s", s)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "context", "--budget", "1"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "narrow with") && !strings.Contains(out2.String(), "showing latest 1 of") {
		t.Fatalf("budget must cap with a pointer, got:\n%s", out2.String())
	}
}

func TestMemoryRetrievalCarriesTrustBanner(t *testing.T) {
	retrievalSeed(t)
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "--lexical", "pooling"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errb.String(), "DATA") {
		t.Fatalf("search must frame content as untrusted DATA, stderr = %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"memory", "show"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errb2.String(), "DATA") {
		t.Fatalf("show must frame content as untrusted DATA, stderr = %q", errb2.String())
	}
}

func TestMemoryContextInvalidSystem(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--system", "nope"}); code != 2 {
		t.Fatalf("exit = %d, want 2 (%q)", code, errb.String())
	}
}
