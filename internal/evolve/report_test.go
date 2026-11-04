package evolve

// T5 (pro-harness, harness2-measurement): `evolve report` renders the
// lesson economics the Fase-4 question needs — per admitted lesson its
// proxy-token price (chars/4 over the rendered cue+lesson+evidence,
// the cost it adds to every context assembly) and its evidence status
// (ok / rot / unchecked), plus totals and retire lines for rot. The
// report judges nothing: a costly lesson that pays is fine, a cheap
// one with rot evidence is not — the guardian decides with these
// numbers, not instead of them.
import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/library"
)

func admitReportLesson(t *testing.T, root string, les library.Lesson) {
	t.Helper()
	if err := library.Admit(root, les, "g"); err != nil {
		t.Fatal(err)
	}
}

func TestReportPricesAndTotals(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	admitReportLesson(t, root, library.Lesson{
		Domain: "go", Task: "", Cue: "keep helpers pure",
		Lesson:     "Pure helpers stay testable without fixtures.",
		Evidence:   []string{"rec_x/1"},
		Confidence: "high", CreatedAt: now,
	})
	rep, err := BuildReport(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Lessons) != 1 {
		t.Fatalf("lessons = %d, want 1", len(rep.Lessons))
	}
	got := rep.Lessons[0]
	wantProxy := (int64(len("keep helpers pure")+len("Pure helpers stay testable without fixtures.")+len("rec_x/1")) + 3) / 4
	if got.ProxyTokens != wantProxy {
		t.Errorf("proxy = %d, want %d", got.ProxyTokens, wantProxy)
	}
	if got.Evidence != EvidenceUnchecked {
		t.Errorf("taskless lesson evidence = %q, want unchecked", got.Evidence)
	}
	if rep.TotalProxy != wantProxy || rep.Admitted != 1 {
		t.Errorf("totals = %+v, want 1 admitted %d proxy", rep, wantProxy)
	}
}

func TestReportFlagsRot(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	taskDir, err := evidence.TaskDirSafe(root, "t9")
	if err != nil {
		t.Fatal(err)
	}
	// Precise semantics: the lesson must cite the receipt that
	// actually fails — task dirt elsewhere does not rot support.
	garbage := filepath.Join(taskDir, "rec_garbage.json")
	admitReportLesson(t, root, library.Lesson{
		Domain: "go", Task: "t9", Cue: "old fact",
		Lesson:     "Something once true.",
		Evidence:   []string{garbage},
		Confidence: "medium", CreatedAt: now,
	})
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(garbage, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := BuildReport(root, "go")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Lessons) != 1 || rep.Lessons[0].Evidence != EvidenceRot {
		t.Fatalf("rot lesson = %+v", rep.Lessons)
	}
	if len(rep.Retire) != 1 {
		t.Fatalf("rot lesson must propose retirement: %+v", rep)
	}
}

func TestReportEmpty(t *testing.T) {
	rep, err := BuildReport(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Admitted != 0 || rep.TotalProxy != 0 || len(rep.Retire) != 0 {
		t.Fatalf("empty store report = %+v", rep)
	}
}
