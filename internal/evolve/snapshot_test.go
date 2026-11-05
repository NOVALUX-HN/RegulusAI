package evolve

// SaveReport/LoadReport round-trip: snapshots pin today's economics
// byte-stably (golden JSON), and malformed snapshots refuse loudly
// instead of comparing garbage.
import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadReportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	want := Report{
		Domain: "go",
		Lessons: []LessonRow{
			{ID: "a", Domain: "go", Cue: "old", ProxyTokens: 10, Evidence: EvidenceOK},
		},
		Admitted: 1, TotalProxy: 10, Rot: 0,
	}
	if err := SaveReport(path, want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Admitted != 1 || got.TotalProxy != 10 || len(got.Lessons) != 1 || got.Lessons[0].ID != "a" {
		t.Fatalf("round trip = %+v", got)
	}
	if string(raw) == "" || raw[len(raw)-1] != '\n' {
		t.Fatal("snapshot must be non-empty newline-terminated JSON")
	}
}

func TestLoadReportRefusesGarbage(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.json")
	if _, err := LoadReport(missing); err == nil {
		t.Fatal("missing snapshot must error")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReport(bad); err == nil {
		t.Fatal("malformed snapshot must error")
	}
}
