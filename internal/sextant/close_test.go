package sextant

import (
	"os"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

func closeChart(class, extra string) string {
	return `# Feature

Domain: go
Class: ` + class + `

## Contract

- Objective: x
- Scope: y
- Acceptance: the happy path
- Verification: g
- Artifacts: a
- Dependencies: none
` + extra + `
## Acceptance

### Scenario: the happy path

- Given a
- When b
- Then c

## Tasks

| ID | Spec | Work | Route | Depends | Status |
|----|------|------|-------|---------|--------|
| T1 | the happy path | build | @regulus-worker | - | doing |
| T2 | the happy path | verify | @regulus-verifier | T1 | todo |

## Log

- L1: authorized.
`
}

func writePassReceipt(t *testing.T, root, evidTask, gate string, at time.Time) string {
	t.Helper()
	dir := evidence.TaskDir(root, evidTask)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: evidTask, Gate: gate, Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: at, DurationMS: 3,
		GitHead: "abc", Schema: receipts.ReceiptSchemaVersion,
		Status: "PASS", Reason: "ok",
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("ok")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	path, err := receipts.Write(dir, rec)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCloseUnknownTask(t *testing.T) {
	root := t.TempDir()
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T9")
	if res.Closed {
		t.Fatal("unknown task must not close")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "T9") {
		t.Fatalf("unmet must name the task, got %v", res.Unmet)
	}
}

func TestCloseNoReceipts(t *testing.T) {
	root := t.TempDir()
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if res.Closed {
		t.Fatal("task without receipts must not close")
	}
	joined := strings.Join(res.Unmet, "\n")
	if !strings.Contains(joined, "no receipts") {
		t.Fatalf("unmet must demand receipts, got %v", res.Unmet)
	}
}

func TestCloseSuccessMutatesDoc(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassReceipt(t, root, "feat/T1", "build", at)
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if !res.Closed {
		t.Fatalf("proven task must close, got %v", res.Unmet)
	}
	if len(res.Receipts) != 1 || res.Verdict != "PASS" {
		t.Fatalf("result must cite evidence, got %+v", res)
	}
	updated := ApplyClose(closeChart("STANDARD", ""), "T1", res)
	if !strings.Contains(updated, "CLOSED T1") {
		t.Fatalf("close must record in Log, got:\n%s", updated)
	}
	tasks, _ := ParseTasks(updated)
	found := false
	for _, task := range tasks {
		if task.ID == "T1" {
			found = true
			if task.Status != StatusDone {
				t.Fatalf("close must flip status to done, got %q", task.Status)
			}
		}
	}
	if !found {
		t.Fatal("task row must survive close")
	}
	// Idempotent: closing again stays closed.
	res2 := CheckClose(updated, root, "feat", "T1")
	if !res2.Closed {
		t.Fatalf("re-close must stay closed, got %v", res2.Unmet)
	}
}

func TestCloseRefusesFailingReceipt(t *testing.T) {
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
	if res.Closed {
		t.Fatal("failing gate must block close")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "build") {
		t.Fatalf("unmet must name the failing gate, got %v", res.Unmet)
	}
}

func TestCloseRefusesTamperedReceipt(t *testing.T) {
	root := t.TempDir()
	path := writePassReceipt(t, root, "feat/T1", "build", time.Unix(1700000000, 0).UTC())
	raw, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(raw), `"stdout": "ok"`, `"stdout": "EVIL"`, 1)), 0o644)
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if res.Closed {
		t.Fatal("tampered evidence must block close")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "verify") {
		t.Fatalf("unmet must name verification, got %v", res.Unmet)
	}
}

func TestCloseCriticalRequiresVerifier(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassReceipt(t, root, "feat/T1", "build", at)
	doc := closeChart("CRITICAL", "- Constraints: none\n- Verifier: T2\n- Completion: scenarios PROVEN\n")
	res := CheckClose(doc, root, "feat", "T1")
	if res.Closed {
		t.Fatal("critical task without verifier evidence must not close")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "T2") {
		t.Fatalf("unmet must name the verifier task, got %v", res.Unmet)
	}
	// The verifier gates by evidence: once feat/T2 receipts pass, T1 may
	// close; T2 itself closes afterwards under the normal rules.
	writePassReceipt(t, root, "feat/T2", "verify", at.Add(time.Second))
	vd := ApplyClose(doc, "T1", CheckClose(doc, root, "feat", "T1"))
	if !CheckClose(vd, root, "feat", "T1").Closed {
		t.Fatalf("critical task with passing verifier evidence must close, got %v", CheckClose(vd, root, "feat", "T1").Unmet)
	}
	if !CheckClose(vd, root, "feat", "T2").Closed {
		t.Fatalf("verifier task closes under normal rules, got %v", CheckClose(vd, root, "feat", "T2").Unmet)
	}
}

func TestCloseRequiresSpecAndContract(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassReceipt(t, root, "feat/T1", "build", at)
	// Broken acceptance (missing Then) blocks close even with receipts.
	broken := strings.Replace(closeChart("STANDARD", ""), "- Then c", "- And c", 1)
	res := CheckClose(broken, root, "feat", "T1")
	if res.Closed {
		t.Fatal("broken spec must block close")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "spec") {
		t.Fatalf("unmet must name the spec, got %v", res.Unmet)
	}
	// STANDARD without any contract blocks close: no structured acceptance.
	noContract := "# F\n\nClass: STANDARD\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 | s | w | inline |\n\n## Log\n\n- L1.\n"
	res = CheckClose(noContract, root, "feat", "T1")
	if res.Closed {
		t.Fatal("missing contract must block close for STANDARD")
	}
}

func TestCloseTrivialLight(t *testing.T) {
	root := t.TempDir()
	doc := "# F\n\nClass: TRIVIAL\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 |  | answer the question | inline |\n\n## Log\n\n- L1.\n"
	res := CheckClose(doc, root, "feat", "T1")
	if !res.Closed {
		t.Fatalf("trivial work closes on stated acceptance alone, got %v", res.Unmet)
	}
}
