// NOV-65: the strict flag turns a missing-tool skip into a failure.
package testenv

import (
	"testing"
)

func TestRequirePresentPasses(t *testing.T) {
	Require(t, "go")
}

func TestStrictFlagReadsEnv(t *testing.T) {
	t.Setenv("REGULUS_STRICT_ENV", "1")
	if !Strict() {
		t.Fatal("strict must be true with the flag set")
	}
}
