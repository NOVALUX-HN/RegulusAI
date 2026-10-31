package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

func initTaskChart(t *testing.T, name string) string {
	t.Helper()
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"chart", "init", name}); code != 0 {
		t.Fatalf("chart init: exit = %d", code)
	}
	return filepath.Join("charts", name+".md")
}

func writeTaskReceipt(t *testing.T, evidTask, gate string, pass bool) {
	t.Helper()
	dir := evidence.TaskDir(".", evidTask)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	code, verdict := 0, receipts.VerdictPass
	if !pass {
		code, verdict = 1, receipts.VerdictFail
	}
	rec := receipts.Receipt{
		Task: evidTask, Gate: gate, Layer: 1, Command: "echo ok",
		ExitCode: code, Verdict: verdict, StartedAt: time.Unix(1700000000, 0).UTC(),
		DurationMS: 3, GitHead: "abc",
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("ok")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	if _, err := receipts.Write(dir, rec); err != nil {
		t.Fatal(err)
	}
}

func TestTaskUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"task"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus task") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestTaskCheckCleanScaffold(t *testing.T) {
	chart := initTaskChart(t, "demo")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "check", chart}); code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "OK") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestTaskCheckFlagsViolation(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("charts", 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n\n## Tasks\n\n| ID | Spec | Work | Route |\n|----|------|------|-------|\n| T1 | S1 | w | @regulus-ghost |\n\n## Log\n\n- L1.\n"
	if err := os.WriteFile(filepath.Join("charts", "bad.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "check", "charts/bad.md"}); code != 1 {
		t.Fatalf("exit = %d, want 1, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "unknown agent") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestTaskStatusShowsBlocker(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("charts", 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n- Scope: y\n- Acceptance: s\n- Verification: g\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When b\n- Then c\n\n## Tasks\n\n| ID | Spec | Work | Route | Status |\n|----|------|------|-------|--------|\n| T1 | s | w | inline | blocked: waiting on key |\n\n## Log\n\n- L1.\n"
	if err := os.WriteFile(filepath.Join("charts", "b.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "status", "charts/b.md"}); code != 0 {
		t.Fatalf("a named blocker is valid state, got %d, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "waiting on key") {
		t.Fatalf("output must name the blocker, got %q", out.String())
	}
	// A bare blocked status with no reason is a violation.
	raw, _ := os.ReadFile("charts/b.md")
	os.WriteFile("charts/b.md", []byte(strings.Replace(string(raw), "blocked: waiting on key", "blocked", 1)), 0o644)
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"task", "status", "charts/b.md"}); code != 1 {
		t.Fatalf("bare blocked must exit 1, got %d, output:\n%s", code, out2.String())
	}
	if !strings.Contains(out2.String(), "blocker") {
		t.Fatalf("output must demand the reason, got %q", out2.String())
	}
}

func TestTaskCloseEndToEnd(t *testing.T) {
	chart := initTaskChart(t, "demo")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "close", chart, "T1"}); code != 1 {
		t.Fatalf("close without receipts must exit 1, got %d, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "no receipts") {
		t.Fatalf("output = %q", out.String())
	}
	writeTaskReceipt(t, "demo/T1", "build", true)
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"task", "close", chart, "T1"}); code != 0 {
		t.Fatalf("close with evidence must exit 0, got %d, output:\n%s", code, out2.String())
	}
	if !strings.Contains(out2.String(), "CLOSED T1") {
		t.Fatalf("output = %q", out2.String())
	}
	data, err := os.ReadFile(chart)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "CLOSED T1") || !strings.Contains(string(data), "done") {
		t.Fatalf("chart must record the closure, got:\n%s", data)
	}
	// Status now reports the verified completion.
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"task", "status", chart}); code != 0 {
		t.Fatalf("closed chart must be clean, got %d, output:\n%s", code, out3.String())
	}
}

func TestTaskCloseRefusesFailingGate(t *testing.T) {
	chart := initTaskChart(t, "demo")
	writeTaskReceipt(t, "demo/T1", "build", false)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "close", chart, "T1"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "failing gates") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestTaskClassify(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "classify", "--code", "--units", "2"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "COMPLEX") {
		t.Fatalf("output = %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"task", "classify", "--security"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "CRITICAL") {
		t.Fatalf("security must force CRITICAL, got %q", out2.String())
	}
}
