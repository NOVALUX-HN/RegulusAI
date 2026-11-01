package envfacts

// H2: corrupt receipts/traces never abort the miner: they are skipped,
// counted in Stats.Corrupt (warned on stderr as path:line), and healthy
// data keeps mining.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

func TestCorruptReceiptSkippedWithoutAborting(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeReceipt(t, root, "feat/T1", "crap", receipts.VerdictFail,
			fmt.Sprintf("run_c%d", i), base.Add(time.Duration(i)*time.Hour))
	}
	if err := os.WriteFile(
		filepath.Join(evidence.TaskDir(root, "feat/T1"), "garbage.json"),
		[]byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, st, err := ReceiptSeries(root)
	if err != nil {
		t.Fatalf("corrupt receipt must not abort the scan, got %v", err)
	}
	if st.Corrupt != 1 {
		t.Fatalf("one corrupt receipt must count as Corrupt=1, got %+v", st)
	}
	if st.Receipts != 3 {
		t.Fatalf("healthy receipts must still scan, got %+v", st)
	}
	if len(rs) != 1 || len(rs[0].Execs) != 3 {
		t.Fatalf("healthy failures must still form one series, got %+v", rs)
	}
}

func TestCorruptTraceLineSkippedWithoutAborting(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		sess := fmt.Sprintf("ses_corrupt%d", i)
		recordTrace(t, root, sess, "read", trace.StatusError, "feat/T1", base.Add(time.Duration(i)*time.Hour))
	}
	// One corrupt line appended to a healthy session.
	f, err := os.OpenFile(trace.File(root, "ses_corrupt0"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not valid json\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	ts, st, err := TraceSeries(root)
	if err != nil {
		t.Fatalf("corrupt trace line must not abort the scan, got %v", err)
	}
	if st.Corrupt != 1 {
		t.Fatalf("one corrupt trace line must count as Corrupt=1, got %+v", st)
	}
	if st.TraceSessions != 3 {
		t.Fatalf("all three sessions must still scan, got %+v", st)
	}
	if len(ts) != 1 {
		t.Fatalf("tool erroring in three sessions must still mine, got %+v", ts)
	}
	if got := FailingSessions(ts[0]); got != 3 {
		t.Fatalf("corrupt line must not drop the healthy session, failing = %d", got)
	}
}
