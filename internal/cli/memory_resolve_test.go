package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/trace"
)

func TestMemoryContextResolveDomainChartPin(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, "charts"), 0755)
	os.WriteFile(filepath.Join(dir, "charts", "app.md"), []byte("# App\n\nDomain: go\n"), 0644)
	// seed one lesson so scoped section is non-empty
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeLesson, Title: "go lesson", Content: "use table tests",
		Domain: "go", Lesson: &almanac.Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low"},
	}, time.Now()); err != nil {
		t.Fatalf("seed lesson: %v", err)
	}

	c, out, _ := newCLI(nil)
	code := c.Run([]string{"memory", "context", "--resolve-domain", "--chart", "charts/app.md"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "domain: go") {
		t.Fatalf("must resolve chart domain, got:\n%s", got)
	}
	if !strings.Contains(got, "go lesson") {
		t.Fatalf("must include scoped lessons, got:\n%s", got)
	}
}

func TestMemoryContextResolveDomainSingleLibrary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "ts"), 0755)
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeLesson, Title: "ts lesson", Content: "strict flags",
		Domain: "ts", Lesson: &almanac.Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low"},
	}, time.Now()); err != nil {
		t.Fatalf("seed lesson: %v", err)
	}

	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--resolve-domain"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "domain: ts") {
		t.Fatalf("single library domain must win, got:\n%s", out.String())
	}
}

func TestMemoryContextResolveDomainFileHit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "go"), 0755)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "ts"), 0755)
	// lesson citing a path for go
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeLesson, Title: "go path lesson", Content: "cover",
		Domain: "go", Lesson: &almanac.Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low"},
		Provenance: &almanac.Provenance{Paths: []string{"internal/cli/memory.go"}},
	}, time.Now()); err != nil {
		t.Fatalf("seed go lesson: %v", err)
	}
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeLesson, Title: "ts lesson", Content: "flags",
		Domain: "ts", Lesson: &almanac.Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low"},
	}, time.Now()); err != nil {
		t.Fatalf("seed ts lesson: %v", err)
	}
	_, _ = trace.Record(dir, trace.Entry{Session: "s1", Message: "m", Call: "c", Tool: "edit", Status: "completed", Files: []string{"internal/cli/memory.go"}}, time.Now())

	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--resolve-domain", "--session", "s1"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "domain: go") {
		t.Fatalf("file-hit must pick go, got:\n%s", out.String())
	}
}

func TestMemoryContextResolveDomainUnfiltered(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--resolve-domain"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "domain: (unfiltered)") {
		t.Fatalf("empty store must be unfiltered, got:\n%s", out.String())
	}
}
