package evolve

// T4 (pro-harness, token-economizer): `evolve tick` fires every k
// traced sessions and proposes — never admits — candidates from two
// deterministic signals: tools failing repeatedly across sessions
// (micro-skill seeds), and admitted lessons whose task evidence no
// longer verifies (contradiction-by-rot). ComputeTick reads only;
// ApplyTick writes candidates + advances the tick state. Dry-run (CLI)
// prints without writing. The guardian still owns every admission.
import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/library"
	"regulus-ai/internal/trace"
)

func recordTickEntry(t *testing.T, root, session, call, tool, status string) {
	t.Helper()
	if _, err := trace.Record(root, trace.Entry{
		Session: session, Message: "m" + call, Call: call, Tool: tool, Status: status,
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestTickNotDue(t *testing.T) {
	root := t.TempDir()
	recordTickEntry(t, root, "s1", "c1", "read", trace.StatusCompleted)
	recordTickEntry(t, root, "s2", "c2", "read", trace.StatusCompleted)
	res, err := ComputeTick(root, 3)
	if err != nil {
		t.Fatal(err)
	}
	if res.Due {
		t.Fatalf("2 sessions with every=3 must not tick: %+v", res)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("no tick means no findings: %+v", res.Findings)
	}
}

func TestTickRepeatedErrorsPropose(t *testing.T) {
	root := t.TempDir()
	recordTickEntry(t, root, "s1", "c1", "gate_run", trace.StatusError)
	recordTickEntry(t, root, "s1", "c2", "gate_run", trace.StatusError)
	recordTickEntry(t, root, "s2", "c3", "gate_run", trace.StatusError)
	recordTickEntry(t, root, "s3", "c4", "read", trace.StatusCompleted)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	res, err := ComputeTick(root, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Due {
		t.Fatalf("3 sessions with every=3 must tick: %+v", res)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("want exactly the gate_run finding, got %+v", res.Findings)
	}
	f := res.Findings[0]
	if f.Kind != FindingRepeatedErrors || f.Domain != "harness" {
		t.Errorf("finding = %+v, want kind=repeated-errors domain=harness", f)
	}
	if len(f.Evidence) == 0 || f.Cue == "" || f.Lesson == "" {
		t.Errorf("finding must carry cue+lesson+evidence: %+v", f)
	}
	if err := ApplyTick(root, res, now); err != nil {
		t.Fatal(err)
	}
	cands, err := library.Candidates(root, "harness")
	if err != nil || len(cands) != 1 {
		t.Fatalf("apply must propose exactly one candidate: %+v, %v", cands, err)
	}
	again, err := ComputeTick(root, 3)
	if err != nil {
		t.Fatal(err)
	}
	if again.Due {
		t.Fatal("state must advance: second tick right after is not due")
	}
}

func TestTickStaleLesson(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	les := library.Lesson{
		Domain: "go", Task: "t1", Cue: "flaky gate", Lesson: "Rerun the gate once before debugging.",
		Evidence: []string{"rec_t1/1"}, Confidence: "medium",
		CreatedAt: now, GitHead: "abc",
	}
	if err := library.Admit(root, les, "g"); err != nil {
		t.Fatal(err)
	}
	taskDir, err := evidence.TaskDirSafe(root, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "rec_garbage.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	recordTickEntry(t, root, "s1", "c1", "read", trace.StatusCompleted)
	recordTickEntry(t, root, "s2", "c2", "read", trace.StatusCompleted)
	recordTickEntry(t, root, "s3", "c3", "read", trace.StatusCompleted)
	res, err := ComputeTick(root, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Due {
		t.Fatal("3 sessions must tick")
	}
	stale := 0
	for _, f := range res.Findings {
		if f.Kind == FindingStaleLesson {
			stale++
			if f.Domain != "go" || len(f.Evidence) == 0 {
				t.Errorf("stale finding must name the domain + failed receipts: %+v", f)
			}
		}
	}
	if stale != 1 {
		t.Fatalf("want exactly one stale-lesson finding, got %+v", res.Findings)
	}
}
