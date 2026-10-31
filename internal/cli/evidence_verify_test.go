package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/receipts"
)

// withoutManifests drops candidate-manifest index files from evidence
// globs: the manifest shares the task dir but is not a receipt.
func withoutManifests(paths []string) []string {
	var out []string
	for _, p := range paths {
		if strings.HasPrefix(filepath.Base(p), "manifest-") {
			continue
		}
		out = append(out, p)
	}
	return out
}

func writeVerifyReceipt(t *testing.T, dir, gate string, at time.Time, stdout string) string {
	t.Helper()
	rec := receipts.Receipt{
		Task: "feat/T1", Gate: gate, Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: at, DurationMS: 3,
		GitHead: "abc",
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture(stdout)
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	path, err := receipts.Write(dir, rec)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEvidenceVerifyClean(t *testing.T) {
	t.Chdir(t.TempDir())
	taskDir := filepath.Join(".regulus", "evidence", "feat__T1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1700000000, 0).UTC()
	writeVerifyReceipt(t, taskDir, "a", start, "out-a")
	writeVerifyReceipt(t, taskDir, "b", start.Add(time.Second), "out-b")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evidence", "verify", "--task", "feat/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "2 checked, 2 passed") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestEvidenceVerifyDetectsTamperAndQuarantines(t *testing.T) {
	t.Chdir(t.TempDir())
	taskDir := filepath.Join(".regulus", "evidence", "feat__T1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeVerifyReceipt(t, taskDir, "a", time.Unix(1700000000, 0).UTC(), "out-a")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), "out-a", "out-EVIL", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evidence", "verify", "--task", "feat/T1"}); code != 1 {
		t.Fatalf("exit = %d, want 1, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "stdout_hash_mismatch") || !strings.Contains(out.String(), "quarantined:") {
		t.Fatalf("output = %q", out.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("tampered receipt must be moved out of the evidence dir")
	}
}

func TestEvidenceVerifyUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "verify", "extra"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evidence verify") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestEvidenceShowPhase1Fields(t *testing.T) {
	t.Chdir(t.TempDir())
	taskDir := filepath.Join(".regulus", "evidence", "feat__T1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: "feat/T1", Gate: "a", Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: time.Unix(1700000000, 0).UTC(),
		DurationMS: 3, GitHead: "abc", RunID: "run_x", Status: "PASS", Reason: "ok",
		Manifest: "manifest-run_x.json", CPUTimeMS: 11, Isolation: "host-contained",
		Classification: "standard", Schema: receipts.ReceiptSchemaVersion,
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("hi")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	path, err := receipts.Write(taskDir, rec)
	if err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evidence", "show", path}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, want := range []string{"status:      PASS (ok)", "run_id:      run_x", "manifest:    manifest-run_x.json", "cpu_time_ms: 11", "isolation:   host-contained", "class:       standard", "schema:      1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("show must contain %q, got:\n%s", want, out.String())
		}
	}
}
