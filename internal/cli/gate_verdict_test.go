package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/guardians"
	"regulus-ai/internal/receipts"
)

func writeVerdictFixture(t *testing.T, evidTask, gate string, pass bool) string {
	t.Helper()
	dir := evidence.TaskDir(".", evidTask)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	code, verdict := 0, receipts.VerdictPass
	if !pass {
		code, verdict = 1, receipts.VerdictFail
	}
	rec := receipts.Receipt{
		Task: evidTask, Gate: gate, Layer: 1, Command: "echo ok",
		ExitCode: code, Verdict: verdict, StartedAt: time.Unix(1700000000, 0).UTC(),
		Schema: receipts.ReceiptSchemaVersion, Status: "PASS", Reason: "ok",
	}
	if !pass {
		rec.Status, rec.Reason = "FAIL", "deterministic_failure"
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("ok")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	path, err := receipts.Write(dir, rec)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := receipts.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return stored.ID
}

func TestGateVerdictUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"gate", "verdict"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus gate verdict") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestGateVerdictPass(t *testing.T) {
	t.Chdir(t.TempDir())
	writeVerdictFixture(t, "demo/T1", "build", true)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"gate", "verdict", "demo/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	s := out.String()
	for _, want := range []string{"deterministic (receipts decide):", "build", "PASS", "semantic (evaluations", "verdict: PASS (ok)", "rates:"} {
		if !strings.Contains(s, want) {
			t.Fatalf("output must contain %q, got:\n%s", want, s)
		}
	}
}

func TestGateVerdictFailAndDispute(t *testing.T) {
	t.Chdir(t.TempDir())
	id := writeVerdictFixture(t, "demo/T1", "build", false)
	if _, err := guardians.Evaluate(".", "demo/T1", "regulus-reviewer", "PASS", "looks fine", []string{id}); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"gate", "verdict", "demo/T1"}); code != 1 {
		t.Fatalf("exit = %d, want 1, output:\n%s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "verdict: FAIL (deterministic_failure)") {
		t.Fatalf("receipts decide, got:\n%s", s)
	}
	if !strings.Contains(s, "DISPUTE") {
		t.Fatalf("dispute must be flagged, got:\n%s", s)
	}
	if !strings.Contains(s, "disagreement=1.00") {
		t.Fatalf("rates must measure disagreement, got:\n%s", s)
	}
}

func TestGateVerdictEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join(".regulus", "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"gate", "verdict", "nope/T9"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "no evidence") {
		t.Fatalf("stderr = %q", errb.String())
	}
}
