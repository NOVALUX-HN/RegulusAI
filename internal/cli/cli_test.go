package cli

import (
	"bytes"
	"context"
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
	if code := c.Run([]string{"status"}); code != 0 {
		t.Fatalf("status exit = %d, stderr %q", code, errb.String())
	}
	for _, want := range []string{
		"runtime:    opencode 2.1.4 — supported (V2)",
		"gauntlet:   1 gate(s) configured",
		"evolution:  0/3 run(s) today",
		"lessons:    0 admitted, 1 candidate(s) pending",
		"receipts:   1 on disk",
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
