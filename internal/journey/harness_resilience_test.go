package journey

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/receipts"
)

// harnessResRun drives the real CLI in-process against a fresh fixture
// directory: a cold CLI per call, state on disk only, Probe stubbed,
// hermetic (only echo/exit/sleep commands plus TempDir — no network, no
// model, no plugin host).
func harnessResRun(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	c := cli.CLI{
		Version: "test",
		Probe: func() doctor.Result {
			return doctor.Result{Detected: "opencode 2.2.0", Major: 2, Supported: true}
		},
		Stdout: &out,
		Stderr: &errb,
	}
	code := c.Run(args)
	return code, out.String() + "\n---stderr---\n" + errb.String()
}

func harnessWriteConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "regulus.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// harnessReceipts globs the task's receipts, excluding index manifests.
func harnessReceipts(t *testing.T, task string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(".regulus", "evidence", receipts.Slug(task), "*.json"))
	paths = withoutManifests(paths)
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

// TestHarnessRejectOnFail proves a failing gate rejects the task: the run
// exits 1 with a FAIL receipt, and task close stays blocked by the
// failing gate (no receipts are added by the refused close).
func TestHarnessRejectOnFail(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if code, out := harnessResRun(t, "chart", "init", "rej"); code != 0 {
		t.Fatalf("chart init: exit=%d out=%q", code, out)
	}
	chart := filepath.Join("charts", "rej.md")
	if code, out := harnessResRun(t, "task", "check", chart); code != 0 {
		t.Fatalf("task check must pass so the gate itself runs: exit=%d out=%q", code, out)
	}
	harnessWriteConfig(t, dir, `{"gates":[{"name":"smoke","layer":1,"command":"exit 1"}]}`)

	code, out := harnessResRun(t, "gate", "run", "--yes", "rej/T1")
	if code != 1 {
		t.Fatalf("failing gate run must exit 1, got exit=%d out=%q", code, out)
	}
	for _, want := range []string{`gauntlet: stopped at gate "smoke"`, "0/1 gates passed", "close stays blocked"} {
		if !strings.Contains(out, want) {
			t.Fatalf("gate run must report %q, got:\n%s", want, out)
		}
	}

	paths := harnessReceipts(t, "rej/T1")
	if len(paths) != 1 {
		t.Fatalf("want exactly 1 FAIL receipt, got %v", paths)
	}
	rec, err := receipts.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if rec.Verdict != receipts.VerdictFail || rec.ExitCode != 1 {
		t.Fatalf("receipt must be fail/exit 1, got %+v", rec)
	}
	if rec.Status != "FAIL" || rec.Reason != "deterministic_failure" {
		t.Fatalf("receipt status must be FAIL/deterministic_failure, got %q/%q", rec.Status, rec.Reason)
	}
	if code, out := harnessResRun(t, "evidence", "show", paths[0]); code != 0 || !strings.Contains(out, "verdict:     fail (exit 1)") {
		t.Fatalf("evidence show: exit=%d out=%q", code, out)
	}

	code, out = harnessResRun(t, "task", "close", chart, "T1")
	if code != 1 {
		t.Fatalf("close must stay blocked: exit=%d out=%q", code, out)
	}
	for _, want := range []string{"cannot close", "failing gates: smoke (exit 1)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("close must report %q, got:\n%s", want, out)
		}
	}
	if after := harnessReceipts(t, "rej/T1"); len(after) != 1 {
		t.Fatalf("refused close must add no receipts, got %v", after)
	}
}

// TestHarnessTimeoutCancels proves a gate over its timeout is killed and
// recorded as infrastructure failure (BLOCKED), and that the timeout is
// visible in the session trace. Note: on Windows/MSYS2 the orphaned
// sleeper can hold Wait until it exits, so the run lasts ~sleep seconds
// there (~1s on POSIX); either way the harness records the timeout
// (stderr note plus BLOCKED/infrastructure_failure, never a pass).
func TestHarnessTimeoutCancels(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	harnessWriteConfig(t, dir, `{"gates":[{"name":"slow","layer":1,"command":"sleep 5","timeout_sec":1}]}`)

	code, out := harnessResRun(t, "gate", "run", "--yes", "slow/T1")
	if code != 1 {
		t.Fatalf("timed-out gate run must exit 1, got exit=%d out=%q", code, out)
	}
	if !strings.Contains(out, `gauntlet: stopped at gate "slow"`) {
		t.Fatalf("gate run must stop at the slow gate, got:\n%s", out)
	}

	paths := harnessReceipts(t, "slow/T1")
	if len(paths) != 1 {
		t.Fatalf("want exactly 1 timeout receipt, got %v", paths)
	}
	rec, err := receipts.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if rec.Verdict != receipts.VerdictFail || rec.ExitCode != -1 {
		t.Fatalf("timeout receipt must be fail/exit -1, got %+v", rec)
	}
	if rec.Status != "BLOCKED" || rec.Reason != "infrastructure_failure" {
		t.Fatalf("timeout receipt must be BLOCKED/infrastructure_failure, got %q/%q", rec.Status, rec.Reason)
	}
	if rec.DurationMS >= 30000 {
		t.Fatalf("the 1s timeout must bound the 5s sleep well under the 600s default, duration=%dms", rec.DurationMS)
	}
	if code, out := harnessResRun(t, "evidence", "show", paths[0]); code != 0 ||
		!strings.Contains(out, "timed out") || !strings.Contains(out, "BLOCKED (infrastructure_failure)") {
		t.Fatalf("evidence show must expose the timeout: exit=%d out=%q", code, out)
	}

	if code, out := harnessResRun(t, "trace", "record", "--session", "resil", "--message", "timeout",
		"--call", "slow-1", "--tool", "gate_run", "--status", "error", "--task", "slow/T1",
		"--files", "regulus.json"); code != 0 {
		t.Fatalf("trace record: exit=%d out=%q", code, out)
	}
	if code, out := harnessResRun(t, "trace", "show", "--session", "resil"); code != 0 || !strings.Contains(out, "1 gate_run error") {
		t.Fatalf("trace show must record the timeout: exit=%d out=%q", code, out)
	}
	if code, out := harnessResRun(t, "trace", "files", "--session", "resil"); code != 0 || !strings.Contains(out, "regulus.json") {
		t.Fatalf("trace files: exit=%d out=%q", code, out)
	}
}

// TestHarnessPartialStopsAtFirstFailure proves a 1/2 partial run stops at
// the failing gate: exactly one PASS receipt plus one FAIL receipt, in
// layer order, with the gauntlet reporting 1/2.
func TestHarnessPartialStopsAtFirstFailure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	harnessWriteConfig(t, dir, `{"gates":[
  {"name":"ok","layer":1,"command":"echo ok"},
  {"name":"bad","layer":2,"command":"exit 1"}
]}`)

	code, out := harnessResRun(t, "gate", "run", "--yes", "part/T1")
	if code != 1 {
		t.Fatalf("partial run must exit 1, got exit=%d out=%q", code, out)
	}
	for _, want := range []string{`gauntlet: stopped at gate "bad"`, "1/2 gates passed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("gate run must report %q, got:\n%s", want, out)
		}
	}

	paths := harnessReceipts(t, "part/T1")
	if len(paths) != 2 {
		t.Fatalf("want exactly 2 receipts (1 PASS + 1 FAIL), got %v", paths)
	}
	byGate := map[string]receipts.Receipt{}
	for _, p := range paths {
		rec, err := receipts.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		byGate[rec.Gate] = rec
	}
	if byGate["ok"].Verdict != receipts.VerdictPass || byGate["ok"].ExitCode != 0 {
		t.Fatalf("first gate must pass, got %+v", byGate["ok"])
	}
	if byGate["bad"].Verdict != receipts.VerdictFail || byGate["bad"].ExitCode != 1 {
		t.Fatalf("second gate must fail, got %+v", byGate["bad"])
	}

	if code, out := harnessResRun(t, "evidence", "list", "--task", "part/T1"); code != 0 ||
		!strings.Contains(out, "ok") || !strings.Contains(out, "bad") {
		t.Fatalf("evidence list must show both gates: exit=%d out=%q", code, out)
	}
}

// TestHarnessResumeAppendsWithoutDuplicating proves fail->fix->run resumes
// by appending: the second run adds exactly one PASS receipt while the
// previous FAIL receipt stays byte-identical (same id, same file).
func TestHarnessResumeAppendsWithoutDuplicating(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	harnessWriteConfig(t, dir, `{"gates":[{"name":"fixme","layer":1,"command":"exit 1"}]}`)
	if code, out := harnessResRun(t, "gate", "run", "--yes", "resume/T1"); code != 1 {
		t.Fatalf("first run must fail: exit=%d out=%q", code, out)
	}
	before := harnessReceipts(t, "resume/T1")
	if len(before) != 1 {
		t.Fatalf("first run must leave exactly 1 receipt, got %v", before)
	}
	failRec, err := receipts.ReadFile(before[0])
	if err != nil {
		t.Fatal(err)
	}
	if failRec.Verdict != receipts.VerdictFail {
		t.Fatalf("first receipt must be FAIL, got %+v", failRec)
	}

	harnessWriteConfig(t, dir, `{"gates":[{"name":"fixme","layer":1,"command":"echo ok"}]}`)
	code, out := harnessResRun(t, "gate", "run", "--yes", "resume/T1")
	if code != 0 || !strings.Contains(out, "gauntlet: 1/1 gates passed") {
		t.Fatalf("fixed run must pass: exit=%d out=%q", code, out)
	}
	after := harnessReceipts(t, "resume/T1")
	if len(after) != 2 {
		t.Fatalf("resume must append exactly one receipt (total 2), got %v", after)
	}
	if _, err := os.Stat(before[0]); err != nil {
		t.Fatalf("previous FAIL receipt must survive the resume: %v", err)
	}
	kept, err := receipts.ReadFile(before[0])
	if err != nil {
		t.Fatal(err)
	}
	if kept.ID != failRec.ID || kept.Verdict != receipts.VerdictFail {
		t.Fatalf("previous receipt must be unchanged, got %+v (was %+v)", kept, failRec)
	}
	var passes, fails int
	for _, p := range after {
		rec, err := receipts.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		switch rec.Verdict {
		case receipts.VerdictPass:
			passes++
		case receipts.VerdictFail:
			fails++
		}
	}
	if passes != 1 || fails != 1 {
		t.Fatalf("want 1 PASS + 1 FAIL after resume, got %d pass + %d fail", passes, fails)
	}
}
