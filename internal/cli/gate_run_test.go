package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/gates"
)

func TestGateRunUnknownSubcommand(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"gate", "frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus gate run") {
		t.Fatalf("stderr must print gate usage, got %q", errb.String())
	}
}

func TestGateRunFlagParseError(t *testing.T) {
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"gate", "run", "--badflag", "demo/T1"}); code != 2 {
		t.Fatalf("bad flag: exit = %d, want 2", code)
	}
}

func TestGateRunMissingAndExtraTask(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"gate", "run"}); code != 2 {
		t.Fatalf("no task: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus gate run [--profile NAME] [--dry-run] <task>") {
		t.Fatalf("stderr must print run usage, got %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"gate", "run", "a/T1", "b/T2"}); code != 2 {
		t.Fatalf("two tasks: exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), "usage: regulus gate run [--profile NAME] [--dry-run] <task>") {
		t.Fatalf("stderr must print run usage, got %q", errb2.String())
	}
}

func TestGateRunDryRunUnknownProfile(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"a","layer":1,"command":"echo hi"}]}`)
	t.Chdir(dir)
	c, _, errb := newCLI(nil)
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"gate", "run", "--dry-run", "--profile", "nope", "demo/T1"}); code != 1 {
		t.Fatalf("unknown dry-run profile: exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "unknown profile") {
		t.Fatalf("stderr must name the problem, got %q", errb.String())
	}
}

func TestGateRunDryRunSortsRequiredWhen(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"docs-check","layer":2,"command":"check-docs","required":false,"when":["*.md"],"on_fail":"advisory","reproducibility":"exact"},
		{"name":"typecheck","layer":1,"command":"echo hi"}
	]}`)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	s := out.String()
	firstType := strings.Index(s, "typecheck")
	firstDocs := strings.Index(s, "docs-check")
	if firstType < 0 || firstDocs < 0 || firstType > firstDocs {
		t.Fatalf("dry-run must sort by layer, got:\n%s", s)
	}
	for _, want := range []string{
		"required always", "optional", "when *.md",
		"on_fail=halt", "on_fail=advisory",
		"reproducibility=exact", "reproducibility=unknown",
		"classification=standard",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("dry-run must contain %q, got:\n%s", want, s)
		}
	}
}

func TestGateRunDryRunClampsTimeout(t *testing.T) {
	dir := writeGatesProject(t, `{"execution":{"max_timeout_sec":60},"gates":[
		{"name":"fast","layer":1,"command":"echo fast","timeout_sec":5},
		{"name":"slow","layer":2,"command":"echo slow","timeout_sec":9999},
		{"name":"plain","layer":3,"command":"echo plain"}
	]}`)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	s := out.String()
	if !strings.Contains(s, `"echo fast" timeout=5s`) {
		t.Fatalf("small timeout must survive, got:\n%s", s)
	}
	if !strings.Contains(s, `"echo slow" timeout=60s`) {
		t.Fatalf("huge timeout must clamp to max, got:\n%s", s)
	}
	if !strings.Contains(s, `"echo plain" timeout=60s`) {
		t.Fatalf("default timeout must clamp to max, got:\n%s", s)
	}
}

func TestGateRunDryRunIsolation(t *testing.T) {
	cfg := `{"execution":{"docker_image":"img:latest"},"gates":[
		{"name":"boxed","layer":1,"command":"echo hi","isolate":"docker"},
		{"name":"plain","layer":2,"command":"echo yo"}
	]}`
	t.Run("sh-value-docker", func(t *testing.T) {
		dir := writeGatesProject(t, cfg)
		t.Chdir(dir)
		c, out, _ := newCLI(nil)
		c.Runner = gates.ShRunner{}
		if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		s := out.String()
		if !strings.Contains(s, "isolation=docker") {
			t.Fatalf("docker gate on sh must be docker, got:\n%s", s)
		}
		if !strings.Contains(s, "isolation=host-contained") {
			t.Fatalf("plain gate on sh must be host-contained, got:\n%s", s)
		}
	})
	t.Run("sh-pointer-host", func(t *testing.T) {
		dir := writeGatesProject(t, cfg)
		t.Chdir(dir)
		c, out, _ := newCLI(nil)
		c.Runner = &gates.ShRunner{}
		if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "isolation=docker") {
			t.Fatalf("pointer sh must also resolve isolation, got:\n%s", out.String())
		}
	})
	t.Run("fake-unknown", func(t *testing.T) {
		dir := writeGatesProject(t, cfg)
		t.Chdir(dir)
		c, out, _ := newCLI(nil)
		c.Runner = &fakeGateRunner{}
		if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "isolation=unknown") {
			t.Fatalf("fake runner must be unknown, got:\n%s", out.String())
		}
		if strings.Contains(out.String(), "isolation=docker") {
			t.Fatalf("fake runner must never claim docker, got:\n%s", out.String())
		}
	})
}

func TestGateRunDryRunReproducibility(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"a","layer":1,"command":"echo a","reproducibility":"exact"},
		{"name":"b","layer":2,"command":"echo b","reproducibility":"statistical"},
		{"name":"c","layer":3,"command":"echo c"}
	]}`)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	s := out.String()
	for _, want := range []string{
		"reproducibility=exact", "reproducibility=statistical", "reproducibility=unknown",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("dry-run must contain %q, got:\n%s", want, s)
		}
	}
}

func TestGateRunDryRunNilRunnerDefaultsToSh(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"a","layer":1,"command":"echo hi"}]}`)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	// Runner stays nil: parse must default to the real shell runner.
	if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "isolation=host-contained") {
		t.Fatalf("nil runner must default to sh isolation, got:\n%s", out.String())
	}
}

func TestExecAndReportReadFileError(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	plan := gateRunArgs{task: "demo/T1", runner: &fakeGateRunner{}}
	if code := c.execAndReport(plan); code != 1 {
		t.Fatalf("missing regulus.json: exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "gate run:") {
		t.Fatalf("stderr must carry the gate run prefix, got %q", errb.String())
	}
}

func TestDryRunGateLineEdgeClamp(t *testing.T) {
	g := gates.Gate{Name: "a", Layer: 1, Command: "echo hi"}
	line := dryRunGateLine(g, 0, "", true)
	if !strings.Contains(line, "timeout=1s") {
		t.Fatalf("max 0 must floor at 1s, got %q", line)
	}
	// Unvalidated values keep their spelling: Load rejects them before the
	// CLI can print them, so cover the fallback directly.
	bogus := dryRunGateLine(gates.Gate{Name: "b", Layer: 1, Command: "echo b", Reproducibility: "bogus"}, 60, "", true)
	if !strings.Contains(bogus, "reproducibility=bogus") {
		t.Fatalf("unknown reproducibility must pass through, got %q", bogus)
	}
	if !dryRunIsSh(gates.ShRunner{}) || !dryRunIsSh(&gates.ShRunner{}) {
		t.Fatal("sh value and pointer must both count as sh")
	}
	if dryRunIsSh(&fakeGateRunner{}) {
		t.Fatal("fake runner must not count as sh")
	}
}

func TestGateRunDryRunWritesNothing(t *testing.T) {
	dir := writeGatesProject(t, `{"execution":{"max_timeout_sec":60,"docker_image":"img:latest"},"gates":[
		{"name":"b","layer":2,"command":"echo b","timeout_sec":5,"isolate":"docker","reproducibility":"statistical","required":false,"when":["*.go"],"on_fail":"advisory"},
		{"name":"a","layer":1,"command":"echo a"}
	]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "--dry-run", "demo/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	s := out.String()
	if strings.Index(s, `"echo a"`) > strings.Index(s, `"echo b"`) {
		t.Fatalf("plan must stay in layer order, got:\n%s", s)
	}
	if len(r.calls) != 0 {
		t.Fatalf("dry-run must execute NOTHING, calls=%v", r.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, ".regulus")); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(filepath.Join(dir, ".regulus"))
		t.Fatalf("dry-run must create no files, .regulus contains %v", entries)
	}
}
