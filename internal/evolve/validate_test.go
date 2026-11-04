package evolve

// P7 (pending-7): `evolve validate` renders the deterministic verdict
// per admitted lesson — keep, drop, or unreviewable — from evidence
// alone. Cost never judges (a costly lesson that pays is fine); rot
// drops (proposed — the guardian still disposes); unchecked stays by
// default. The report informs humans; validate gates automation.
import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/library"
)

func admitValidateLesson(t *testing.T, root string, les library.Lesson) {
	t.Helper()
	if err := library.Admit(root, les, "g"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateVerdicts(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	taskDir, err := evidence.TaskDirSafe(root, "tv")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	okPath := filepath.Join(taskDir, "rec_ok.json")
	if err := os.WriteFile(okPath, []byte(`{"schema_version":1,"stdout":"ok","stdout_sha256":"2689367b205c16ce32ed4200942b8b8b1e262dfc70d9bc9fbc77c49699a4f1df","stderr":"","stderr_sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "rec_bad.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	admitValidateLesson(t, root, library.Lesson{
		Domain: "go", Task: "tv", Cue: "good fact", Lesson: "True and checked.",
		Evidence: []string{okPath}, Confidence: "high", CreatedAt: now,
	})
	admitValidateLesson(t, root, library.Lesson{
		Domain: "go", Task: "tv", Cue: "dead fact", Lesson: "Once true.",
		Evidence: []string{filepath.Join(taskDir, "rec_bad.json")}, Confidence: "medium", CreatedAt: now.Add(time.Second),
	})
	admitValidateLesson(t, root, library.Lesson{
		Domain: "go", Task: "", Cue: "lore", Lesson: "No receipts cited.",
		Evidence: []string{"rec_x/1"}, Confidence: "low", CreatedAt: now.Add(2 * time.Second),
	})
	got, err := Validate(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"good fact": "keep", "dead fact": "drop", "lore": "unreviewable"}
	if len(got) != 3 {
		t.Fatalf("verdicts = %d, want 3", len(got))
	}
	for _, v := range got {
		if want[v.Cue] != v.Verdict {
			t.Errorf("cue %q verdict = %q, want %q (%s)", v.Cue, v.Verdict, want[v.Cue], v.Reason)
		}
		if v.Reason == "" {
			t.Errorf("cue %q carries no reason", v.Cue)
		}
	}
}

func TestValidateFiltersLesson(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	admitValidateLesson(t, root, library.Lesson{
		Domain: "go", Task: "", Cue: "only", Lesson: "Solo.",
		Evidence: []string{"rec_x/1"}, Confidence: "low", CreatedAt: now,
	})
	got, err := Validate(root, "go", "missing-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unknown id must yield no verdicts, got %+v", got)
	}
}
