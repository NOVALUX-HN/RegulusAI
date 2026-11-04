package cli

// A1 (almanac-prime): `memory search` annotates every hit with its
// cited support — [supported], [rot], [unchecked] — and sinks rot
// below the rest (stable). Engram ranks by similarity plus model
// judgments; Almanac grades by evidence and says so on the line.
import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/evidence"
)

func writeSupportCLIReceipt(t *testing.T, taskDir, name, content string) string {
	t.Helper()
	p := filepath.Join(taskDir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func saveSupportObs(t *testing.T, root, title string, prov *almanac.Provenance, now time.Time) time.Time {
	t.Helper()
	_, err := almanac.Save(root, &almanac.Observation{
		Type: almanac.TypeDiscovery, Title: title, Content: "factfile body", Provenance: prov,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return now.Add(time.Second)
}

func TestMemorySearchAnnotatesSupport(t *testing.T) {
	t.Chdir(t.TempDir())
	root, _ := os.Getwd()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	taskDir, err := evidence.TaskDirSafe(root, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("ok"))
	okSHA := hex.EncodeToString(sum[:])
	empty := sha256.Sum256(nil)
	emptySHA := hex.EncodeToString(empty[:])
	okPath := writeSupportCLIReceipt(t, taskDir, "rec_ok.json",
		`{"schema_version":1,"stdout":"ok","stdout_sha256":"`+okSHA+`","stderr":"","stderr_sha256":"`+emptySHA+`"}`)
	writeSupportCLIReceipt(t, taskDir, "rec_bad.json", "{not json")
	now = saveSupportObs(t, root, "factfile supported one", &almanac.Provenance{Task: "t1", Receipts: []string{okPath}}, now)
	now = saveSupportObs(t, root, "factfile rot one", &almanac.Provenance{Task: "t1", Receipts: []string{filepath.Join(taskDir, "rec_bad.json")}}, now)
	_ = saveSupportObs(t, root, "factfile plain one", nil, now)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "--lexical", "factfile"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"[supported]", "[rot]", "[unchecked]"} {
		if !strings.Contains(got, want) {
			t.Errorf("search must annotate %q, got:\n%s", want, got)
		}
	}
	rot := strings.Index(got, "factfile rot one")
	sup := strings.Index(got, "factfile supported one")
	plain := strings.Index(got, "factfile plain one")
	if rot < 0 || sup < 0 || plain < 0 || !(rot > sup && rot > plain) {
		t.Fatalf("rot must sink below the rest, got:\n%s", got)
	}
}
