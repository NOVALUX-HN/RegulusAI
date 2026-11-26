package protocol

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// phasePractices is the vocabulary the phase named, verbatim from NOV-150:
// "subtract, premise, root cause, TDD-when-appropriate, small units, behavior,
// boundaries, idempotency, evidence, regressions y rollback".
//
// Pinning it here means dropping a practice is a reviewed act, not a tidy-up.
var phasePractices = []string{
	"subtract", "premise", "root-cause", "tdd", "small-units",
	"behavior", "boundaries", "idempotency", "evidence", "regression", "rollback",
}

func TestTableIsValid(t *testing.T) {
	if errs := Validate(All()); len(errs) != 0 {
		t.Fatalf("protocol table is invalid:\n%s", strings.Join(errs, "\n"))
	}
}

func TestEveryPhasePracticeIsDeclared(t *testing.T) {
	got := make(map[string]bool)
	for _, p := range All() {
		got[p.ID] = true
	}
	for _, id := range phasePractices {
		if !got[id] {
			t.Errorf("practice %q is named by the phase but missing from the table", id)
		}
	}
	if len(All()) != len(phasePractices) {
		t.Errorf("table has %d practices, phase names %d", len(All()), len(phasePractices))
	}
}

// TestCitedCodeStillExists is the tripwire that makes this package worth having.
//
// Every citation is a marker string that must still be in the file it names. If
// somebody deletes the zero-skipped check in sextant.close, or the size gate in
// regulus.json, this fails: the table cannot keep claiming enforcement that no
// longer exists.
func TestCitedCodeStillExists(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, p := range All() {
		if p.File == "" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p.File)))
		if err != nil {
			t.Errorf("%s: cited file %s is unreadable: %v", p.ID, p.File, err)
			continue
		}
		if !strings.Contains(string(body), p.Marker) {
			t.Errorf("%s: %s no longer contains %q - the practice now claims enforcement (or advice) that is gone", p.ID, p.File, p.Marker)
		}
	}
}

// TestEnforcedPracticesAreBlocking guards the distinction the whole package exists
// for: a practice marked enforced must cite code, and a practice with no oracle
// must not pretend to have one.
func TestEnforcedPracticesAreBlocking(t *testing.T) {
	for _, p := range All() {
		if p.Enforcement == Enforced && p.File == "" {
			t.Errorf("%s: marked enforced without a citation", p.ID)
		}
	}
}

// TestDocumentListsEveryPractice keeps the prose and the table together.
//
// A protocol nobody can read in one place cannot be audited as one, and a document
// that drifts from the table is worse than none: it is the same defect that
// docdrift caught twice on this project.
func TestDocumentListsEveryPractice(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "docs", "engineering-protocol.md"))
	if err != nil {
		t.Fatalf("docs/engineering-protocol.md is unreadable: %v", err)
	}
	doc := string(body)
	for _, p := range All() {
		if !strings.Contains(doc, p.ID) {
			t.Errorf("docs/engineering-protocol.md never mentions practice %q", p.ID)
		}
	}
}
