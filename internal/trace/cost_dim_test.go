package trace

// NOV-39 (S14): attribution dimensions (phase/agent/hook) fold through
// the same fold/taskKey machinery as tasks, legacy lines land in the
// "(unattributed)" buckets, and per-tool local proxy costs are labeled
// measured-local — never host counts.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func seedDimWorld(t *testing.T, root string, now time.Time) {
	t.Helper()
	calls := []Entry{
		{Session: "s1", Message: "m1", Call: "c1", Tool: "read", Status: StatusCompleted,
			Task: "T1", Phase: "plan", Agent: "worker-a", Hook: "pre",
			DurationMS: 100, OutputBytes: 1000},
		{Session: "s1", Message: "m2", Call: "c2", Tool: "write", Status: StatusError,
			Task: "T1", Phase: "plan", Agent: "worker-a", Hook: "pre",
			DurationMS: 200, OutputBytes: 2000},
		{Session: "s2", Message: "m3", Call: "c3", Tool: "read", Status: StatusCompleted,
			Task: "T2", Phase: "act", Agent: "worker-b",
			DurationMS: 50, OutputBytes: 500},
		{Session: "s2", Message: "m4", Call: "c4", Tool: "bash", Status: StatusCompleted},
	}
	for i, e := range calls {
		if _, err := Record(root, e, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("seed call %d: %v", i, err)
		}
	}
}

func TestDimensionsRoundTrip(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	saved, err := Record(root, Entry{
		Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted,
		Task: "T1", Phase: "plan", Agent: "worker-a", Hook: "pre",
	}, now)
	if err != nil {
		t.Fatalf("Record with dimensions: %v", err)
	}
	if saved.Phase != "plan" || saved.Agent != "worker-a" || saved.Hook != "pre" {
		t.Fatalf("dimensions did not round-trip: %+v", saved)
	}
	got, err := List(root, "s")
	if err != nil || len(got) != 1 {
		t.Fatalf("List = %v (%v), want 1 entry", got, err)
	}
	if got[0].Phase != "plan" || got[0].Agent != "worker-a" || got[0].Hook != "pre" {
		t.Fatalf("listed dimensions lost: %+v", got[0])
	}
}

func TestDimensionsRefuseMultiline(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	base := Entry{Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted}
	for field, mutate := range map[string]func(*Entry){
		"phase": func(e *Entry) { e.Phase = "a\nb" },
		"agent": func(e *Entry) { e.Agent = "a\rb" },
		"hook":  func(e *Entry) { e.Hook = "a\nb" },
	} {
		e := base
		mutate(&e)
		if _, err := Record(root, e, now); err == nil {
			t.Fatalf("want refusal for multiline %s, got nil", field)
		}
	}
	if got, _ := List(root, "s"); len(got) != 0 {
		t.Fatalf("refused records must store nothing, got %d entries", len(got))
	}
}

func TestLegacyLinesLandInUnattributedDimensions(t *testing.T) {
	root := t.TempDir()
	// A line written before the NOV-39 fields existed: no dimensions.
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
	if got[0].Phase != "" || got[0].Agent != "" || got[0].Hook != "" {
		t.Fatalf("legacy line must decode with empty dimensions: %+v", got[0])
	}
	for dim, fold := range map[string]func(string) ([]TaskCost, error){
		"phase": CostByPhase, "agent": CostByAgent, "hook": CostByHook,
	} {
		costs, err := fold(root)
		if err != nil {
			t.Fatalf("CostBy%s: %v", dim, err)
		}
		if len(costs) != 1 || costs[0].Task != "(unattributed)" || costs[0].Calls != 1 {
			t.Fatalf("legacy %s bucket = %+v, want 1 unattributed call", dim, costs)
		}
	}
}

func TestCostByDimensionsGroupsAndAudits(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedDimWorld(t, root, now)

	phases, err := CostByPhase(root)
	if err != nil {
		t.Fatalf("CostByPhase: %v", err)
	}
	if len(phases) != 3 {
		t.Fatalf("got %d phase buckets, want 3 (plan, act, unattributed)", len(phases))
	}
	// Sorted: "(unattributed)" < "act" < "plan" in byte order.
	if phases[0].Task != "(unattributed)" || phases[1].Task != "act" || phases[2].Task != "plan" {
		t.Fatalf("phase buckets not sorted: %v", phases)
	}
	plan := phases[2]
	if plan.Calls != 2 || plan.Errors != 1 || plan.Entries != 2 {
		t.Fatalf("plan calls/errors/entries = %d/%d/%d, want 2/1/2", plan.Calls, plan.Errors, plan.Entries)
	}
	if plan.TotalDurationMS != 300 || plan.TotalOutputBytes != 3000 {
		t.Fatalf("plan proxy = %dms/%dB, want 300ms/3000B", plan.TotalDurationMS, plan.TotalOutputBytes)
	}
	if !plan.DurationKnown || !plan.BytesKnown {
		t.Fatalf("plan proxy must read known: %+v", plan)
	}
	if plan.TokensKnown || plan.TotalInputTokens != 0 || plan.TotalOutputTokens != 0 {
		t.Fatalf("plan host tokens must stay unknown: %+v", plan)
	}
	if !plan.LocalTokensKnown || plan.TotalLocalTokens <= 0 {
		t.Fatalf("plan local proxy must measure: %+v", plan)
	}

	agents, err := CostByAgent(root)
	if err != nil {
		t.Fatalf("CostByAgent: %v", err)
	}
	if len(agents) != 3 {
		t.Fatalf("got %d agent buckets, want 3 (worker-a, worker-b, unattributed)", len(agents))
	}

	hooks, err := CostByHook(root)
	if err != nil {
		t.Fatalf("CostByHook: %v", err)
	}
	if len(hooks) != 2 {
		t.Fatalf("got %d hook buckets, want 2 (pre, unattributed)", len(hooks))
	}
	if hooks[0].Task != "(unattributed)" || hooks[1].Task != "pre" {
		t.Fatalf("hook buckets not sorted: %v", hooks)
	}
	if hooks[1].Calls != 2 {
		t.Fatalf("pre hook calls = %d, want 2", hooks[1].Calls)
	}

	// Auditability: a hand fold over List with the same key function
	// reproduces every dimension bucket.
	for dim, tc := range map[string]struct {
		key  func(Entry) string
		fold func(string) ([]TaskCost, error)
	}{
		"task":  {taskKey, CostByTask},
		"phase": {phaseKey, CostByPhase},
		"agent": {agentKey, CostByAgent},
		"hook":  {hookKey, CostByHook},
	} {
		key, fold := tc.key, tc.fold
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
				k := key(e)
				c := audit[k]
				if c.Task == "" {
					c.Task = k
				}
				c.fold(s, e)
				audit[k] = c
			}
		}
		costs, err := fold(root)
		if err != nil {
			t.Fatalf("CostBy%s: %v", dim, err)
		}
		if len(costs) != len(audit) {
			t.Fatalf("%s buckets = %d, audit = %d", dim, len(costs), len(audit))
		}
		for _, c := range costs {
			a := audit[c.Task]
			if a.Calls != c.Calls || a.Errors != c.Errors ||
				a.TotalDurationMS != c.TotalDurationMS || a.TotalOutputBytes != c.TotalOutputBytes ||
				a.TotalInputTokens != c.TotalInputTokens || a.TotalOutputTokens != c.TotalOutputTokens ||
				a.TotalLocalTokens != c.TotalLocalTokens ||
				a.DurationKnown != c.DurationKnown || a.BytesKnown != c.BytesKnown || a.TokensKnown != c.TokensKnown ||
				a.LocalTokensKnown != c.LocalTokensKnown ||
				strings.Join(a.Sessions, ",") != strings.Join(c.Sessions, ",") {
				t.Fatalf("%s bucket %q not reproducible by hand fold:\nreport %+v\naudit  %+v", dim, c.Task, c, a)
			}
		}
	}
}

func TestFingerprintIgnoresDimensions(t *testing.T) {
	plain := Fingerprint([]Entry{{Tool: "read", Status: StatusCompleted}})
	dimmed := Fingerprint([]Entry{{
		Tool: "read", Status: StatusCompleted,
		Task: "T1", Phase: "plan", Agent: "worker-a", Hook: "pre",
		DurationMS: 999, OutputBytes: 999, InputTokens: 999, OutputTokens: 999,
	}})
	if len(plain) != 1 || len(dimmed) != 1 || plain[0] != dimmed[0] {
		t.Fatalf("dimensions changed the fingerprint: %v vs %v", plain, dimmed)
	}
}

func TestCostByToolLabelsMeasuredLocal(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedDimWorld(t, root, now)

	costs, err := CostByTool(root)
	if err != nil {
		t.Fatalf("CostByTool: %v", err)
	}
	if len(costs) != 3 {
		t.Fatalf("got %d tool buckets, want 3 (bash, read, write)", len(costs))
	}
	if costs[0].Tool != "bash" || costs[1].Tool != "read" || costs[2].Tool != "write" {
		t.Fatalf("tool buckets not sorted by tool: %v", costs)
	}
	byTool := map[string]ToolCost{}
	for _, c := range costs {
		byTool[c.Tool] = c
	}
	if byTool["read"].Calls != 2 || len(byTool["read"].Sessions) != 2 {
		t.Errorf("read = %+v, want 2 calls across 2 sessions", byTool["read"])
	}
	if byTool["write"].Errors != 1 {
		t.Errorf("write errors = %d, want 1", byTool["write"].Errors)
	}
	for _, c := range costs {
		if !c.LocalTokensKnown || c.TotalLocalTokens <= 0 {
			t.Errorf("tool %s local proxy must measure: %+v", c.Tool, c)
		}
		if c.LocalTokensSource != "measured-local" {
			t.Errorf("tool %s source = %q, want explicit measured-local", c.Tool, c.LocalTokensSource)
		}
	}

	// Auditability: hand-folded EstimateLocalTokens sums square.
	sessions, err := Sessions(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{}
	for _, s := range sessions {
		entries, err := List(root, s)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			want[e.Tool] += EstimateLocalTokens(e)
		}
	}
	for _, c := range costs {
		if c.TotalLocalTokens != want[c.Tool] {
			t.Errorf("tool %s local = %d, hand fold = %d", c.Tool, c.TotalLocalTokens, want[c.Tool])
		}
	}
}

func TestCostByToolEmptyRoot(t *testing.T) {
	got, err := CostByTool(t.TempDir())
	if err != nil {
		t.Fatalf("CostByTool on empty root: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty root tool costs = %d rows, want 0", len(got))
	}
}

func TestCostByToolMalformedLineFails(t *testing.T) {
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
	for dim, fold := range map[string]func(string) ([]TaskCost, error){
		"phase": CostByPhase, "agent": CostByAgent, "hook": CostByHook,
	} {
		if _, err := fold(root); err == nil {
			t.Fatalf("CostBy%s hid a malformed line; want the error surfaced", dim)
		}
	}
	if _, err := CostByTool(root); err == nil {
		t.Fatal("CostByTool hid a malformed line; want the error surfaced")
	}
}
