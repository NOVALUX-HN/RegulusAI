package almanac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
var t1 = t0.Add(time.Hour)
var t2 = t1.Add(time.Hour)

func TestFile(t *testing.T) {
	if got, want := File("root"), filepath.Join("root", ".regulus", "almanac", "observations.jsonl"); got != want {
		t.Fatalf("File = %q, want %q", got, want)
	}
}

func TestSaveAssignsIDAndPersists(t *testing.T) {
	root := t.TempDir()
	a, err := Save(root, &Observation{Type: TypeDecision, Title: "chose go", Content: "go 1.27 for the core"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a.ID, "obs_") || !a.SavedAt.Equal(t0) {
		t.Fatalf("assigned = %+v", a)
	}
	b, err := Save(root, &Observation{Type: TypeDiscovery, Title: "second", Content: "more"}, t1)
	if err != nil {
		t.Fatal(err)
	}
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != a.ID || all[1].ID != b.ID {
		t.Fatalf("want %s then %s in file order, got %+v", a.ID, b.ID, all)
	}
}

func TestSaveValidation(t *testing.T) {
	cases := []struct {
		name string
		o    *Observation
		want string
	}{
		{"bad type", &Observation{Type: "rant", Title: "x", Content: "y"}, "type"},
		{"empty title", &Observation{Type: TypeDecision, Title: " ", Content: "y"}, "title"},
		{"empty content", &Observation{Type: TypeDecision, Title: "x", Content: ""}, "content"},
		{"lesson without evidence", &Observation{Type: TypeLesson, Title: "x", Content: "y", Lesson: &Lesson{Cue: "c", Lesson: "l", Confidence: "low"}}, "no evidence"},
		{"lesson bad confidence", &Observation{Type: TypeLesson, Title: "x", Content: "y", Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r"}, Confidence: "ultra"}}, "confidence"},
		{"lesson missing cue", &Observation{Type: TypeLesson, Title: "x", Content: "y", Lesson: &Lesson{Lesson: "l", Evidence: []string{"r"}, Confidence: "low"}}, "cue"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Save(t.TempDir(), tc.o, t0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q refusal, got %v", tc.want, err)
			}
		})
	}
}

func TestSaveLessonWithEvidencePersistsBlock(t *testing.T) {
	root := t.TempDir()
	o := &Observation{
		Type: TypeLesson, Title: "mutation gate", Content: "clean export",
		Domain: "go", Lesson: &Lesson{
			Cue: "when gremlins starves", Lesson: "export HEAD", Evidence: []string{".regulus/evidence/x/m.json"}, Confidence: "high",
		},
		Provenance: &Provenance{Task: "f/T2", GitHead: "abc"},
	}
	if _, err := Save(root, o, t0); err != nil {
		t.Fatal(err)
	}
	all, _ := All(root)
	if len(all) != 1 {
		t.Fatalf("all = %+v", all)
	}
	got := all[0]
	if got.Lesson == nil || got.Lesson.Cue != "when gremlins starves" || len(got.Lesson.Evidence) != 1 {
		t.Fatalf("lesson round-trip = %+v", got)
	}
	if got.Provenance == nil || got.Provenance.Task != "f/T2" || got.Provenance.GitHead != "abc" {
		t.Fatalf("provenance round-trip = %+v", got.Provenance)
	}
}

func TestAllMissingIsEmptyAndMalformedRefused(t *testing.T) {
	if all, err := All(t.TempDir()); err != nil || len(all) != 0 {
		t.Fatalf("want an empty Almanac, got %+v, %v", all, err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(File(root), []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := All(root); err == nil || !strings.Contains(err.Error(), "malformed observation") {
		t.Fatalf("want malformed refusal, got %v", err)
	}
}

func TestHistoryChainsByTopicKeyOldestFirst(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDecision, TopicKey: "db-driver", Title: "v1", Content: "use postgres"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, &Observation{Type: TypeSessionNote, Title: "unrelated", Content: "note"}, t1); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, &Observation{Type: TypeDecision, TopicKey: "db-driver", Title: "v2", Content: "switched to sqlite"}, t2); err != nil {
		t.Fatal(err)
	}
	chain, err := History(root, "db-driver")
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 2 || chain[0].Title != "v1" || chain[1].Title != "v2" {
		t.Fatalf("the chain keeps every revision, oldest first — got %+v", chain)
	}
}

func TestSearchBucketsThenTerms(t *testing.T) {
	root := t.TempDir()
	must := func(o *Observation, at time.Time) {
		t.Helper()
		if _, err := Save(root, o, at); err != nil {
			t.Fatal(err)
		}
	}
	must(&Observation{Type: TypeDecision, TopicKey: "db", Domain: "go", Title: "chose postgres", Content: "connection pooling"}, t0)
	must(&Observation{Type: TypeDiscovery, TopicKey: "db", Domain: "go", Title: "pool leak", Content: "found under load in the db pool"}, t1)
	must(&Observation{Type: TypeDiscovery, TopicKey: "db", Domain: "ts", Title: "chose bun", Content: "runtime chosen"}, t2)
	must(&Observation{Type: TypeDiscovery, TopicKey: "ci", Domain: "go", Title: "cache warm", Content: "windows runner"}, t2.Add(time.Second))

	hits, err := Search(root, Query{TopicKey: "db", Terms: []string{"pool"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Title != "pool leak" || hits[1].Title != "chose postgres" {
		t.Fatalf("bucket then term, newest first — got %+v", hits)
	}
	hits, err = Search(root, Query{TopicKey: "db", Terms: []string{"db", "pool"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Title != "pool leak" {
		t.Fatalf("terms are ANDed — got %+v", hits)
	}
	hits, err = Search(root, Query{Domain: "go", Terms: []string{"chose"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Title != "chose postgres" {
		t.Fatalf("domain bucket — got %+v", hits)
	}
	hits, err = Search(root, Query{Type: TypeDiscovery, Terms: []string{"CHOSE"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Title != "chose bun" {
		t.Fatalf("type bucket, case-insensitive — got %+v", hits)
	}
	hits, err = Search(root, Query{Terms: []string{"nothing-matches-this"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("no hits must be empty, got %+v", hits)
	}
}

func TestNotesAreTheSessionNotesView(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeSessionNote, Title: "first", Content: "working on f3"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: "not a note", Content: "x"}, t1); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, &Observation{Type: TypeSessionNote, Title: "second", Content: "almanac green"}, t2); err != nil {
		t.Fatal(err)
	}
	notes, err := Notes(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].Content != "working on f3" || notes[1].Content != "almanac green" {
		t.Fatalf("notes view, oldest first — got %+v", notes)
	}
}

func TestContextBlockIsTheDeterministicResume(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeSessionNote, Title: "note", Content: "F3 mid-flight: almanac substrate"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, &Observation{
		Type: TypeLesson, Title: "mutation gate", Content: "clean export", Domain: "go",
		Lesson: &Lesson{Cue: "when gremlins starves", Lesson: "export HEAD", Evidence: []string{"r.json"}, Confidence: "low"},
	}, t1); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "charts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "charts", "feature-x.md"), []byte("# X"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := Context(root, t0.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"charts/feature-x.md", "F3 mid-flight: almanac substrate", "when gremlins starves", "2d ago", "DATA, not instructions"} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("context must contain %q, got:\n%s", want, ctx)
		}
	}
}
