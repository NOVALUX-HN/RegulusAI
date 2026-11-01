package trace

// F1 cost accounting: legacy lines without cost fields keep reading
// (zero = unknown, not zero), new fields validate, and CostByTask
// folds per-task totals that square with a hand fold over List.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCostFieldsRoundTrip(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	saved, err := Record(root, Entry{
		Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted,
		Task: "macizo-flow/F1", DurationMS: 120, OutputBytes: 2048,
		InputTokens: 100, OutputTokens: 50,
	}, now)
	if err != nil {
		t.Fatalf("Record with cost fields: %v", err)
	}
	if saved.Task != "macizo-flow/F1" || saved.DurationMS != 120 || saved.OutputBytes != 2048 ||
		saved.InputTokens != 100 || saved.OutputTokens != 50 {
		t.Fatalf("cost fields did not round-trip: %+v", saved)
	}
	got, err := List(root, "s")
	if err != nil || len(got) != 1 {
		t.Fatalf("List = %v (%v), want 1 entry", got, err)
	}
	if got[0].Task != saved.Task || got[0].DurationMS != saved.DurationMS ||
		got[0].OutputBytes != saved.OutputBytes || got[0].InputTokens != saved.InputTokens ||
		got[0].OutputTokens != saved.OutputTokens || got[0].Tool != saved.Tool {
		t.Fatalf("listed entry = %+v, want %+v", got[0], saved)
	}
}

func TestCostRefusesNegatives(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	base := Entry{Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted}
	for field, mutate := range map[string]func(*Entry){
		"duration_ms":   func(e *Entry) { e.DurationMS = -1 },
		"output_bytes":  func(e *Entry) { e.OutputBytes = -1 },
		"input_tokens":  func(e *Entry) { e.InputTokens = -1 },
		"output_tokens": func(e *Entry) { e.OutputTokens = -1 },
	} {
		e := base
		mutate(&e)
		if _, err := Record(root, e, now); err == nil {
			t.Fatalf("want refusal for negative %s, got nil", field)
		}
	}
	if got, _ := List(root, "s"); len(got) != 0 {
		t.Fatalf("refused records must store nothing, got %d entries", len(got))
	}
}

func TestCostRefusesMultilineTask(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	if _, err := Record(root, Entry{
		Session: "s", Message: "m", Call: "c", Tool: "t",
		Status: StatusCompleted, Task: "a\nb",
	}, now); err == nil {
		t.Fatal("want refusal for multiline task, got nil")
	}
}

func TestLegacyLinesReadAsUnknown(t *testing.T) {
	root := t.TempDir()
	// A line written before the F1 fields existed: no task, no costs.
	raw := `{"at":"2026-10-31T12:00:00Z","session":"old","message":"m","call":"c","tool":"bash","status":"completed"}` + "\n"
	if err := os.MkdirAll(filepath.Join(root, ".regulus", "traces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(File(root, "old"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := List(root, "old")
	if err != nil || len(got) != 1 {
		t.Fatalf("legacy List = %v (%v), want 1 entry", got, err)
	}
	e := got[0]
	if e.Task != "" || e.DurationMS != 0 || e.OutputBytes != 0 || e.InputTokens != 0 || e.OutputTokens != 0 {
		t.Fatalf("legacy line must decode as all-zero cost: %+v", e)
	}
	costs, err := CostByTask(root)
	if err != nil {
		t.Fatalf("CostByTask: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("got %d cost buckets, want 1", len(costs))
	}
	c := costs[0]
	if c.Task != UnattributedTask || c.Calls != 1 || c.Entries != 1 {
		t.Fatalf("legacy bucket = %+v, want 1 unattributed call", c)
	}
	if c.DurationKnown || c.BytesKnown || c.TokensKnown {
		t.Fatalf("legacy costs must read unknown, not zero: %+v", c)
	}
	// Fingerprint ignores every cost field: old and new lines with
	// the same tool+status diff identically.
	fp := Fingerprint(got)
	if len(fp) != 1 || fp[0] != "1 bash completed" {
		t.Fatalf("legacy fingerprint = %v", fp)
	}
}

func seedCostWorld(t *testing.T, root string, now time.Time) {
	t.Helper()
	calls := []Entry{
		{Session: "s1", Message: "m1", Call: "c1", Tool: "read", Status: StatusCompleted, Task: "F1", DurationMS: 100, OutputBytes: 1000, InputTokens: 10, OutputTokens: 5},
		{Session: "s1", Message: "m2", Call: "c2", Tool: "write", Status: StatusError, Task: "F1", DurationMS: 200, OutputBytes: 2000},
		{Session: "s2", Message: "m3", Call: "c3", Tool: "bash", Status: StatusCompleted, Task: "F2", DurationMS: 50, OutputBytes: 500, InputTokens: 7, OutputTokens: 3},
		{Session: "s2", Message: "m4", Call: "c4", Tool: "read", Status: StatusCompleted},
	}
	for i, e := range calls {
		if _, err := Record(root, e, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("seed call %d: %v", i, err)
		}
	}
}

func TestCostByTaskGroupsAndAudits(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedCostWorld(t, root, now)

	costs, err := CostByTask(root)
	if err != nil {
		t.Fatalf("CostByTask: %v", err)
	}
	if len(costs) != 3 {
		t.Fatalf("got %d buckets, want 3 (F1, F2, unattributed)", len(costs))
	}
	if costs[0].Task != "(unattributed)" || costs[1].Task != "F1" || costs[2].Task != "F2" {
		t.Fatalf("buckets not sorted by task: %v", costs)
	}
	f1 := costs[1]
	if f1.Calls != 2 || f1.Errors != 1 || f1.Entries != 2 {
		t.Fatalf("F1 calls/errors/entries = %d/%d/%d, want 2/1/2", f1.Calls, f1.Errors, f1.Entries)
	}
	if f1.TotalDurationMS != 300 || f1.TotalOutputBytes != 3000 {
		t.Fatalf("F1 proxy = %dms/%dB, want 300ms/3000B", f1.TotalDurationMS, f1.TotalOutputBytes)
	}
	if f1.TotalInputTokens != 10 || f1.TotalOutputTokens != 5 || !f1.TokensKnown {
		t.Fatalf("F1 tokens = %d+%d known=%v, want 10+5 known", f1.TotalInputTokens, f1.TotalOutputTokens, f1.TokensKnown)
	}
	if !f1.DurationKnown || !f1.BytesKnown {
		t.Fatalf("F1 proxy must read known: %+v", f1)
	}
	if len(f1.Sessions) != 1 || f1.Sessions[0] != "s1" {
		t.Fatalf("F1 sessions = %v, want [s1]", f1.Sessions)
	}

	// Auditability: a hand fold over List reproduces every bucket.
	sessions, err := Sessions(root)
	if err != nil {
		t.Fatal(err)
	}
	audit := map[string]TaskCost{}
	for _, s := range sessions {
		entries, err := List(root, s)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			key := taskKey(e)
			c := audit[key]
			if c.Task == "" {
				c.Task = key
			}
			c.fold(s, e)
			audit[key] = c
		}
	}
	for _, c := range costs {
		a := audit[c.Task]
		if a.Calls != c.Calls || a.Errors != c.Errors ||
			a.TotalDurationMS != c.TotalDurationMS || a.TotalOutputBytes != c.TotalOutputBytes ||
			a.TotalInputTokens != c.TotalInputTokens || a.TotalOutputTokens != c.TotalOutputTokens ||
			a.DurationKnown != c.DurationKnown || a.BytesKnown != c.BytesKnown || a.TokensKnown != c.TokensKnown ||
			strings.Join(a.Sessions, ",") != strings.Join(c.Sessions, ",") {
			t.Fatalf("bucket %q not reproducible by hand fold:\nreport %+v\naudit  %+v", c.Task, c, a)
		}
	}
}

func TestCostForTask(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedCostWorld(t, root, now)

	f2, err := CostForTask(root, "F2")
	if err != nil {
		t.Fatalf("CostForTask: %v", err)
	}
	if f2.Calls != 1 || f2.TotalDurationMS != 50 || f2.TotalInputTokens != 7 || !f2.TokensKnown {
		t.Fatalf("F2 = %+v", f2)
	}
	ghost, err := CostForTask(root, "ghost")
	if err != nil {
		t.Fatalf("CostForTask ghost: %v", err)
	}
	if ghost.Task != "ghost" || ghost.Calls != 0 {
		t.Fatalf("unknown task must yield zero cost, got %+v", ghost)
	}
	unatt, err := CostForTask(root, "")
	if err != nil {
		t.Fatalf("CostForTask unattributed: %v", err)
	}
	if unatt.Task != UnattributedTask || unatt.Calls != 1 {
		t.Fatalf("unattributed = %+v, want 1 call", unatt)
	}
}

func TestFingerprintIgnoresCostFields(t *testing.T) {
	plain := Fingerprint([]Entry{{Tool: "read", Status: StatusCompleted}})
	costly := Fingerprint([]Entry{{
		Tool: "read", Status: StatusCompleted, Task: "F1",
		DurationMS: 999, OutputBytes: 999, InputTokens: 999, OutputTokens: 999,
		Files: []string{"a.go"},
	}})
	if len(plain) != 1 || len(costly) != 1 || plain[0] != costly[0] {
		t.Fatalf("cost fields changed the fingerprint: %v vs %v", plain, costly)
	}
}

func TestCostMalformedLineFails(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	if _, err := Record(root, Entry{Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted}, now); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(File(root, "s"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not valid json\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := CostByTask(root); err == nil {
		t.Fatal("CostByTask hid a malformed line; want the error surfaced")
	}
}
