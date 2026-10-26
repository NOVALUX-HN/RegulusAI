package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t1 = time.Date(2026, 10, 26, 6, 0, 0, 0, time.UTC)

func TestNotesFile(t *testing.T) {
	got := NotesFile("root")
	want := filepath.Join("root", ".regulus", "memory", "session.md")
	if got != want {
		t.Fatalf("NotesFile = %q, want %q", got, want)
	}
}

func TestNoteAppendsTimestampedLinesOldestFirst(t *testing.T) {
	root := t.TempDir()
	if err := Note(root, "first note", t1); err != nil {
		t.Fatal(err)
	}
	if err := Note(root, "second note", t1.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	out, err := Show(root)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 notes, got %q", out)
	}
	if !strings.Contains(lines[0], "2026-10-26T06:00:00Z") || !strings.Contains(lines[0], "first note") {
		t.Fatalf("first line = %q", lines[0])
	}
	if !strings.Contains(lines[1], "second note") {
		t.Fatalf("second line = %q", lines[1])
	}
}

func TestNoteEmptyTextIsError(t *testing.T) {
	if err := Note(t.TempDir(), "", t1); err == nil {
		t.Fatal("empty note text must be an error")
	}
}

func TestNoteCreatesDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "deep", "project")
	if err := Note(root, "note", t1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(NotesFile(root)); err != nil {
		t.Fatal(err)
	}
}

func TestShowMissingFileIsEmptySession(t *testing.T) {
	out, err := Show(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("want an empty session, got %q", out)
	}
}
