// Package memory holds the F1 session-notes slice of the Almanac: an
// append-only, timestamped session file that the OpenCode compaction hook
// injects as data (docs/memory.md).
package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NotesFile returns the session-notes path under a project root.
func NotesFile(root string) string {
	return filepath.Join(root, ".regulus", "memory", "session.md")
}

// Note appends one timestamped note. Empty text is an error. Notes are
// single lines: embedded newlines collapse to spaces, keeping the file
// one-note-per-line and deterministic.
func Note(root, text string, now time.Time) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("note text must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(NotesFile(root)), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(NotesFile(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line := fmt.Sprintf("- %s %s\n", now.UTC().Format(time.RFC3339), strings.ReplaceAll(text, "\n", " "))
	_, err = f.WriteString(line)
	return err
}

// Show returns the notes, oldest first. A missing file is an empty session.
func Show(root string) (string, error) {
	data, err := os.ReadFile(NotesFile(root))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}
