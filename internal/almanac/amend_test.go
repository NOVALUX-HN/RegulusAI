package almanac

import (
	"errors"
	"testing"
	"time"
)

// TestAmendCorroborationSurvivesAsARevision is the wiring that gives Corroborate
// a durable surface: the corroboration survives a read-back, the old revision
// retires, and the derived confidence moves.
func TestAmendCorroborationSurvivesAsARevision(t *testing.T) {
	root := t.TempDir()
	original, err := Save(root, &Observation{
		Type: "discovery", Title: "corroborate me", Content: "claim", Author: "a1",
		Confidence: ConfidenceLow,
	}, now())
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	amended, err := Amend(root, original.ID, func(o *Observation) error {
		if err := Corroborate(o, "host-b"); err != nil {
			return err
		}
		return Corroborate(o, "host-c")
	}, "operator", now().Add(time.Minute))
	if err != nil {
		t.Fatalf("amend: %v", err)
	}
	if amended.ID == original.ID {
		t.Fatal("an amendment must mint a new revision, not reuse the id")
	}
	if len(amended.Corroborations) != 2 {
		t.Fatalf("corroborations = %v, want two", amended.Corroborations)
	}

	// The confidence the evidence supports is now medium, and it survives a
	// fresh read of the log.
	hits, err := Search(root, Query{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("default retrieval must show only the live revision, got %d", len(hits))
	}
	if got := DerivedConfidence(hits[0]); got != ConfidenceMedium {
		t.Fatalf("derived confidence after amendment = %q, want medium", got)
	}
	if len(hits[0].Corroborations) != 2 {
		t.Fatalf("the corroborations did not survive the read-back: %v", hits[0].Corroborations)
	}

	// The superseded revision stays auditable.
	all, err := Search(root, Query{IncludeSuperseded: true, IncludeStale: true})
	if err != nil {
		t.Fatalf("search all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("both revisions must stay auditable, got %d", len(all))
	}
}

// TestAmendRevalidationRenewsFreshness: the same surface is what Revalidate
// needed, and an expired claim is retrievable again after it.
func TestAmendRevalidationRenewsFreshness(t *testing.T) {
	root := t.TempDir()
	past := now().Add(-48 * time.Hour)
	original, err := Save(root, &Observation{
		Type: "discovery", Title: "renew me", Content: "claim", Author: "a1",
		ExpiresAt: &past,
	}, now())
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if hits, _ := Search(root, Query{}); len(hits) != 0 {
		t.Fatalf("precondition: the expired claim must be hidden, got %d", len(hits))
	}

	if _, err := Amend(root, original.ID, func(o *Observation) error {
		return Revalidate(o, "operator", now(), 7*24*time.Hour)
	}, "operator", now()); err != nil {
		t.Fatalf("amend: %v", err)
	}

	hits, err := Search(root, Query{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("a revalidated claim must be retrievable again, got %d", len(hits))
	}
	if hits[0].RevalidatedAt == nil {
		t.Fatal("revalidated_at must survive the revision")
	}
}

// TestAmendRefusesUnsignedAndUnknown keeps the primitive honest: no actor and no
// target means no revision.
func TestAmendRefusesUnsignedAndUnknown(t *testing.T) {
	root := t.TempDir()
	o, err := Save(root, &Observation{Type: "discovery", Title: "t", Content: "c", Author: "a1"}, now())
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := Amend(root, o.ID, func(*Observation) error { return nil }, "  ", now()); err == nil {
		t.Fatal("an amendment without an actor must be refused")
	}
	if _, err := Amend(root, o.ID, nil, "operator", now()); err == nil {
		t.Fatal("an amendment without a mutation must be refused")
	}
	if _, err := Amend(root, "obs_missing", func(*Observation) error { return nil }, "operator", now()); err == nil {
		t.Fatal("an amendment of an unknown id must be refused")
	}
	// A refused amendment leaves exactly one record.
	hits, _ := Search(root, Query{IncludeStale: true})
	if len(hits) != 1 {
		t.Fatalf("refusals must not write anything, got %d records", len(hits))
	}
}

// TestAmendRefusedMutationWritesNothing: a mutation that errors halfway must not
// leave a partial revision behind.
func TestAmendRefusedMutationWritesNothing(t *testing.T) {
	root := t.TempDir()
	o, err := Save(root, &Observation{Type: "discovery", Title: "t", Content: "c", Author: "a1"}, now())
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := Amend(root, o.ID, func(x *Observation) error {
		x.Corroborations = append(x.Corroborations, "half")
		return errRefusedForTest
	}, "operator", now()); err == nil {
		t.Fatal("a failing mutation must be reported")
	}
	hits, _ := Search(root, Query{IncludeStale: true})
	if len(hits) != 1 || len(hits[0].Corroborations) != 0 {
		t.Fatalf("a refused mutation must not write, got %+v", hits)
	}
}

var errRefusedForTest = errors.New("refused for test")
