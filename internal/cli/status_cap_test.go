package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"regulus-ai/internal/receipts"
)

// F6 (S2-S3): status must not full-scan the time aggregation. The
// aggregation caps at the newest 200 receipt files by mtime; small
// stores (<200) stay byte-identical to a full scan.
func writeTimeReceipt(t *testing.T, dir, name, task string, ms int64, modTime time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	data := fmt.Sprintf(`{"id":"%s","task":%q,"gate":"g","duration_ms":%d}`, name, task, ms)
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCapStatusTimeFilesUnderCapIdentical(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	var found []string
	for i := 0; i < 5; i++ {
		found = append(found, writeTimeReceipt(t, dir, fmt.Sprintf("r-%02d.json", i), "t", int64(i+1), base.Add(time.Duration(i)*time.Minute)))
	}
	capped := capStatusTimeFiles(found)
	if len(capped) != len(found) {
		t.Fatalf("under cap: got %d files, want %d (byte-identical full scan)", len(capped), len(found))
	}
	want := map[string]bool{}
	for _, p := range found {
		want[p] = true
	}
	for _, p := range capped {
		if !want[p] {
			t.Fatalf("under cap: capped set differs from full scan: %s missing", p)
		}
	}
}

func TestCapStatusTimeFilesCapsAt200Newest(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	var found []string
	for i := 0; i < 250; i++ {
		// Oldest file i=0 gets 1ms, newest i=249 gets 250ms; tasks split
		// so totals prove the newest 200 were used.
		found = append(found, writeTimeReceipt(t, dir, fmt.Sprintf("r-%03d.json", i), "t", int64(i+1), base.Add(time.Duration(i)*time.Minute)))
	}
	capped := capStatusTimeFiles(found)
	if len(capped) != 200 {
		t.Fatalf("over cap: got %d files, want newest 200", len(capped))
	}
	// Oldest 50 (r-000..r-049) carry 1..50ms; newest 200 carry 51..250ms
	// = sum 51+...+250 = 30100. A full scan would give 1+...+250 = 31375.
	var total int64
	for _, p := range capped {
		rec, err := receipts.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		total += rec.DurationMS
	}
	if total != 30100 {
		t.Fatalf("capped totals = %d, want 30100 (newest 200 only)", total)
	}
	for i := 0; i < 50; i++ {
		old := filepath.Join(dir, fmt.Sprintf("r-%03d.json", i))
		for _, p := range capped {
			if p == old {
				t.Fatalf("capped set must exclude oldest file %s", old)
			}
		}
	}
}

// F6 (S2-S3): the gremlins probe runs once per process — a second
// status call must reuse the memoized result even if the binary
// disappears from PATH/Go-bin in between.
func TestGremlinsProbeMemoizedPerProcess(t *testing.T) {
	resetGremlinsStatusCache()
	bin := t.TempDir()
	name := "gremlins"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if !gremlinsAvailableForStatus() {
		t.Fatal("first probe with gremlins on PATH must resolve")
	}
	// Remove the binary from view: memoized second call must still hit.
	t.Setenv("PATH", "")
	emptyHome := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", emptyHome)
	} else {
		t.Setenv("HOME", emptyHome)
	}
	if !gremlinsAvailableForStatus() {
		t.Fatal("RED: second probe must reuse memoized result (one probe per process)")
	}
}
