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

func manifestSum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestCloseLegacyWithoutManifestNeverCloses(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassReceipt(t, root, "feat/T1", "build", at)
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if res.Closed {
		t.Fatal("legacy receipt without manifest must never close")
	}
	joined := strings.Join(res.Unmet, "\n")
	if !strings.Contains(joined, "no manifest") && !strings.Contains(joined, "no receipts") {
		t.Fatalf("unmet must demand a manifest, got %v", res.Unmet)
	}
}

func TestCloseLatestIgnoresHistoricFail(t *testing.T) {
	root := t.TempDir()
	oldAt := time.Unix(1700000000, 0).UTC()
	newAt := oldAt.Add(time.Hour)
	writeManifestRun(t, root, "feat/T1", "run_histfail00001", oldAt, []string{"build"}, receipts.VerdictFail, 1)
	_, want := writePassRun(t, root, "feat/T1", "run_histpass00002", newAt, "build")
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if !res.Closed {
		t.Fatalf("latest green manifest must close despite historic FAIL, got %v", res.Unmet)
	}
	if res.RunID != "run_histpass00002" {
		t.Fatalf("must cite latest run, got %q", res.RunID)
	}
	if len(res.Receipts) != 1 || res.Receipts[0] != want[0] {
		t.Fatalf("receipts must be only the latest manifest set, got %+v want %+v", res.Receipts, want)
	}
	// Append-only: the historic FAIL receipt stays on disk and visible.
	entries, err := evidence.List(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	foundFail := false
	for _, e := range entries {
		if e.Receipt.Verdict != "pass" {
			foundFail = true
		}
	}
	if !foundFail {
		t.Fatal("historic FAIL receipt must remain on disk and visible in evidence list")
	}
	updated := ApplyClose(closeChart("STANDARD", ""), "T1", res)
	if !strings.Contains(updated, "run_histpass00002") {
		t.Fatalf("close record must cite the latest run id, got:\n%s", updated)
	}
}

func TestCloseLatestFailBlocksDespiteHistoricPass(t *testing.T) {
	root := t.TempDir()
	oldAt := time.Unix(1700000000, 0).UTC()
	newAt := oldAt.Add(time.Hour)
	writePassRun(t, root, "feat/T1", "run_histpass00003", oldAt, "build")
	writeManifestRun(t, root, "feat/T1", "run_histfail00004", newAt, []string{"build"}, receipts.VerdictFail, 1)
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if res.Closed {
		t.Fatal("latest failing manifest must block even with historic PASS")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "failing gates: build") {
		t.Fatalf("unmet must name the latest failing gate, got %v", res.Unmet)
	}
	if res.RunID != "run_histfail00004" {
		t.Fatalf("must cite latest run even when blocked, got %q", res.RunID)
	}
}

func TestCloseHistoricTamperIgnored(t *testing.T) {
	root := t.TempDir()
	oldAt := time.Unix(1700000000, 0).UTC()
	newAt := oldAt.Add(time.Hour)
	_, oldPaths := writePassRun(t, root, "feat/T1", "run_histold00005", oldAt, "build")
	raw, _ := os.ReadFile(oldPaths[0])
	os.WriteFile(oldPaths[0], []byte(strings.Replace(string(raw), `"stdout": "ok-build"`, `"stdout": "EVIL"`, 1)), 0o644)
	writePassRun(t, root, "feat/T1", "run_histnew00006", newAt, "build")
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if !res.Closed {
		t.Fatalf("historic tamper outside the latest manifest must be ignored, got %v", res.Unmet)
	}
}

func TestCloseEmptyManifestBlocks(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	dir := evidence.TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := manifest.Manifest{Schema: manifest.SchemaVersion, RunID: "run_empty0000001", Task: "feat/T1", CreatedAt: at}
	if _, err := manifest.Write(dir, m); err != nil {
		t.Fatal(err)
	}
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if res.Closed {
		t.Fatal("empty manifest must fail closed")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "empty manifest") {
		t.Fatalf("unmet must name empty manifest, got %v", res.Unmet)
	}
}

func TestCloseSkippedBlocks(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	_, paths := writePassRun(t, root, "feat/T1", "run_skip00000001", at, "build")
	// Rewrite the latest manifest with a Skipped entry (partial run never proves).
	dir := evidence.TaskDir(root, "feat/T1")
	rec, err := receipts.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	sum := manifestSum(data)
	m := manifest.Manifest{
		Schema: manifest.SchemaVersion, RunID: "run_skip00000001",
		Task: "feat/T1", CreatedAt: at.Add(time.Minute),
		Artifacts: []manifest.ArtifactRef{{ReceiptID: rec.ID, File: filepath.Base(paths[0]), SHA256: sum, Gate: "build", Status: "PASS", Reason: "ok"}},
		Skipped:   []string{"coverage-floor"},
	}
	if _, err := manifest.Write(dir, m); err != nil {
		t.Fatal(err)
	}
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if res.Closed {
		t.Fatal("manifest with skipped gates must fail closed")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "skipped") {
		t.Fatalf("unmet must name skipped gates, got %v", res.Unmet)
	}
}

func TestCloseMalformedCandidateBlocksWithoutFallback(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassRun(t, root, "feat/T1", "run_good00000001", at, "build")
	dir := evidence.TaskDir(root, "feat/T1")
	bad := filepath.Join(dir, "manifest-run_bad00000001.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if res.Closed {
		t.Fatal("malformed manifest candidate must block without silent fallback")
	}
	if !strings.Contains(strings.Join(res.Unmet, "\n"), "manifest_malformed") {
		t.Fatalf("unmet must name manifest_malformed, got %v", res.Unmet)
	}
}

func TestCloseTieBreakLexicographic(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writeManifestRun(t, root, "feat/T1", "run_aaa000000001", at, []string{"build"}, receipts.VerdictFail, 1)
	writePassRun(t, root, "feat/T1", "run_zzz000000001", at, "build")
	res := CheckClose(closeChart("STANDARD", ""), root, "feat", "T1")
	if !res.Closed {
		t.Fatalf("lexicographically larger FileName(RunID) wins ties, got %v", res.Unmet)
	}
	if res.RunID != "run_zzz000000001" {
		t.Fatalf("tie-break must pick run_zzz, got %q", res.RunID)
	}
}

func TestCloseVerifierLatestOnly(t *testing.T) {
	root := t.TempDir()
	at := time.Unix(1700000000, 0).UTC()
	writePassRun(t, root, "feat/T1", "run_vermain00001", at, "build")
	doc := closeChart("CRITICAL", "- Constraints: none\n- Verifier: T2\n- Completion: scenarios PROVEN\n")
	writeManifestRun(t, root, "feat/T2", "run_verold0000001", at, []string{"verify"}, receipts.VerdictFail, 1)
	writePassRun(t, root, "feat/T2", "run_vernew0000001", at.Add(time.Hour), "verify")
	res := CheckClose(doc, root, "feat", "T1")
	if !res.Closed {
		t.Fatalf("verifier historic FAIL must be ignored when its latest manifest passes, got %v", res.Unmet)
	}
}
