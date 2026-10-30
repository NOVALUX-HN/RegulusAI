package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/contract"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/library"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
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
	if got, want := out.String(), "version:    test (no package.json — nothing to compare)\nopencode 2.1.0 — supported (V2)\nplugin: cannot be verified outside OpenCode — open this repo in OpenCode to load .opencode/plugins/regulus.ts\n"; got != want {
		t.Fatalf("output = %q, want %q (version line, then the runtime's own version line)", got, want)
	}
}

func TestDoctorWarnsOnVersionMismatch(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("package.json", []byte(`{"version":"9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(func() doctor.Result {
		return doctor.Result{Detected: "opencode 2.1.0", Major: 2, Supported: true}
	})
	if code := c.Run([]string{"doctor"}); code != 0 {
		t.Fatalf("a version warning must not fail doctor, exit = %d", code)
	}
	if !strings.Contains(out.String(), "WARNING — binary says test but package.json says 9.9.9") {
		t.Fatalf("output = %q", out.String())
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

func TestReflectProposesAndAdmitsByDomain(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	doc := "feature.md"
	content := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When regulus runs\n- Then the exit code is 0\n"
	if err := os.WriteFile(doc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Join(".regulus", "evidence", "tiny__T1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1700000000, 0).UTC()
	if _, err := receipts.Write(taskDir, receipts.Receipt{
		Task: "tiny/T1", Gate: "typecheck", Layer: 1, Command: "c",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: start, DurationMS: 2,
	}); err != nil {
		t.Fatal(err)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{
		"reflect", "--domain", "go", "--task", "tiny/T1",
		"--cue", "when debt is paid", "--lesson", "cover the uncovered paths", doc,
	}); code != 0 {
		t.Fatalf("reflect exit = %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "candidate proposed: les_") {
		t.Fatalf("reflect output = %q", out.String())
	}
	id := candidateID(t, out.String())
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"library", "candidates", "--domain", "go"}); code != 0 {
		t.Fatalf("candidates exit = %d", code)
	}
	if !strings.Contains(out2.String(), "go (pending):") || !strings.Contains(out2.String(), "when debt is paid") {
		t.Fatalf("candidates output = %q", out2.String())
	}
	c3, out3, errb3 := newCLI(nil)
	if code := c3.Run([]string{"library", "admit", "--domain", "go", "--candidate", id, "--guardian", "regulus-gate-verifier"}); code != 0 {
		t.Fatalf("admit exit = %d, stderr %q", code, errb3.String())
	}
	if !strings.Contains(out3.String(), "lesson admitted") || !strings.Contains(out3.String(), "regulus-gate-verifier") {
		t.Fatalf("admit output = %q", out3.String())
	}
	c4, out4, _ := newCLI(nil)
	if code := c4.Run([]string{"library", "list", "--domain", "go"}); code != 0 {
		t.Fatalf("list exit = %d", code)
	}
	if !strings.Contains(out4.String(), "go:") || !strings.Contains(out4.String(), "when debt is paid") {
		t.Fatalf("list output = %q", out4.String())
	}
	c5, out5, _ := newCLI(nil)
	if code := c5.Run([]string{"library", "show", "--domain", "go"}); code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	if !strings.Contains(out5.String(), "Library — go") {
		t.Fatalf("show output = %q", out5.String())
	}
}

var candidateIDPattern = regexp.MustCompile(`candidate proposed: (les_[0-9a-f]+)`)

func candidateID(t *testing.T, out string) string {
	t.Helper()
	m := candidateIDPattern.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("no candidate id in %q", out)
	}
	return m[1]
}

func TestLibraryShowAndCandidatesEmpty(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"library", "show", "--domain", "go"}); code != 0 {
		t.Fatalf("show exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(no lessons admitted yet)") {
		t.Fatalf("show output = %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"library", "candidates"}); code != 0 {
		t.Fatalf("candidates exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "(no candidates pending)") {
		t.Fatalf("candidates output = %q", out2.String())
	}
}

func TestLibraryAdmitUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"library", "admit"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus library admit") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"library", "admit", "--domain", "go", "--candidate", "les_x"}); code != 2 {
		t.Fatalf("missing guardian: exit = %d, want 2", code)
	}
}

func TestBudgetRecordProbe(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"budget", "record", "--kind", "probe", "--detail", "probed the mutation gate"}); code != 0 {
		t.Fatalf("record exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "probe recorded") {
		t.Fatalf("output = %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"budget", "show"}); code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	if !strings.Contains(out2.String(), "probe-run") || !strings.Contains(out2.String(), "probed the mutation gate") {
		t.Fatalf("budget show = %q", out2.String())
	}
	c3, _, errb := newCLI(nil)
	if code := c3.Run([]string{"budget", "record", "--kind", "reflect", "--detail", "x"}); code != 2 {
		t.Fatalf("kind reflect must be refused: exit = %d", code)
	}
	if !strings.Contains(errb.String(), "only probe") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestStatusReportsTheProjectTruthfully(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("regulus.json", []byte(`{"gates":[{"name":"typecheck","layer":1,"command":"echo ok"}],"evolution":{"max_per_day":3}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// receipts on disk
	if err := os.MkdirAll(filepath.Join(".regulus", "evidence", "t__1"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1700000000, 0).UTC()
	if _, err := receipts.Write(filepath.Join(".regulus", "evidence", "t__1"), receipts.Receipt{
		Task: "t/1", Gate: "typecheck", Layer: 1, Command: "c",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: start, DurationMS: 2,
	}); err != nil {
		t.Fatal(err)
	}
	// a pending candidate
	if err := os.MkdirAll(filepath.Join(".regulus", "library", "go", "candidates", "les_p1"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# les_p1\n\n- **domain**: go\n- **cue**: c\n- **lesson**: l\n- **evidence**: r.json\n- **confidence**: low\n- **created_at**: " + start.Format(time.RFC3339) + "\n- **status**: candidate\n"
	if err := os.WriteFile(filepath.Join(".regulus", "library", "go", "candidates", "les_p1", "LESSON.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, errb := newCLI(func() doctor.Result {
		return doctor.Result{Detected: "opencode 2.1.4", Major: 2, Supported: true}
	})
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"status"}); code != 0 {
		t.Fatalf("status exit = %d, stderr %q", code, errb.String())
	}
	for _, want := range []string{
		"runtime:    opencode 2.1.4 — supported (V2)",
		"gauntlet:   1 gate(s) configured",
		"evolution:  0/3 run(s) today",
		"lessons:    0 admitted, 1 candidate(s) pending",
		"receipts:   1 on disk",
		"mutation:    gremlins",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status must report %q, got:\n%s", want, out.String())
		}
	}
}

func TestStatusReportsMissingPiecesHonestly(t *testing.T) {
	c, out, _ := newCLI(func() doctor.Result {
		return doctor.Result{Reason: "opencode binary not found on PATH — install OpenCode V2"}
	})
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"status"}); code != 0 {
		t.Fatalf("status exit = %d, want 0 (status always reports)", code)
	}
	if !strings.Contains(out.String(), "runtime:    NOT SUPPORTED — opencode binary not found") {
		t.Fatalf("status output = %q", out.String())
	}
	if !strings.Contains(out.String(), "gauntlet:   not configured") {
		t.Fatalf("status output = %q", out.String())
	}
	if !strings.Contains(out.String(), "receipts:   0 on disk") {
		t.Fatalf("status output = %q", out.String())
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"status", "extra"}); code != 2 {
		t.Fatalf("status extra arg: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus status") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestLibraryForgetRetainCycle(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	for i, task := range []string{"tiny/T1", "tiny/T2"} {
		l := library.Lesson{
			ID:         library.NewID(now.Add(time.Duration(i) * time.Second)),
			Domain:     "go",
			Task:       task,
			Cue:        "cue for " + task,
			Lesson:     "lesson for " + task,
			Evidence:   []string{".regulus/evidence/tiny__T1/typecheck-1.json"},
			Confidence: library.ConfidenceLow,
			GitHead:    "abc123",
			CreatedAt:  now.Add(time.Duration(i) * time.Second),
		}
		if err := library.Admit(".", l, "regulus-gate-verifier"); err != nil {
			t.Fatal(err)
		}
	}
	// forget drops the snapshot, not the lesson
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"library", "forget", "--domain", "go", "--task", "tiny/T1"}); code != 0 {
		t.Fatalf("forget exit = %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "forgotten: harnesses/tiny__T1/") {
		t.Fatalf("forget output = %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(".regulus", "library", "go", "harnesses", "tiny__T1")); !os.IsNotExist(err) {
		t.Fatal("forget must remove the task snapshot")
	}
	// unknown task is a no-op success
	cU, outU, errbU := newCLI(nil)
	if code := cU.Run([]string{"library", "forget", "--domain", "go", "--task", "tiny/NONE"}); code != 0 {
		t.Fatalf("unknown forget exit = %d, stderr %q", code, errbU.String())
	}
	if !strings.Contains(outU.String(), "forgotten: harnesses/tiny__NONE/") {
		t.Fatalf("unknown forget output = %q", outU.String())
	}
	// retain keeps strictly-before tasks and drops the bound and later
	cR, outR, errbR := newCLI(nil)
	if code := cR.Run([]string{"library", "retain", "--domain", "go", "--task", "tiny/T2"}); code != 0 {
		t.Fatalf("retain exit = %d, stderr %q", code, errbR.String())
	}
	if !strings.Contains(outR.String(), "retained: tiny/T1 (domain go)") {
		t.Fatalf("retain output = %q", outR.String())
	}
	lessons, err := library.List(".", "go")
	if err != nil || len(lessons) != 1 || lessons[0].Task != "tiny/T1" {
		t.Fatalf("retain must drop tiny/T2 and later, got %+v (%v)", lessons, err)
	}
	// retaining before everything empties the domain
	cR2, outR2, errbR2 := newCLI(nil)
	if code := cR2.Run([]string{"library", "retain", "--domain", "go", "--task", "a"}); code != 0 {
		t.Fatalf("retain-all exit = %d, stderr %q", code, errbR2.String())
	}
	if !strings.Contains(outR2.String(), "no tasks before a") {
		t.Fatalf("retain-all output = %q", outR2.String())
	}
}

func TestLibraryForgetRetainUsage(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{
		{"library", "forget"},
		{"library", "forget", "--domain", "go"},
		{"library", "forget", "--task", "tiny/T1"},
		{"library", "forget", "--domain", "go", "--task", "tiny/T1", "extra"},
		{"library", "retain"},
		{"library", "retain", "--domain", "go"},
		{"library", "retain", "--task", "tiny/T1"},
		{"library", "retain", "--domain", "go", "--task", "tiny/T1", "extra"},
	} {
		c, _, errb := newCLI(nil)
		if code := c.Run(args); code != 2 {
			t.Fatalf("args %v: exit = %d, want 2", args, code)
		}
		want := "usage: regulus library " + args[1]
		if !strings.Contains(errb.String(), want) {
			t.Fatalf("args %v: stderr must print %q, got %q", args, want, errb.String())
		}
	}
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"library", "forget", "--domain"}); code != 2 {
		t.Fatalf("missing flag value: exit = %d, want 2", code)
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"library", "frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand: exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), "forget|retain") {
		t.Fatalf("unknown subcommand usage must name the new subcommands, got %q", errb2.String())
	}
}

func TestLibraryListDomainEmpty(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"library", "list", "--domain", "typescript"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "typescript: (no lessons admitted yet)") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestLibraryListFlagErrors(t *testing.T) {
	c, _, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"library", "list", "--domain"}); code != 2 {
		t.Fatalf("missing flag value: exit = %d, want 2", code)
	}
	if code := c.Run([]string{"library", "list", "extra"}); code != 2 {
		t.Fatalf("stray positional: exit = %d, want 2", code)
	}
}

func TestLibraryListMalformedLessonRefused(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	lessonDir := filepath.Join(".regulus", "library", "go", "lessons", "les_bad")
	if err := os.MkdirAll(lessonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# les_bad\n\n- **created_at**: not-a-time\n"
	if err := os.WriteFile(filepath.Join(lessonDir, "LESSON.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"library", "list"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "parsing") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestMemorySaveUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"memory", "save"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus memory save") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "save", "--type", "decision", "--title", "t"}); code != 2 {
		t.Fatalf("missing content: exit = %d, want 2", code)
	}
}

func TestMemorySaveRefusesLessonType(t *testing.T) {
	c, _, errb := newCLI(nil)
	t.Chdir(t.TempDir())
	code := c.Run([]string{"memory", "save", "--type", "lesson", "--title", "t", "content"})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "regulus reflect") {
		t.Fatalf("stderr must route lessons through reflect, got %q", errb.String())
	}
}

func TestMemorySaveSearchHistoryContext(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "chose postgres", "--topic-key", "db", "pooling everywhere"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "moved to sqlite", "--topic-key", "db", "simpler ops"}); code != 0 {
		t.Fatalf("save 2: exit = %d", code)
	}
	c2, out, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "postgres"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	if !strings.Contains(out.String(), "chose postgres") || !strings.Contains(out.String(), "1 hit(s)") {
		t.Fatalf("search output = %q", out.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"memory", "history", "--topic-key", "db"}); code != 0 {
		t.Fatalf("history: exit = %d", code)
	}
	if !strings.Contains(out3.String(), "chose postgres") || !strings.Contains(out3.String(), "moved to sqlite") {
		t.Fatalf("history output = %q", out3.String())
	}
	c4, _, _ := newCLI(nil)
	if code := c4.Run([]string{"memory", "note", "f3 in flight"}); code != 0 {
		t.Fatalf("note: exit = %d", code)
	}
	c5, out5, _ := newCLI(nil)
	if code := c5.Run([]string{"memory", "context"}); code != 0 {
		t.Fatalf("context: exit = %d", code)
	}
	if !strings.Contains(out5.String(), "f3 in flight") {
		t.Fatalf("context must surface the latest note, got:\n%s", out5.String())
	}
}

func TestMemoryPinRelateExport(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "v1 decision", "first"}); code != 0 {
		t.Fatalf("save: exit = %d", code)
	}
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "v2 decision", "second"}); code != 0 {
		t.Fatalf("save 2: exit = %d", code)
	}
	// grab the ids via search
	cS, outS, _ := newCLI(nil)
	if code := cS.Run([]string{"memory", "search", "decision"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	ids := regexp.MustCompile(`(obs_[0-9a-f]+)`).FindAllString(outS.String(), -1)
	if len(ids) < 2 {
		t.Fatalf("need two ids, got %v", ids)
	}
	first, second := ids[1], ids[0] // newest first in search output
	// pin / unpin
	cP, outP, _ := newCLI(nil)
	if code := cP.Run([]string{"memory", "pin", first}); code != 0 {
		t.Fatalf("pin: exit = %d", code)
	}
	if !strings.Contains(outP.String(), "pinned: "+first) {
		t.Fatalf("pin output = %q", outP.String())
	}
	cU, outU, _ := newCLI(nil)
	if code := cU.Run([]string{"memory", "unpin", first}); code != 0 {
		t.Fatalf("unpin: exit = %d", code)
	}
	if !strings.Contains(outU.String(), "unpinned: "+first) {
		t.Fatalf("unpin output = %q", outU.String())
	}
	// relate
	cR, outR, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "relate", "--id", first, "--relation", "supersedes", "--to", second}); code != 0 {
		t.Fatalf("relate: exit = %d", code)
	}
	if !strings.Contains(outR.String(), "supersedes") {
		t.Fatalf("relate output = %q", outR.String())
	}
	// relate with bad relation
	cB, _, errb := newCLI(nil)
	if code := cB.Run([]string{"memory", "relate", "--id", first, "--relation", "kinda", "--to", second}); code != 1 {
		t.Fatalf("bad relation: exit = %d", code)
	}
	if !strings.Contains(errb.String(), "relation") {
		t.Fatalf("bad relation stderr = %q", errb.String())
	}
	// export
	cE, outE, _ := newCLI(nil)
	if code := cE.Run([]string{"memory", "export"}); code != 0 {
		t.Fatalf("export: exit = %d", code)
	}
	if !strings.Contains(outE.String(), "almanac.jsonl") || !strings.Contains(outE.String(), "INDEX.md") {
		t.Fatalf("export output = %q", outE.String())
	}
	idx, err := os.ReadFile(filepath.Join(".regulus-export", "INDEX.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(idx), "v1 decision") || !strings.Contains(string(idx), "supersedes") {
		t.Fatalf("INDEX = \n%s", string(idx))
	}
}

func TestSearchShowsCommitsBehind(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-m", "x")
	head1 := strings.TrimSpace(mustRun(t, dir, "git", "rev-parse", "HEAD"))
	// save an observation pinned to head1
	var out, errb bytes.Buffer
	c := CLI{Version: "test", Probe: nil, Stdout: &out, Stderr: &errb}
	c.Probe = func() doctor.Result { return doctor.Result{Detected: "x", Major: 2, Supported: true} }
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "old observation", "made at head1"}); code != 0 {
		t.Fatalf("save: exit=%d %s", code, errb.String())
	}
	// record the git head in the provenance: we re-save with a runner override is complex;
	// instead patch the observation directly via almanac file
	// (the CLI save doesn't carry git head for non-lesson types; the distance test works via the runner)
	git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-m", "y")
	git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-m", "z")
	// rewrite the observation's provenance with head1
	path := filepath.Join(".regulus", "almanac", "observations.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	patched := strings.Replace(string(data), `"provenance":{"git_head":""`, `"provenance":{"git_head":"`+head1+`"`, 1)
	if patched == string(data) {
		t.Skip("provenance patch not found — git_head empty already means no distance")
	}
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "observation"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	if !strings.Contains(out2.String(), "commits behind") {
		t.Fatalf("search must annotate git distance, got:\n%s", out2.String())
	}
}

func mustRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return string(out)
}

func TestSurfaceProposeListApplyUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"surface"}); code != 2 {
		t.Fatalf("surface bare: exit = %d", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus surface") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"surface", "propose", "--target", "cmd/main.go", "--content", "x", "--reason", "r"}); code != 1 {
		t.Fatalf("whitelist refusal: exit = %d", code)
	}
	if !strings.Contains(errb2.String(), "whitelist") {
		t.Fatalf("whitelist stderr = %q", errb2.String())
	}
	c3, _, _ := newCLI(nil)
	if code := c3.Run([]string{"surface", "propose", "--target", "docs/x.md", "--content", "c", "--reason", "r"}); code != 0 {
		t.Fatalf("propose: exit = %d", code)
	}
	c4, out4, _ := newCLI(nil)
	if code := c4.Run([]string{"surface", "list"}); code != 0 {
		t.Fatalf("list: exit = %d", code)
	}
	if !strings.Contains(out4.String(), "proposed") || !strings.Contains(out4.String(), "docs/x.md") {
		t.Fatalf("list output = %q", out4.String())
	}
	c5, _, _ := newCLI(nil)
	if code := c5.Run([]string{"surface", "apply"}); code != 2 {
		t.Fatalf("apply bare: exit = %d", code)
	}
	c6, _, _ := newCLI(nil)
	if code := c6.Run([]string{"surface", "frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand: exit = %d", code)
	}
}

func TestSurfaceApplyRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	// Apply never creates files: the target must already exist.
	if err := os.MkdirAll("docs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("docs", "learned.md"), []byte("# Learned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"surface", "propose", "--target", "docs/learned.md", "--content", "## Learned: surface edits work", "--reason", "V2 proved it"}); code != 0 {
		t.Fatalf("propose: exit = %d", code)
	}
	idPattern := regexp.MustCompile(`proposed: (sfx_[0-9a-f]+)`)
	m := idPattern.FindStringSubmatch(out.String())
	if len(m) != 2 {
		t.Fatalf("no sfx id in %q", out.String())
	}
	id := m[1]
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"surface", "apply", "--id", id, "--guardian", "regulus-reviewer"}); code != 0 {
		t.Fatalf("apply: exit = %d", code)
	}
	if !strings.Contains(out2.String(), "applied") || !strings.Contains(out2.String(), "regulus-reviewer") {
		t.Fatalf("apply output = %q", out2.String())
	}
	data, err := os.ReadFile(filepath.Join("docs", "learned.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## Learned: surface edits work") || !strings.Contains(string(data), "regulus:surface-edit") {
		t.Fatalf("learned.md = \n%s", string(data))
	}
	c3, _, errb3 := newCLI(nil)
	if code := c3.Run([]string{"surface", "apply", "--id", id, "--guardian", "regulus-reviewer"}); code != 1 {
		t.Fatalf("re-apply: exit = %d", code)
	}
	if !strings.Contains(errb3.String(), "already applied") {
		t.Fatalf("re-apply stderr = %q", errb3.String())
	}
}

func TestCommitsBehindSuffix(t *testing.T) {
	// with a fake runner returning count output
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-list --count abc123..HEAD": {stdout: "42\n"},
	}}
	c, _, _ := newCLI(nil)
	c.Runner = r
	suffix := c.commitsBehindSuffix(almanac.Observation{
		Provenance: &almanac.Provenance{GitHead: "abc123"},
	})
	if !strings.Contains(suffix, "42 commits behind") {
		t.Fatalf("suffix = %q", suffix)
	}
	// no provenance → empty
	c2, _, _ := newCLI(nil)
	c2.Runner = r
	if s := c2.commitsBehindSuffix(almanac.Observation{}); s != "" {
		t.Fatalf("no provenance: suffix = %q", s)
	}
	// git error → empty (graceful)
	rErr := &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-list --count bad..HEAD": {err: errors.New("bad ref")},
	}}
	c3, _, _ := newCLI(nil)
	c3.Runner = rErr
	if s := c3.commitsBehindSuffix(almanac.Observation{Provenance: &almanac.Provenance{GitHead: "bad"}}); s != "" {
		t.Fatalf("error: suffix = %q", s)
	}
}

func TestChartInitAndList(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"chart", "init", "my-feature"}); code != 0 {
		t.Fatalf("init: exit = %d", code)
	}
	slashPath := filepath.Join("charts", "my-feature.md")
	if !strings.Contains(out.String(), slashPath) {
		t.Fatalf("init output = %q", out.String())
	}
	// the scaffold carries the Domain template line
	data, err := os.ReadFile(slashPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Domain: <name>") {
		t.Fatalf("scaffold must carry the Domain template line, got:\n%s", data)
	}
	// the scaffold passes spec check
	cS, outS, _ := newCLI(nil)
	if code := cS.Run([]string{"spec", "check", "charts/my-feature.md"}); code != 0 {
		t.Fatalf("scaffold spec check: exit = %d", code)
	}
	if !strings.Contains(outS.String(), "spec OK: 1 scenario(s)") {
		t.Fatalf("scaffold must pass spec check, got %q", outS.String())
	}
	// double init is refused
	cD, _, errb := newCLI(nil)
	if code := cD.Run([]string{"chart", "init", "my-feature"}); code != 1 {
		t.Fatalf("double init: exit = %d", code)
	}
	if !strings.Contains(errb.String(), "already exists") {
		t.Fatalf("double init stderr = %q", errb.String())
	}
	// invalid name
	cI, _, _ := newCLI(nil)
	if code := cI.Run([]string{"chart", "init", "my feature"}); code != 2 {
		t.Fatalf("invalid name: exit = %d", code)
	}
	// list shows the chart with its scenario count
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"chart", "list"}); code != 0 {
		t.Fatalf("list: exit = %d", code)
	}
	if !strings.Contains(out2.String(), slashPath) || !strings.Contains(out2.String(), "OK (1 scenario)") {
		t.Fatalf("list output = %q", out2.String())
	}
	// usage
	c3, _, errb3 := newCLI(nil)
	if code := c3.Run([]string{"chart"}); code != 2 {
		t.Fatalf("chart bare: exit = %d", code)
	}
	if !strings.Contains(errb3.String(), "usage: regulus chart") {
		t.Fatalf("stderr = %q", errb3.String())
	}
}

func TestChartListEmpty(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"chart", "list"}); code != 0 {
		t.Fatalf("list empty: exit = %d", code)
	}
	if !strings.Contains(out.String(), "(no charts)") {
		t.Fatalf("list output = %q", out.String())
	}
}

func TestLibraryDomainsEmpty(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"library", "domains"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if out.String() != "" {
		t.Fatalf("output = %q, want empty", out.String())
	}
}

func TestLibraryDomainsListsSorted(t *testing.T) {
	c, out, _ := newCLI(nil)
	root := t.TempDir()
	t.Chdir(root)
	for _, d := range []string{"ts", "go"} {
		if err := os.MkdirAll(filepath.Join(root, ".regulus", "library", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if code := c.Run([]string{"library", "domains"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if out.String() != "go\nts\n" {
		t.Fatalf("output = %q, want sorted domains", out.String())
	}
}

func TestLibraryDomainsRejectsExtraArgs(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"library", "domains", "extra"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus library domains") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestTraceUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"trace"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus trace record") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"trace", "frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("stderr = %q", errb2.String())
	}
}

func TestTraceRecordShowCycle(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "record", "--session", "ses1", "--message", "msg1", "--call", "call1", "--tool", "read", "--status", "completed"}); code != 0 {
		t.Fatalf("record exit = %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "trace recorded") {
		t.Fatalf("record output = %q", out.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"trace", "record", "--session", "ses1", "--message", "msg2", "--call", "call2", "--tool", "write", "--status", "error"}); code != 0 {
		t.Fatalf("record 2 exit = %d, stderr %q", code, errb2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"trace", "show", "--session", "ses1"}); code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	if got, want := out3.String(), "1 read completed\n2 write error\n"; got != want {
		t.Fatalf("show output = %q, want %q", got, want)
	}
}

func TestTraceRecordUsage(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "record"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus trace record") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"trace", "record", "--session", "s", "--message", "m", "--call", "c", "--tool", "t"}); code != 2 {
		t.Fatalf("missing status: exit = %d, want 2", code)
	}
	c3, _, _ := newCLI(nil)
	if code := c3.Run([]string{"trace", "record", "--session", "s", "--message", "m", "--call", "c", "--tool", "t", "--status", "completed", "extra"}); code != 2 {
		t.Fatalf("stray positional: exit = %d, want 2", code)
	}
}

func TestTraceRecordBadStatusRefused(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "record", "--session", "s", "--message", "m", "--call", "c", "--tool", "t", "--status", "done"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "completed or error") {
		t.Fatalf("stderr must name the valid statuses, got %q", errb.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"trace", "show", "--session", "s"}); code != 0 {
		t.Fatalf("show exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "(no trace for session s)") {
		t.Fatalf("refused record must leave the trace empty, got %q", out2.String())
	}
}

func TestTraceShowEmptySession(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"trace", "show", "--session", "ghost"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got, want := out.String(), "(no trace for session ghost)\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestTraceShowUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "show"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus trace show") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c2.Run([]string{"trace", "show", "--session", "s", "extra"}); code != 2 {
		t.Fatalf("stray positional: exit = %d, want 2", code)
	}
}

func TestProbeUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"probe"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus probe contrast") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"probe", "frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand: exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("stderr = %q", errb2.String())
	}
}

func TestProbeContrastUsage(t *testing.T) {
	t.Chdir(t.TempDir())
	cases := [][]string{
		{"probe", "contrast"},
		{"probe", "contrast", "--task", "demo/T1"},
		{"probe", "contrast", "--task", "demo/T1", "--gate", "typecheck", "--a", "echo hi"},
		{"probe", "contrast", "--task", "demo/T1", "--gate", "typecheck", "--a", "echo hi", "--b", "echo bye", "extra"},
		{"probe", "contrast", "--task", "../evil", "--gate", "typecheck", "--a", "echo hi", "--b", "echo bye"},
	}
	for _, args := range cases {
		c, _, errb := newCLI(nil)
		if code := c.Run(args); code != 2 {
			t.Fatalf("args %v: exit = %d, want 2", args, code)
		}
		if !strings.Contains(errb.String(), "usage: regulus probe contrast") && !strings.Contains(errb.String(), "invalid task id") {
			t.Fatalf("args %v: stderr must carry usage, got %q", args, errb.String())
		}
	}
	// a bare flag parse error is also a usage error (flag defaults go to
	// stderr, as with the other commands)
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"probe", "contrast", "--task"}); code != 2 {
		t.Fatalf("missing flag value: exit = %d, want 2", code)
	}
}

func TestProbeContrastCycle(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"echo hi":                     {stdout: "hi\n"},
		"echo bye":                    {stdout: "bye\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, out, errb := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"probe", "contrast", "--task", "demo/T1", "--gate", "typecheck", "--a", "echo hi", "--b", "echo bye"}); code != 0 {
		t.Fatalf("contrast exit = %d, stderr %q", code, errb.String())
	}
	for _, want := range []string{"echo hi", "echo bye", "hi", "bye", "- A:", "+ B:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("contrast output must contain %q, got:\n%s", want, out.String())
		}
	}
	found, _ := filepath.Glob(filepath.Join(".regulus", "evidence", "demo__T1", "probe", "*.json"))
	if len(found) != 2 {
		t.Fatalf("want 2 real probe receipts under the probe dir, got %v", found)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"budget", "show"}); code != 0 {
		t.Fatalf("budget show exit = %d", code)
	}
	if !strings.Contains(out2.String(), "probe-run") || !strings.Contains(out2.String(), "sha256") {
		t.Fatalf("contrast must record one probe-run entry with the diff digest, got:\n%s", out2.String())
	}
}

func TestProbeContrastIdentical(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"typecheck","layer":1,"command":"echo same"}]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"echo same":                   {stdout: "same\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, out, errb := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"probe", "contrast", "--task", "demo/T1", "--gate", "typecheck", "--a", "echo same", "--b", "echo same"}); code != 0 {
		t.Fatalf("identical contrast exit = %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "no differences") {
		t.Fatalf("identical receipts must be reported, got %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"budget", "show"}); code != 0 {
		t.Fatalf("budget show exit = %d", code)
	}
	if !strings.Contains(out2.String(), "probe-run") {
		t.Fatalf("identical runs still record the probe, got:\n%s", out2.String())
	}
}

func TestProbeContrastRefusalRecordsNothing(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, _, errb := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"probe", "contrast", "--task", "demo/T1", "--gate", "nope", "--a", "echo hi", "--b", "echo bye"}); code != 1 {
		t.Fatalf("unknown gate: exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), `unknown gate "nope"`) {
		t.Fatalf("stderr must name the unknown gate, got %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	c2.Runner = r
	t.Chdir(t.TempDir())
	if code := c2.Run([]string{"probe", "contrast", "--task", "demo/T1", "--gate", "typecheck", "--a", "echo hi", "--b", "echo bye"}); code != 1 {
		t.Fatalf("missing regulus.json: exit = %d, want 1", code)
	}
	if !strings.Contains(errb2.String(), "no regulus.json") {
		t.Fatalf("stderr must name the missing config, got %q", errb2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"budget", "show"}); code != 0 {
		t.Fatalf("budget show exit = %d", code)
	}
	if !strings.Contains(out3.String(), "(no budget entries)") {
		t.Fatalf("refused probes must leave no probe entry, got:\n%s", out3.String())
	}
	_ = dir
}

func TestProbeComposeUsage(t *testing.T) {
	t.Chdir(t.TempDir())
	cases := [][]string{
		{"probe", "compose"},
		{"probe", "compose", "--task", "demo/T1"},
		{"probe", "compose", "--task", "demo/T1", "--gate", "typecheck", "--base", "echo base", "--a", "echo hi"},
		{"probe", "compose", "--task", "demo/T1", "--gate", "typecheck", "--base", "echo base", "--a", "echo hi", "--b", "echo bye", "extra"},
		{"probe", "compose", "--task", "../evil", "--gate", "typecheck", "--base", "echo base", "--a", "echo hi", "--b", "echo bye"},
	}
	for _, args := range cases {
		c, _, errb := newCLI(nil)
		if code := c.Run(args); code != 2 {
			t.Fatalf("args %v: exit = %d, want 2", args, code)
		}
		if !strings.Contains(errb.String(), "usage: regulus probe compose") && !strings.Contains(errb.String(), "invalid task id") {
			t.Fatalf("args %v: stderr must carry usage, got %q", args, errb.String())
		}
	}
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"probe", "compose", "--task"}); code != 2 {
		t.Fatalf("missing flag value: exit = %d, want 2", code)
	}
}

func TestProbeComposeCycle(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"echo base":                   {stdout: "l1\nl2\nl3\n"},
		"echo hi":                     {stdout: "A1\nl2\nl3\n"},
		"echo bye":                    {stdout: "l1\nl2\nB3\n"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, out, errb := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"probe", "compose", "--task", "demo/T1", "--gate", "typecheck", "--base", "echo base", "--a", "echo hi", "--b", "echo bye"}); code != 0 {
		t.Fatalf("compose exit = %d, stderr %q", code, errb.String())
	}
	for _, want := range []string{"## Composed stdout", "A1\nl2\nB3", "(no conflicts)"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("compose output must contain %q, got:\n%s", want, out.String())
		}
	}
	found, _ := filepath.Glob(filepath.Join(".regulus", "evidence", "demo__T1", "probe", "*.json"))
	if len(found) != 3 {
		t.Fatalf("want 3 real probe receipts under the probe dir, got %v", found)
	}
	leaked, _ := filepath.Glob(filepath.Join(".regulus", "evidence", "demo__T1", "*.json"))
	if len(leaked) != 0 {
		t.Fatalf("compose must not write gate-level receipts, got %v", leaked)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"budget", "show"}); code != 0 {
		t.Fatalf("budget show exit = %d", code)
	}
	if !strings.Contains(out2.String(), "probe-run") || !strings.Contains(out2.String(), "sha256") {
		t.Fatalf("compose must record one probe-run entry with the report digest, got:\n%s", out2.String())
	}
}

func TestProbeComposeConflictIsEvidence(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"echo base":                   {stdout: "out-base\n", code: 0},
		"echo hi":                     {stdout: "out-a\n", code: 1},
		"echo bye":                    {stdout: "out-b\n", code: 2},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, out, errb := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"probe", "compose", "--task", "demo/T1", "--gate", "typecheck", "--base", "echo base", "--a", "echo hi", "--b", "echo bye"}); code != 0 {
		t.Fatalf("conflicted compose exit = %d (conflicts are evidence, not failure), stderr %q", code, errb.String())
	}
	for _, want := range []string{"## Conflicts", "- exit_code: base 0, A 1, B 2", "out-a", "out-b", "## Composed exit_code\n0\n"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("compose output must contain %q, got:\n%s", want, out.String())
		}
	}
}

func TestProbeComposeRefusalRecordsNothing(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	c, _, errb := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"probe", "compose", "--task", "demo/T1", "--gate", "nope", "--base", "echo base", "--a", "echo hi", "--b", "echo bye"}); code != 1 {
		t.Fatalf("unknown gate: exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), `unknown gate "nope"`) {
		t.Fatalf("stderr must name the unknown gate, got %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	c2.Runner = r
	t.Chdir(t.TempDir())
	if code := c2.Run([]string{"probe", "compose", "--task", "demo/T1", "--gate", "typecheck", "--base", "echo base", "--a", "echo hi", "--b", "echo bye"}); code != 1 {
		t.Fatalf("missing regulus.json: exit = %d, want 1", code)
	}
	if !strings.Contains(errb2.String(), "no regulus.json") {
		t.Fatalf("stderr must name the missing config, got %q", errb2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"budget", "show"}); code != 0 {
		t.Fatalf("budget show exit = %d", code)
	}
	if !strings.Contains(out3.String(), "(no budget entries)") {
		t.Fatalf("refused composes must leave no probe entry, got:\n%s", out3.String())
	}
	_ = dir
}

func TestTraceRecordFilesAndFilesCmd(t *testing.T) {
	c, _, _ := newCLI(nil)
	root := t.TempDir()
	t.Chdir(root)
	if code := c.Run([]string{"trace", "record", "--session", "s", "--message", "m", "--call", "c1", "--tool", "gate_run", "--status", "completed", "--files", "a.go,b.go"}); code != 0 {
		t.Fatalf("record exit = %d", code)
	}
	c2, _, _ := newCLI(nil)
	_ = c2
	if code := c.Run([]string{"trace", "record", "--session", "s", "--message", "m", "--call", "c2", "--tool", "memory_search", "--status", "error", "--files", "b.go,c.go"}); code != 0 {
		t.Fatalf("record exit = %d", code)
	}
	cf, fout, _ := newCLI(nil)
	if code := cf.Run([]string{"trace", "files", "--session", "s"}); code != 0 {
		t.Fatalf("files exit = %d", code)
	}
	if fout.String() != "a.go\nb.go\nc.go\n" {
		t.Fatalf("files = %q, want distinct in call order", fout.String())
	}
	show, sout, _ := newCLI(nil)
	if code := show.Run([]string{"trace", "show", "--session", "s"}); code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	if sout.String() != "1 gate_run completed\n2 memory_search error\n" {
		t.Fatalf("fingerprint changed with files, got %q", sout.String())
	}
}

func TestTraceFilesEmptyAndUsage(t *testing.T) {
	c, out, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := c.Run([]string{"trace", "files", "--session", "nobody"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(no trace for session nobody)") {
		t.Fatalf("output = %q", out.String())
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"trace", "files"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus trace files") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestMemorySearchPathBucket(t *testing.T) {
	c, _, _ := newCLI(nil)
	root := t.TempDir()
	t.Chdir(root)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "picked x", "--path", "internal/cli/cli.go", "why x"}); code != 0 {
		t.Fatalf("save exit = %d", code)
	}
	cs, out, _ := newCLI(nil)
	if code := cs.Run([]string{"memory", "search", "--path", "internal/cli/cli.go", "picked"}); code != 0 {
		t.Fatalf("search exit = %d", code)
	}
	if !strings.Contains(out.String(), "picked x") {
		t.Fatalf("path bucket missed, got %q", out.String())
	}
	cm, outm, _ := newCLI(nil)
	if code := cm.Run([]string{"memory", "search", "--path", "elsewhere.go", "picked"}); code != 0 {
		t.Fatalf("search exit = %d", code)
	}
	if !strings.Contains(outm.String(), "no matching observations") {
		t.Fatalf("path miss should be empty, got %q", outm.String())
	}
}

func TestStatusReportsTimePerTask(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("regulus.json", []byte(`{"gates":[{"name":"typecheck","layer":1,"command":"echo ok"}],"evolution":{"max_per_day":3}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1700000000, 0).UTC()
	write := func(task, slug string, ms int64) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(".regulus", "evidence", slug), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := receipts.Write(filepath.Join(".regulus", "evidence", slug), receipts.Receipt{
			Task: task, Gate: "typecheck", Layer: 1, Command: "c",
			ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: start, DurationMS: ms,
		}); err != nil {
			t.Fatal(err)
		}
	}
	write("t/1", "t__1", 500)
	write("t/1", "t__1", 1500)
	write("t/2", "t__2", 125000)
	c, out, _ := newCLI(func() doctor.Result {
		return doctor.Result{Detected: "opencode 2.1.4", Major: 2, Supported: true}
	})
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"status"}); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	for _, want := range []string{"time:       t/1 2s (2 receipt(s))", "time:       t/2 2m5s (1 receipt(s))"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status must report %q, got:\n%s", want, out.String())
		}
	}
}

func TestSkillsListCycle(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll(filepath.Join(".opencode", "skills", "wf"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: wf-flow\ndescription: Fires when work starts.\n---\n\n## What I do\nThings.\n"
	if err := os.WriteFile(filepath.Join(".opencode", "skills", "wf", "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"skills", "list"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "wf-flow — Fires when work starts.") {
		t.Fatalf("output = %q", out.String())
	}
	empty, _, _ := newCLI(nil)
	t.Chdir(t.TempDir())
	if code := empty.Run([]string{"skills", "list"}); code != 0 {
		t.Fatalf("empty exit = %d", code)
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"skills", "bogus"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus skills list") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestEvolveProposeCycle(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.MkdirAll(filepath.Join(".regulus", "evidence", "demo__T1"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1700000000, 0).UTC()
	if _, err := receipts.Write(filepath.Join(".regulus", "evidence", "demo__T1"), receipts.Receipt{
		Task: "demo/T1", Gate: "typecheck", Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: start, DurationMS: 7,
	}); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "propose", "--task", "demo/T1", "--domain", "go"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{
		"# Evidence pack — task demo/T1, domain go",
		"## Task receipts",
		"## Candidate template",
		"dimension:",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("pack must contain %q, got:\n%s", want, out.String())
		}
	}
}

func TestEvolveProposeRefusals(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "propose", "--task", "demo/T9", "--domain", "go"}); code != 1 {
		t.Fatalf("no-evidence exit = %d, want 1", code)
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"evolve", "propose", "--task", "demo/T1"}); code != 2 {
		t.Fatalf("usage exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evolve propose") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c3, _, errb3 := newCLI(nil)
	if code := c3.Run([]string{"evolve", "bogus"}); code != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", code)
	}
	if !strings.Contains(errb3.String(), "unknown subcommand") {
		t.Fatalf("stderr = %q", errb3.String())
	}
}

func TestInitScaffoldsFreshRepo(t *testing.T) {
	for _, stack := range []string{"go", "node", "smoke"} {
		root := t.TempDir()
		t.Chdir(root)
		c, out, _ := newCLI(nil)
		if code := c.Run([]string{"init", "--stack", stack}); code != 0 {
			t.Fatalf("[%s] exit = %d", stack, code)
		}
		data, err := os.ReadFile("regulus.json")
		if err != nil {
			t.Fatalf("[%s] regulus.json: %v", stack, err)
		}
		cfg, err := gates.Load("regulus.json")
		if err != nil || len(cfg.Gates) == 0 {
			t.Fatalf("[%s] config must validate with gates: %+v (%v)", stack, cfg, err)
		}
		_ = data
		if st, err := os.Stat("charts"); err != nil || !st.IsDir() {
			t.Fatalf("[%s] charts/ missing", stack)
		}
		gi, err := os.ReadFile(".gitignore")
		if err != nil || !strings.Contains(string(gi), ".regulus/") {
			t.Fatalf("[%s] .gitignore must ignore .regulus/: %q (%v)", stack, gi, err)
		}
		if !strings.Contains(out.String(), "initialized:") {
			t.Fatalf("[%s] output = %q", stack, out.String())
		}
	}
}

func TestInitRefusesOverwriteAndBadStack(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.WriteFile("regulus.json", []byte(`{"gates":[{"name":"g","layer":1,"command":"true"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"init"}); code != 1 {
		t.Fatalf("overwrite exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "already exists") {
		t.Fatalf("stderr = %q", errb.String())
	}
	t.Chdir(t.TempDir())
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"init", "--stack", "cobol"}); code != 2 {
		t.Fatalf("bad stack exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), "unknown stack") {
		t.Fatalf("stderr = %q", errb2.String())
	}
}

func TestInitKeepsExistingGitignoreEntry(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".gitignore", []byte("node_modules/\n.regulus/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"init", "--stack", "smoke"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	gi, _ := os.ReadFile(".gitignore")
	if strings.Count(string(gi), ".regulus/") != 1 {
		t.Fatalf("must not duplicate the entry, got:\n%s", gi)
	}
}

func TestMemoryPruneDryRunByDefault(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	if _, err := almanac.Save(".", &almanac.Observation{Type: almanac.TypeDecision, Title: "stale", Content: "c"}, old); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "prune"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "stale") || !strings.Contains(out.String(), "dry run") {
		t.Fatalf("dry run must list and refuse: %q", out.String())
	}
	all, _ := almanac.All(".")
	if len(all) != 1 {
		t.Fatal("dry run must not remove anything")
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "prune", "--yes"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "pruned 1 observation(s)") {
		t.Fatalf("output = %q", out2.String())
	}
	all, _ = almanac.All(".")
	if len(all) != 0 {
		t.Fatalf("after --yes, log = %+v", all)
	}
}

func TestMemoryPruneKeepsLessonsAndPinned(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	if _, err := almanac.Save(".", &almanac.Observation{Type: almanac.TypeLesson, Title: "old lesson", Content: "c",
		Lesson: &almanac.Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r"}, Confidence: "low"}}, old); err != nil {
		t.Fatal(err)
	}
	pinned, err := almanac.Save(".", &almanac.Observation{Type: almanac.TypeDecision, Title: "keep me", Content: "c"}, old)
	if err != nil {
		t.Fatal(err)
	}
	if err := almanac.Pin(".", pinned.ID, true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "prune", "--yes"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "nothing to prune") {
		t.Fatalf("lessons+pinned must be immune, got %q", out.String())
	}
}

func TestTraceSessionsAndPruneCycle(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	rec := func(s string) {
		t.Helper()
		c, _, _ := newCLI(nil)
		if code := c.Run([]string{"trace", "record", "--session", s, "--message", "m", "--call", "c", "--tool", "t", "--status", "completed"}); code != 0 {
			t.Fatalf("record exit = %d", code)
		}
	}
	rec("s-a")
	rec("s-b")
	rec("s-b")
	cs, sout, _ := newCLI(nil)
	if code := cs.Run([]string{"trace", "sessions"}); code != 0 {
		t.Fatalf("sessions exit = %d", code)
	}
	if sout.String() != "s-a\ns-b\n" {
		t.Fatalf("sessions = %q", sout.String())
	}
	cp, pout, _ := newCLI(nil)
	if code := cp.Run([]string{"trace", "prune", "--session", "s-b", "--keep", "1"}); code != 0 {
		t.Fatalf("prune exit = %d", code)
	}
	if !strings.Contains(pout.String(), "1 entr(y/ies) kept") {
		t.Fatalf("output = %q", pout.String())
	}
	entries, err := trace.List(".", "s-b")
	if err != nil || len(entries) != 1 {
		t.Fatalf("after prune = %+v (%v)", entries, err)
	}
}

func TestInitHarnessFromCopiesConfigNotEnvironment(t *testing.T) {
	src := t.TempDir()
	oc := filepath.Join(src, ".opencode")
	for _, p := range []string{
		filepath.Join(oc, "discipline.md"),
		filepath.Join(oc, "tool-guidance.md"),
		filepath.Join(oc, "agents", "regulus-proposer.md"),
		filepath.Join(oc, "skills", "regulus-workflow", "SKILL.md"),
		filepath.Join(oc, "plugins", "regulus.ts"),
		filepath.Join(oc, "node_modules", "junk.js"),
		filepath.Join(oc, "package.json"),
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(t.TempDir())
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"init", "--harness-from", src}); code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), ".opencode copied from") {
		t.Fatalf("output = %q", out.String())
	}
	for _, p := range []string{
		filepath.Join(".opencode", "discipline.md"),
		filepath.Join(".opencode", "tool-guidance.md"),
		filepath.Join(".opencode", "agents", "regulus-proposer.md"),
		filepath.Join(".opencode", "skills", "regulus-workflow", "SKILL.md"),
		filepath.Join(".opencode", "plugins", "regulus.ts"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("config %s must be copied: %v", p, err)
		}
	}
	for _, p := range []string{
		filepath.Join(".opencode", "node_modules"),
		filepath.Join(".opencode", "package.json"),
	} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("environment %s must NOT be copied", p)
		}
	}
}

func TestInitHarnessFromRefusals(t *testing.T) {
	// missing .opencode in the source
	empty := t.TempDir()
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"init", "--harness-from", empty}); code != 1 {
		t.Fatalf("missing source exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "no .opencode directory") {
		t.Fatalf("stderr = %q", errb.String())
	}
	// target already has .opencode
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, ".opencode", "skills", "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, ".opencode", "skills", "s", "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".opencode", 0o755); err != nil {
		t.Fatal(err)
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"init", "--harness-from", src}); code != 1 {
		t.Fatalf("overwrite exit = %d, want 1", code)
	}
	if !strings.Contains(errb2.String(), "refusing to overwrite") {
		t.Fatalf("stderr = %q", errb2.String())
	}
}

func TestMemorySearchMarksRelations(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	a, err := almanac.Save(".", &almanac.Observation{Type: almanac.TypeDecision, Title: "old choice", Content: "x"}, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	b, err := almanac.Save(".", &almanac.Observation{Type: almanac.TypeDecision, Title: "new choice", Content: "x"}, time.Unix(1700000060, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := almanac.Relate(".", b.ID, almanac.RelationSupersedes, a.ID, time.Unix(1700000120, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if err := almanac.Relate(".", a.ID, almanac.RelationConflictsWith, b.ID, time.Unix(1700000130, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "search", "choice"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(out.String(), "old choice") || !strings.Contains(out.String(), b.ID) {
		t.Fatalf("superseded must hide by default (its line, not its id-as-conflict-partner), got %q", out.String())
	}
	if !strings.Contains(out.String(), "[conflicts: "+a.ID+"]") {
		t.Fatalf("conflict must be marked on the visible side, got %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "--include-superseded", "choice"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "[superseded by "+b.ID+"]") || !strings.Contains(out2.String(), "[conflicts: "+b.ID+"]") {
		t.Fatalf("include-superseded must mark both sides, got %q", out2.String())
	}
}

func TestMemoryDoctorCycle(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if _, err := almanac.Save(".", &almanac.Observation{Type: almanac.TypeDecision, Title: "a", Content: "c"}, time.Unix(1700000000, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "doctor"}); code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, errb.String())
	}
	for _, want := range []string{"observations: 1", "decision: 1", "malformed:  none", "orphan pins: none", "orphan relations: none"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor must report %q, got:\n%s", want, out.String())
		}
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"memory", "doctor", "extra"}); code != 2 {
		t.Fatalf("usage exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), "usage: regulus memory doctor") {
		t.Fatalf("stderr = %q", errb2.String())
	}
}

func TestMemoryImportCycle(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := almanac.Save(".", &almanac.Observation{Type: almanac.TypeDecision, Title: "mine", Content: "c"}, time.Unix(1700000000, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	bundle := "bundle.jsonl"
	obs, _ := almanac.All(".")
	raw, _ := json.Marshal(obs[0])
	other := `{"id":"obs_cli1","type":"discovery","title":"theirs","content":"c","saved_at":"2026-10-26T08:00:00Z"}`
	if err := os.WriteFile(bundle, []byte(string(raw)+"\n"+other+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "import", bundle}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "imported 1, skipped 1") {
		t.Fatalf("output = %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "import", bundle}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out2.String(), "imported 0, skipped 2") {
		t.Fatalf("re-import output = %q", out2.String())
	}
	c3, _, _ := newCLI(nil)
	if code := c3.Run([]string{"memory", "import"}); code != 2 {
		t.Fatalf("usage exit = %d, want 2", code)
	}
}
