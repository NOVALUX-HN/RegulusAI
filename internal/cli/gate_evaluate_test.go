package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

func writeEvalReceipt(t *testing.T, evidTask string) string {
	t.Helper()
	dir := evidence.TaskDir(".", evidTask)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: evidTask, Gate: "build", Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: time.Unix(1700000000, 0).UTC(),
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

func TestGateEvaluateUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"gate", "evaluate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus gate evaluate") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestGateEvaluateRecords(t *testing.T) {
	t.Chdir(t.TempDir())
	id := writeEvalReceipt(t, "demo/T1")
	c, out, _ := newCLI(nil)
	code := c.Run([]string{"gate", "evaluate", "--task", "demo/T1",
		"--guardian", "regulus-evidence-auditor", "--verdict", "PASS",
		"--note", "every claim cites a receipt", "--evidence", id})
	if code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "recorded: regulus-evidence-auditor judges PASS") {
		t.Fatalf("output = %q", out.String())
	}
	entries, err := os.ReadDir(filepath.Join(".regulus", "evidence", "demo__T1"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Name() == "evaluations.jsonl" {
			found = true
		}
	}
	if !found {
		t.Fatalf("evaluations log must exist, dir: %v", entries)
	}
}

func TestGateEvaluateRejects(t *testing.T) {
	t.Chdir(t.TempDir())
	writeEvalReceipt(t, "demo/T1")
	cases := [][]string{
		{"--task", "demo/T1", "--guardian", "regulus-worker", "--verdict", "PASS", "--note", "x"},
		{"--task", "demo/T1", "--guardian", "regulus-ghost", "--verdict", "PASS", "--note", "x"},
		{"--task", "demo/T1", "--guardian", "regulus-gate-verifier", "--verdict", "PASS", "--note", "x", "--evidence", "rec_nope"},
		{"--task", "demo/T1", "--guardian", "regulus-gate-verifier", "--verdict", "MAYBE", "--note", "x"},
	}
	for _, a := range cases {
		c, _, errb := newCLI(nil)
		if code := c.Run(append([]string{"gate", "evaluate"}, a...)); code != 1 {
			t.Fatalf("args %v must exit 1, stderr %q", a, errb.String())
		}
	}
}
