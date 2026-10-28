// Package reflect produces evidence-gated lesson candidates from a
// feature document and its receipts (PRD R-E1, R-E2), under the
// evolution budget cap (R-E4). Admission is two-phase: reflect PROPOSES a
// candidate; a guardian validates it; reflect.Admit completes the flow
// (library + INDEX + Almanac observation + ledger).
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
	"regulus-ai/internal/gates"
	"regulus-ai/internal/library"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/spec"
)

// Input is one reflect invocation. Dimension is the harness² INV/DOM
// separation ("" = unset, "inv" = how the agent works, "dom" = what a
// practitioner knows).
type Input struct {
	FeatureDoc string
	Domain     string
	Dimension  string
	Task       string
	Cue        string
	Lesson     string
	Confidence string // "" defaults to low
	Now        time.Time
}

// Run validates the feature document's spec contract, requires receipts
// for the task (evidence gate), enforces the evolution cap, and proposes
// a lesson candidate. It does NOT admit: the guardian validates, then
// Admit completes the flow.
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
	if err := library.ValidateDimension(in.Dimension); err != nil {
		return library.Lesson{}, err
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

	cap := EvolutionCap(root)
	today, err := budget.TodayEvolutionCount(root, in.Now)
	if err != nil {
		return library.Lesson{}, err
	}
	if today >= cap {
		if err := budget.Record(root, budget.Entry{
			Kind:      budget.KindReflect,
			Timestamp: in.Now.UTC(),
			Task:      in.Task,
			Domain:    in.Domain,
			Outcome:   budget.OutcomeRefusedCap,
			Detail:    fmt.Sprintf("cap %d reached", cap),
		}); err != nil {
			return library.Lesson{}, err
		}
		return library.Lesson{}, fmt.Errorf("evolution cap reached: %d run(s) today, cap %d (regulus.json evolution.max_per_day)", today, cap)
	}
	tcap := TaskCap(root)
	tcount, err := budget.TaskEvolutionCount(root, in.Task, in.Now)
	if err != nil {
		return library.Lesson{}, err
	}
	if tcount >= tcap {
		if err := budget.Record(root, budget.Entry{
			Kind:      budget.KindReflect,
			Timestamp: in.Now.UTC(),
			Task:      in.Task,
			Domain:    in.Domain,
			Outcome:   budget.OutcomeRefusedCap,
			Detail:    fmt.Sprintf("task cap %d reached", tcap),
		}); err != nil {
			return library.Lesson{}, err
		}
		return library.Lesson{}, fmt.Errorf("task evolution cap reached: task %q has %d run(s) today, task cap %d (regulus.json evolution.max_per_task)", in.Task, tcount, tcap)
	}

	l := library.Lesson{
		ID:         library.NewID(in.Now),
		Domain:     in.Domain,
		Dimension:  in.Dimension,
		Task:       in.Task,
		Cue:        in.Cue,
		Lesson:     in.Lesson,
		Evidence:   evidence,
		Confidence: confidence,
		GitHead:    gitHead,
		CreatedAt:  in.Now.UTC(),
	}
	if err := library.Propose(root, l); err != nil {
		return library.Lesson{}, err
	}
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindReflect,
		Timestamp: in.Now.UTC(),
		Task:      in.Task,
		Domain:    in.Domain,
		Outcome:   budget.OutcomeCandidateProposed,
		Detail:    fmt.Sprintf("%d receipts", len(evidence)),
	}); err != nil {
		return library.Lesson{}, err
	}
	return l, nil
}

// Admit completes the flow after guardian validation: the candidate moves
// into the library (INDEX rebuilt), the lesson observation lands in the
// Almanac (one substrate; LESSON.md stays the human-readable view) and
// the ledger records who validated the admission.
func Admit(root, domain, id, guardian, gitHead string, now time.Time) (library.Lesson, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cands, err := library.Candidates(root, domain)
	if err != nil {
		return library.Lesson{}, err
	}
	var found *library.Lesson
	for i := range cands {
		if cands[i].ID == id {
			found = &cands[i]
			break
		}
	}
	if found == nil {
		return library.Lesson{}, fmt.Errorf("no candidate %q pending in domain %q", id, domain)
	}
	adm := *found
	if err := library.Admit(root, adm, guardian); err != nil {
		return library.Lesson{}, err
	}
	if err := library.RemoveCandidate(root, domain, id); err != nil {
		return library.Lesson{}, err
	}
	adm.Status = "admitted"
	adm.Guardian = guardian
	if _, err := almanac.Save(root, &almanac.Observation{
		Type: almanac.TypeLesson, Title: adm.Cue, Content: adm.Lesson,
		Domain: adm.Domain,
		Lesson: &almanac.Lesson{
			Cue: adm.Cue, Lesson: adm.Lesson, Evidence: adm.Evidence, Confidence: adm.Confidence,
			Dimension: adm.Dimension,
		},
		Provenance: &almanac.Provenance{Task: adm.Task, GitHead: gitHead, Receipts: adm.Evidence},
	}, now); err != nil {
		return library.Lesson{}, err
	}
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindReflect,
		Timestamp: now.UTC(),
		Task:      adm.Task,
		Domain:    adm.Domain,
		Outcome:   budget.OutcomeAdmitted,
		Detail:    "guardian " + guardian,
	}); err != nil {
		return library.Lesson{}, err
	}
	return adm, nil
}

// EvolutionCap reads the per-day reflect+probe cap from regulus.json;
// missing or unset config means the default.
func EvolutionCap(root string) int {
	cfg, err := gates.Load(filepath.Join(root, "regulus.json"))
	if err != nil || cfg.Evolution == nil || cfg.Evolution.MaxPerDay <= 0 {
		return budget.DefaultEvolutionCapPerDay
	}
	return cfg.Evolution.MaxPerDay
}

// TaskCap bounds one task's share of the daily evolution budget. An
// explicit max_per_task wins; otherwise half the daily cap (minimum 1)
// so a single glottonous task cannot eat the whole day.
func TaskCap(root string) int {
	daily := EvolutionCap(root)
	cfg, err := gates.Load(filepath.Join(root, "regulus.json"))
	if err == nil && cfg.Evolution != nil && cfg.Evolution.MaxPerTask > 0 {
		return cfg.Evolution.MaxPerTask
	}
	half := daily / 2
	if half < 1 {
		half = 1
	}
	return half
}

func taskReceipts(root, task string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(root, ".regulus", "evidence", receipts.Slug(task), "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
