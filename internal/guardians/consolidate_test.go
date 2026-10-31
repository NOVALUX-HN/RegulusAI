package guardians

import (
	"os"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/manifest"
	"regulus-ai/internal/receipts"
)

func writeVerdictReceipt(t *testing.T, root, evidTask, gate, status, verdict string, code int) string {
	return writeVerdictReceiptLinked(t, root, evidTask, gate, status, verdict, code, "")
}

func writeVerdictReceiptLinked(t *testing.T, root, evidTask, gate, status, verdict string, code int, manifestFile string) string {
	t.Helper()
	dir := evidence.TaskDir(root, evidTask)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: evidTask, Gate: gate, Layer: 1, Command: "echo ok",
		ExitCode: code, Verdict: verdict, StartedAt: time.Unix(1700000000, 0).UTC(),
		GitHead: "abc", Schema: receipts.ReceiptSchemaVersion,
		Status: status, Reason: "ok", Manifest: manifestFile,
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

func linkManifest(t *testing.T, root, evidTask, runID string, refs ...manifest.ArtifactRef) {
	t.Helper()
	m := manifest.Manifest{Schema: manifest.SchemaVersion, RunID: runID, Task: evidTask, Artifacts: refs}
	if _, err := manifest.Write(evidence.TaskDir(root, evidTask), m); err != nil {
		t.Fatal(err)
	}
}

func TestConsolidateAllPass(t *testing.T) {
	root := t.TempDir()
	id := writeVerdictReceiptLinked(t, root, "feat/T1", "build", "PASS", receipts.VerdictPass, 0, "manifest-run_x.json")
	linkManifest(t, root, "feat/T1", "run_x", manifest.ArtifactRef{
		ReceiptID: id, File: "build.json", SHA256: "zz", Gate: "build", Status: "PASS", Reason: "ok",
	})
	con, err := Consolidate(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if con.Verdict != "PASS" || con.Reason != "ok" {
		t.Fatalf("want PASS/ok, got %+v", con)
	}
	if len(con.Gates) != 1 || con.Gates[0].Status != "PASS" {
		t.Fatalf("per-gate verdicts must list receipts, got %+v", con.Gates)
	}
}

func TestConsolidateFailWins(t *testing.T) {
	root := t.TempDir()
	writeVerdictReceipt(t, root, "feat/T1", "build", "PASS", receipts.VerdictPass, 0)
	writeVerdictReceipt(t, root, "feat/T1", "lint", "FAIL", receipts.VerdictFail, 1)
	writeVerdictReceipt(t, root, "feat/T1", "slow", "BLOCKED", receipts.VerdictFail, -1)
	con, err := Consolidate(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if con.Verdict != "FAIL" {
		t.Fatalf("deterministic failure beats blocked, got %s", con.Verdict)
	}
}

func TestConsolidateBlocked(t *testing.T) {
	root := t.TempDir()
	writeVerdictReceipt(t, root, "feat/T1", "slow", "BLOCKED", receipts.VerdictFail, -1)
	con, err := Consolidate(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if con.Verdict != "BLOCKED" {
		t.Fatalf("want BLOCKED, got %s", con.Verdict)
	}
}

func TestConsolidateLegacyReceipts(t *testing.T) {
	root := t.TempDir()
	dir := evidence.TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: "feat/T1", Gate: "build", Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: time.Unix(1700000000, 0).UTC(),
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("ok")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	if _, err := receipts.Write(dir, rec); err != nil {
		t.Fatal(err)
	}
	con, err := Consolidate(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if con.Verdict != "PASS" || con.Legacy != 1 || con.EvidenceLinked != 0 {
		t.Fatalf("legacy receipts derive from pass/fail, got %+v", con)
	}
}

func TestConsolidateEmptyTask(t *testing.T) {
	root := t.TempDir()
	if _, err := Consolidate(root, "nope/T9"); err == nil {
		t.Fatal("task without evidence must fail consolidation")
	}
}

func TestConsolidateAgreementAndDispute(t *testing.T) {
	root := t.TempDir()
	id := writeVerdictReceipt(t, root, "feat/T1", "build", "FAIL", receipts.VerdictFail, 1)
	if _, err := Evaluate(root, "feat/T1", "regulus-evidence-auditor", "FAIL", "gate failed as specified", []string{id}); err != nil {
		t.Fatal(err)
	}
	if _, err := Evaluate(root, "feat/T1", "regulus-reviewer", "PASS", "looks fine to me", []string{id}); err != nil {
		t.Fatal(err)
	}
	con, err := Consolidate(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if con.Verdict != "FAIL" {
		t.Fatalf("evaluations must never override receipts, got %s", con.Verdict)
	}
	if con.Agree != 1 || con.Dispute != 1 {
		t.Fatalf("want 1 agree + 1 dispute, got %+v", con)
	}
	disputed := false
	for _, e := range con.Evaluations {
		if e.Guardian == "regulus-reviewer" && e.Disputed {
			disputed = true
		}
		if e.Guardian == "regulus-evidence-auditor" && e.Disputed {
			t.Fatalf("agreeing evaluation must not dispute: %+v", e)
		}
	}
	if !disputed {
		t.Fatalf("dispute must be flagged, got %+v", con.Evaluations)
	}
	if con.DisagreementRate != 0.5 {
		t.Fatalf("disagreement rate must be 1/2, got %v", con.DisagreementRate)
	}
}

func TestConsolidateInconclusive(t *testing.T) {
	root := t.TempDir()
	id := writeVerdictReceipt(t, root, "feat/T1", "probe", "INCONCLUSIVE", receipts.VerdictFail, -1)
	if _, err := Evaluate(root, "feat/T1", "regulus-gate-verifier", "INCONCLUSIVE", "no receipts yet", []string{id}); err != nil {
		t.Fatal(err)
	}
	con, err := Consolidate(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if con.Verdict != "INCONCLUSIVE" {
		t.Fatalf("want INCONCLUSIVE, got %s", con.Verdict)
	}
	if con.GatesInconclusive != 1 || con.EvalsInconclusive != 1 {
		t.Fatalf("inconclusive must be counted both sides, got %+v", con)
	}
}

func TestConsolidateCompleteness(t *testing.T) {
	root := t.TempDir()
	writeVerdictReceiptLinked(t, root, "feat/T1", "a", "PASS", receipts.VerdictPass, 0, "manifest-run_x.json")
	writeVerdictReceipt(t, root, "feat/T1", "b", "PASS", receipts.VerdictPass, 0)
	linkManifest(t, root, "feat/T1", "run_x")
	con, err := Consolidate(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if con.EvidenceTotal != 2 || con.EvidenceLinked != 1 || con.EvidenceCompleteness != 0.5 {
		t.Fatalf("one of two linked scores 0.5, got %+v", con)
	}
}
