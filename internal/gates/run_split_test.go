package gates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// C3 SPLIT+COVER: ramas sin cubrir en run (746, 760-772, 793, 818, 832,
// 880, 888). Cada test fija mensajes/códigos/receipts idénticos.

func TestRunSplitRefusedShRunnerIsolation(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "bad", Layer: 1, Command: "rm -rf /"}}}
	r := ShRunner{Dir: t.TempDir()}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.FailedGate != "bad" {
		t.Fatalf("res = %+v", res)
	}
	if res.Status != "BLOCKED" || res.Reason != "policy_violation" {
		t.Fatalf("status=%q reason=%q", res.Status, res.Reason)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.Isolation != IsolationHostContained {
		t.Fatalf("refused ShRunner isolation = %q, want host-contained", rec.Isolation)
	}
	if !strings.Contains(rec.Stderr, "policy: destructive command refused") {
		t.Fatalf("refused stderr = %q", rec.Stderr)
	}
	if rec.ExitCode != -1 {
		t.Fatalf("refused exit = %d, want -1", rec.ExitCode)
	}
}

func TestRunSplitShRunnerCouldNotRun(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "echo hi"}}}
	r := ShRunner{Dir: filepath.Join(t.TempDir(), "does-not-exist-xyz")}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatalf("spawn failure must fail: %+v", res)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, "gate could not run") {
		t.Fatalf("stderr must name the runner failure, got %q", rec.Stderr)
	}
	if rec.ExitCode != -1 {
		t.Fatalf("exit = %d, want -1", rec.ExitCode)
	}
	if rec.Status != "BLOCKED" || rec.Reason != "infrastructure_failure" {
		t.Fatalf("receipt = %q/%q", rec.Status, rec.Reason)
	}
}

func TestRunSplitGenericRerrWithStderr(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "spawn-error"}}}
	r := &fakeRunner{results: map[string]fakeResult{
		"spawn-error": {stderr: "prev", err: errors.New("sh: not found")},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("must fail")
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, "prev") || !strings.Contains(rec.Stderr, "could not run") {
		t.Fatalf("stderr must join prev and note, got %q", rec.Stderr)
	}
	if rec.ExitCode != -1 {
		t.Fatalf("exit = %d, want -1", rec.ExitCode)
	}
}

func TestRunSplitMetricExtractErrWithStderr(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "coverage", Layer: 5, Command: "cover-cmd",
		Metric: &Metric{Extract: ExtractLastLineFloat, Threshold: 80, Operator: OpAtLeast},
	}}}
	r := &fakeRunner{results: map[string]fakeResult{
		"cover-cmd": {stdout: "no number here", stderr: "prev"},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("unextractable metric must fail")
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.MetricValue != nil {
		t.Fatal("MetricValue must stay nil")
	}
	if !strings.Contains(rec.Stderr, "prev") || !strings.Contains(rec.Stderr, "could not extract") {
		t.Fatalf("stderr must join prev and metric note, got %q", rec.Stderr)
	}
}

func TestRunSplitMetricMissWithStderr(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "coverage-floor", Layer: 5, Command: "cover-cmd",
		Metric: &Metric{Extract: ExtractLastLineFloat, Threshold: 80, Operator: OpAtLeast},
	}}}
	r := &fakeRunner{results: map[string]fakeResult{
		"cover-cmd": {stdout: "72.5", stderr: "prev"},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("threshold miss must fail")
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, "prev") || !strings.Contains(rec.Stderr, "does not meet") {
		t.Fatalf("stderr must join prev and threshold note, got %q", rec.Stderr)
	}
}

func TestRunSplitCondNoteWithStderr(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "opt", Layer: 1, Command: "check-opt",
		Required: boolPtr(false), When: []string{"*.go"},
	}}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {err: errors.New("no git")},
		"check-opt": {stdout: "ok", stderr: "prev"},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if res.Executed != 1 {
		t.Fatalf("unevaluable must run, got %+v", res)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, "prev") || !strings.Contains(rec.Stderr, "unevaluable") {
		t.Fatalf("stderr must join prev and cond note, got %q", rec.Stderr)
	}
}

func TestRunSplitRequiredCondNoteWithStderr(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "req", Layer: 1, Command: "check-req", When: []string{"*.go"},
	}}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {stdout: " M README.md\x00"},
		"check-req": {stdout: "ok", stderr: "prev"},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if res.Executed != 1 {
		t.Fatalf("required must run, got %+v", res)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, "prev") || !strings.Contains(rec.Stderr, "ran anyway") {
		t.Fatalf("stderr must join prev and required note, got %q", rec.Stderr)
	}
}

func TestRunSplitWriteReceiptErr(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "ok"}}}
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), cfg, blocker, "t", "", &fakeRunner{})
	if err == nil || !strings.Contains(err.Error(), "gate a: writing receipt") {
		t.Fatalf("want writing-receipt error, got %v", err)
	}
}
