package trace

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestRecordRefusesEmptyFields(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	full := Entry{Session: "s", Message: "m", Call: "c", Tool: "tool", Status: StatusCompleted}
	cases := []Entry{
		{Message: "m", Call: "c", Tool: "tool", Status: StatusCompleted},
		{Session: "s", Call: "c", Tool: "tool", Status: StatusCompleted},
		{Session: "s", Message: "m", Tool: "tool", Status: StatusCompleted},
		{Session: "s", Message: "m", Call: "c", Status: StatusCompleted},
		{Session: "s", Message: "m", Call: "c", Tool: "tool"},
		{Session: "s", Message: "m", Call: "c", Tool: "tool", Status: "done"},
	}
	for i, e := range cases {
		if _, err := Record(root, e, now); err == nil {
			t.Fatalf("case %d (%+v): want refusal, got nil", i, e)
		}
	}
	// the valid entry still records after the refusals
	if _, err := Record(root, full, now); err != nil {
		t.Fatalf("valid record: %v", err)
	}
	got, err := List(root, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("refusals must leave stored data unchanged: got %d entries, want 1", len(got))
	}
}

func TestRecordBadStatusRefused(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	if _, err := Record(root, Entry{Session: "s", Message: "m", Call: "c", Tool: "t", Status: "ok"}, now); err == nil {
		t.Fatal("want refusal for status ok, got nil")
	}
	if _, err := Record(root, Entry{Session: "s", Message: "m", Call: "c", Tool: "t", Status: "Completed"}, now); err == nil {
		t.Fatal("want refusal for status Completed (case-sensitive), got nil")
	}
	if got, _ := List(root, "s"); len(got) != 0 {
		t.Fatalf("refused record must not store data: got %d entries", len(got))
	}
}

func TestListOrderRoundTrip(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	want := []Entry{
		{Session: "s", Message: "m1", Call: "c1", Tool: "read", Status: StatusCompleted},
		{Session: "s", Message: "m2", Call: "c2", Tool: "write", Status: StatusError},
		{Session: "s", Message: "m3", Call: "c3", Tool: "bash", Status: StatusCompleted},
	}
	for _, e := range want {
		if _, err := Record(root, e, now); err != nil {
			t.Fatal(err)
		}
	}
	got, err := List(root, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Call != want[i].Call || got[i].Tool != want[i].Tool || got[i].Status != want[i].Status {
			t.Fatalf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFingerprintShape(t *testing.T) {
	got := Fingerprint([]Entry{
		{Tool: "read", Status: StatusCompleted},
		{Tool: "write", Status: StatusError},
	})
	if len(got) != 2 || got[0] != "1 read completed" || got[1] != "2 write error" {
		t.Fatalf("fingerprint = %q, want [\"1 read completed\" \"2 write error\"]", got)
	}
	if len(Fingerprint(nil)) != 0 {
		t.Fatal("empty trace must fingerprint to zero lines")
	}
}

func TestListMissingSessionEmpty(t *testing.T) {
	got, err := List(t.TempDir(), "nope")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("missing session = %v, want empty", got)
	}
}

func TestRecordFilesRoundTrip(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	e, err := Record(root, Entry{
		Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted,
		Files: []string{"internal/cli/cli.go", "docs/memory.md"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Files) != 2 {
		t.Fatalf("files = %v", e.Files)
	}
	entries, err := List(root, "s")
	if err != nil || len(entries) != 1 || len(entries[0].Files) != 2 {
		t.Fatalf("round-trip = %+v (%v)", entries, err)
	}
	fp := Fingerprint(entries)
	if len(fp) != 1 || fp[0] != "1 t completed" {
		t.Fatalf("fingerprint must ignore files, got %v", fp)
	}
}

func TestRecordRefusesBadFiles(t *testing.T) {
	many := make([]string, 0, 21)
	for i := 0; i < 21; i++ {
		many = append(many, "a.go")
	}
	now := time.Unix(1700000000, 0).UTC()
	for _, files := range [][]string{many, {""}, {"ok.go", "  "}, {"bad\npath.go"}} {
		_, err := Record(t.TempDir(), Entry{
			Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted, Files: files,
		}, now)
		if err == nil {
			t.Fatalf("want files refusal for %q", files)
		}
	}
}

func TestSessionsListsSorted(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	for _, s := range []string{"zeta", "alpha"} {
		if _, err := Record(root, Entry{Session: s, Message: "m", Call: "c", Tool: "t", Status: StatusCompleted}, now); err != nil {
			t.Fatal(err)
		}
	}
	sessions, err := Sessions(root)
	if err != nil || len(sessions) != 2 || sessions[0] != "alpha" || sessions[1] != "zeta" {
		t.Fatalf("sessions = %+v (%v)", sessions, err)
	}
	empty, err := Sessions(t.TempDir())
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing traces dir = %+v (%v)", empty, err)
	}
}

func TestPruneKeepsLastN(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1700000000, 0).UTC()
	for i := 0; i < 5; i++ {
		if _, err := Record(root, Entry{Session: "s", Message: "m", Call: "c", Tool: "tool-" + fmt.Sprint(i), Status: StatusCompleted}, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	kept, err := Prune(root, "s", 2)
	if err != nil || kept != 2 {
		t.Fatalf("kept = %d (%v)", kept, err)
	}
	entries, err := List(root, "s")
	if err != nil || len(entries) != 2 {
		t.Fatalf("after prune = %+v (%v)", entries, err)
	}
	fp := Fingerprint(entries)
	if len(fp) != 2 || fp[0] != "1 tool-3 completed" || fp[1] != "2 tool-4 completed" {
		t.Fatalf("prune must keep call order tail, got %v", fp)
	}
	// keep >= len is a no-op (file untouched).
	before, _ := os.ReadFile(File(root, "s"))
	if kept, err := Prune(root, "s", 10); err != nil || kept != 2 {
		t.Fatalf("no-op prune = %d (%v)", kept, err)
	}
	after, _ := os.ReadFile(File(root, "s"))
	if string(before) != string(after) {
		t.Fatal("no-op prune must not touch the file")
	}
	// unknown session is a no-op success.
	if kept, err := Prune(root, "nobody", 2); err != nil || kept != 0 {
		t.Fatalf("unknown = %d (%v)", kept, err)
	}
}
