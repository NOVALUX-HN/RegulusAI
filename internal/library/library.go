// Package library is the lesson library (Harness² structure, made
// persistent): evidence-gated lessons under .regulus/library/<domain>/
// with an INDEX.md regenerated from disk (PRD R-E2).
package library

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"regulus-ai/internal/receipts"
)

// Confidences.
const (
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)

// Lesson dimensions (harness² INV/DOM separation): INV lessons are
// about how the agent works (procedure, tool use, completion); DOM
// lessons are about what a practitioner knows (analytic moves,
// audience, structure, consistency). Empty means unset (legacy
// lessons admitted before dimensions existed).
const (
	DimensionINV = "inv"
	DimensionDOM = "dom"
)

// Lesson is one lesson candidate or admitted lesson. Evidence cites
// receipt paths — a lesson without evidence is refused (PRD R-E2,
// docs/memory.md §2.7). Status is candidate until a guardian validates
// the admission.
type Lesson struct {
	ID         string
	Domain     string
	Dimension  string // "" (unset, legacy) | "inv" | "dom"
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
	if err := ValidateDimension(l.Dimension); err != nil {
		return err
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

// ValidateDimension accepts the unset legacy value and the two
// harness² dimensions, and refuses everything else.
func ValidateDimension(d string) error {
	switch d {
	case "", DimensionINV, DimensionDOM:
		return nil
	default:
		return fmt.Errorf("dimension must be %q or %q, got %q", DimensionINV, DimensionDOM, d)
	}
}

func (l Lesson) render() string {
	dimension := ""
	if l.Dimension != "" {
		dimension = "- **dimension**: " + l.Dimension + "\n"
	}
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
		dimension,
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
// snapshots the executable harness surface plus an EXPERIENCE.md note
// under harnesses/<task-slug>/ and rebuilds the domain INDEX.
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
	if err := snapshotHarness(root, l.Domain, l.Task); err != nil {
		return err
	}
	if err := writeExperience(root, l, guardian); err != nil {
		return err
	}
	return RebuildIndex(root, l.Domain)
}

// harnessDir returns the snapshot directory for one task inside a
// domain library: <domain>/harnesses/<task-slug>/. The slug reuses the
// receipts convention ("/" becomes "__") so evidence paths and
// harness snapshots name a task the same way.
func harnessDir(root, domain, task string) string {
	return filepath.Join(Dir(root, domain), "harnesses", receipts.Slug(task))
}

// experienceFile returns the EXPERIENCE.md path for one task snapshot.
func experienceFile(root, domain, task string) string {
	return filepath.Join(harnessDir(root, domain, task), "EXPERIENCE.md")
}

// writeExperience records why the harness snapshot was adopted: the
// guardian who validated it, the cue that triggered the lesson, and the
// transferable lesson with its evidence and confidence.
func writeExperience(root string, l Lesson, guardian string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", l.Task)
	fmt.Fprintf(&b, "- adopted: %s\n", guardian)
	fmt.Fprintf(&b, "- domain: %s\n", l.Domain)
	fmt.Fprintf(&b, "- task: %s\n", l.Task)
	fmt.Fprintf(&b, "- confidence: %s\n", l.Confidence)
	fmt.Fprintf(&b, "- git_head: %s\n", l.GitHead)
	fmt.Fprintf(&b, "- created_at: %s\n", l.CreatedAt.Format(time.RFC3339))
	b.WriteString("\n## Why this was chosen\n\n")
	b.WriteString(l.Cue + "\n")
	b.WriteString("\n## Transferable lesson\n\n")
	b.WriteString(l.Lesson + "\n")
	b.WriteString("\n- evidence: " + strings.Join(l.Evidence, ", ") + "\n")
	fmt.Fprintf(&b, "- confidence: %s\n", l.Confidence)
	if err := os.MkdirAll(harnessDir(root, l.Domain, l.Task), 0o755); err != nil {
		return err
	}
	return os.WriteFile(experienceFile(root, l.Domain, l.Task), []byte(b.String()), 0o644)
}

// snapshotHarness copies the executable harness surface at admit time
// into harnesses/<task-slug>/tree/, mirroring repo-relative paths:
// .opencode/agents, skills and plugins (*.md), .opencode/discipline.md,
// .opencode/tool-guidance.md, and scripts/* — each only when present.
// Scoping rule: docs/ is NEVER copied — prose, large, and versioned
// elsewhere; the tree holds only the executable surface an adopted
// lesson ran against.
func snapshotHarness(root, domain, task string) error {
	tree := filepath.Join(harnessDir(root, domain, task), "tree")
	var rels []string
	for _, dir := range []string{
		filepath.Join(".opencode", "agents"),
		filepath.Join(".opencode", "skills"),
		filepath.Join(".opencode", "plugins"),
	} {
		matches, err := filepath.Glob(filepath.Join(root, dir, "*.md"))
		if err != nil {
			return err
		}
		for _, m := range matches {
			if rel, err := filepath.Rel(root, m); err == nil {
				rels = append(rels, rel)
			}
		}
	}
	for _, f := range []string{
		filepath.Join(".opencode", "discipline.md"),
		filepath.Join(".opencode", "tool-guidance.md"),
	} {
		if st, err := os.Stat(filepath.Join(root, f)); err == nil && !st.IsDir() {
			rels = append(rels, f)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "scripts"))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			rels = append(rels, filepath.Join("scripts", e.Name()))
		}
	}
	if len(rels) == 0 {
		return nil
	}
	for _, rel := range rels {
		src := filepath.Join(root, rel)
		dst := filepath.Join(tree, rel)
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// treeFileCount counts the regular files under a snapshot tree.
// A missing tree (lessons admitted before snapshots existed) is 0 files.
func treeFileCount(tree string) int {
	n := 0
	_ = filepath.Walk(tree, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
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
		case "dimension":
			l.Dimension = val
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
// Each lesson becomes a snapshot block pointing at its adopted harness
// tree and its EXPERIENCE.md note:
//
//	## harnesses/<slug>/
//
//	- harness: harnesses/<slug>/tree/ (<N> files)
//	- why: harnesses/<slug>/EXPERIENCE.md
//	- cue: ...
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
			slug := receipts.Slug(l.Task)
			n := treeFileCount(filepath.Join(harnessDir(root, domain, l.Task), "tree"))
			fmt.Fprintf(&b, "## harnesses/%s/\n", slug)
			fmt.Fprintf(&b, "- harness: harnesses/%s/tree/ (%d files)\n", slug, n)
			fmt.Fprintf(&b, "- why: harnesses/%s/EXPERIENCE.md\n", slug)
			if strings.TrimSpace(l.Cue) != "" {
				fmt.Fprintf(&b, "- cue: %s\n", l.Cue)
			}
			b.WriteString("\n")
		}
	}
	if err := os.MkdirAll(Dir(root, domain), 0o755); err != nil {
		return err
	}
	return os.WriteFile(IndexFile(root, domain), []byte(b.String()), 0o644)
}

// Forget drops one task snapshot: its harnesses/<slug>/ tree and note
// are removed and the INDEX is rebuilt. Forgetting an unknown task is a
// no-op success. Admitted lessons are left untouched — Forget prunes
// snapshots, not lessons.
func Forget(root, domain, task string) error {
	if strings.TrimSpace(domain) == "" || strings.TrimSpace(task) == "" {
		return errors.New("forget requires a domain and a task")
	}
	if _, err := os.Stat(harnessDir(root, domain, task)); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(harnessDir(root, domain, task)); err != nil {
		return err
	}
	return RebuildIndex(root, domain)
}

// Retain keeps only the lessons whose task sorts strictly before <task>
// (Go string ordering on the raw task id, applied uniformly so retain
// is deterministic across machines) and drops <task> and everything
// after it: their lesson entries and their harnesses/<slug>/ snapshots.
// The INDEX is rebuilt afterwards.
func Retain(root, domain, task string) error {
	if strings.TrimSpace(domain) == "" || strings.TrimSpace(task) == "" {
		return errors.New("retain requires a domain and a task")
	}
	lessons, err := List(root, domain)
	if err != nil {
		return err
	}
	for _, l := range lessons {
		if l.Task >= task {
			if err := os.RemoveAll(filepath.Dir(lessonFile(root, domain, l.ID))); err != nil {
				return err
			}
			if err := os.RemoveAll(harnessDir(root, domain, l.Task)); err != nil {
				return err
			}
		}
	}
	return RebuildIndex(root, domain)
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
