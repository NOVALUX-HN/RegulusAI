package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/manifest"
	"regulus-ai/internal/receipts"
)

func writeReceipt(t *testing.T, dir, gate string, at time.Time, mutate func(*receipts.Receipt)) string {
	t.Helper()
	rec := receipts.Receipt{
		Task: "t", Gate: gate, Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: at, DurationMS: 3,
		GitHead: "abc",
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("out-" + gate)
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	if mutate != nil {
		mutate(&rec)
	}
	path, err := receipts.Write(dir, rec)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func writeManifestFor(t *testing.T, dir string, refs []manifest.ArtifactRef) string {
	t.Helper()
	m := manifest.Manifest{
		Schema:    manifest.SchemaVersion,
		RunID:     "run_verifytest0001",
		Task:      "t",
		Artifacts: refs,
	}
	path, err := manifest.Write(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func fileSHA(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestVerifyCleanLegacySet(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, dir, "a", t0, nil)
	writeReceipt(t, dir, "b", t0.Add(time.Second), nil)
	rep, err := Verify(root, "feat/T1", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked != 2 || rep.Passed != 2 || rep.Legacy != 2 {
		t.Fatalf("want 2/2/legacy2, got %+v", rep)
	}
	if len(rep.Failed) != 0 || len(rep.Quarantined) != 0 {
		t.Fatalf("clean set must not fail: %+v", rep)
	}
}

func TestVerifyDetectsStdoutTamperAndQuarantines(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeReceipt(t, dir, "a", t0, nil)
	// Tamper: rewrite the stored stdout without updating its hash.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), "out-a", "out-EVIL", 1)
	if tampered == string(raw) {
		t.Fatal("fixture must contain the planted output")
	}
	if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Verify(root, "feat/T1", true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed != 0 || len(rep.Failed) != 1 {
		t.Fatalf("tamper must fail exactly one receipt: %+v", rep)
	}
	if rep.Failed[0].Reason != "stdout_hash_mismatch" {
		t.Fatalf("want stdout_hash_mismatch, got %+v", rep.Failed[0])
	}
	if len(rep.Quarantined) != 1 {
		t.Fatalf("tampered receipt must be quarantined: %+v", rep)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("quarantined file must be moved out of the evidence dir")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() == "corrupt" && !e.IsDir() {
			t.Fatal("corrupt must be a directory")
		}
	}
	// The quarantined file survives under corrupt/ (recovery, never delete).
	qpath := filepath.Join(dir, "corrupt", filepath.Base(path))
	if _, err := os.Stat(qpath); err != nil {
		t.Fatalf("quarantined file must be preserved: %v", err)
	}
}

func TestVerifyMalformedReceipt(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "broken-1.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Verify(root, "feat/T1", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Failed) != 1 || rep.Failed[0].Reason != "malformed" {
		t.Fatalf("want one malformed failure: %+v", rep)
	}
	if len(rep.Quarantined) != 1 {
		t.Fatalf("malformed file must be quarantined: %+v", rep)
	}
}

func TestVerifyManifestCrossCheck(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeReceipt(t, dir, "a", t0, func(r *receipts.Receipt) {
		r.Schema = receipts.ReceiptSchemaVersion
		r.RunID = "run_verifytest0001"
		r.Status = "PASS"
		r.Reason = "ok"
		r.Manifest = "manifest-run_verifytest0001.json"
	})
	writeManifestFor(t, dir, []manifest.ArtifactRef{{
		ReceiptID: mustReadID(t, path), File: filepath.Base(path),
		SHA256: fileSHA(t, path), Gate: "a", Status: "PASS", Reason: "ok",
	}})
	rep, err := Verify(root, "feat/T1", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed != 1 || len(rep.Failed) != 0 || rep.Manifests != 1 {
		t.Fatalf("manifest-linked receipt must verify: %+v", rep)
	}
}

func TestVerifyManifestMissing(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, dir, "a", t0, func(r *receipts.Receipt) {
		r.Schema = receipts.ReceiptSchemaVersion
		r.Status = "PASS"
		r.Reason = "ok"
		r.Manifest = "manifest-run_absent.json"
	})
	rep, err := Verify(root, "feat/T1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Failed) != 1 || rep.Failed[0].Reason != "manifest_missing" {
		t.Fatalf("want manifest_missing: %+v", rep)
	}
}

func TestVerifyManifestUnlisted(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, dir, "a", t0, func(r *receipts.Receipt) {
		r.Schema = receipts.ReceiptSchemaVersion
		r.Status = "PASS"
		r.Reason = "ok"
		r.Manifest = "manifest-run_verifytest0001.json"
	})
	// Manifest exists but does not list the receipt.
	writeManifestFor(t, dir, nil)
	rep, err := Verify(root, "feat/T1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Failed) != 1 || rep.Failed[0].Reason != "manifest_unlisted" {
		t.Fatalf("want manifest_unlisted: %+v", rep)
	}
}

func TestVerifyUnsupportedSchemaNotQuarantined(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeReceipt(t, dir, "a", t0, func(r *receipts.Receipt) {
		r.Schema = receipts.ReceiptSchemaVersion + 100
	})
	rep, err := Verify(root, "feat/T1", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Failed) != 1 || rep.Failed[0].Reason != "unsupported_schema" {
		t.Fatalf("want unsupported_schema: %+v", rep)
	}
	if len(rep.Quarantined) != 0 {
		t.Fatal("future-schema receipts must never be quarantined (no data loss)")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("future-schema receipt must stay in place")
	}
}

func TestVerifyEmptyAndMissingTask(t *testing.T) {
	root := t.TempDir()
	rep, err := Verify(root, "nope/T9", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked != 0 || len(rep.Failed) != 0 {
		t.Fatalf("empty task must verify clean: %+v", rep)
	}
}

func mustReadID(t *testing.T, path string) string {
	t.Helper()
	rec, err := receipts.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return rec.ID
}
