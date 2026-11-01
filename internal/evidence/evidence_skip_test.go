package evidence

// H2: the tolerant reader skips corrupt receipts with a count instead
// of aborting the scan. Strict List keeps refusing (see
// TestListRejectsMalformedReceipt); ListSkipping is the miner's path.

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"regulus-ai/internal/receipts"
)

func TestListSkippingCountsCorruptWithoutAborting(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.Write(dir, Receipt(t0, "ok", receipts.VerdictPass)); err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.Write(dir, Receipt(t0.Add(time.Second), "ok2", receipts.VerdictFail)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "garbage.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, corrupt, err := ListSkipping(root, "")
	if err != nil {
		t.Fatalf("ListSkipping must not abort on a corrupt receipt, got %v", err)
	}
	if corrupt != 1 {
		t.Fatalf("one corrupt file must count as corrupt=1, got %d", corrupt)
	}
	if len(entries) != 2 {
		t.Fatalf("healthy receipts must still list, got %d entries", len(entries))
	}
	if _, _, err := ListSkipping(t.TempDir(), ""); err != nil {
		t.Fatalf("empty store must read clean, got %v", err)
	}
}
