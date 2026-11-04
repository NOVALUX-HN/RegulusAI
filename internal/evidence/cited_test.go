package evidence

// CitedRot is the shared support semantics (almanac SupportOf, evolve
// report/tick/validate): only a CITED receipt failing verification
// rots support. A task with failures elsewhere leaves other citations
// intact — accusing them would punish lessons for rot they never
// cited. Unverifiable stores fail open (checked=false), never accuse.
import (
	"os"
	"path/filepath"
	"testing"
)

func writeCitedReceipt(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCitedRotGrades(t *testing.T) {
	root := t.TempDir()
	taskDir, err := TaskDirSafe(root, "t1")
	if err != nil {
		t.Fatal(err)
	}
	writeCitedReceipt(t, taskDir, "rec_ok.json", `{"schema_version":1,"stdout":"ok","stdout_sha256":"2689367b205c16ce32ed4200942b8b8b1e262dfc70d9bc9fbc77c49699a4f1df","stderr":"","stderr_sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}`)
	writeCitedReceipt(t, taskDir, "rec_bad.json", "{not json")
	okPath := filepath.Join(taskDir, "rec_ok.json")
	badPath := filepath.Join(taskDir, "rec_bad.json")
	if rot, checked := CitedRot(root, "t1", []string{okPath}); rot || !checked {
		t.Errorf("cited-ok = rot=%v checked=%v, want false/true", rot, checked)
	}
	if rot, checked := CitedRot(root, "t1", []string{badPath}); !rot || !checked {
		t.Errorf("cited-bad = rot=%v checked=%v, want true/true", rot, checked)
	}
	if rot, checked := CitedRot(root, "t1", nil); rot || checked {
		t.Errorf("no citations = rot=%v checked=%v, want false/false", rot, checked)
	}
	if rot, checked := CitedRot(root, "ghost", []string{"rec_x/1"}); rot || checked {
		t.Errorf("missing dir = rot=%v checked=%v, want false/false", rot, checked)
	}
	if rot, checked := CitedRot(root, "", []string{okPath}); rot || checked {
		t.Errorf("empty task = rot=%v checked=%v, want false/false", rot, checked)
	}
	// Quarantined citations count as failed: the bytes survive under
	// corrupt/ but the integrity check already condemned them.
	corrupt := filepath.Join(taskDir, "corrupt")
	if err := os.MkdirAll(corrupt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corrupt, "rec_old.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if rot, checked := CitedRot(root, "t1", []string{filepath.Join(corrupt, "rec_old.json")}); !rot || !checked {
		t.Errorf("quarantined citation = rot=%v checked=%v, want true/true", rot, checked)
	}
}
