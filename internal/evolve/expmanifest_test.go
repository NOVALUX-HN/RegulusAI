package evolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePinned(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

func testManifest(t *testing.T, root string) ExperimentManifest {
	t.Helper()
	candPath := filepath.Join(root, "candidate", "manifest-run_x.json")
	candSHA := writePinned(t, candPath, `{"run_id":"run_x"}`)
	splitPath := filepath.Join(root, "bench", "f7-splits.json")
	splitSHA := writePinned(t, splitPath, `{"name":"f7-split-v1","version":1}`)
	cfgPath := filepath.Join(root, "evals", "config.json")
	cfgSHA := writePinned(t, cfgPath, `{"evaluator":"f7-eval"}`)
	return ExperimentManifest{
		Schema: ExperimentSchemaAlias(), Protocol: F7ProtocolVersion,
		ExperimentID: "exp_f7test",
		Candidate:    ManifestCandidateRef{ManifestPath: candPath, RunID: "run_x", SHA256: candSHA},
		Baseline:     "macizo-bench-v3",
		Suite:        "macizo", SuiteVersion: "macizo-bench-v3",
		Split:     ManifestSplitRef{File: splitPath, Name: "f7-split-v1", Version: 1, SHA256: splitSHA},
		Seed:      20261114,
		ConfigRef: cfgPath, ConfigSHA256: cfgSHA,
		Budget:    ManifestBudget{MaxWallTimeMS: 60000, MaxTokens: 1000, MaxCandidates: 4, MaxConcurrency: 2, MaxRetries: 1, MaxDiffBytes: 4096},
		Evaluator: "f7-eval", EvaluatorVer: "v1",
		Results: []ManifestResult{
			{TaskID: "B1", Status: ManifestPass, ReceiptID: "rec_1", DetailSHA256: sha256HexBytes([]byte("ok"))},
			{TaskID: "B2", Status: ManifestFail, ReceiptID: "rec_2", DetailSHA256: sha256HexBytes([]byte("no"))},
			{TaskID: "B3", Status: ManifestTimeout},
		},
		Decisions: []ManifestDecision{{Verdict: "REJECT", By: "ana", At: "2026-11-14T00:00:00Z"}},
		Notes:     "exploration over B1-B3",
	}
}

// ExperimentSchemaAlias keeps the test decoupled from the const name drift.
func ExperimentSchemaAlias() int { return ExpManifestSchema }

// Manifest round-trips through the store and verifies: reconstructing from
// the manifest (Load + Verify) must hold while every referenced file is
// intact.
func TestExpManifestRoundTripReconstructs(t *testing.T) {
	root := t.TempDir()
	m := testManifest(t, root)
	path, err := WriteExperimentManifest(root, m)
	if err != nil {
		t.Fatalf("write must hold: %v", err)
	}
	if !strings.HasSuffix(path, "exp_f7test.json") {
		t.Fatalf("manifest path must cite the experiment, got %q", path)
	}
	loaded, err := LoadExperimentManifest(root, "exp_f7test")
	if err != nil {
		t.Fatalf("load must hold: %v", err)
	}
	if loaded.ExperimentID != m.ExperimentID || loaded.Seed != m.Seed || len(loaded.Results) != 3 {
		t.Fatalf("reconstructed manifest differs: %+v", loaded)
	}
	if err := VerifyExperimentManifest(root, loaded); err != nil {
		t.Fatalf("verify must hold on intact refs: %v", err)
	}
	pass, fail, infra := SummarizeManifest(loaded)
	if pass != 1 || fail != 1 || infra != 1 {
		t.Fatalf("metrics must stay separated (pass/fail/infra), got %d/%d/%d", pass, fail, infra)
	}
	idx, err := ListManifests(root)
	if err != nil || len(idx) != 1 {
		t.Fatalf("compact index must list one row, got %+v (%v)", idx, err)
	}
	if idx[0].ExperimentID != "exp_f7test" || idx[0].Results != 3 || idx[0].Pass != 1 || idx[0].Infra != 1 {
		t.Fatalf("index row must carry counts, got %+v", idx[0])
	}
	refs := RetentionRefs(loaded)
	if len(refs) < 4 {
		t.Fatalf("retention refs must name candidate+split+config+receipts, got %v", refs)
	}
	for _, r := range refs {
		if strings.Contains(r, "ok") || strings.Contains(r, "run_x\"") {
			t.Fatalf("refs must be pointers, never bodies: %v", refs)
		}
	}
}

// Drift in any referenced file breaks verification with the file named.
func TestExpManifestBlocksDrift(t *testing.T) {
	root := t.TempDir()
	m := testManifest(t, root)
	if _, err := WriteExperimentManifest(root, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadExperimentManifest(root, "exp_f7test")
	if err != nil {
		t.Fatal(err)
	}
	// Mutate the split after pinning: drift must fail.
	splitPath := loaded.Split.File
	f, err := os.OpenFile(splitPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(" "); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := VerifyExperimentManifest(root, loaded); err == nil {
		t.Fatal("split drift must fail verification")
	} else if !strings.Contains(err.Error(), "split") {
		t.Fatalf("drift error must name the split ref, got %v", err)
	}
}

// A manifest declaring a wrong hash is refused at write; a manifest whose
// reference vanished fails verify as missing (never as pass).
func TestExpManifestBlocksBrokenHashAndMissing(t *testing.T) {
	root := t.TempDir()
	m := testManifest(t, root)
	m.Candidate.SHA256 = strings.Repeat("0", 64)
	if _, err := WriteExperimentManifest(root, m); err == nil {
		t.Fatal("broken candidate hash must be refused at write")
	} else if !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("refusal must name the hash mismatch, got %v", err)
	}
	m2 := testManifest(t, root)
	m2.ExperimentID = "exp_missing"
	if _, err := WriteExperimentManifest(root, m2); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadExperimentManifest(root, "exp_missing")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(loaded.ConfigRef); err != nil {
		t.Fatal(err)
	}
	if err := VerifyExperimentManifest(root, loaded); err == nil {
		t.Fatal("missing config must fail verification")
	} else if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing error must say missing, got %v", err)
	}
}

// Secrets in free text never land on disk redacted: the stored notes carry
// the marker, not the value.
func TestExpManifestRedactsSecrets(t *testing.T) {
	root := t.TempDir()
	m := testManifest(t, root)
	m.ExperimentID = "exp_redact"
	m.Notes = "note api_key=secret123 bearer abc.def-ghi"
	if _, err := WriteExperimentManifest(root, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadExperimentManifest(root, "exp_redact")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(loaded.Notes, "secret123") || strings.Contains(loaded.Notes, "abc.def-ghi") {
		t.Fatalf("notes must be redacted on disk, got %q", loaded.Notes)
	}
	if !strings.Contains(loaded.Notes, RedactedMarker) {
		t.Fatalf("redacted notes must carry the marker, got %q", loaded.Notes)
	}
}

// Recovery is on demand: the index stays compact while ResolveReceipt
// fetches the exact evidence file only when asked.
func TestExpManifestOnDemandRecovery(t *testing.T) {
	root := t.TempDir()
	m := testManifest(t, root)
	m.ExperimentID = "exp_resolve"
	m.Results[0].ReceiptID = "rec_resolve_1"
	if _, err := WriteExperimentManifest(root, m); err != nil {
		t.Fatal(err)
	}
	evDir := filepath.Join(root, ".regulus", "evidence", "bench__B1")
	if err := os.MkdirAll(evDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recPath := filepath.Join(evDir, "gate-1.json")
	if err := os.WriteFile(recPath, []byte(`{"id":"rec_resolve_1","task":"bench/B1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveReceipt(root, "rec_resolve_1")
	if err != nil {
		t.Fatalf("resolve must find the receipt: %v", err)
	}
	if got != recPath {
		t.Fatalf("resolve must return the evidence path, got %q want %q", got, recPath)
	}
	if _, err := ResolveReceipt(root, "rec_absent"); err == nil {
		t.Fatal("absent receipt must fail")
	}
}
