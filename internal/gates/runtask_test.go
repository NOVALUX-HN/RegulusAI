package gates

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRunTaskWritesManifestAndReceipts(t *testing.T) {
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
	if res.Failed {
		t.Fatalf("res = %+v", res)
	}
	if res.Status != "PASS" || res.Reason != "ok" {
		t.Fatalf("result status=%q reason=%q", res.Status, res.Reason)
	}
	if mpath == "" {
		t.Fatal("manifest path must not be empty")
	}
	if _, err := os.Stat(mpath); err != nil {
		t.Fatalf("manifest must exist: %v", err)
	}
	if len(res.Receipts) != 1 {
		t.Fatalf("receipts = %v", res.Receipts)
	}
	if _, err := os.Stat(res.Receipts[0]); err != nil {
		t.Fatalf("receipt must exist: %v", err)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.Status != "PASS" || rec.Reason != "ok" {
		t.Fatalf("receipt status=%q reason=%q", rec.Status, rec.Reason)
	}
	if rec.Schema != 1 {
		t.Fatalf("schema = %d, want 1", rec.Schema)
	}
}

func TestRunTaskInvalidReproducibilityErrors(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Reproducibility: "bogus"}}}
	_, _, err := RunTask(context.Background(), cfg, []byte(`{}`), root, "t", "v", "c", "", &fakeRunner{})
	if err == nil {
		t.Fatal("invalid reproducibility must error")
	}
	// no manifest must have been written
	matches, _ := filepath.Glob(filepath.Join(root, ".regulus", "evidence", "*", "manifest-*.json"))
	if len(matches) != 0 {
		t.Fatalf("no manifest on invalid repro, got %v", matches)
	}
	matches2, _ := filepath.Glob(filepath.Join(root, ".regulus", "manifests", "*.json"))
	if len(matches2) != 0 {
		t.Fatalf("no manifest anywhere, got %v", matches2)
	}
}

func TestRunTaskFailedStillWritesManifest(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "fail-cmd"}}}
	r := &fakeRunner{results: map[string]fakeResult{"fail-cmd": {code: 1}}}
	res, mpath, err := RunTask(context.Background(), cfg, []byte(`{}`), root, "t", "v", "c", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("must be failed")
	}
	if res.Status != "FAIL" || res.Reason != "deterministic_failure" {
		t.Fatalf("failed result status=%q reason=%q", res.Status, res.Reason)
	}
	if _, err := os.Stat(mpath); err != nil {
		t.Fatalf("manifest must exist even on failure: %v", err)
	}
}

func TestRunTaskValidateErrorWritesNoManifest(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Gates: []Gate{}} // invalid: no gates
	_, _, err := RunTask(context.Background(), cfg, []byte(`{}`), root, "t", "v", "c", "", &fakeRunner{})
	if err == nil {
		t.Fatal("invalid config must error")
	}
	// walk root for any manifest
	var found []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.HasPrefix(filepath.Base(p), "manifest-") {
			found = append(found, p)
		}
		return nil
	})
	if len(found) != 0 {
		t.Fatalf("no manifest on validate error, got %v", found)
	}
}

func TestParallelRunTaskUniqueManifests(t *testing.T) {
	root := t.TempDir()
	var wg sync.WaitGroup
	type out struct {
		res   Result
		mpath string
		err   error
	}
	outs := make([]out, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "ok"}}}
			res, mpath, err := RunTask(context.Background(), cfg, []byte(`{}`), root, "par/T1", "v", "c", "", &fakeRunner{})
			outs[i] = out{res, mpath, err}
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	for _, o := range outs {
		if o.err != nil {
			t.Fatal(o.err)
		}
		if seen[o.mpath] {
			t.Fatalf("duplicate manifest %q", o.mpath)
		}
		seen[o.mpath] = true
		if _, err := os.Stat(o.mpath); err != nil {
			t.Fatalf("manifest missing: %v", err)
		}
	}
}
