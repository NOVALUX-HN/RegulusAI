package gates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/manifest"
)

// D2 SPLIT: buildManifestParams + attachRefs extracted from RunTask.
// Params must pin root/task/run/version/contract/gates/policy;
// refs must fingerprint every receipt; the run shape stays identical.

func TestSplitBuildManifestParams(t *testing.T) {
	specs, err := buildGateSpecs(Config{Gates: []Gate{
		{Name: "a", Layer: 1, Command: "ok-cmd", Reproducibility: "exact"},
		{Name: "b", Layer: 2, Command: "other", TimeoutSec: 7, Category: "test"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Execution: &ExecutionConfig{MaxTimeoutSec: 120, Allowlist: []string{"go"}}}
	p := buildManifestParams("/repo", "feat/T1", "run_x", "v1", "c1", []byte(`{}`), cfg, specs)
	if p.Root != "/repo" || p.Task != "feat/T1" || p.RunID != "run_x" {
		t.Fatalf("identity = %+v", p)
	}
	if p.RegulusVersion != "v1" || p.Contract != "c1" {
		t.Fatalf("versions = %+v", p)
	}
	if len(p.Gates) != 2 || p.Gates[0].Reproducibility != "exact" || p.Gates[1].Reproducibility != "unknown" {
		t.Fatalf("specs = %+v", p.Gates)
	}
	if p.MaxTimeoutSec != 120 || !p.AllowlistEnforced || p.AllowDestructive {
		t.Fatalf("policy = %+v", p)
	}
	if p.Isolation != IsolationHostContained {
		t.Fatalf("isolation = %q", p.Isolation)
	}
	docker := Config{Execution: &ExecutionConfig{DockerImage: "img"}}
	pd := buildManifestParams("/repo", "t", "r", "v", "c", nil, docker, nil)
	if pd.Isolation != IsolationDocker {
		t.Fatalf("docker isolation = %q", pd.Isolation)
	}
}

func TestSplitBuildGateSpecsInvalid(t *testing.T) {
	_, err := buildGateSpecs(Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Reproducibility: "bogus"}}})
	if err == nil || err.Error() != `manifest: gate "a": invalid reproducibility "bogus"` {
		t.Fatalf("want exact invalid-repro error, got %v", err)
	}
}

func TestSplitCollectAttachRefs(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "ok-cmd"}}}
	res, _, err := RunTask(context.Background(), cfg, []byte(`{}`), root, "feat/T1", "v", "c", "", &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Receipts) != 1 {
		t.Fatalf("receipts = %v", res.Receipts)
	}
	refs, err := collectArtifactRefs(res.Receipts)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %+v", refs)
	}
	data, err := os.ReadFile(res.Receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	rec := readReceiptFile(t, res.Receipts[0])
	r := refs[0]
	if r.ReceiptID != rec.ID || r.File != filepath.Base(res.Receipts[0]) {
		t.Fatalf("ref identity = %+v, receipt id = %q", r, rec.ID)
	}
	if r.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("ref sha = %q", r.SHA256)
	}
	if r.Gate != rec.Gate || r.Status != rec.Status || r.Reason != rec.Reason {
		t.Fatalf("ref verdict = %+v, receipt = %+v", r, rec)
	}
	var m manifest.Manifest
	attachRefs(&m, refs, []string{"skipped-gate"})
	if len(m.Artifacts) != 1 || m.Artifacts[0] != r {
		t.Fatalf("attached = %+v", m.Artifacts)
	}
	if len(m.Skipped) != 1 || m.Skipped[0] != "skipped-gate" {
		t.Fatalf("skipped = %v", m.Skipped)
	}
}

func TestSplitRunTaskIdentical(t *testing.T) {
	root := t.TempDir()
	cfgJSON := `{"gates":[{"name":"a","layer":1,"command":"ok-cmd","reproducibility":"exact"}]}`
	cfgPath := filepath.Join(root, "regulus.json")
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfgBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	res, mpath, err := RunTask(context.Background(), cfg, cfgBytes, root, "feat/T1", "test-version", "regulus.contract/v1", "", &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed || res.Status != "PASS" || res.Reason != "ok" {
		t.Fatalf("res = %+v", res)
	}
	wantDir := filepath.Join(root, ".regulus", "evidence", "feat__T1")
	if !strings.HasPrefix(mpath, wantDir) || filepath.Base(mpath)[:9] != "manifest-" {
		t.Fatalf("mpath = %q, want manifest under %q", mpath, wantDir)
	}
	m, err := manifest.ReadFile(mpath)
	if err != nil {
		t.Fatal(err)
	}
	if m.Task != "feat/T1" || m.RunID == "" {
		t.Fatalf("manifest identity = %+v", m)
	}
	if len(m.Artifacts) != len(res.Receipts) {
		t.Fatalf("artifacts %d != receipts %d", len(m.Artifacts), len(res.Receipts))
	}
	if len(m.Config.Gates) != 1 || m.Config.Gates[0].Name != "a" {
		t.Fatalf("manifest gates = %+v", m.Config.Gates)
	}
}
