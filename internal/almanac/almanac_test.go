package almanac

import (
	"os"
	"os/exec"
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

func TestSearchPinnedFirst(t *testing.T) {
	root := t.TempDir()
	old, _ := Save(root, &Observation{Type: TypeDecision, Title: "older result", Content: "matches"}, t0)
	_, _ = Save(root, &Observation{Type: TypeDecision, Title: "newer result", Content: "matches"}, t1)
	if err := Pin(root, old.ID, true, t2); err != nil {
		t.Fatal(err)
	}
	hits, err := Search(root, Query{Terms: []string{"matches"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 hits, got %+v", hits)
	}
	if hits[0].ID != old.ID {
		t.Fatalf("pinned must come first: hits[0] = %+v", hits[0])
	}
}

func TestContextShowsPinnedSection(t *testing.T) {
	root := t.TempDir()
	a, _ := Save(root, &Observation{Type: TypeDecision, Title: "the key decision", Content: "important"}, t0)
	_, _ = Save(root, &Observation{Type: TypeSessionNote, Title: "note", Content: "regular"}, t1)
	if err := Pin(root, a.ID, true, t2); err != nil {
		t.Fatal(err)
	}
	ctx, err := Context(root, t2.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx, "Pinned:") {
		t.Fatalf("context must show a Pinned section, got:\n%s", ctx)
	}
	if !strings.Contains(ctx, "the key decision") {
		t.Fatalf("context must show the pinned decision, got:\n%s", ctx)
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

func TestPinUnpinLastEventWins(t *testing.T) {
	root := t.TempDir()
	a, _ := Save(root, &Observation{Type: TypeDecision, Title: "first", Content: "x"}, t0)
	b, _ := Save(root, &Observation{Type: TypeDecision, Title: "second", Content: "y"}, t1)
	if err := Pin(root, a.ID, true, t2); err != nil {
		t.Fatal(err)
	}
	if err := Pin(root, b.ID, true, t2); err != nil {
		t.Fatal(err)
	}
	pins, err := PinnedIDs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !pins[a.ID] || !pins[b.ID] {
		t.Fatalf("both pinned, got %v", pins)
	}
	if err := Pin(root, b.ID, false, t2.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	pins, _ = PinnedIDs(root)
	if pins[b.ID] {
		t.Fatal("the last event must win: b was unpinned")
	}
	if !pins[a.ID] {
		t.Fatal("a stays pinned")
	}
	if err := Pin(root, "obs_nope", true, t2); err == nil || !strings.Contains(err.Error(), "no observation") {
		t.Fatalf("want no-observation refusal, got %v", err)
	}
}

func TestRelateValidatesRelationAndIds(t *testing.T) {
	root := t.TempDir()
	a, _ := Save(root, &Observation{Type: TypeDecision, Title: "v1", Content: "x"}, t0)
	b, _ := Save(root, &Observation{Type: TypeDecision, Title: "v2", Content: "y"}, t1)
	if err := Relate(root, a.ID, RelationSupersedes, b.ID, t2); err != nil {
		t.Fatal(err)
	}
	rels, err := Relations(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || rels[0].From != a.ID || rels[0].Relation != RelationSupersedes || rels[0].To != b.ID {
		t.Fatalf("relations = %+v", rels)
	}
	if err := Relate(root, a.ID, "kinda-related", b.ID, t2); err == nil || !strings.Contains(err.Error(), "relation") {
		t.Fatalf("want relation refusal, got %v", err)
	}
	if err := Relate(root, a.ID, RelationConflictsWith, "obs_nope", t2); err == nil || !strings.Contains(err.Error(), "no observation") {
		t.Fatalf("want id refusal, got %v", err)
	}
	if err := Relate(root, a.ID, RelationRelated, a.ID, t2); err == nil || !strings.Contains(err.Error(), "itself") {
		t.Fatalf("want self-relation refusal, got %v", err)
	}
}

func TestNormalizeProjectName(t *testing.T) {
	cases := []struct{ remote, dir, want string }{
		{"https://github.com/Owner/Repo.git", "fallback", "owner/repo"},
		{"git@github.com:Owner/Repo.git", "fallback", "owner/repo"},
		{"https://gitlab.com/group/sub/repo.git", "fallback", "sub/repo"},
		{"", "My-Project", "my-project"},
		{"not a url at all", "dir", "dir"},
	}
	for _, tc := range cases {
		if got := NormalizeProjectName(tc.remote, tc.dir); got != tc.want {
			t.Fatalf("NormalizeProjectName(%q, %q) = %q, want %q", tc.remote, tc.dir, got, tc.want)
		}
	}
}

func TestProjectNameRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-m", "x")
	if got := ProjectName(dir); got != filepath.Base(dir) {
		t.Fatalf("no remote → directory name, got %q", got)
	}
	git("remote", "add", "origin", "https://github.com/Andragon/RegulusAI.git")
	if got := ProjectName(dir); got != "andragon/regulusai" {
		t.Fatalf("remote → normalized, got %q", got)
	}
}

func TestExportWritesBundle(t *testing.T) {
	root := t.TempDir()
	a, _ := Save(root, &Observation{Type: TypeDecision, Title: "chose go", Content: "stdlib only"}, t0)
	b, _ := Save(root, &Observation{
		Type: TypeLesson, Title: "mutation", Content: "clean export", Domain: "go",
		Lesson: &Lesson{Cue: "when gremlins starves", Lesson: "export", Evidence: []string{"r"}, Confidence: "high"},
	}, t1)
	if err := Pin(root, b.ID, true, t2); err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, b.ID, RelationSupersedes, a.ID, t2); err != nil {
		t.Fatal(err)
	}
	paths, err := Export(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("want the jsonl + INDEX, got %v", paths)
	}
	data, err := os.ReadFile(filepath.Join(root, ".regulus-export", "almanac.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("the bundle carries every observation, got %d lines", len(lines))
	}
	index, err := os.ReadFile(filepath.Join(root, ".regulus-export", "INDEX.md"))
	if err != nil {
		t.Fatal(err)
	}
	idx := string(index)
	for _, want := range []string{"chose go", "when gremlins starves", "Pinned", "supersedes", a.ID, b.ID} {
		if !strings.Contains(idx, want) {
			t.Fatalf("INDEX must contain %q, got:\n%s", want, idx)
		}
	}
}

func TestLessonDimensionRoundTripAndBucket(t *testing.T) {
	root := t.TempDir()
	inv := &Observation{Type: TypeLesson, Title: "inv lesson", Content: "how to run gates",
		Domain: "go", Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low", Dimension: "inv"}}
	dom := &Observation{Type: TypeLesson, Title: "dom lesson", Content: "how to structure",
		Domain: "go", Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"rec_1.json"}, Confidence: "low", Dimension: "dom"}}
	if _, err := Save(root, inv, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, dom, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	hits, err := Search(root, Query{Dimension: "inv"})
	if err != nil || len(hits) != 1 || hits[0].Title != "inv lesson" {
		t.Fatalf("dimension bucket = %+v (%v)", hits, err)
	}
	if hits[0].Lesson == nil || hits[0].Lesson.Dimension != "inv" {
		t.Fatalf("dimension must survive the round-trip, got %+v", hits[0].Lesson)
	}
	all, err := Search(root, Query{})
	if err != nil || len(all) != 2 {
		t.Fatalf("unfiltered search = %+v (%v)", all, err)
	}
}

func TestLessonDimensionValidation(t *testing.T) {
	bad := &Observation{Type: TypeLesson, Title: "t", Content: "c",
		Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r"}, Confidence: "low", Dimension: "sideways"}}
	if _, err := Save(t.TempDir(), bad, t0); err == nil || !strings.Contains(err.Error(), "dimension") {
		t.Fatalf("want dimension refusal, got %v", err)
	}
}

func TestProvenancePathBucket(t *testing.T) {
	root := t.TempDir()
	a := &Observation{Type: TypeDecision, Title: "chose x", Content: "why x",
		Provenance: &Provenance{Task: "T1", Paths: []string{"internal/cli/cli.go"}}}
	b := &Observation{Type: TypeDecision, Title: "chose y", Content: "why y",
		Provenance: &Provenance{Task: "T1", Paths: []string{"other/file.go"}}}
	if _, err := Save(root, a, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, b, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	hits, err := Search(root, Query{Path: "internal/cli/cli.go"})
	if err != nil || len(hits) != 1 || hits[0].Title != "chose x" {
		t.Fatalf("path bucket = %+v (%v)", hits, err)
	}
	none, err := Search(root, Query{Path: "missing/file.go"})
	if err != nil || len(none) != 0 {
		t.Fatalf("path miss = %+v (%v)", none, err)
	}
}

func TestPruneSelectsOldUnpinnedNonLessons(t *testing.T) {
	root := t.TempDir()
	old := t0.Add(-30 * 24 * time.Hour)
	recent := t0
	save := func(o *Observation, at time.Time) Observation {
		t.Helper()
		saved, err := Save(root, o, at)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	// NOTE: ids derive from SavedAt, so every save needs a distinct
	// timestamp or ids collide.
	oldNote := save(&Observation{Type: TypeDecision, Title: "old", Content: "c"}, old)
	_ = save(&Observation{Type: TypeDecision, Title: "new", Content: "c"}, recent)
	_ = save(&Observation{Type: TypeLesson, Title: "old lesson", Content: "c",
		Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r"}, Confidence: "low"}}, old.Add(time.Second))
	pinnedOld := save(&Observation{Type: TypeDecision, Title: "pinned", Content: "c"}, old.Add(2*time.Second))
	if err := Pin(root, pinnedOld.ID, true, recent); err != nil {
		t.Fatal(err)
	}
	cutoff := t0.Add(-7 * 24 * time.Hour)
	got, err := Prune(root, func(o Observation) bool { return !o.SavedAt.Before(cutoff) })
	if err != nil || len(got) != 1 || got[0].ID != oldNote.ID {
		t.Fatalf("prune selection = %+v (%v)", got, err)
	}
	// ApplyPrune drops exactly the selected ones.
	if err := ApplyPrune(root, got); err != nil {
		t.Fatal(err)
	}
	all, err := All(root)
	if err != nil || len(all) != 3 {
		t.Fatalf("after prune = %+v (%v)", all, err)
	}
	for _, o := range all {
		if o.ID == oldNote.ID {
			t.Fatal("pruned observation must be gone")
		}
	}
	// Empty selection is a no-op (log byte-identical).
	before, _ := os.ReadFile(File(root))
	if err := ApplyPrune(root, nil); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(File(root))
	if string(before) != string(after) {
		t.Fatal("empty ApplyPrune must not touch the log")
	}
}

func TestSearchLatestCollapsesTopicChains(t *testing.T) {
	root := t.TempDir()
	save := func(title string, at time.Time) Observation {
		t.Helper()
		s, err := Save(root, &Observation{Type: TypeDecision, Title: title, Content: "rev", TopicKey: "db"}, at)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	mid := save("v2-the-middle", t0.Add(time.Minute))
	_ = mid
	save("v1-the-oldest", t0)
	save("v3-the-latest", t0.Add(2*time.Minute))
	// Default: a topic key is a coexisting group — every hit returns.
	all, err := Search(root, Query{})
	if err != nil || len(all) != 3 {
		t.Fatalf("default must return the whole group, got %d (%v)", len(all), err)
	}
	// --latest collapses the chain to the newest revision.
	hits, err := Search(root, Query{Latest: true})
	if err != nil || len(hits) != 1 || hits[0].Title != "v3-the-latest" {
		t.Fatalf("--latest must collapse to the newest, got %+v (%v)", hits, err)
	}
	// A pinned older revision overrides recency under --latest.
	if err := Pin(root, mid.ID, true, t0.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	hits, err = Search(root, Query{Latest: true})
	if err != nil || len(hits) != 1 || hits[0].Title != "v2-the-middle" {
		t.Fatalf("a pinned revision must win the chain, got %+v (%v)", hits, err)
	}
}

func TestSearchAnyMode(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDiscovery, Title: "postgres chosen", Content: "db"}, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(root, &Observation{Type: TypeDiscovery, Title: "redis cache", Content: "kv"}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	anded, err := Search(root, Query{Terms: []string{"postgres", "redis"}})
	if err != nil || len(anded) != 0 {
		t.Fatalf("AND must find nothing, got %+v (%v)", anded, err)
	}
	any, err := Search(root, Query{Terms: []string{"postgres", "redis"}, Any: true})
	if err != nil || len(any) != 2 {
		t.Fatalf("--any must find both, got %+v (%v)", any, err)
	}
}

func TestSearchLimitAndConfidenceRank(t *testing.T) {
	root := t.TempDir()
	older, err := Save(root, &Observation{Type: TypeLesson, Title: "high older", Content: "x",
		Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r"}, Confidence: "high"}}, t0)
	if err != nil {
		t.Fatal(err)
	}
	_ = older
	if _, err := Save(root, &Observation{Type: TypeLesson, Title: "low newer", Content: "x",
		Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r"}, Confidence: "low"}}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	hits, err := Search(root, Query{Type: TypeLesson})
	if err != nil || len(hits) != 2 || hits[0].Title != "high older" {
		t.Fatalf("confidence must outrank recency among lessons, got %+v (%v)", hits, err)
	}
	limited, err := Search(root, Query{Type: TypeLesson, Limit: 1})
	if err != nil || len(limited) != 1 || limited[0].Title != "high older" {
		t.Fatalf("limit must keep the top hit, got %+v (%v)", limited, err)
	}
}

func TestSearchHidesSuperseded(t *testing.T) {
	root := t.TempDir()
	old, err := Save(root, &Observation{Type: TypeDecision, Title: "old choice", Content: "x"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	new, err := Save(root, &Observation{Type: TypeDecision, Title: "new choice", Content: "x"}, t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, new.ID, RelationSupersedes, old.ID, t0.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	hits, err := Search(root, Query{})
	if err != nil || len(hits) != 1 || hits[0].ID != new.ID {
		t.Fatalf("superseded must hide by default, got %+v (%v)", hits, err)
	}
	shown, err := Search(root, Query{IncludeSuperseded: true})
	if err != nil || len(shown) != 2 {
		t.Fatalf("--include-superseded must show both, got %+v (%v)", shown, err)
	}
	// A pruned superseder releases the target: swap roles by pruning
	// the superseder (a non-lesson, unpinned) through the prune door.
	if err := Pin(root, old.ID, false, t0.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	removed, err := Prune(root, func(o Observation) bool { return o.ID != new.ID })
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPrune(root, removed); err != nil {
		t.Fatal(err)
	}
	hits, err = Search(root, Query{})
	if err != nil || len(hits) != 1 || hits[0].ID != old.ID {
		t.Fatalf("a pruned superseder must release its target, got %+v (%v)", hits, err)
	}
}
