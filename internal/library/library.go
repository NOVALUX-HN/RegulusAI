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

// Lesson is one lesson candidate or admitted lesson. Evidence cites
// receipt paths — a lesson without evidence is refused (PRD R-E2,
// docs/memory.md §2.7). Status is candidate until a guardian validates
// the admission.
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
	Status     string // "candidate" | "admitted"
	Guardian   string // who validated the admission
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
	guardian := ""
	if l.Guardian != "" {
		guardian = "- **guardian**: " + l.Guardian + "\n"
	}
	status := ""
	if l.Status != "" {
		status = "- **status**: " + l.Status + "\n"
	}
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
		status,
		guardian,
		"",
	}, "\n")
}

// CandidateDir returns the domain candidates path under a project root.
func CandidateDir(root, domain string) string {
	return filepath.Join(Dir(root, domain), "candidates")
}

func candidateFile(root, domain, id string) string {
	return filepath.Join(CandidateDir(root, domain), id, "LESSON.md")
}

// Propose validates one lesson and writes it as a candidate under
// <domain>/candidates/ — awaiting guardian validation. No INDEX change.
func Propose(root string, l Lesson) error {
	if err := validate(l); err != nil {
		return err
	}
	if l.ID == "" {
		l.ID = NewID(l.CreatedAt)
	}
	l.Status = "candidate"
	if err := os.MkdirAll(filepath.Dir(candidateFile(root, l.Domain, l.ID)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(candidateFile(root, l.Domain, l.ID), []byte(l.render()), 0o644)
}

// Candidates lists a domain's pending candidates, oldest first.
func Candidates(root, domain string) ([]Lesson, error) {
	paths, err := filepath.Glob(filepath.Join(CandidateDir(root, domain), "*", "LESSON.md"))
	if err != nil {
		return nil, err
	}
	var cands []Lesson
	for _, p := range paths {
		l, err := parseLesson(p)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", p, err)
		}
		cands = append(cands, l)
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].CreatedAt.Before(cands[j].CreatedAt) })
	return cands, nil
}

// Admit validates and writes one guardian-validated lesson, then
// rebuilds the domain INDEX.
func Admit(root string, l Lesson, guardian string) error {
	if err := validate(l); err != nil {
		return err
	}
	if strings.TrimSpace(guardian) == "" {
		return errors.New("guardian must not be empty: only a guardian-validated lesson is admitted")
	}
	if l.ID == "" {
		l.ID = NewID(l.CreatedAt)
	}
	l.Status = "admitted"
	l.Guardian = guardian
	if err := os.MkdirAll(filepath.Dir(lessonFile(root, l.Domain, l.ID)), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(lessonFile(root, l.Domain, l.ID), []byte(l.render()), 0o644); err != nil {
		return err
	}
	return RebuildIndex(root, l.Domain)
}

// RemoveCandidate deletes a candidate after admission.
func RemoveCandidate(root, domain, id string) error {
	return os.RemoveAll(filepath.Dir(candidateFile(root, domain, id)))
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
		case "status":
			l.Status = val
		case "guardian":
			l.Guardian = val
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
