// Package reflect produces evidence-gated lesson candidates from a feature
// document and its receipts (PRD R-E1, R-E2; the F1 evolution slice).
package reflect

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/budget"
	"regulus-ai/internal/library"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/spec"
)

// Input is one reflect invocation.
type Input struct {
	FeatureDoc string
	Domain     string
	Task       string
	Cue        string
	Lesson     string
	Confidence string // "" defaults to low
	Now        time.Time
}

// Run validates the feature document's spec contract, requires receipts for
// the task (evidence gate), admits the lesson and records the budget entry.
func Run(root string, in Input, gitHead string) (library.Lesson, error) {
	data, err := os.ReadFile(in.FeatureDoc)
	if err != nil {
		return library.Lesson{}, fmt.Errorf("cannot read feature document: %w", err)
	}
	if _, violations := spec.Check(string(data)); len(violations) > 0 {
		return library.Lesson{}, fmt.Errorf("feature document fails the spec contract: %s", strings.Join(violations, "; "))
	}
	if strings.TrimSpace(in.Cue) == "" {
		return library.Lesson{}, errors.New("cue must not be empty")
	}
	if strings.TrimSpace(in.Lesson) == "" {
		return library.Lesson{}, errors.New("lesson must not be empty")
	}
	confidence := in.Confidence
	if confidence == "" {
		confidence = library.ConfidenceLow
	}
	evidence, err := taskReceipts(root, in.Task)
	if err != nil {
		return library.Lesson{}, err
	}
	if len(evidence) == 0 {
		return library.Lesson{}, errors.New("no evidence: run the gauntlet (regulus gate run) before reflecting")
	}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}
	l := library.Lesson{
		ID:         library.NewID(in.Now),
		Domain:     in.Domain,
		Task:       in.Task,
		Cue:        in.Cue,
		Lesson:     in.Lesson,
		Evidence:   evidence,
		Confidence: confidence,
		GitHead:    gitHead,
		CreatedAt:  in.Now.UTC(),
	}
	if err := library.Admit(root, l); err != nil {
		return library.Lesson{}, err
	}
	if err := budget.Record(root, budget.Entry{
		Kind:      "reflect",
		Timestamp: in.Now.UTC(),
		Task:      in.Task,
		Domain:    in.Domain,
		Outcome:   "candidate-admitted",
		Detail:    fmt.Sprintf("%d receipts", len(evidence)),
	}); err != nil {
		return library.Lesson{}, err
	}
	// One substrate: the lesson also lands in the Almanac (the library
	// LESSON.md stays the human-readable view of the same admission).
	if _, err := almanac.Save(root, &almanac.Observation{
		Type: almanac.TypeLesson, Title: l.Cue, Content: l.Lesson,
		Domain: l.Domain,
		Lesson: &almanac.Lesson{
			Cue: l.Cue, Lesson: l.Lesson, Evidence: l.Evidence, Confidence: l.Confidence,
		},
		Provenance: &almanac.Provenance{Task: l.Task, GitHead: gitHead, Receipts: l.Evidence},
	}, in.Now); err != nil {
		return library.Lesson{}, err
	}
	return l, nil
}

func taskReceipts(root, task string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".regulus", "evidence", receipts.Slug(task), "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
