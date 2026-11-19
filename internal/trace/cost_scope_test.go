// CostScope tests: direct fold over Record/List on a temp root — fast,
// deterministic, no shell. Covers every dim switch (task/agent/host +
// default + unknown error) and every keep predicate (task/agent/run).
package trace

import (
	"testing"
	"time"
)

func seedScopeWorld(t *testing.T, root string, now time.Time) {
	t.Helper()
	calls := []Entry{
		{Session: "s1", Message: "m1", Call: "c1", Tool: "read", Status: StatusCompleted,
			Task: "T1", Agent: "worker-a", ExecutionID: "run-1", Host: "h1",
			DurationMS: 100, OutputBytes: 1000},
		{Session: "s1", Message: "m2", Call: "c2", Tool: "write", Status: StatusError,
			Task: "T1", Agent: "worker-b", ExecutionID: "run-2", Host: "h1",
			DurationMS: 200, OutputBytes: 2000},
		{Session: "s2", Message: "m3", Call: "c3", Tool: "bash", Status: StatusCompleted,
			Task: "T2", Agent: "worker-a", ExecutionID: "run-1", Host: "h2",
			DurationMS: 50, OutputBytes: 500},
		{Session: "s2", Message: "m4", Call: "c4", Tool: "read", Status: StatusCompleted},
	}
	for i, e := range calls {
		if _, err := Record(root, e, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("seed call %d: %v", i, err)
		}
	}
}

func scopeBucketsByName(costs []TaskCost) map[string]TaskCost {
	out := map[string]TaskCost{}
	for _, c := range costs {
		out[c.Task] = c
	}
	return out
}

func TestCostScopeDefaultsToTask(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	costs, err := CostScope(root, "", "", "", "")
	if err != nil {
		t.Fatalf("CostScope default: %v", err)
	}
	byName := scopeBucketsByName(costs)
	if len(costs) != 3 {
		t.Fatalf("got %d buckets, want 3 (T1, T2, unattributed): %v", len(costs), costs)
	}
	if byName["T1"].Calls != 2 || byName["T2"].Calls != 1 || byName[UnattributedTask].Calls != 1 {
		t.Fatalf("task buckets = %+v, want T1:2 T2:1 unattributed:1", byName)
	}
	if byName["T1"].Errors != 1 || byName["T1"].TotalDurationMS != 300 {
		t.Fatalf("T1 fold = %+v, want 1 error and 300ms", byName["T1"])
	}
}

func TestCostScopeTaskDimExplicit(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	costs, err := CostScope(root, "task", "", "", "")
	if err != nil {
		t.Fatalf("CostScope task: %v", err)
	}
	if len(costs) != 3 {
		t.Fatalf("got %d task buckets, want 3", len(costs))
	}
}

func TestCostScopeAgentDim(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	costs, err := CostScope(root, "agent", "", "", "")
	if err != nil {
		t.Fatalf("CostScope agent: %v", err)
	}
	byName := scopeBucketsByName(costs)
	if len(costs) != 3 {
		t.Fatalf("got %d agent buckets, want 3 (worker-a, worker-b, unattributed)", len(costs))
	}
	if byName["worker-a"].Calls != 2 || byName["worker-b"].Calls != 1 || byName[UnattributedAgent].Calls != 1 {
		t.Fatalf("agent buckets = %+v, want worker-a:2 worker-b:1 unattributed:1", byName)
	}
}

func TestCostScopeHostDim(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	costs, err := CostScope(root, "host", "", "", "")
	if err != nil {
		t.Fatalf("CostScope host: %v", err)
	}
	byName := scopeBucketsByName(costs)
	if len(costs) != 3 {
		t.Fatalf("got %d host buckets, want 3 (h1, h2, unattributed)", len(costs))
	}
	if byName["h1"].Calls != 2 || byName["h2"].Calls != 1 || byName[UnattributedAgent].Calls != 1 {
		t.Fatalf("host buckets = %+v, want h1:2 h2:1 unattributed:1", byName)
	}
}

func TestCostScopeUnknownDimFails(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	if _, err := CostScope(root, "phase", "", "", ""); err == nil {
		t.Fatal("want naming error for unknown dim, got nil")
	} else if got := err.Error(); got != `cost grouping must be task, agent or host, got "phase"` {
		t.Fatalf("err = %q, want exact naming error", got)
	}
}

func TestCostScopeTaskFilter(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	costs, err := CostScope(root, "task", "T1", "", "")
	if err != nil {
		t.Fatalf("CostScope task filter: %v", err)
	}
	if len(costs) != 1 || costs[0].Task != "T1" || costs[0].Calls != 2 {
		t.Fatalf("task=T1 scope = %+v, want one T1 bucket with 2 calls", costs)
	}
}

func TestCostScopeAgentFilter(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	costs, err := CostScope(root, "task", "", "worker-a", "")
	if err != nil {
		t.Fatalf("CostScope agent filter: %v", err)
	}
	byName := scopeBucketsByName(costs)
	if len(costs) != 2 || byName["T1"].Calls != 1 || byName["T2"].Calls != 1 {
		t.Fatalf("agent=worker-a scope = %+v, want T1:1 T2:1", byName)
	}
}

func TestCostScopeRunFilter(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	costs, err := CostScope(root, "task", "", "", "run-1")
	if err != nil {
		t.Fatalf("CostScope run filter: %v", err)
	}
	byName := scopeBucketsByName(costs)
	if len(costs) != 2 || byName["T1"].Calls != 1 || byName["T2"].Calls != 1 {
		t.Fatalf("run=run-1 scope = %+v, want T1:1 T2:1", byName)
	}
}

func TestCostScopeCombinedFilters(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	costs, err := CostScope(root, "agent", "T1", "worker-a", "run-1")
	if err != nil {
		t.Fatalf("CostScope combined: %v", err)
	}
	if len(costs) != 1 || costs[0].Task != "worker-a" || costs[0].Calls != 1 {
		t.Fatalf("combined scope = %+v, want one worker-a bucket with 1 call", costs)
	}
}

func TestCostScopeNoMatchYieldsEmpty(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	seedScopeWorld(t, root, now)

	for _, tc := range [][4]string{
		{"task", "missing", "", ""},
		{"task", "", "missing", ""},
		{"task", "", "", "missing"},
	} {
		costs, err := CostScope(root, tc[0], tc[1], tc[2], tc[3])
		if err != nil {
			t.Fatalf("CostScope %v: %v", tc, err)
		}
		if len(costs) != 0 {
			t.Fatalf("CostScope %v = %+v, want no buckets", tc, costs)
		}
	}
}
