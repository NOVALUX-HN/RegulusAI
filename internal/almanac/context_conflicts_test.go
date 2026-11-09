package almanac

// A2 (almanac-prime): the resume block names live contradictions.
// Engram hides conflicts behind model judgments; Almanac shows both
// sides as DATA — a conflict where both endpoints are listed gets a
// Conflicts note (read both before trusting either). One-sided pairs
// (partner filtered out of this block) stay silent: half a
// contradiction is worse than none.
import (
	"strings"
	"testing"
	"time"
)

func TestContextShowsLiveConflicts(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, err := Save(root, &Observation{Type: TypeDiscovery, Title: "old deploy path", Content: "Deploy via script."}, now)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Save(root, &Observation{Type: TypeDiscovery, Title: "new deploy path", Content: "Deploy via pipeline."}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, a.ID, RelationConflictsWith, b.ID, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	ctx, err := Context(root, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx, "Conflicts") {
		t.Fatalf("block must name the contradiction, got:\n%s", ctx)
	}
	if !strings.Contains(ctx, a.ID) || !strings.Contains(ctx, b.ID) {
		t.Fatalf("block must name both sides, got:\n%s", ctx)
	}
}

func TestContextSilentWithoutConflicts(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	if _, err := Save(root, &Observation{Type: TypeDiscovery, Title: "solo", Content: "alone."}, now); err != nil {
		t.Fatal(err)
	}
	ctx, err := Context(root, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ctx, "Conflicts") {
		t.Fatalf("block without contradictions must stay silent, got:\n%s", ctx)
	}
}
