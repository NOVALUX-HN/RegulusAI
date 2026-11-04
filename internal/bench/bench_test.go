package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuiteBand(t *testing.T) {
	tasks := Suite()
	// 8 behavior tasks within the 5-10 band, plus reported-never-judged
	// cost tasks (B7, B10, B11): measurement rides along, judgment stays
	// on behavior.
	if len(tasks) < 5 || len(tasks) > 12 {
		t.Fatalf("suite has %d tasks, want 5-12", len(tasks))
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		if task.ID == "" || task.Title == "" || task.Kind == "" || task.Prompt == "" {
			t.Fatalf("task %+v misses visible fields", task)
		}
		if task.Check == nil {
			t.Fatalf("task %s has no hidden check", task.ID)
		}
		if seen[task.ID] {
			t.Fatalf("duplicate task id %s", task.ID)
		}
		seen[task.ID] = true
	}
}

func TestSuitePromptsHideAnswers(t *testing.T) {
	for _, task := range Suite() {
		lower := strings.ToLower(task.Prompt)
		for _, leak := range []string{"want 2", "2.5", "traversal", "sha", "rollback", "estimatelocaltokens", "local proxy"} {
			if strings.Contains(lower, leak) {
				t.Fatalf("task %s prompt leaks its check %q", task.ID, leak)
			}
		}
	}
}

// P1: the B8-B10 prompts are golden — fixed reference strings. Any edit
// must update this pin deliberately, never drift silently.
func TestSuiteGoldenPromptsPinned(t *testing.T) {
	golden := map[string]string{
		"B8":  "Persist three harbor field notes in a fresh local memory, then surface the note about restoring service after a failed deploy.",
		"B9":  "Select the full and quick gate profiles over a realistic twelve-gate gauntlet and refuse unknown profiles.",
		"B10": "Measure golden token estimates over fixed trace entries and fold them into per-task totals.",
	}
	for _, task := range Suite() {
		want, ok := golden[task.ID]
		if !ok {
			continue
		}
		if task.Prompt != want {
			t.Fatalf("task %s prompt drifted from its golden:\n got: %q\nwant: %q", task.ID, task.Prompt, want)
		}
	}
	if len(golden) != 3 {
		t.Fatalf("golden pin covers %d tasks, want 3", len(golden))
	}
}

// T0 (pro-harness): B11 pins the lesson-context cost golden — the
// deterministic proxy-token price of one Almanac resume assembly over
// a fixed fixture store. Fase 4 diffs this number per lesson; the
// check reports, never judges (like wall-clock durations).
func TestB11LessonContextCostGolden(t *testing.T) {
	var b11 *Task
	for i, task := range Suite() {
		if task.ID == "B11" {
			b11 = &Suite()[i]
			break
		}
	}
	if b11 == nil {
		t.Fatal("suite has no B11 lesson-context-cost task")
	}
	wantPrompt := "Assemble one Almanac resume block over a fixed fixture store and report its token-proxy cost."
	if b11.Prompt != wantPrompt {
		t.Fatalf("B11 prompt drifted:\n got: %q\nwant: %q", b11.Prompt, wantPrompt)
	}
	detail, err := b11.Check()
	if err != nil {
		t.Fatalf("B11 check failed: %v", err)
	}
	for _, want := range []string{"fixture lessons", "proxy tokens"} {
		if !strings.Contains(detail, want) {
			t.Errorf("B11 detail must contain %q, got %q", want, detail)
		}
	}
}

func TestRunAllPassOffline(t *testing.T) {
	rep := Run("tree", "abc123")
	if rep.Suite != SuiteVersion {
		t.Fatalf("suite = %q, want %q", rep.Suite, SuiteVersion)
	}
	if rep.Passed != len(rep.Tasks) || rep.Failed != 0 {
		t.Fatalf("want all pass, got %d/%d", rep.Passed, len(rep.Tasks))
	}
	if rep.Surface != "tree" || rep.GitHead != "abc123" {
		t.Fatalf("report identity lost: %+v", rep)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	rep := Run("tree", "abc")
	path := filepath.Join(t.TempDir(), "sub", "rep.json")
	if err := Save(path, rep); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Passed != rep.Passed || len(back.Tasks) != len(rep.Tasks) {
		t.Fatalf("round-trip mismatch: %+v vs %+v", back.Passed, rep.Passed)
	}
	for i, task := range back.Tasks {
		if task.ID != rep.Tasks[i].ID || task.Pass != rep.Tasks[i].Pass {
			t.Fatalf("task %d changed in round-trip", i)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing report must error")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("malformed report must error")
	}
}

func TestCompareAdopt(t *testing.T) {
	base := Run("tree", "a")
	cand := Run("candidate", "b")
	cmp := Compare(base, cand)
	if cmp.Verdict != "ADOPT" {
		t.Fatalf("identical green runs: verdict = %s (%s)", cmp.Verdict, cmp.Reason)
	}
	if len(cmp.Rows) != len(base.Tasks) {
		t.Fatalf("rows = %d, want %d", len(cmp.Rows), len(base.Tasks))
	}
}

func TestCompareRejectOnRegression(t *testing.T) {
	base := Run("tree", "a")
	cand := Run("candidate", "b")
	cand.Tasks[0].Pass = false
	cand.Passed--
	cmp := Compare(base, cand)
	if cmp.Verdict != "REJECT" {
		t.Fatalf("regressed run: verdict = %s, want REJECT", cmp.Verdict)
	}
	if cmp.Regressions != 1 {
		t.Fatalf("regressions = %d, want 1", cmp.Regressions)
	}
}

func TestCompareCountsDroppedOnly(t *testing.T) {
	base := Run("tree", "a")
	cand := base
	cand.Passed = base.Passed - 1
	cmp := Compare(base, cand)
	if cmp.Verdict != "REJECT" {
		t.Fatalf("dropped count: verdict = %s, want REJECT", cmp.Verdict)
	}
}

func TestCompareImprovementKept(t *testing.T) {
	base := Run("tree", "a")
	base.Tasks[0].Pass = false
	base.Passed--
	cand := Run("candidate", "b")
	cmp := Compare(base, cand)
	if cmp.Verdict != "ADOPT" {
		t.Fatalf("fixed task: verdict = %s, want ADOPT", cmp.Verdict)
	}
	if cmp.Improvements != 1 {
		t.Fatalf("improvements = %d, want 1", cmp.Improvements)
	}
}

func TestEachCheckPasses(t *testing.T) {
	for _, task := range Suite() {
		detail, err := task.Check()
		if err != nil {
			t.Fatalf("task %s check failed: %v", task.ID, err)
		}
		if strings.TrimSpace(detail) == "" {
			t.Fatalf("task %s check returned empty detail", task.ID)
		}
	}
}

// P1: B8 pins the real retrieval path on its golden corpus.
func TestB8RetrievalGolden(t *testing.T) {
	detail, err := checkRetrievalGolden()
	if err != nil {
		t.Fatalf("B8 check failed: %v", err)
	}
	for _, want := range []string{"rollback drill", "1/2/0"} {
		if !strings.Contains(detail, want) {
			t.Errorf("B8 detail must contain %q, got %q", want, detail)
		}
	}
}

// P1: B9 pins the real twelve-gate profile contract.
func TestB9RealGateProfile(t *testing.T) {
	detail, err := checkRealGateProfile()
	if err != nil {
		t.Fatalf("B9 check failed: %v", err)
	}
	for _, want := range []string{"full=12", "quick=12", "unknown refused"} {
		if !strings.Contains(detail, want) {
			t.Errorf("B9 detail must contain %q, got %q", want, detail)
		}
	}
}

// P1: B10 pins golden T1 proxy values plus the per-task fold.
func TestB10TokenProxyGolden(t *testing.T) {
	detail, err := checkTokenProxyGolden()
	if err != nil {
		t.Fatalf("B10 check failed: %v", err)
	}
	for _, want := range []string{"5/8/7", "13 tokens", "host unknown"} {
		if !strings.Contains(detail, want) {
			t.Errorf("B10 detail must contain %q, got %q", want, detail)
		}
	}
}

// P1: B3 now machine-checks the spec hardening — Then-less,
// single-word and disordered scenarios all refuse.
func TestB3RefusesHardenedShapes(t *testing.T) {
	detail, err := checkSpecGWT()
	if err != nil {
		t.Fatalf("B3 check failed: %v", err)
	}
	for _, want := range []string{"Then-less", "single-word", "disordered"} {
		if !strings.Contains(detail, want) {
			t.Errorf("B3 detail must contain %q, got %q", want, detail)
		}
	}
}

// P1: the v3 suite keeps Compare ADOPT semantics — zero regressions and
// no pass-count drop across green runs, now over 11 tasks.
func TestCompareAdoptV2(t *testing.T) {
	base := Run("tree", "a")
	cand := Run("candidate", "b")
	if len(base.Tasks) != 11 || base.Passed != 11 {
		t.Fatalf("v3 suite = %d tasks %d passed, want 11/11", len(base.Tasks), base.Passed)
	}
	cmp := Compare(base, cand)
	if cmp.Verdict != "ADOPT" {
		t.Fatalf("identical green runs: verdict = %s (%s)", cmp.Verdict, cmp.Reason)
	}
}

// P1: B1-B7 keep their IDs, titles and kinds so v1 baseline rows stay
// comparable task by task.
func TestLegacyTasksUnchanged(t *testing.T) {
	want := map[string][2]string{
		"B1": {"cover-crap-formula", "cover"},
		"B2": {"split-bounded-parse", "split"},
		"B3": {"prose-spec-gwt", "prose"},
		"B4": {"veracity-receipt-roundtrip", "veracity"},
		"B5": {"gate-selection", "gates"},
		"B6": {"security-evidence-jail", "security"},
		"B7": {"honest-zero-cost", "cost"},
	}
	for _, task := range Suite() {
		w, ok := want[task.ID]
		if !ok {
			continue
		}
		if task.Title != w[0] || task.Kind != w[1] {
			t.Fatalf("task %s drifted: got title %q kind %q, want %q %q",
				task.ID, task.Title, task.Kind, w[0], w[1])
		}
		delete(want, task.ID)
	}
	if len(want) != 0 {
		t.Fatalf("legacy tasks missing from the suite: %v", want)
	}
}

// T1 honesty: B7 keeps host tokens unknown (0-regresiones semantics:
// Compare still ADOPTs on green runs) while reporting the
// measured-local proxy in its detail.
func TestB7HonestLocalProxy(t *testing.T) {
	var b7 *Task
	tasks := Suite()
	for i := range tasks {
		if tasks[i].ID == "B7" {
			b7 = &tasks[i]
			break
		}
	}
	if b7 == nil {
		t.Fatal("B7 task absent from the suite")
	}
	detail, err := b7.Check()
	if err != nil {
		t.Fatalf("B7 check failed: %v", err)
	}
	for _, want := range []string{"host tokens unknown", "local proxy"} {
		if !strings.Contains(detail, want) {
			t.Errorf("B7 detail must contain %q, got %q", want, detail)
		}
	}
}
