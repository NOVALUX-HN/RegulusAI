package sextant

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/manifest"
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

// writeManifestRun writes one manifest-backed run: receipts carrying
// RunID/Manifest plus the manifest linking them (hash-verified). It is the
// only green fixture: legacy receipts without a manifest never close.
func writeManifestRun(t *testing.T, root, evidTask, runID string, at time.Time, gates []string, verdict string, exitCode int) (string, []string) {
	t.Helper()
	dir := evidence.TaskDir(root, evidTask)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mfile := manifest.FileName(runID)
	status, reason := "PASS", "ok"
	if verdict != receipts.VerdictPass {
		status, reason = "FAIL", "deterministic_failure"
	}
	var paths []string
	for i, gate := range gates {
		rec := receipts.Receipt{
			Task: evidTask, Gate: gate, Layer: 1, Command: "echo ok",
			ExitCode: exitCode, Verdict: verdict, StartedAt: at.Add(time.Duration(i) * time.Second), DurationMS: 3,
			GitHead: "abc", Schema: receipts.ReceiptSchemaVersion,
			Status: status, Reason: reason,
			RunID: runID, Manifest: mfile,
		}
		rec.Stdout, rec.StdoutSHA = receipts.Capture("ok-" + gate)
		rec.Stderr, rec.StderrSHA = receipts.Capture("")
		path, err := receipts.Write(dir, rec)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	var refs []manifest.ArtifactRef
	for i, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		back, err := receipts.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, manifest.ArtifactRef{
			ReceiptID: back.ID, File: filepath.Base(p),
			SHA256: hex.EncodeToString(sum[:]),
			Gate:   gates[i], Status: status, Reason: reason,
		})
	}
	m := manifest.Manifest{
		Schema: manifest.SchemaVersion, RunID: runID,
		Task: evidTask, CreatedAt: at, Artifacts: refs,
	}
	mpath, err := manifest.Write(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	return mpath, paths
}

func writePassRun(t *testing.T, root, evidTask, runID string, at time.Time, gates ...string) (string, []string) {
	t.Helper()
	return writeManifestRun(t, root, evidTask, runID, at, gates, receipts.VerdictPass, 0)
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
	_, want := writePassRun(t, root, "feat/T1", "run_closepass000001", at, "build")
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if !res.Closed {
		t.Fatalf("proven task must close, got %v", res.Unmet)
	}
	if len(res.Receipts) != 1 || res.Verdict != "PASS" {
		t.Fatalf("result must cite evidence, got %+v", res)
	}
	if res.RunID != "run_closepass000001" {
		t.Fatalf("result must cite run id, got %+v", res)
	}
	if res.Receipts[0] != want[0] {
		t.Fatalf("receipts must be the latest manifest set, got %+v want %+v", res.Receipts, want)
	}
	updated := ApplyClose(closeChart("STANDARD", ""), "T1", res)
	if !strings.Contains(updated, "CLOSED T1") {
		t.Fatalf("close must record in Log, got:\n%s", updated)
	}
	if !strings.Contains(updated, "run_closepass000001") {
		t.Fatalf("close record must cite the run id, got:\n%s", updated)
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
	at := time.Unix(1700000000, 0).UTC()
	writeManifestRun(t, root, "feat/T1", "run_closefail000001", at, []string{"build"}, receipts.VerdictFail, 1)
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
	at := time.Unix(1700000000, 0).UTC()
	_, paths := writePassRun(t, root, "feat/T1", "run_closetamper001", at, "build")
	raw, _ := os.ReadFile(paths[0])
	os.WriteFile(paths[0], []byte(strings.Replace(string(raw), `"stdout": "ok-build"`, `"stdout": "EVIL"`, 1)), 0o644)
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
	writePassRun(t, root, "feat/T1", "run_closecrit000001", at, "build")
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
	writePassRun(t, root, "feat/T2", "run_closecrit000002", at.Add(time.Second), "verify")
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
	writePassRun(t, root, "feat/T1", "run_closespec00001", at, "build")
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
