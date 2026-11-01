package trace

// H2: the tolerant reader skips corrupt JSONL lines with a count
// instead of aborting the session. Strict List keeps refusing with a
// line-naming error; ListSkipping is the miner's path.

import (
	"os"
	"testing"
	"time"
)

func TestListSkippingCountsCorruptWithoutAborting(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	for _, tool := range []string{"read", "write"} {
		if _, err := Record(root, Entry{
			Session: "s", Message: "m-" + tool, Call: "c-" + tool,
			Tool: tool, Status: StatusCompleted,
		}, now); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.OpenFile(File(root, "s"), os.O_APPEND|os.O_WRONLY, 0o644)
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
	if _, err := List(root, "s"); err == nil {
		t.Fatal("strict List must still refuse a corrupt line")
	}
	entries, corrupt, err := ListSkipping(root, "s")
	if err != nil {
		t.Fatalf("ListSkipping must not abort on a corrupt line, got %v", err)
	}
	if corrupt != 1 {
		t.Fatalf("one corrupt line must count as corrupt=1, got %d", corrupt)
	}
	if len(entries) != 2 {
		t.Fatalf("healthy entries must still list, got %d", len(entries))
	}
	if got, corrupt, err := ListSkipping(t.TempDir(), "nope"); err != nil || len(got) != 0 || corrupt != 0 {
		t.Fatalf("missing trace must read empty, got %+v %d %v", got, corrupt, err)
	}
}
