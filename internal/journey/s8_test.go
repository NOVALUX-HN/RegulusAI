package journey

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/executil"
	"regulus-ai/internal/receipts"
)

// TestS8GateEfficacy guards the mutation/vulncheck gates against
// echo-skipped fallbacks: a gate whose command masks failure with
// `|| echo ... skipped` always exits 0 without doing work, so a green
// gauntlet would prove nothing. The repo-root regulus.json and the
// gauntlet workflow must contain no such fallback (S8).
func TestS8GateEfficacy(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	// regulus.json: every gate command must do real work — no
	// `|| echo ... skipped` mask that turns a missing tool into a pass.
	data, err := os.ReadFile(filepath.Join(repoRoot, "regulus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Gates []struct {
			Name    string `json:"name"`
			Command string `json:"command"`
		} `json:"gates"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("regulus.json must parse: %v", err)
	}
	if len(cfg.Gates) == 0 {
		t.Fatal("regulus.json must declare at least one gate")
	}
	foundVuln := false
	foundMutation := false
	for _, g := range cfg.Gates {
		if g.Name == "vulncheck" {
			foundVuln = true
		}
		if g.Name == "mutation" {
			foundMutation = true
		}
		lower := strings.ToLower(g.Command)
		if strings.Contains(g.Command, "||") && strings.Contains(lower, "echo") && strings.Contains(lower, "skipp") {
			t.Fatalf("gate %q masks failure with an echo-skipped fallback: %q — a missing tool must fail the gate, not pass it", g.Name, g.Command)
		}
	}
	if !foundVuln {
		t.Fatal("regulus.json must declare the vulncheck gate")
	}
	if !foundMutation {
		t.Fatal("regulus.json must declare the mutation gate")
	}

	// gauntlet.yml: no step may pass via an echo-skipped fallback either.
	yml, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "gauntlet.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(yml), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(line, "||") && strings.Contains(lower, "echo") && strings.Contains(lower, "skipp") {
			t.Fatalf("gauntlet.yml masks failure with an echo-skipped fallback: %q", strings.TrimSpace(line))
		}
	}
}

// s8Run drives the real CLI in-process against a fresh fixture directory,
// following the existing journey fixture style (v06/f4): a cold CLI per
// call, state on disk only, deterministic with no model or network.
func s8Run(t *testing.T, args ...string) (int, string) {
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

// TestS8YoloOnOffJourney walks safe-YOLO end to end: status starts off
// without a config, on creates the template with the destructive denies,
// status reports the edit mode, off restores ask without leaving edits
// auto-approved, and denies survive the round trip.
func TestS8YoloOnOffJourney(t *testing.T) {
	t.Chdir(t.TempDir())

	code, out := s8Run(t, "yolo", "status")
	if code != 0 || !strings.Contains(out, "yolo: off") {
		t.Fatalf("status without config: exit=%d out=%q", code, out)
	}

	code, out = s8Run(t, "yolo", "on")
	if code != 0 || !strings.Contains(out, "yolo: on") {
		t.Fatalf("yolo on: exit=%d out=%q", code, out)
	}
	data, err := os.ReadFile("opencode.json")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, deny := range []string{`"rm *": "deny"`, `"git rm*": "deny"`, `"git checkout --*": "deny"`} {
		if !strings.Contains(text, deny) {
			t.Fatalf("template must keep destructive deny %s, got:\n%s", deny, text)
		}
	}
	if !strings.Contains(text, `"*.env"`) {
		t.Fatalf("template must ask for .env edits, got:\n%s", text)
	}

	code, out = s8Run(t, "yolo", "status")
	if code != 0 || !strings.Contains(out, "yolo: on") || !strings.Contains(out, "edit") {
		t.Fatalf("status after on must report on + edit mode: exit=%d out=%q", code, out)
	}

	code, out = s8Run(t, "yolo", "off")
	if code != 0 || !strings.Contains(out, "yolo: off") {
		t.Fatalf("yolo off: exit=%d out=%q", code, out)
	}
	data, err = os.ReadFile("opencode.json")
	if err != nil {
		t.Fatal(err)
	}
	text = string(data)
	if !strings.Contains(text, `"*": "ask"`) {
		t.Fatalf("off must restore the ask catch-all, got:\n%s", text)
	}
	if strings.Contains(text, `"edit": "allow"`) {
		t.Fatalf("off must not leave edits auto-approved, got:\n%s", text)
	}
	for _, deny := range []string{`"rm *": "deny"`, `"git restore*": "deny"`, `"git rebase*": "deny"`} {
		if !strings.Contains(text, deny) {
			t.Fatalf("off must preserve deny %s, got:\n%s", deny, text)
		}
	}

	code, out = s8Run(t, "yolo", "status")
	if code != 0 || !strings.Contains(out, "yolo: off") || !strings.Contains(out, "edit") {
		t.Fatalf("status after off must report off + edit mode: exit=%d out=%q", code, out)
	}
}

var s8SfxIDPattern = regexp.MustCompile(`proposed: (sfx_[0-9a-f]+)`)

// TestS8SurfaceApplyJourney walks the harness-surface gate end to end:
// propose appends nothing yet, apply needs a named guardian, the edit
// lands marker-delimited in the existing whitelisted file, and the
// budget ledger records proposed then applied (KindSurface).
func TestS8SurfaceApplyJourney(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("docs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("docs", "learned.md"), []byte("# Learned\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out := s8Run(t, "surface", "propose", "--target", "docs/learned.md", "--content", "## Learned: surface edits work", "--reason", "S8 journey proves the gate")
	if code != 0 || !strings.Contains(out, "proposed: sfx_") {
		t.Fatalf("propose: exit=%d out=%q", code, out)
	}
	m := s8SfxIDPattern.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("no sfx id in %q", out)
	}
	id := m[1]

	code, out = s8Run(t, "surface", "list")
	if code != 0 || !strings.Contains(out, id) || !strings.Contains(out, "proposed") {
		t.Fatalf("list must show the proposal: exit=%d out=%q", code, out)
	}

	code, out = s8Run(t, "surface", "apply", "--id", id, "--guardian", "regulus-reviewer")
	if code != 0 || !strings.Contains(out, "applied") || !strings.Contains(out, "regulus-reviewer") {
		t.Fatalf("apply: exit=%d out=%q", code, out)
	}
	data, err := os.ReadFile(filepath.Join("docs", "learned.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## Learned: surface edits work") || !strings.Contains(string(data), "regulus:surface-edit "+id) {
		t.Fatalf("applied content must land marker-delimited, got:\n%s", data)
	}

	entries, err := budget.Show(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Kind != budget.KindSurface || entries[0].Outcome != budget.OutcomeSurfaceProposed || entries[1].Kind != budget.KindSurface || entries[1].Outcome != budget.OutcomeSurfaceApplied {
		t.Fatalf("ledger must record proposed then applied, got %+v", entries)
	}
}

// TestS8ProbeComposeTaskCapJourney walks the probe compose per-task cap
// end to end: the first compose on a task runs all three variants and
// records one probe-run entry with real receipts under the task slug's
// probe dir; the second compose on the same task is refused, records
// refused-cap, and runs nothing new.
func TestS8ProbeComposeTaskCapJourney(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("regulus.json", []byte(`{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}],"evolution":{"max_per_day":10,"max_per_task":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out := s8Run(t, "probe", "compose", "--task", "demo/T1", "--gate", "typecheck", "--base", "echo base", "--a", "echo a", "--b", "echo b")
	if code != 0 || !strings.Contains(out, "compose gate") {
		t.Fatalf("first compose: exit=%d out=%q", code, out)
	}
	evidenceDir := filepath.Join(".regulus", "evidence", receipts.Slug("demo/T1"), "probe")
	before, err := filepath.Glob(filepath.Join(evidenceDir, "*.json"))
	if err != nil || len(before) != 3 {
		t.Fatalf("first compose must write 3 probe receipts, got %v (%v)", before, err)
	}

	code, out = s8Run(t, "probe", "compose", "--task", "demo/T1", "--gate", "typecheck", "--base", "echo base", "--a", "echo a", "--b", "echo b")
	if code == 0 || !strings.Contains(out, "task evolution cap") {
		t.Fatalf("second compose must hit the per-task cap: exit=%d out=%q", code, out)
	}
	after, err := filepath.Glob(filepath.Join(evidenceDir, "*.json"))
	if err != nil || len(after) != 3 {
		t.Fatalf("refused compose must write no new receipts, got %v (%v)", after, err)
	}

	entries, err := budget.Show(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want probe-run + refused-cap, got %+v", entries)
	}
	if entries[0].Kind != budget.KindProbe || entries[0].Outcome != budget.OutcomeProbeRun || entries[0].Task != "demo/T1" {
		t.Fatalf("first entry must be the probe run, got %+v", entries[0])
	}
	last := entries[len(entries)-1]
	if last.Kind != budget.KindProbe || last.Outcome != budget.OutcomeRefusedCap || last.Task != "demo/T1" {
		t.Fatalf("last entry must be the probe refusal, got %+v", last)
	}
}

// TestS8WindowsHideJourney proves hidden children still do real work:
// HideWindow keeps a direct command usable, the gate runner (which
// marks every child via executil.HideWindow) still passes a real gate
// with a receipt, and the plugin spawns with windowsHide so the
// opencode server stays windowless on Windows.
func TestS8WindowsHideJourney(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())

	cmd := exec.Command("go", "version")
	executil.HideWindow(cmd)
	if cmd.Path == "" {
		t.Fatal("HideWindow must not clear the command path")
	}
	if out, err := cmd.CombinedOutput(); err != nil || len(out) == 0 {
		t.Fatalf("command must still run after HideWindow: %v %q", err, out)
	}

	if err := os.WriteFile("regulus.json", []byte(`{"gates":[{"name":"typecheck","layer":1,"command":"echo windows-hide-probe"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := s8Run(t, "gate", "run", "hide/T1")
	if code != 0 || !strings.Contains(out, "gauntlet: 1/1 gates passed") {
		t.Fatalf("hidden gate run: exit=%d out=%q", code, out)
	}
	got, err := filepath.Glob(filepath.Join(".regulus", "evidence", receipts.Slug("hide/T1"), "*.json"))
	got = withoutManifests(got)
	if err != nil || len(got) != 1 {
		t.Fatalf("hidden gate must leave one receipt, got %v (%v)", got, err)
	}

	plugin, err := os.ReadFile(filepath.Join(repoRoot, ".opencode", "plugins", "regulus.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plugin), "windowsHide: true") {
		t.Fatal("plugin must spawn the core with windowsHide: true")
	}
}
