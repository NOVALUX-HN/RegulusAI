package trace

// T3 (pro-harness, token-economizer): UsageByTool folds the trace logs
// into per-tool usage — calls plus distinct session count — the
// measured input the plugin's toolset pruning reads. Deterministic
// from the same JSONL lines List reads; an uncalled tool never appears
// (absence is the prune signal, carried by the session count).
import (
	"testing"
	"time"
)

func TestUsageByToolFoldsCallsAndSessions(t *testing.T) {
	root := t.TempDir()
	mk := func(session, tool string) {
		t.Helper()
		if _, err := Record(root, Entry{
			Session: session, Message: "m", Call: "c-" + session + "-" + tool,
			Tool: tool, Status: StatusCompleted,
		}, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	mk("s1", "read")
	mk("s1", "read")
	mk("s1", "edit")
	mk("s2", "read")
	mk("s3", "edit")
	got, err := UsageByTool(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("usage rows = %d, want 2 (read, edit)", len(got))
	}
	byTool := map[string]ToolUsage{}
	for _, u := range got {
		byTool[u.Tool] = u
	}
	if byTool["read"].Calls != 3 || len(byTool["read"].Sessions) != 2 {
		t.Errorf("read = %+v, want 3 calls across 2 sessions", byTool["read"])
	}
	if byTool["edit"].Calls != 2 || len(byTool["edit"].Sessions) != 2 {
		t.Errorf("edit = %+v, want 2 calls across 2 sessions", byTool["edit"])
	}
}

func TestUsageByToolEmptyRoot(t *testing.T) {
	got, err := UsageByTool(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("empty root usage = %d rows, want 0", len(got))
	}
	sess, err := UsageSessions(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if sess != 0 {
		t.Fatalf("empty root sessions = %d, want 0", sess)
	}
}
