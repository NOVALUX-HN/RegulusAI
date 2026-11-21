// Coverage for the missing-tool branches of testenv.Require: the
// skip path executes in-process (strict explicitly off, so the skip
// is the asserted behavior, not an accident), while the strict
// fail-closed path executes in a re-executed child of the same test
// binary (portable: os.Executable, no shell) whose nonzero exit the
// parent asserts. Probes use exec.LookPath with t.Skip when the
// tool is absent. No sleeps anywhere.
package testenv

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestStrictDefaultsFalse(t *testing.T) {
	t.Setenv("REGULUS_STRICT_ENV", "")
	if Strict() {
		t.Fatal("strict must default to false without the flag")
	}
}

func TestRequireMissingSkipsDocumented(t *testing.T) {
	// The skip below IS the behavior under test: with strict off, a
	// missing tool skips instead of failing. Run it in a subtest so
	// the parent still passes and the skip stays visible.
	t.Run("skip", func(t *testing.T) {
		t.Setenv("REGULUS_STRICT_ENV", "")
		Require(t, "definitely-not-a-regulus-tool-xyz")
		t.Fatal("Require must skip before returning on a missing tool")
	})
}

func TestRequireGoPresent(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go not on PATH: %v", err)
	}
	Require(t, "go")
}

// TestTestenvStrictChild is re-executed by
// TestRequireStrictRefusesMissing with the strict flag set: it must
// fail closed on the missing tool. Never run directly.
func TestTestenvStrictChild(t *testing.T) {
	if os.Getenv("GO_WANT_TESTENV_HELPER") != "1" {
		return
	}
	Require(t, "definitely-not-a-regulus-tool-xyz")
}

func TestRequireStrictRefusesMissing(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("cannot locate the test binary: %v", err)
	}
	cmd := exec.Command(exe, "-test.run", "TestTestenvStrictChild$")
	cmd.Env = append(os.Environ(),
		"GO_WANT_TESTENV_HELPER=1",
		"REGULUS_STRICT_ENV=1",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("strict child must fail closed on a missing tool, out: %s", out)
	}
	if !strings.Contains(string(out), "skip refused") {
		t.Fatalf("strict refusal must name the refused skip, got:\n%s", out)
	}
}

func TestProbeLookPathAbsent(t *testing.T) {
	if _, err := exec.LookPath("definitely-not-a-regulus-tool-xyz"); err == nil {
		t.Fatal("phantom tool must not resolve on PATH")
	}
}
