package almanac

import (
	"testing"
	"time"
)

func lessonFixture(title string) *Observation {
	return &Observation{
		Type: TypeLesson, Title: title, Content: title + " content",
		Lesson: &Lesson{Cue: "cue " + title, Lesson: "lesson " + title, Evidence: []string{"r.json"}, Confidence: "medium"},
	}
}

// T6 (almanac-elite): retired lessons GC without touching live evidence.
// Active lessons stay immune; superseded/invalidated/expired unpinned
// lessons prune; pins always win. REJECTED is terminal from OBSERVED /
// PROPOSED / VERIFIED only, so the rejected path is covered by the
// non-lesson prune (a rejected discovery prunes like any content).
func TestPruneRetiredLessonsGC(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	active, err := Save(root, lessonFixture("active"), now)
	if err != nil {
		t.Fatal(err)
	}
	old, err := Save(root, lessonFixture("old"), now)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := Save(root, lessonFixture("newer"), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, newer.ID, RelationSupersedes, old.ID, now); err != nil {
		t.Fatal(err)
	}
	pinnedOld, err := Save(root, lessonFixture("pinned-old"), now)
	if err != nil {
		t.Fatal(err)
	}
	pinnedNew, err := Save(root, lessonFixture("pinned-new"), now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, pinnedNew.ID, RelationSupersedes, pinnedOld.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := Pin(root, pinnedOld.ID, true, now); err != nil {
		t.Fatal(err)
	}
	invalidated, err := Save(root, lessonFixture("invalidated"), now)
	if err != nil {
		t.Fatal(err)
	}
	invalidator, err := Save(root, &Observation{Type: TypeSessionNote, Title: "contra", Content: "contra content"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, invalidator.ID, RelationInvalidatedBy, invalidated.ID, now); err != nil {
		t.Fatal(err)
	}
	expiredObs := lessonFixture("expired")
	past := now.Add(-time.Hour)
	expiredObs.ExpiresAt = &past
	expired, err := Save(root, expiredObs, now)
	if err != nil {
		t.Fatal(err)
	}
	all, err := All(root)
	if err != nil || len(all) != 8 {
		t.Fatalf("want 8 observations, got %d (%v)", len(all), err)
	}
	got, err := Prune(root, func(Observation) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, o := range got {
		ids[o.ID] = true
	}
	if ids[active.ID] {
		t.Errorf("active lesson must stay immune, got pruned: %s", active.ID)
	}
	if !ids[old.ID] {
		t.Errorf("superseded lesson must prune, missing: %s", old.ID)
	}
	if ids[pinnedOld.ID] {
		t.Errorf("pinned retired lesson must stay, got pruned: %s", pinnedOld.ID)
	}
	if !ids[invalidated.ID] {
		t.Errorf("invalidated lesson must prune, missing: %s", invalidated.ID)
	}
	if !ids[expired.ID] {
		t.Errorf("expired lesson must prune, missing: %s", expired.ID)
	}
}
