package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/doctor"
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
