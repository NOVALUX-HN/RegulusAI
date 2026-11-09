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

// T8 (S2,S3) cheap fallback: unpinned-multi (domains>1, no pin) trims the
// default --lessons 3 to 1 newest lesson plus per-domain search hints.
func TestMemoryContextResolveDomainUnpinnedMultiCheapFallback(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "go"), 0755)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "ts"), 0755)
	base := time.Now()
	seeds := []struct {
		title, content, domain string
	}{
		{"old go lesson", "old go content", "go"},
		{"mid ts lesson", "mid ts content", "ts"},
		{"new go lesson", "new go content", "go"},
	}
	for i, s := range seeds {
		if _, err := almanac.Save(dir, &almanac.Observation{
			Type: almanac.TypeLesson, Title: s.title, Content: s.content,
			Domain: s.domain, Lesson: &almanac.Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low"},
		}, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("seed lesson %d: %v", i, err)
		}
	}

	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--resolve-domain"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "reason: unfiltered-multi") {
		t.Fatalf("must resolve unfiltered-multi, got:\n%s", got)
	}
	idx := strings.Index(got, "Lessons scoped:")
	if idx < 0 {
		t.Fatalf("must render Lessons scoped section, got:\n%s", got)
	}
	scoped := got[idx:]
	if n := strings.Count(scoped, "  - ["); n != 1 {
		t.Fatalf("unpinned-multi must render exactly 1 scoped lesson, got %d in:\n%s", n, got)
	}
	if !strings.Contains(scoped, "memory search --domain go") {
		t.Fatalf("must hint per-domain search for go, got:\n%s", got)
	}
	if !strings.Contains(scoped, "memory search --domain ts") {
		t.Fatalf("must hint per-domain search for ts, got:\n%s", got)
	}
}

// T8 (S2,S3): explicit --lessons N wins over the cheap fallback.
func TestMemoryContextResolveDomainUnpinnedMultiExplicitLessonsWins(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "go"), 0755)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "ts"), 0755)
	base := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := almanac.Save(dir, &almanac.Observation{
			Type: almanac.TypeLesson, Title: "lesson", Content: "content",
			Domain: "go", Lesson: &almanac.Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low"},
		}, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("seed lesson %d: %v", i, err)
		}
	}

	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--resolve-domain", "--lessons", "3"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	got := out.String()
	idx := strings.Index(got, "Lessons scoped:")
	if idx < 0 {
		t.Fatalf("must render Lessons scoped section, got:\n%s", got)
	}
	if n := strings.Count(got[idx:], "  - ["); n != 3 {
		t.Fatalf("explicit --lessons 3 must render 3 lessons, got %d in:\n%s", n, got)
	}
}

// T8 (S2,S3): pinned resolutions keep full --lessons behavior (no trim,
// no fallback hints needed for the trim path).
func TestMemoryContextResolveDomainPinnedKeepsFullLessons(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, "charts"), 0755)
	os.WriteFile(filepath.Join(dir, "charts", "app.md"), []byte("# App\n\nDomain: go\n"), 0644)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "go"), 0755)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "ts"), 0755)
	base := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := almanac.Save(dir, &almanac.Observation{
			Type: almanac.TypeLesson, Title: "go lesson", Content: "content",
			Domain: "go", Lesson: &almanac.Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low"},
		}, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("seed lesson %d: %v", i, err)
		}
	}

	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--resolve-domain", "--chart", "charts/app.md"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "reason: chart:charts/app.md") {
		t.Fatalf("must resolve chart pin, got:\n%s", got)
	}
	idx := strings.Index(got, "Lessons scoped:")
	if idx < 0 {
		t.Fatalf("must render Lessons scoped section, got:\n%s", got)
	}
	if n := strings.Count(got[idx:], "  - ["); n != 3 {
		t.Fatalf("pinned chart must keep full --lessons 3, got %d in:\n%s", n, got)
	}
}

// R3-001 (R1 review, reliability BLOCKER): scoping flags without --resolve-domain
// must refuse (exit 2), never silently discard scoping and exit 0.
func TestMemoryContextResolveDomainFlagsNeedResolve(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{
		{"memory", "context", "--chart", "charts/app.md"},
		{"memory", "context", "--session", "s1"},
		{"memory", "context", "--lessons", "5"},
	} {
		c, _, _ := newCLI(nil)
		if code := c.Run(args); code != 2 {
			t.Fatalf("%v: exit = %d, want 2 (scoping flags need --resolve-domain)", args, code)
		}
	}
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context"}); code != 0 {
		t.Fatalf("plain context exit = %d, want 0", code)
	}
}

// Split pin (C1 S2-S3): lone-chart fallback keeps its byte-identical
// outcome — a single charts/*.md with a Domain: line resolves.
func TestResolveLoneChartSingle(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "charts"), 0755)
	os.WriteFile(filepath.Join(dir, "charts", "app.md"), []byte("# App\n\nDomain: go\n"), 0644)
	d, reason, ok := resolveLoneChart(dir)
	if !ok || d != "go" || reason != "lone-chart:app.md" {
		t.Fatalf("got (%q, %q, %v), want (go, lone-chart:app.md, true)", d, reason, ok)
	}
}

// Split pin (C1 S2-S3): lone-chart misses on zero/multiple candidates
// or a candidate without a Domain: line — resolution moves on.
func TestResolveLoneChartMiss(t *testing.T) {
	mkcharts := func(t *testing.T, files map[string]string) string {
		t.Helper()
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "charts"), 0755)
		for name, body := range files {
			os.WriteFile(filepath.Join(dir, "charts", name), []byte(body), 0644)
		}
		return dir
	}
	cases := map[string]struct {
		dir string
	}{
		"no charts dir": {dir: t.TempDir()},
		"two candidates": {dir: mkcharts(t, map[string]string{
			"a.md": "# A\n\nDomain: go\n", "b.md": "# B\n\nDomain: ts\n",
		})},
		"single without domain": {dir: mkcharts(t, map[string]string{
			"app.md": "# App\n\nno domain here\n",
		})},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if d, reason, ok := resolveLoneChart(tc.dir); ok || d != "" || reason != "" {
				t.Fatalf("got (%q, %q, %v), want miss", d, reason, ok)
			}
		})
	}
}

// Split pin (C1 S2-S3): a session whose trace files match no lesson
// path falls through to unfiltered-multi — same outcome as the inline
// loop, with the cheap-fallback trim intact.
func TestMemoryContextResolveDomainTraceMissFallsThrough(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "go"), 0755)
	os.MkdirAll(filepath.Join(dir, ".regulus", "library", "ts"), 0755)
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeLesson, Title: "go lesson", Content: "cover",
		Domain: "go", Lesson: &almanac.Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low"},
		Provenance: &almanac.Provenance{Paths: []string{"internal/cli/memory.go"}},
	}, time.Now()); err != nil {
		t.Fatalf("seed go lesson: %v", err)
	}
	_, _ = trace.Record(dir, trace.Entry{Session: "s9", Message: "m", Call: "c", Tool: "edit", Status: "completed", Files: []string{"unrelated/file.txt"}}, time.Now())

	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "context", "--resolve-domain", "--session", "s9"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "reason: unfiltered-multi") {
		t.Fatalf("trace miss must fall through to unfiltered-multi, got:\n%s", got)
	}
}
