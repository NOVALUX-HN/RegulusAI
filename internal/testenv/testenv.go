// Test environment guards (NOV-65, S5): tools the suite needs must
// be present. Require skips the test when the tool is absent — unless
// REGULUS_STRICT_ENV=1, when the absence fails instead. CI sets the
// strict flag so silently-skipped coverage can never go green;
// environments without the tool keep the informative skip.
package testenv

import (
	"os"
	"os/exec"
	"testing"
)

// Strict reports whether skips must fail instead of skipping.
func Strict() bool { return os.Getenv("REGULUS_STRICT_ENV") == "1" }

// Require asserts tool is on PATH: skip (or fail under strict).
func Require(t *testing.T, tool string) {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		if Strict() {
			t.Fatalf("%s not on PATH and REGULUS_STRICT_ENV=1 (skip refused)", tool)
		}
		t.Skipf("%s not available", tool)
	}
}
