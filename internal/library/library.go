// Package library is the lesson library (Harness² structure, made
// persistent): evidence-gated lessons under .regulus/library/<domain>/
// with an INDEX.md regenerated from disk (PRD R-E2).
package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Confidences.
const (
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)

// Lesson is one admitted lesson candidate. Evidence cites receipt paths —
// a lesson without evidence is refused (PRD R-E2, docs/memory.md §2.7).
type Lesson struct {
	ID         string
	Domain     string
	Task       string
	Cue        string
	Lesson     string
	Evidence   []string
	Confidence string
	GitHead    string
	CreatedAt  time.Time
}

// NewID returns a lesson id for a timestamp.
func NewID(now time.Time) string {
	return "les_" + strconv.FormatInt(now.UnixNano(), 16)
}

// Dir returns the domain library path under a project root.
func Dir(root, domain string) string {
	return filepath.Join(root, ".regulus", "library", domain)
}

// IndexFile returns the domain INDEX.md path.
func IndexFile(root, domain string) string {
	return filepath.Join(Dir(root, domain), "INDEX.md")
}

func lessonFile(root, domain, id string) string {
	return filepath.Join(Dir(root, domain), "lessons", id, "LESSON.md")
}

func validate(l Lesson) error {
	if strings.TrimSpace(l.Domain) == "" {
		return errors.New("domain must not be empty")
	}
	if strings.TrimSpace(l.Cue) == "" {
		return errors.New("cue must not be empty")
	}
	if strings.TrimSpace(l.Lesson) == "" {
		return errors.New("lesson must not be empty")
	}
	if len(l.Evidence) == 0 {
		return errors.New("no evidence: a lesson must cite at least one receipt")
	}
	switch l.Confidence {
	case ConfidenceLow, ConfidenceMedium, ConfidenceHigh:
	default:
		return fmt.Errorf("confidence must be low, medium or high, got %q", l.Confidence)
	}
	return nil
}

func (l Lesson) render() string {
	return strings.Join([]string{
		"# " + l.ID,
		"",
		"- **domain**: " + l.Domain,
		"- **task**: " + l.Task,
		"- **cue**: " + l.Cue,
		"- **lesson**: " + l.Lesson,
		"- **evidence**: " + strings.Join(l.Evidence, ", "),
		"- **confidence**: " + l.Confidence,
		"- **git_head**: " + l.GitHead,
		"- **created_at**: " + l.CreatedAt.Format(time.RFC3339),
		"",
	}, "\n")
}

// Admit validates and writes one lesson, then rebuilds the domain INDEX.
func Admit(root string, l Lesson) error {
	if err := validate(l); err != nil {
		return err
	}
	if l.ID == "" {
		l.ID = NewID(l.CreatedAt)
	}
	if err := os.MkdirAll(filepath.Dir(lessonFile(root, l.Domain, l.ID)), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(lessonFile(root, l.Domain, l.ID), []byte(l.render()), 0o644); err != nil {
		return err
	}
	return RebuildIndex(root, l.Domain)
}

// List returns every lesson of a domain, oldest first. A missing library
// is empty.
func List(root, domain string) ([]Lesson, error) {
	paths, err := filepath.Glob(filepath.Join(Dir(root, domain), "lessons", "*", "LESSON.md"))
	if err != nil {
		return nil, err
	}
	var lessons []Lesson
	for _, p := range paths {
		l, err := parseLesson(p)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", p, err)
		}
		lessons = append(lessons, l)
	}
	sort.Slice(lessons, func(i, j int) bool { return lessons[i].CreatedAt.Before(lessons[j].CreatedAt) })
	return lessons, nil
}

func parseLesson(path string) (Lesson, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Lesson{}, err
	}
	var l Lesson
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "- **") {
			continue
		}
		rest := strings.TrimPrefix(line, "- **")
		idx := strings.Index(rest, "**: ")
		if idx < 0 {
			continue
		}
		key, val := rest[:idx], rest[idx+4:]
		switch key {
		case "domain":
			l.Domain = val
		case "task":
			l.Task = val
		case "cue":
			l.Cue = val
		case "lesson":
			l.Lesson = val
		case "evidence":
			for _, e := range strings.Split(val, ",") {
				if e = strings.TrimSpace(e); e != "" {
					l.Evidence = append(l.Evidence, e)
				}
			}
		case "confidence":
			l.Confidence = val
		case "git_head":
			l.GitHead = val
		case "created_at":
			t, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return Lesson{}, fmt.Errorf("created_at %q: %w", val, err)
			}
			l.CreatedAt = t
		}
	}
	if l.ID == "" {
		l.ID = filepath.Base(filepath.Dir(path))
	}
	return l, nil
}

// RebuildIndex regenerates the domain INDEX.md from the lessons on disk.
func RebuildIndex(root, domain string) error {
	lessons, err := List(root, domain)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Library — %s\n\n", domain)
	b.WriteString("Lessons admitted with evidence. Each entry cites its receipts.\n\n")
	if len(lessons) == 0 {
		b.WriteString("_(empty — no lessons admitted yet)_\n")
	} else {
		for _, l := range lessons {
			fmt.Fprintf(&b, "- `%s` — %s (confidence: %s, evidence: %d receipt(s), task: %s)\n",
				l.ID, l.Cue, l.Confidence, len(l.Evidence), l.Task)
		}
	}
	if err := os.MkdirAll(Dir(root, domain), 0o755); err != nil {
		return err
	}
	return os.WriteFile(IndexFile(root, domain), []byte(b.String()), 0o644)
}

// Domains lists the domains that have libraries on disk, sorted.
func Domains(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, ".regulus", "library"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var domains []string
	for _, e := range entries {
		if e.IsDir() {
			domains = append(domains, e.Name())
		}
	}
	sort.Strings(domains)
	return domains, nil
}
