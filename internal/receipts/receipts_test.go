package receipts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaptureShort(t *testing.T) {
	text, sha := Capture("hello")
	if text != "hello" {
		t.Fatalf("short output must be stored verbatim, got %q", text)
	}
	sum := sha256.Sum256([]byte("hello"))
	if sha != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha must pin the content, got %q", sha)
	}
}

func TestCaptureLongTruncatesAndPinsFullHash(t *testing.T) {
	long := strings.Repeat("y", MaxCapture+5000)
	text, sha := Capture(long)
	if len(text) >= len(long) {
		t.Fatal("long output must be truncated in the receipt")
	}
	if !strings.Contains(text, "truncated") {
		t.Fatalf("truncated capture must carry the marker, got %q", text)
	}
	sum := sha256.Sum256([]byte(long))
	if sha != hex.EncodeToString(sum[:]) {
		t.Fatal("sha must pin the FULL content, not the truncation")
	}
}

func TestSlug(t *testing.T) {
	if got := Slug("regulus-ai-v1/T2"); got != "regulus-ai-v1__T2" {
		t.Fatalf("slug = %q, want regulus-ai-v1__T2", got)
	}
	if got := Slug("plain"); got != "plain" {
		t.Fatalf("slug = %q, want plain", got)
	}
}

func TestWriteAppendOnlyNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	start := time.Unix(1700000000, 0).UTC()
	r1 := Receipt{Task: "t", Gate: "lint", Layer: 1, Command: "x", Verdict: VerdictPass, ExitCode: 0, StartedAt: start}
	p1, err := Write(dir, r1)
	if err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(p1)
	if err != nil {
		t.Fatal(err)
	}
	// Same gate, same timestamp: must advance to a new slot, never overwrite.
	p2, err := Write(dir, r1)
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatal("same-slot write must advance to a new file")
	}
	now, err := os.ReadFile(p1)
	if err != nil {
		t.Fatal(err)
	}
	if string(orig) != string(now) {
		t.Fatal("the first receipt must be unchanged")
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(entries) != 2 {
		t.Fatalf("want 2 receipts, got %v", entries)
	}
}

func TestWriteAssignsIDAndSerializesFields(t *testing.T) {
	p, err := Write(t.TempDir(), Receipt{
		Task: "t/x", Gate: "unit-tests", Layer: 4, Command: "go test ./...",
		Verdict: VerdictPass, ExitCode: 0, StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var rec Receipt
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rec.ID, "rec_") {
		t.Fatalf("Write must assign an id, got %q", rec.ID)
	}
	if rec.Task != "t/x" || rec.Gate != "unit-tests" || rec.Verdict != VerdictPass {
		t.Fatalf("round-tripped receipt = %+v", rec)
	}
}

func TestWriteCreatesMissingDirectories(t *testing.T) {
	p, err := Write(filepath.Join(t.TempDir(), "evidence", "task__1"), Receipt{
		Gate: "lint", Verdict: VerdictPass, StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}

func TestWriteSanitizesGateNameInFilename(t *testing.T) {
	p, err := Write(t.TempDir(), Receipt{
		Gate: "Weird Name/2", Verdict: VerdictPass, StartedAt: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(p)
	if strings.ContainsAny(name, " /\\") {
		t.Fatalf("filename must be safe, got %q", name)
	}
	if !strings.HasPrefix(name, "weird-name-2-") {
		t.Fatalf("sanitized filename = %q", name)
	}
}

func TestReadFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	start := time.Unix(1700000000, 0).UTC()
	p, err := Write(dir, Receipt{
		Task: "t/x", Gate: "lint", Layer: 2, Command: "x",
		ExitCode: 1, Verdict: VerdictFail, StartedAt: start,
		DurationMS: 5, Stdout: "out", Stderr: "err", GitHead: "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.Task != "t/x" || got.Gate != "lint" || got.Layer != 2 ||
		got.ExitCode != 1 || got.Verdict != VerdictFail || got.Stdout != "out" ||
		got.Stderr != "err" || got.GitHead != "abc" || got.DurationMS != 5 {
		t.Fatalf("round-tripped receipt = %+v", got)
	}
	if !got.StartedAt.Equal(start) {
		t.Fatalf("started_at = %v, want %v", got.StartedAt, start)
	}
}

func TestReadFileErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadFile(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("a missing receipt must be an error")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(bad); err == nil || !strings.Contains(err.Error(), "malformed receipt") {
		t.Fatalf("want malformed-receipt refusal, got %v", err)
	}
}
