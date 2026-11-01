package sextant

import (
	"os"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

func joinUnmet(res CloseResult) string {
	return strings.Join(res.Unmet, "\n")
}

func mustContain(t *testing.T, res CloseResult, want string) {
	t.Helper()
	if res.Closed {
		t.Fatalf("expected open result, got closed with receipts %v", res.Receipts)
	}
	if !strings.Contains(joinUnmet(res), want) {
		t.Fatalf("unmet must contain %q, got %v", want, res.Unmet)
	}
}

func TestSplitContractMissing(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassReceipt(t, root, "feat/T1", "build", at)
	doc := "# F\n\nClass: STANDARD\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 | s | w | inline |\n\n## Log\n\n- L1.\n"
	res := CheckClose(doc, root, "feat", "T1")
	mustContain(t, res, "no structured ## Contract")
}

func TestSplitSpecBroken(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassReceipt(t, root, "feat/T1", "build", at)
	broken := strings.Replace(closeChart("STANDARD", ""), "- Then c", "- And c", 1)
	res := CheckClose(broken, root, "feat", "T1")
	mustContain(t, res, "spec")
}

func TestSplitDepsBlocked(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	// T2 depends on T1 (see closeChart); T1 stays open so only deps blocks T2.
	writePassReceipt(t, root, "feat/T2", "verify", at)
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T2")
	mustContain(t, res, "dependency T1 is not closed")
}

func TestSplitEvidenceNone(t *testing.T) {
	root := t.TempDir()
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	mustContain(t, res, "no receipts for task")
}

func TestSplitEvidenceFailingGate(t *testing.T) {
	root := t.TempDir()
	dir := evidence.TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: "feat/T1", Gate: "build", Layer: 1, Command: "echo ok",
		ExitCode: 1, Verdict: receipts.VerdictFail, StartedAt: time.Unix(1700000000, 0).UTC(),
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("boom")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	if _, err := receipts.Write(dir, rec); err != nil {
		t.Fatal(err)
	}
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	mustContain(t, res, "failing gates: build (exit 1)")
}

func TestSplitEvidenceTampered(t *testing.T) {
	root := t.TempDir()
	path := writePassReceipt(t, root, "feat/T1", "build", time.Unix(1700000000, 0).UTC())
	raw, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(raw), `"stdout": "ok"`, `"stdout": "EVIL"`, 1)), 0o644)
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	mustContain(t, res, "evidence verify:")
}

func TestSplitVerifierMissing(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassReceipt(t, root, "feat/T1", "build", at)
	doc := closeChart("CRITICAL", "- Constraints: none\n- Verifier: T2\n- Completion: scenarios PROVEN\n")
	res := CheckClose(doc, root, "feat", "T1")
	mustContain(t, res, "verifier task T2 has no receipts")
}

func TestSplitVerifierFailingGate(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassReceipt(t, root, "feat/T1", "build", at)
	dir := evidence.TaskDir(root, "feat/T2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: "feat/T2", Gate: "verify", Layer: 1, Command: "echo ok",
		ExitCode: 1, Verdict: receipts.VerdictFail, StartedAt: at,
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("boom")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	if _, err := receipts.Write(dir, rec); err != nil {
		t.Fatal(err)
	}
	doc := closeChart("CRITICAL", "- Constraints: none\n- Verifier: T2\n- Completion: scenarios PROVEN\n")
	res := CheckClose(doc, root, "feat", "T1")
	mustContain(t, res, "verifier gate verify did not pass")
}

func TestSplitCleanClose(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassReceipt(t, root, "feat/T1", "build", at)
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if !res.Closed || res.Verdict != "PASS" {
		t.Fatalf("proven task must close PASS, got %+v", res)
	}
	if len(res.Receipts) != 1 {
		t.Fatalf("clean close must cite 1 receipt, got %+v", res)
	}
}

func TestSplitTrivialEarlyReturn(t *testing.T) {
	root := t.TempDir()
	doc := "# F\n\nClass: TRIVIAL\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 |  | answer the question | inline |\n\n## Log\n\n- L1.\n"
	res := CheckClose(doc, root, "feat", "T1")
	if !res.Closed || res.Verdict != "PASS" {
		t.Fatalf("trivial must close early without receipts, got %+v", res)
	}
	if len(res.Unmet) != 0 {
		t.Fatalf("trivial early-return must leave no unmet, got %v", res.Unmet)
	}
}
