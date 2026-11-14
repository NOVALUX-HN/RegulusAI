package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/gates"
)

func stubLookPath(ok map[string]bool) func(string) (string, error) {
	return func(tool string) (string, error) {
		if ok[tool] {
			return "/bin/" + tool, nil
		}
		return "", errors.New("not found")
	}
}

func TestPreflightFirstToken(t *testing.T) {
	for cmd, want := range map[string]string{
		"go run ./cmd/regulus crap --profile x": "go",
		"go build ./...":                        "go",
		"gremlins unleash ./internal/receipts":  "gremlins",
		"govulncheck ./...":                     "govulncheck",
		"bunx tsc --noEmit":                     "bunx",
		"bun test ./lib/":                       "bun",
		`"go" version`:                          "go",
		"C:\\tools\\gremlins.exe unleash":       "gremlins",
		"./regulus.exe crap --profile x":        "regulus",
		"regulus spec check charts/":            "regulus",
		"":                                      "",
	} {
		if got := preflightFirstToken(cmd); got != want {
			t.Fatalf("preflightFirstToken(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func TestPreflightSkipsShellResolved(t *testing.T) {
	// LookPath fails for EVERYTHING: shell builtins and the regulus
	// self-reference must still not be reported; only the truly
	// unknown external tool is.
	never := func(string) (string, error) { return "", errors.New("not found") }
	selected := []gates.Gate{
		{Name: "a", Layer: 1, Command: "echo ok"},
		{Name: "b", Layer: 2, Command: "ECHO caps-builtin"},
		{Name: "c", Layer: 3, Command: "regulus spec check charts/"},
		{Name: "d", Layer: 4, Command: "./regulus.exe crap --profile x"},
		{Name: "e", Layer: 5, Command: "true"},
		{Name: "f", Layer: 6, Command: "nonexistent-tool-xyz --flag"},
	}
	missing := preflightMissingTools(selected, never)
	if len(missing) != 1 {
		t.Fatalf("only the unknown tool may be reported, missing = %v", missing)
	}
	if !strings.Contains(missing[0], "nonexistent-tool-xyz") {
		t.Fatalf("missing must name the unknown tool, got %q", missing[0])
	}
}

func TestPreflightHints(t *testing.T) {
	for tool, want := range map[string]string{
		"gremlins":    "go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0",
		"govulncheck": "go install golang.org/x/vuln/cmd/govulncheck@v1.9.0",
		"bun":         "https://bun.sh",
		"bunx":        "https://bun.sh",
		"go":          "https://go.dev/dl/",
	} {
		if got := gateToolHint(tool); got != want {
			t.Fatalf("gateToolHint(%q) = %q, want %q", tool, got, want)
		}
	}
	if got := gateToolHint("nonexistent-tool-xyz"); got == "" {
		t.Fatal("unknown tools must still carry a hint")
	}
}

func TestPreflightMissingDedupes(t *testing.T) {
	selected := []gates.Gate{
		{Name: "a", Layer: 1, Command: "go build ./..."},
		{Name: "b", Layer: 2, Command: "go vet ./..."},
		{Name: "c", Layer: 3, Command: "nonexistent-tool-xyz --flag"},
		{Name: "d", Layer: 4, Command: "nonexistent-tool-xyz other"},
	}
	missing := preflightMissingTools(selected, stubLookPath(map[string]bool{"go": true}))
	if len(missing) != 1 {
		t.Fatalf("dedupe: missing = %v, want 1 entry", missing)
	}
	if !strings.Contains(missing[0], "nonexistent-tool-xyz") {
		t.Fatalf("missing must name the tool, got %q", missing[0])
	}
	if !strings.Contains(missing[0], "install:") {
		t.Fatalf("missing must carry a hint, got %q", missing[0])
	}
}

func TestPreflightAllPresentNoFail(t *testing.T) {
	selected := []gates.Gate{{Name: "a", Layer: 1, Command: "go version"}}
	if missing := preflightMissingTools(selected, exec.LookPath); len(missing) != 0 {
		t.Fatalf("go is on PATH in dev/CI: missing = %v", missing)
	}
}

func TestGateRunPreflightFailsFast(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"ghost","layer":1,"command":"nonexistent-tool-xyz --flag"}
	]}`)
	t.Chdir(dir)
	c, out, errb := newCLI(nil)
	c.Runner = gates.ShRunner{}
	if code := c.Run([]string{"gate", "run", "--yes", "demo/T1"}); code != 1 {
		t.Fatalf("missing tool: exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "missing required tools:") {
		t.Fatalf("stderr must list the missing tools, got %q", errb.String())
	}
	if !strings.Contains(errb.String(), "nonexistent-tool-xyz") {
		t.Fatalf("stderr must name the tool, got %q", errb.String())
	}
	if !strings.Contains(errb.String(), "install:") {
		t.Fatalf("stderr must carry a hint, got %q", errb.String())
	}
	if strings.Contains(out.String(), "layer 1") || strings.Contains(out.String(), "gauntlet:") {
		t.Fatalf("no layer may run before the preflight fail, stdout:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".regulus")); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(filepath.Join(dir, ".regulus"))
		t.Fatalf("preflight fail must write 0 receipts, .regulus contains %v", entries)
	}
}

func TestGateRunPreflightListsSelectedOnly(t *testing.T) {
	cfg := `{"gates":[
		{"name":"req","layer":1,"command":"go version"},
		{"name":"opt","layer":2,"command":"nonexistent-tool-xyz --flag","required":false,"profiles":["full"]}
	]}`
	t.Run("quick-skips-missing", func(t *testing.T) {
		dir := writeGatesProject(t, cfg)
		t.Chdir(dir)
		c, _, errb := newCLI(nil)
		c.Runner = gates.ShRunner{}
		if code := c.Run([]string{"gate", "run", "--yes", "--profile", "quick", "demo/T1"}); code != 0 {
			t.Fatalf("quick selects required only: exit = %d, want 0, stderr:\n%s", code, errb.String())
		}
	})
	t.Run("full-catches-missing", func(t *testing.T) {
		dir := writeGatesProject(t, cfg)
		t.Chdir(dir)
		c, _, errb := newCLI(nil)
		c.Runner = gates.ShRunner{}
		if code := c.Run([]string{"gate", "run", "--yes", "demo/T1"}); code != 1 {
			t.Fatalf("default selects the missing gate: exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "nonexistent-tool-xyz") {
			t.Fatalf("stderr must name the tool, got %q", errb.String())
		}
		if _, serr := os.Stat(filepath.Join(dir, ".regulus")); !os.IsNotExist(serr) {
			entries, _ := os.ReadDir(filepath.Join(dir, ".regulus"))
			t.Fatalf("preflight fail must write 0 receipts, .regulus contains %v", entries)
		}
	})
}

func TestGateRunPreflightSmokeGoPresent(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"goversion","layer":1,"command":"go version"}
	]}`)
	t.Chdir(dir)
	c, out, errb := newCLI(nil)
	c.Runner = gates.ShRunner{}
	if code := c.Run([]string{"gate", "run", "--yes", "demo/T1"}); code != 0 {
		t.Fatalf("go present: exit = %d, want 0, stderr:\n%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "gauntlet: 1/1 gates passed") {
		t.Fatalf("smoke must pass the gauntlet, got:\n%s", out.String())
	}
}

func TestGateRunDryRunSkipsPreflight(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"ghost","layer":1,"command":"nonexistent-tool-xyz --flag"}
	]}`)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	c.Runner = gates.ShRunner{}
	if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
		t.Fatalf("dry-run only prints the plan: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "nonexistent-tool-xyz") {
		t.Fatalf("plan must show the command, got:\n%s", out.String())
	}
}

func TestGateRunFakeRunnerSkipsPreflight(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"ghost","layer":1,"command":"nonexistent-tool-xyz --flag"}
	]}`)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"gate", "run", "demo/T1"}); code != 0 {
		t.Fatalf("test doubles provide their own commands: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "gauntlet: 1/1 gates passed") {
		t.Fatalf("fake run must pass, got:\n%s", out.String())
	}
}

// writeTrustedMarker marks dir as a trusted repo (S6): gates may run
// there once policy consent (--yes or an allowlist) is satisfied.
func writeTrustedMarker(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".regulus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".regulus", "trusted"), []byte("operator consent marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// S6: an untrusted repo (no .regulus/trusted) without --yes prints the
// plan (command|dir|timeout|isolation) and refuses: exit 1, nothing
// executed, nothing written. In CI the refusal is unconditional without
// --yes (no interactive fallback).
func TestGateRunUntrustedRefusesWithoutYes(t *testing.T) {
	for _, ci := range []string{"", "true"} {
		name := "local"
		if ci != "" {
			name = "ci"
		}
		t.Run(name, func(t *testing.T) {
			if ci != "" {
				t.Setenv("CI", ci)
			}
			dir := writeGatesProject(t, `{"gates":[{"name":"hi","layer":1,"command":"echo hi"}]}`)
			t.Chdir(dir)
			c, out, errb := newCLI(nil)
			c.Runner = gates.ShRunner{}
			if code := c.Run([]string{"gate", "run", "demo/T1"}); code != 1 {
				t.Fatalf("untrusted without --yes: exit = %d, want 1", code)
			}
			s := out.String()
			for _, want := range []string{`"echo hi"`, "dir=", "timeout=", "isolation=", "allowlist=deny"} {
				if !strings.Contains(s, want) {
					t.Fatalf("refusal must print the plan with %q, got:\n%s", want, s)
				}
			}
			stderr := errb.String()
			if !strings.Contains(stderr, "untrusted") || !strings.Contains(stderr, "--yes") {
				t.Fatalf("stderr must demand --yes for the untrusted repo, got %q", stderr)
			}
			if strings.Contains(s, "gauntlet:") {
				t.Fatalf("no gate may run before consent, stdout:\n%s", s)
			}
			if _, err := os.Stat(filepath.Join(dir, ".regulus")); !os.IsNotExist(err) {
				entries, _ := os.ReadDir(filepath.Join(dir, ".regulus"))
				t.Fatalf("untrusted refusal must write NOTHING, .regulus contains %v", entries)
			}
		})
	}
}

// S6: --yes on an untrusted repo proceeds and the executed receipt
// records consent:true. --yes is the explicit consent in every
// environment, CI included.
func TestGateRunUntrustedYesProceedsWithConsent(t *testing.T) {
	for _, ci := range []string{"", "true"} {
		name := "local"
		if ci != "" {
			name = "ci"
		}
		t.Run(name, func(t *testing.T) {
			if ci != "" {
				t.Setenv("CI", ci)
			}
			dir := writeGatesProject(t, `{"gates":[{"name":"hi","layer":1,"command":"echo hi"}]}`)
			t.Chdir(dir)
			c, out, errb := newCLI(nil)
			c.Runner = gates.ShRunner{}
			if code := c.Run([]string{"gate", "run", "--yes", "demo/T1"}); code != 0 {
				t.Fatalf("untrusted with --yes: exit = %d, want 0, stderr:\n%s", code, errb.String())
			}
			if !strings.Contains(out.String(), "gauntlet: 1/1 gates passed") {
				t.Fatalf("consented run must pass, got:\n%s", out.String())
			}
			found, _ := filepath.Glob(filepath.Join(dir, ".regulus", "evidence", "demo__T1", "*.json"))
			found = withoutManifests(found)
			if len(found) != 1 {
				t.Fatalf("want 1 receipt, got %v", found)
			}
			data, err := os.ReadFile(found[0])
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "consent:true") {
				t.Fatalf("consented receipt must record consent:true, got:\n%s", data)
			}
		})
	}
}

// S6: deny-by-default on a TRUSTED repo without --yes refuses WITH a
// Refused receipt (BLOCKED/policy_violation, no exit code): the gate
// never reaches a runner.
func TestGateRunTrustedEmptyAllowlistDeniesWithReceipt(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"hi","layer":1,"command":"echo hi"}]}`)
	writeTrustedMarker(t, dir)
	t.Chdir(dir)
	c, _, errb := newCLI(nil)
	c.Runner = gates.ShRunner{}
	if code := c.Run([]string{"gate", "run", "demo/T1"}); code != 1 {
		t.Fatalf("trusted without allowlist/--yes: exit = %d, want 1, stderr:\n%s", code, errb.String())
	}
	if strings.Contains(errb.String(), "untrusted") {
		t.Fatalf("trusted repo must hit the allowlist denial, not the trust gate, stderr:\n%s", errb.String())
	}
	found, _ := filepath.Glob(filepath.Join(dir, ".regulus", "evidence", "demo__T1", "*.json"))
	found = withoutManifests(found)
	if len(found) != 1 {
		t.Fatalf("denial must write one Refused receipt, got %v", found)
	}
	data, err := os.ReadFile(found[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"status": "BLOCKED"`, `"reason": "policy_violation"`, `"exit_code": -1`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("Refused receipt must contain %s, got:\n%s", want, data)
		}
	}
}

// S6: --dry-run on an untrusted repo only inspects: exit 0, plan on
// stdout, nothing executed, nothing written.
func TestGateRunDryRunUntrustedWritesNothing(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"hi","layer":1,"command":"echo hi"}]}`)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	c.Runner = gates.ShRunner{}
	if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
		t.Fatalf("dry-run on untrusted repo: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), `"echo hi"`) {
		t.Fatalf("dry-run must print the plan, got:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".regulus")); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(filepath.Join(dir, ".regulus"))
		t.Fatalf("dry-run must write NOTHING, .regulus contains %v", entries)
	}
}

// S6: dry-run lines carry the allowlist verdict plus the destructive mark.
func TestDryRunLineAllowlistVerdict(t *testing.T) {
	allow := dryRunGateLine(gates.Gate{Name: "a", Layer: 1, Command: "echo hi"}, 3600, "", true, "echo")
	if !strings.Contains(allow, "allowlist=allow") {
		t.Fatalf("allowlisted command must read allow, got %q", allow)
	}
	if !strings.Contains(allow, "destructive=false") {
		t.Fatalf("standard command must mark destructive=false, got %q", allow)
	}
	deny := dryRunGateLine(gates.Gate{Name: "a", Layer: 1, Command: "echo hi"}, 3600, "", true)
	if !strings.Contains(deny, "allowlist=deny") {
		t.Fatalf("empty allowlist must deny by default, got %q", deny)
	}
	bad := dryRunGateLine(gates.Gate{Name: "x", Layer: 1, Command: "rm -rf /"}, 3600, "", true)
	if !strings.Contains(bad, "destructive=true") {
		t.Fatalf("destructive command must mark destructive=true, got %q", bad)
	}
	if !strings.Contains(bad, "classification=destructive") {
		t.Fatalf("destructive command must keep its classification, got %q", bad)
	}
}
