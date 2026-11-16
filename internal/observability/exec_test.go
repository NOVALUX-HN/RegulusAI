package observability

// F11.1/NOV-87 (spec S1): versioned execution events — schema,
// idempotent writes, ordered reads, crash reconstruction, delegation
// attribution, limits and pre-store redaction.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func seedExec(t *testing.T, root, runID string, types []string, at time.Time) []ExecEvent {
	t.Helper()
	var out []ExecEvent
	for i, typ := range types {
		e, dup, err := RecordExec(root, ExecEvent{
			RunID: runID, Type: typ, Status: ExecStarted,
			TaskID: "task-1", SessionID: "sess-1",
		}, at.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatalf("RecordExec %s: %v", typ, err)
		}
		if dup {
			t.Fatalf("RecordExec %s: unexpected duplicate", typ)
		}
		out = append(out, e)
	}
	return out
}

func TestExecRecordRoundTrip(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	saved, dup, err := RecordExec(root, ExecEvent{
		RunID: "run-1", TaskID: "task-1", AgentExecutionID: "agent-1",
		DelegationID: "dlg-1", SessionID: "sess-1",
		Type: ExecToolCall, Status: ExecRunning,
		Host: "opencode", Model: "m1", HostVersion: "v1",
		DurationMS: 1500, Payload: map[string]any{"tool": "bash"},
	}, at)
	if err != nil {
		t.Fatalf("RecordExec: %v", err)
	}
	if dup {
		t.Fatal("first write must not report duplicate")
	}
	if saved.SchemaVersion != ExecSchemaVersion {
		t.Errorf("schema_version = %d, want %d", saved.SchemaVersion, ExecSchemaVersion)
	}
	if saved.EventID == "" {
		t.Error("event_id must be minted when empty")
	}
	if !saved.At.Equal(at) {
		t.Errorf("at = %v, want %v", saved.At, at)
	}
	if saved.MonoMS == 0 {
		t.Error("monotonic_ms must be filled when zero")
	}
	events, malformed, dups, err := LoadExec(root)
	if err != nil {
		t.Fatalf("LoadExec: %v", err)
	}
	if malformed != 0 || dups != 0 {
		t.Errorf("malformed=%d dups=%d, want 0/0", malformed, dups)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	got := events[0]
	for _, id := range []struct {
		name, want, have string
	}{
		{"run_id", "run-1", got.RunID},
		{"task_id", "task-1", got.TaskID},
		{"agent_execution_id", "agent-1", got.AgentExecutionID},
		{"delegation_id", "dlg-1", got.DelegationID},
		{"session_id", "sess-1", got.SessionID},
	} {
		if id.have != id.want {
			t.Errorf("%s = %q, want %q", id.name, id.have, id.want)
		}
	}
}

func TestExecDuplicateIdempotent(t *testing.T) {
	root := t.TempDir()
	at := time.Now().UTC()
	first, dup, err := RecordExec(root, ExecEvent{
		EventID: "evt_fixed_1", RunID: "run-1",
		Type: ExecRunStart, Status: ExecStarted,
	}, at)
	if err != nil || dup {
		t.Fatalf("first RecordExec: err=%v dup=%v", err, dup)
	}
	second, dup, err := RecordExec(root, ExecEvent{
		EventID: "evt_fixed_1", RunID: "run-1",
		Type: ExecRunStart, Status: ExecStarted,
	}, at.Add(time.Hour))
	if err != nil {
		t.Fatalf("duplicate RecordExec: %v", err)
	}
	if !dup {
		t.Error("second write with same event_id must report dup=true")
	}
	if second.EventID != first.EventID {
		t.Errorf("dup event_id = %q, want %q", second.EventID, first.EventID)
	}
	events, _, dups, err := LoadExec(root)
	if err != nil {
		t.Fatalf("LoadExec: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("got %d stored lines, want 1 (duplicate must not append)", len(events))
	}
	if dups != 0 {
		t.Errorf("dups = %d, want 0 (writer already dedups)", dups)
	}
}

func TestExecOutOfOrderReadsSorted(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	// Record newest-first: the timeline must still read oldest-first.
	for i := 2; i >= 0; i-- {
		if _, _, err := RecordExec(root, ExecEvent{
			EventID: fmt.Sprintf("evt_ord_%d", i), RunID: "run-1",
			Type: ExecToolCall, Status: ExecRunning,
		}, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("RecordExec: %v", err)
		}
	}
	events, _, _, err := QueryExec(root, ExecFilter{RunID: "run-1"})
	if err != nil {
		t.Fatalf("QueryExec: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].At.Before(events[i-1].At) {
			t.Fatalf("timeline out of order at %d: %v before %v", i, events[i].At, events[i-1].At)
		}
	}
}

func TestExecConcurrentWriters(t *testing.T) {
	root := t.TempDir()
	at := time.Now().UTC()
	const n = 32
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := RecordExec(root, ExecEvent{
				EventID: fmt.Sprintf("evt_conc_%d", i), RunID: "run-1",
				Type: ExecToolCall, Status: ExecRunning,
			}, at)
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	events, malformed, _, err := LoadExec(root)
	if err != nil {
		t.Fatalf("LoadExec: %v", err)
	}
	if malformed != 0 {
		t.Errorf("malformed = %d, want 0", malformed)
	}
	if len(events) != n {
		t.Errorf("got %d events, want %d (concurrent appends must all land)", len(events), n)
	}
}

func TestExecCrashReconstruction(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	// Crashed run: start without any terminal run.end.
	seedExec(t, root, "run-crash", []string{ExecRunStart, ExecTaskStart, ExecToolCall}, base)
	tl, err := ReconstructRun(root, "run-crash")
	if err != nil {
		t.Fatalf("ReconstructRun: %v", err)
	}
	if !tl.Crashed {
		t.Error("run without terminal run.end must read as crashed")
	}
	if tl.Status != ExecCrashed {
		t.Errorf("status = %q, want %q", tl.Status, ExecCrashed)
	}
	if len(tl.Events) != 3 {
		t.Errorf("recovered %d events, want 3", len(tl.Events))
	}
	// Closed run: terminal run.end wins.
	end, _, err := RecordExec(root, ExecEvent{
		RunID: "run-crash", Type: ExecRunEnd, Status: ExecSucceeded,
	}, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("RecordExec end: %v", err)
	}
	_ = end
	tl, err = ReconstructRun(root, "run-crash")
	if err != nil {
		t.Fatalf("ReconstructRun: %v", err)
	}
	if tl.Crashed {
		t.Error("run with terminal run.end must not read as crashed")
	}
	if tl.Status != ExecSucceeded {
		t.Errorf("status = %q, want %q", tl.Status, ExecSucceeded)
	}
	if tl.DurationMS != int64(time.Hour/time.Millisecond) {
		t.Errorf("duration_ms = %d, want %d", tl.DurationMS, int64(time.Hour/time.Millisecond))
	}
	// Unknown run: empty timeline, incomplete verdict, no error.
	tl, err = ReconstructRun(root, "run-missing")
	if err != nil {
		t.Fatalf("ReconstructRun missing: %v", err)
	}
	if len(tl.Events) != 0 || tl.Status != ExecIncomplete {
		t.Errorf("missing run = %d events/%q, want 0/incomplete", len(tl.Events), tl.Status)
	}
}

func TestExecMalformedTolerated(t *testing.T) {
	root := t.TempDir()
	at := time.Now().UTC()
	if _, _, err := RecordExec(root, ExecEvent{RunID: "run-1", Type: ExecRunStart, Status: ExecStarted}, at); err != nil {
		t.Fatalf("RecordExec: %v", err)
	}
	f, err := os.OpenFile(ExecFile(root), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "{this is not json")
	fmt.Fprintln(f, `{"schema_version":999,"event_id":"evt_future","run_id":"run-1"}`)
	f.Close()
	events, malformed, _, err := LoadExec(root)
	if err != nil {
		t.Fatalf("LoadExec must tolerate malformed lines, got: %v", err)
	}
	if malformed != 2 {
		t.Errorf("malformed = %d, want 2 (garbage + unknown schema)", malformed)
	}
	if len(events) != 1 {
		t.Errorf("got %d healthy events, want 1", len(events))
	}
	tl, err := ReconstructRun(root, "run-1")
	if err != nil {
		t.Fatalf("ReconstructRun: %v", err)
	}
	if tl.Malformed != 2 || len(tl.Events) != 1 {
		t.Errorf("timeline = %d events/%d malformed, want 1/2", len(tl.Events), tl.Malformed)
	}
}

func TestExecSecretsRedacted(t *testing.T) {
	root := t.TempDir()
	secret := "sk-live-REDACTME-EXEC-UNIQUE-99"
	saved, _, err := RecordExec(root, ExecEvent{
		RunID: "run-1", Type: ExecToolResult, Status: ExecFailed,
		ErrorDetails: "call failed with api_key=" + secret,
		Payload:      map[string]any{"token": secret, "tool": "bash"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("RecordExec: %v", err)
	}
	if saved.ErrorDetails == "" || strings.Contains(saved.ErrorDetails, secret) {
		t.Errorf("returned event leaks secret: %q", saved.ErrorDetails)
	}
	if v, _ := saved.Payload["token"].(string); v == secret {
		t.Errorf("returned payload leaks secret: %q", v)
	}
	raw, err := os.ReadFile(ExecFile(root))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Errorf("exec log leaks secret on disk: %s", raw)
	}
	if !strings.Contains(string(raw), Redacted) {
		t.Errorf("exec log has no %q marker: %s", Redacted, raw)
	}
}

func TestExecTerminalStatesUnequivocal(t *testing.T) {
	for _, s := range []string{ExecSucceeded, ExecFailed, ExecTimedOut, ExecCancelled} {
		if !IsTerminal(s) {
			t.Errorf("IsTerminal(%q) = false, want true", s)
		}
	}
	for _, s := range []string{ExecStarted, ExecRunning, ExecCrashed, ExecIncomplete, "bogus"} {
		if IsTerminal(s) {
			t.Errorf("IsTerminal(%q) = true, want false", s)
		}
	}
	root := t.TempDir()
	for _, bad := range []ExecEvent{
		{RunID: "r", Type: "nope", Status: ExecStarted},
		{RunID: "r", Type: ExecRunStart, Status: "bogus"},
		{RunID: "r", Type: ExecRunStart, Status: ExecStarted, SchemaVersion: 999},
		{RunID: "r\ninjected", Type: ExecRunStart, Status: ExecStarted},
		{RunID: "r", Type: ExecRunStart, Status: ExecStarted, DurationMS: -1},
	} {
		if _, _, err := RecordExec(root, bad, time.Now().UTC()); err == nil {
			t.Errorf("RecordExec(%+v) must fail validation", bad)
		}
	}
}

func TestExecDelegationAttribution(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	if _, _, err := RecordExec(root, ExecEvent{
		RunID: "run-1", Type: ExecRunStart, Status: ExecStarted,
	}, at); err != nil {
		t.Fatal(err)
	}
	if _, _, err := RecordExec(root, ExecEvent{
		RunID: "run-1", Type: ExecDelegationRequest, Status: ExecRunning,
		AgentExecutionID: "agent_parent", DelegationID: "dlg-7",
		Payload: map[string]any{"to": "agent_child"},
	}, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := RecordExec(root, ExecEvent{
		RunID: "run-1", Type: ExecDelegationResult, Status: ExecSucceeded,
		AgentExecutionID: "agent_child", DelegationID: "dlg-7",
	}, at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	tl, err := ReconstructRun(root, "run-1")
	if err != nil {
		t.Fatalf("ReconstructRun: %v", err)
	}
	if len(tl.Delegations) != 1 {
		t.Fatalf("got %d delegations, want 1", len(tl.Delegations))
	}
	d := tl.Delegations[0]
	if d.FromAgent != "agent_parent" || d.ToAgent != "agent_child" {
		t.Errorf("delegation = %q -> %q, want agent_parent -> agent_child", d.FromAgent, d.ToAgent)
	}
	if d.Status != ExecSucceeded || d.Request == nil || d.Result == nil {
		t.Errorf("delegation not closed: status=%q req=%v res=%v", d.Status, d.Request != nil, d.Result != nil)
	}
}

func TestExecNoFullTranscripts(t *testing.T) {
	root := t.TempDir()
	big := strings.Repeat("x", MaxExecStringValue+100)
	saved, _, err := RecordExec(root, ExecEvent{
		RunID: "run-1", Type: ExecToolResult, Status: ExecSucceeded,
		Payload: map[string]any{"transcript": big, "tool": "bash"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("RecordExec: %v", err)
	}
	got, _ := saved.Payload["transcript"].(string)
	if strings.Contains(got, strings.Repeat("x", MaxExecStringValue+1)) {
		t.Error("oversized payload string must be truncated, full value stored")
	}
	if saved.Payload["truncated"] != true {
		t.Error("truncated payload must carry truncated=true")
	}
	// Refuse payloads that stay oversized after truncation.
	many := map[string]any{}
	for i := 0; i < MaxExecPayloadKeys+1; i++ {
		many[fmt.Sprintf("k%d", i)] = "v"
	}
	if _, _, err := RecordExec(root, ExecEvent{
		RunID: "run-1", Type: ExecContext, Status: ExecRunning, Payload: many,
	}, time.Now().UTC()); err == nil {
		t.Error("payload beyond MaxExecPayloadKeys must be refused")
	}
}

func TestExecIDsUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := MintEventID()
		if err != nil {
			t.Fatalf("MintEventID: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate minted id %q", id)
		}
		seen[id] = true
		if !strings.HasPrefix(id, "evt_") {
			t.Errorf("event id %q must carry its prefix", id)
		}
	}
	for _, fn := range []func() (string, error){MintRunID, MintAgentExecutionID, MintDelegationID} {
		id, err := fn()
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		if strings.ContainsAny(id, "\n\r") || strings.TrimSpace(id) == "" {
			t.Errorf("minted id %q must be a single non-empty line", id)
		}
	}
}

func TestPruneExecRetention(t *testing.T) {
	root := t.TempDir()
	at := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if _, _, err := RecordExec(root, ExecEvent{
			EventID: fmt.Sprintf("evt_prune_%d", i), RunID: "run-1",
			Type: ExecToolCall, Status: ExecRunning,
		}, at.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	kept, err := PruneExec(root, 2)
	if err != nil {
		t.Fatalf("PruneExec: %v", err)
	}
	if kept != 2 {
		t.Errorf("kept = %d, want 2", kept)
	}
	events, _, _, err := LoadExec(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].EventID != "evt_prune_3" {
		t.Errorf("prune must keep the last 2 in order, got %+v", events)
	}
	if _, err := PruneExec(filepath.Join(root, "missing"), 2); err != nil {
		t.Errorf("PruneExec on missing log must read as empty, got: %v", err)
	}
}
