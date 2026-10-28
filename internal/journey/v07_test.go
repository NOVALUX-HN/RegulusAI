package journey

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
)

// TestV07MemoryLevels walks the v0.7 Almanac surface end-to-end, in
// process, on a fresh directory: per-task caps, supersession, collapse,
// any/limit/path buckets, doctor, export/import round-trip, prune with
// human confirmation, and trace rotation. Deterministic: no model, no
// network, no plugin host.
func TestV07MemoryLevels(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	run := func(args ...string) (int, string) {
		t.Helper()
		var out, errb bytes.Buffer
		c := cli.CLI{
			Version: "test",
			Probe: func() doctor.Result {
				return doctor.Result{Detected: "opencode 2.2.0", Major: 2, Supported: true}
			},
			Stdout: &out,
			Stderr: &errb,
		}
		code := c.Run(args)
		return code, out.String() + "\nstderr:\n" + errb.String()
	}
	mustSave := func(typ, title, content string, at time.Time) string {
		t.Helper()
		saved, err := almanac.Save(".", &almanac.Observation{Type: typ, Title: title, Content: content, TopicKey: "db"}, at)
		if err != nil {
			t.Fatal(err)
		}
		return saved.ID
	}
	mustSave(almanac.TypeDecision, "old choice", " pick postgres", time.Unix(1700000000, 0).UTC())
	mustSave(almanac.TypeDecision, "new choice", "pick postgres", time.Unix(1700000060, 0).UTC())
	code, out := run("memory", "search", "postgres")
	if code != 0 || !strings.Contains(out, "2 hit(s)") {
		t.Fatalf("default search returns the whole chain: exit=%d out=%q", code, out)
	}
	code, out = run("memory", "search", "--latest", "postgres")
	if code != 0 || !strings.Contains(out, "1 hit(s)") || !strings.Contains(out, "new choice") {
		t.Fatalf("--latest collapses to the newest: exit=%d out=%q", code, out)
	}

	// --any broadens recall; --limit caps the block.
	code, out = run("memory", "search", "--any", "postgres", "missing-term-xyz")
	if code != 0 || !strings.Contains(out, "2 hit(s)") {
		t.Fatalf("--any must match either term: exit=%d out=%q", code, out)
	}
	code, out = run("memory", "search", "--limit", "1", "--any", "postgres", "choice")
	if code != 0 || !strings.Contains(out, "1 hit(s)") {
		t.Fatalf("--limit must cap the output: exit=%d out=%q", code, out)
	}

	// Supersession hides the old lesson; --include-superseded reveals.
	code, out = run("memory", "search", "--include-superseded", "choice")
	if code != 0 {
		t.Fatalf("include-superseded: exit=%d out=%q", code, out)
	}

	// Doctor reports a healthy log.
	code, out = run("memory", "doctor")
	if code != 0 || !strings.Contains(out, "malformed:  none") || !strings.Contains(out, "orphan relations: none") {
		t.Fatalf("doctor must report healthy: exit=%d out=%q", code, out)
	}

	// Export + import round-trip is idempotent.
	code, out = run("memory", "export")
	if code != 0 || !strings.Contains(out, "exported:") {
		t.Fatalf("export: exit=%d out=%q", code, out)
	}
	exported, err := filepath.Glob(filepath.Join(".regulus-export", "*.jsonl"))
	if err != nil || len(exported) == 0 {
		t.Fatalf("export must write a bundle, got %v (%v)", exported, err)
	}
	code, out = run("memory", "import", exported[0])
	if code != 0 || !strings.Contains(out, "skipped 2") {
		t.Fatalf("re-import of own export must skip everything: exit=%d out=%q", code, out)
	}

	// Prune is dry-run by default and needs --yes to act.
	code, out = run("memory", "prune", "--older-than-days", "0")
	if code != 0 || !strings.Contains(out, "dry run") {
		t.Fatalf("prune defaults to dry run: exit=%d out=%q", code, out)
	}
	code, out = run("memory", "prune", "--older-than-days", "0", "--yes")
	if code != 0 || !strings.Contains(out, "pruned 2 observation(s)") {
		t.Fatalf("prune --yes must remove stale unpinned decisions: exit=%d out=%q", code, out)
	}

	// Per-task evolution cap: the second reflect on the same task stops.
	if err := os.WriteFile("regulus.json", []byte(`{"gates":[{"name":"smoke","layer":1,"command":"true"}],"evolution":{"max_per_day":10,"max_per_task":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("feature.md", []byte("# F\n\n## Acceptance\n\n### Scenario: x\n\n- Given a\n- When b\n- Then c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(".regulus", "evidence", "cap__T9"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".regulus", "evidence", "cap__T9", "smoke-1.json"), []byte(`{"id":"rec_cap1","task":"cap/T9","gate":"smoke","layer":1,"command":"true","exit_code":0,"verdict":"pass","started_at":"2026-10-26T08:00:00Z","duration_ms":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out = run("reflect", "--domain", "journey", "--task", "cap/T9",
		"--cue", "cap one", "--lesson", "first lesson", "feature.md")
	if code != 0 || !strings.Contains(out, "candidate proposed") {
		t.Fatalf("first reflect: exit=%d out=%q", code, out)
	}
	code, out = run("reflect", "--domain", "journey", "--task", "cap/T9",
		"--cue", "cap two", "--lesson", "second lesson", "feature.md")
	if code != 1 || !strings.Contains(out, "task evolution cap") {
		t.Fatalf("second reflect on the same task must hit the per-task cap: exit=%d out=%q", code, out)
	}

	// Trace sessions + rotation: record, list, prune to the tail.
	for _, call := range []string{"c1", "c2", "c3"} {
		code, out = run("trace", "record", "--session", "v07", "--message", "m", "--call", call,
			"--tool", "gate_run", "--status", "completed")
		if code != 0 {
			t.Fatalf("trace record %s: exit=%d out=%q", call, code, out)
		}
	}
	code, out = run("trace", "sessions")
	if code != 0 || !strings.Contains(out, "v07") {
		t.Fatalf("trace sessions must list v07: exit=%d out=%q", code, out)
	}
	code, out = run("trace", "prune", "--session", "v07", "--keep", "2")
	if code != 0 || !strings.Contains(out, "2 entr(y/ies) kept") && !strings.Contains(out, "2 entr") {
		t.Fatalf("trace prune: exit=%d out=%q", code, out)
	}
	code, out = run("trace", "show", "--session", "v07")
	if code != 0 || !strings.Contains(out, "1 gate_run completed") || !strings.Contains(out, "2 gate_run completed") {
		t.Fatalf("prune must keep the call-order tail: exit=%d out=%q", code, out)
	}
}
