package trace

// NOV-34 (S17): correlated executions share one execution id. The gate
// run's runID travels as trace Entry execution_id (optional, omitempty);
// projections join execution_id==RunID; legacy lines without an id fold
// into the "(unattributed)" bucket; instrumentation failures are errors,
// never panics.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecutionIDsRoundTrip(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	saved, err := Record(root, Entry{
		Session: "s1", Message: "m1", Call: "c1", Tool: "read",
		Status: StatusCompleted, Task: "T1",
		ExecutionID: "run_x", CorrelationID: "corr_x",
	}, now)
	if err != nil {
		t.Fatalf("Record with execution ids: %v", err)
	}
	if saved.ExecutionID != "run_x" || saved.CorrelationID != "corr_x" {
		t.Fatalf("saved ids = %q/%q, want run_x/corr_x", saved.ExecutionID, saved.CorrelationID)
	}
	entries, err := List(root, "s1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List = %d entries, want 1", len(entries))
	}
	if entries[0].ExecutionID != "run_x" || entries[0].CorrelationID != "corr_x" {
		t.Errorf("listed ids = %q/%q, want run_x/corr_x", entries[0].ExecutionID, entries[0].CorrelationID)
	}
}

func TestExecutionIDsValidateSingleLine(t *testing.T) {
	for name, e := range map[string]Entry{
		"execution_id":   {Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted, ExecutionID: "a\nb"},
		"correlation_id": {Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted, CorrelationID: "a\rb"},
	} {
		root := t.TempDir()
		if _, err := Record(root, e, time.Now()); err == nil {
			t.Errorf("%s with newline: want a single-line validation error, got nil", name)
		} else if !strings.Contains(err.Error(), "single line") {
			t.Errorf("%s error = %q, want it to name the single-line rule", name, err)
		}
		if entries, lerr := List(root, "s"); lerr != nil || len(entries) != 0 {
			t.Errorf("%s: rejected entry must leave no stored data (entries=%d err=%v)", name, len(entries), lerr)
		}
	}
}

func TestFingerprintIgnoresExecutionIDs(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	plain := t.TempDir()
	tagged := t.TempDir()
	base := Entry{Session: "s", Message: "m", Call: "c", Tool: "read", Status: StatusCompleted, Task: "T1"}
	if _, err := Record(plain, base, now); err != nil {
		t.Fatalf("Record plain: %v", err)
	}
	withIDs := base
	withIDs.ExecutionID = "run_x"
	withIDs.CorrelationID = "corr_x"
	if _, err := Record(tagged, withIDs, now); err != nil {
		t.Fatalf("Record tagged: %v", err)
	}
	pEntries, err := List(plain, "s")
	if err != nil {
		t.Fatalf("List plain: %v", err)
	}
	tEntries, err := List(tagged, "s")
	if err != nil {
		t.Fatalf("List tagged: %v", err)
	}
	pfp, tfp := Fingerprint(pEntries), Fingerprint(tEntries)
	if len(pfp) != 1 || len(tfp) != 1 || pfp[0] != tfp[0] {
		t.Errorf("fingerprint plain=%v tagged=%v: execution ids must not change contrast diffs", pfp, tfp)
	}
}

func TestExecutionKeyBucketsLegacy(t *testing.T) {
	if got := ExecutionKey(Entry{}); got != UnattributedExecution {
		t.Errorf("ExecutionKey(empty) = %q, want %q", got, UnattributedExecution)
	}
	if got := ExecutionKey(Entry{ExecutionID: "run_x"}); got != "run_x" {
		t.Errorf("ExecutionKey(tagged) = %q, want run_x", got)
	}
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	if _, err := Record(root, Entry{
		Session: "s", Message: "m", Call: "c", Tool: "read", Status: StatusCompleted,
	}, now); err != nil {
		t.Fatalf("Record legacy: %v", err)
	}
	entries, err := List(root, "s")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	buckets := map[string]int{}
	for _, e := range entries {
		buckets[ExecutionKey(e)]++
	}
	if buckets[UnattributedExecution] != 1 || len(buckets) != 1 {
		t.Errorf("legacy buckets = %v, want only {(unattributed):1}", buckets)
	}
}

func TestCountByExecutionCorrelatesRuns(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	rec := func(session, call, tool string, status, execID string, at time.Time) {
		t.Helper()
		if _, err := Record(root, Entry{
			Session: session, Message: "m" + call, Call: call, Tool: tool,
			Status: status, Task: "T1", ExecutionID: execID,
		}, at); err != nil {
			t.Fatalf("Record %s/%s/%s: %v", session, call, execID, err)
		}
	}
	rec("s1", "c1", "read", StatusCompleted, "run_a", now)
	rec("s1", "c2", "write", StatusError, "run_a", now.Add(time.Second))
	rec("s1", "c3", "read", StatusCompleted, "run_b", now.Add(2*time.Second))
	rec("s2", "c4", "bash", StatusCompleted, "run_a", now.Add(3*time.Second))
	rec("s2", "c5", "bash", StatusCompleted, "", now.Add(4*time.Second))

	counts, err := CountByExecution(root)
	if err != nil {
		t.Fatalf("CountByExecution: %v", err)
	}
	if len(counts) != 2 {
		t.Fatalf("counts keys = %v, want exactly run_a/run_b (legacy stays unattributed)", counts)
	}
	if counts["run_a"].Calls != 3 || counts["run_a"].Errors != 1 {
		t.Errorf("run_a = %+v, want {Calls:3 Errors:1}", counts["run_a"])
	}
	if counts["run_b"].Calls != 1 || counts["run_b"].Errors != 0 {
		t.Errorf("run_b = %+v, want {Calls:1 Errors:0}", counts["run_b"])
	}
}

func TestCountByExecutionMalformedIsError(t *testing.T) {
	root := t.TempDir()
	if _, err := Record(root, Entry{
		Session: "s", Message: "m", Call: "c", Tool: "read", Status: StatusCompleted,
	}, time.Now()); err != nil {
		t.Fatalf("Record: %v", err)
	}
	f, err := os.OpenFile(File(root, "s"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open trace for append: %v", err)
	}
	if _, err := f.WriteString("{not valid json\n"); err != nil {
		f.Close()
		t.Fatalf("append garbage: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close trace: %v", err)
	}
	if _, err := CountByExecution(root); err == nil {
		t.Fatal("CountByExecution silently accepted a malformed trace; want a defined error")
	} else if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("malformed error = %q, want it to name line 2", err)
	}
}

func TestRecordFailureIsErrorNotPanic(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Record on an unwritable root panicked: %v", r)
		}
	}()
	if _, err := Record(blocker, Entry{
		Session: "s", Message: "m", Call: "c", Tool: "read", Status: StatusCompleted,
	}, time.Now()); err == nil {
		t.Fatal("Record on an unwritable root: want an error, got nil")
	}
}
