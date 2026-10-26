package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/receipts"
)

func newCLI(probe func() doctor.Result) (CLI, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return CLI{Version: "test", Probe: probe, Stdout: &out, Stderr: &errb}, &out, &errb
}

func TestVersionFlag(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"--version"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := out.String(); got != "regulus test\n" {
		t.Fatalf("output = %q, want %q", got, "regulus test\n")
	}
}

func TestContractCommand(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"contract"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := out.String(); got != contract.Version+"\n" {
		t.Fatalf("output = %q, want %q", got, contract.Version+"\n")
	}
}

func TestHelpCommand(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"help"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Usage:") || !strings.Contains(out.String(), "doctor") {
		t.Fatal("help must list usage and the doctor command")
	}
}

func TestNoArgsIsUsageError(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run(nil); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "Usage:") {
		t.Fatal("no args must print usage to stderr")
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown command "frobnicate"`) {
		t.Fatalf("stderr must name the unknown command, got %q", errb.String())
	}
}

func TestDoctorSupported(t *testing.T) {
	c, out, _ := newCLI(func() doctor.Result {
		return doctor.Result{Detected: "opencode 2.1.0", Major: 2, Supported: true}
	})
	if code := c.Run([]string{"doctor"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "supported (V2)") {
		t.Fatalf("output must report supported V2, got %q", out.String())
	}
}

func TestDoctorRefusalExitsOne(t *testing.T) {
	c, _, errb := newCLI(func() doctor.Result {
		return doctor.Result{Reason: "OpenCode V1 is not supported — Regulus requires V2. Install OpenCode V2."}
	})
	if code := c.Run([]string{"doctor"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "V1 is not supported") {
		t.Fatalf("stderr must carry the refusal reason, got %q", errb.String())
	}
}

func writeTempFeature(t *testing.T, doc string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "feature.md")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSpecCheckOK(t *testing.T) {
	c, out, errb := newCLI(nil)
	path := writeTempFeature(t, "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture project\n- When regulus spec check runs\n- Then the exit code is 0\n")
	if code := c.Run([]string{"spec", "check", path}); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "spec OK: 1 scenario(s)") {
		t.Fatalf("output = %q, want scenario count", out.String())
	}
}

func TestSpecCheckViolationsListed(t *testing.T) {
	c, _, errb := newCLI(nil)
	path := writeTempFeature(t, "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture project\n\n## Log\n")
	if code := c.Run([]string{"spec", "check", path}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "missing When step") || !strings.Contains(errb.String(), "missing Then step") {
		t.Fatalf("stderr must list every violation, got %q", errb.String())
	}
}

func TestSpecCheckMissingFile(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"spec", "check", "does-not-exist.md"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "cannot read") {
		t.Fatalf("stderr must report the unreadable file, got %q", errb.String())
	}
}

func TestSpecSubcommandUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"spec"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus spec check") {
		t.Fatalf("stderr must print spec usage, got %q", errb.String())
	}
	if code := c.Run([]string{"spec", "frobnicate"}); code != 2 {
		t.Fatalf("unknown spec subcommand: exit = %d, want 2", code)
	}
}

type fakeGateResult struct {
	stdout, stderr string
	code           int
	err            error
}

type fakeGateRunner struct {
	results map[string]fakeGateResult
	calls   []string
}

func (f *fakeGateRunner) Run(_ context.Context, command string) (string, string, int, error) {
	f.calls = append(f.calls, command)
	if res, ok := f.results[command]; ok {
		return res.stdout, res.stderr, res.code, res.err
	}
	return "ok", "", 0, nil
}

func TestDoctorSupportedOutputLine(t *testing.T) {
	c, out, _ := newCLI(func() doctor.Result {
		return doctor.Result{Detected: "opencode 2.1.0", Major: 2, Supported: true}
	})
	if code := c.Run([]string{"doctor"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got, want := out.String(), "opencode 2.1.0 — supported (V2)\n"; got != want {
		t.Fatalf("output = %q, want %q (the runtime's own version line, not doubled)", got, want)
	}
}

func writeGatesProject(t *testing.T, cfgJSON string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "regulus.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestGateRunUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"gate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus gate run") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestGateRunInvalidTaskID(t *testing.T) {
	for _, task := range []string{"../evil", "a..b", `a\b`, ""} {
		c, _, errb := newCLI(nil)
		if code := c.Run([]string{"gate", "run", task}); code != 2 {
			t.Fatalf("task %q: exit = %d, want 2", task, code)
		}
		if !strings.Contains(errb.String(), "invalid task id") {
			t.Fatalf("task %q: stderr must name the invalid task id, got %q", task, errb.String())
		}
	}
}

func TestGateRunMissingConfig(t *testing.T) {
	c, _, errb := newCLI(nil)
	c.Runner = &fakeGateRunner{}
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"gate", "run", "demo/T1"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "no regulus.json") {
		t.Fatalf("stderr must name the missing config, got %q", errb.String())
	}
}

func TestGateRunAllPass(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"typecheck","layer":1,"command":"echo hi"},
		{"name":"unit-tests","layer":2,"command":"echo ok"}
	]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "regulus-ai-v1/T2"}); code != 0 {
		t.Fatalf("exit = 0 expected; stdout: %q", out.String())
	}
	if !strings.Contains(out.String(), "gauntlet: 2/2 gates passed") {
		t.Fatalf("stdout = %q", out.String())
	}
	found, _ := filepath.Glob(filepath.Join(".regulus", "evidence", "regulus-ai-v1__T2", "*.json"))
	if len(found) != 2 {
		t.Fatalf("want 2 receipts under the task slug dir, got %v", found)
	}
}

func TestGateRunStopsAtFailure(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"typecheck","layer":1,"command":"boom"},
		{"name":"unit-tests","layer":2,"command":"echo ok"}
	]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"boom":                        {code: 1, stderr: "type error"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "demo/T1"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "stopped at gate") {
		t.Fatalf("stdout must report the stop, got %q", out.String())
	}
	for _, call := range r.calls {
		if call == "echo ok" {
			t.Fatal("no gate may run after a failure")
		}
	}
}

func TestMemoryUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus memory") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"memory", "frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), "unknown subcommand") {
		t.Fatalf("stderr = %q", errb2.String())
	}
}

func TestMemoryNoteRequiresText(t *testing.T) {
	c, _, errb := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"memory", "note"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus memory note") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestMemoryNoteShowRoundTrip(t *testing.T) {
	c, _, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"memory", "note", "session", "started", "here"}); code != 0 {
		t.Fatalf("note exit = %d, want 0", code)
	}
	c2, out, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "show"}); code != 0 {
		t.Fatalf("show exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "session started here") {
		t.Fatalf("show output = %q", out.String())
	}
}

func TestMemoryShowEmptySession(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"memory", "show"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(no session notes)") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestReflectUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"reflect"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus reflect") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestReflectRefusesNoEvidence(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "feature.md")
	content := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When regulus runs\n- Then the exit code is 0\n"
	if err := os.WriteFile(doc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, errb := newCLI(nil)
	t.Chdir(dir)
	if code := c.Run([]string{
		"reflect",
		"--domain", "go", "--task", "feature/T1",
		"--cue", "c", "--lesson", "l",
		doc,
	}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "no evidence") {
		t.Fatalf("stderr must name the missing evidence, got %q", errb.String())
	}
}

func TestBudgetShowUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"budget"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus budget show") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"budget", "frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestBudgetShowEmpty(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"budget", "show"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(no budget entries)") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestLibraryListUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"library"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus library list") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestLibraryListEmpty(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"library", "list"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(no lessons admitted yet)") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCrapUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"crap"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus crap") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func writeCrapFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package pkg

func Simple() int {
	return 1
}
`
	if err := os.WriteFile(filepath.Join(dir, "pkg", "calc.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	content := "mode: set\nfixture/pkg/calc.go:4.9,4.17 1 1\n"
	if err := os.WriteFile(filepath.Join(dir, "cover.out"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCrapReportsAndEnforcesMax(t *testing.T) {
	writeCrapFixture(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"crap", "--profile", "cover.out", "--max", "30"}); code != 0 {
		t.Fatalf("exit = %d, want 0 (CRAP 1 under max 30)", code)
	}
	if !strings.Contains(out.String(), "Simple (complexity 1, coverage 100.0%) CRAP 1.00") {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(out.String(), "CRAP max: 1.00") {
		t.Fatalf("output = %q", out.String())
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"crap", "--profile", "cover.out", "--max", "0.5"}); code != 1 {
		t.Fatalf("exit = %d, want 1 (CRAP 1 over max 0.5)", code)
	}
	if !strings.Contains(errb.String(), "exceeds") {
		t.Fatalf("stderr must name the violation, got %q", errb.String())
	}
}

func TestEvidenceUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evidence") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestEvidenceListEmpty(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"evidence", "list"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(no receipts yet)") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestEvidenceListAndShow(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	taskDir := filepath.Join(".regulus", "evidence", "feat__T1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1700000000, 0).UTC()
	path1, err := receipts.Write(taskDir, receipts.Receipt{
		Task: "feat/T1", Gate: "typecheck", Layer: 1, Command: "go build ./...",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: start,
		DurationMS: 7, Stdout: "built", GitHead: "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.Write(taskDir, receipts.Receipt{
		Task: "feat/T1", Gate: "lint", Layer: 2, Command: "go vet ./...",
		ExitCode: 1, Verdict: receipts.VerdictFail, StartedAt: start.Add(time.Second),
		DurationMS: 3, Stderr: "boom",
	}); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evidence", "list"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "typecheck") || !strings.Contains(out.String(), "lint") || !strings.Contains(out.String(), "exit=1") {
		t.Fatalf("list output = %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"evidence", "show", path1}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, want := range []string{"verdict:     pass", "built", "typecheck", "stdout (sha256"} {
		if !strings.Contains(out2.String(), want) {
			t.Fatalf("show output must contain %q, got:\n%s", want, out2.String())
		}
	}
}

func TestEvidenceShowMissingFile(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "show", "nope.json"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "cannot read") {
		t.Fatalf("stderr = %q", errb.String())
	}
}
