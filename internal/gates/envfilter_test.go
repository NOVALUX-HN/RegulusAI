package gates

import (
	"slices"
	"strings"
	"testing"
)

// NOV-81: EnvUnset must win even when the gate declares its own Env
// (filter first, gate-declared vars last). Regression test for the old
// RunStructured branch that ignored EnvUnset with cmd.Env present.
func TestBuildEnvUnsetWinsOverExtra(t *testing.T) {
	t.Setenv("PATH", "/bin")
	t.Setenv("DROP", "secret")
	t.Setenv("KEEP", "1")
	got := buildEnv([]string{"EXTRA=yes"}, []string{"DROP"}, nil, true)
	has := func(kv string) bool { return slices.Contains(got, kv) }
	if !has("EXTRA=yes") {
		t.Fatalf("gate-declared vars must survive, got %q", got)
	}
	for _, kv := range got {
		if strings.HasPrefix(kv, "DROP=") {
			t.Fatalf("unset must win over inheritance, got %q", got)
		}
	}
}

// NOV-81: allowlist mode hides non-declared names by default.
func TestBuildEnvAllowlistHidesByDefault(t *testing.T) {
	t.Setenv("REGULUS_TEST_VISIBLE_NOISE", "noise")
	t.Setenv("PATH", "/bin")
	got := buildEnv(nil, nil, nil, false)
	for _, kv := range got {
		if strings.HasPrefix(kv, "REGULUS_TEST_VISIBLE_NOISE=") {
			t.Fatalf("allowlist must hide undeclared names, got %q", got)
		}
	}
	found := false
	for _, kv := range got {
		if strings.HasPrefix(kv, "PATH=") {
			found = true
		}
	}
	if !found {
		t.Fatalf("allowlist must keep PATH, got %q", got)
	}
}

// NOV-81: FullEnv restores legacy inherit-everything (minus unset).
func TestBuildEnvFullEnvEscape(t *testing.T) {
	t.Setenv("REGULUS_TEST_VISIBLE_NOISE", "noise")
	got := buildEnv(nil, []string{"OTHER_UNSET"}, nil, true)
	found := false
	for _, kv := range got {
		if strings.HasPrefix(kv, "REGULUS_TEST_VISIBLE_NOISE=") {
			found = true
		}
	}
	if !found {
		t.Fatalf("FullEnv must inherit everything, got %q", got)
	}
}

// NOV-81: unset values are collected for redaction even though they
// never reach the child environment.
func TestSecretValuesCollectsUnset(t *testing.T) {
	env := []string{"KEEP=1", "GONE=s3cr3t-value"}
	got := SecretValues(env, []string{"GONE"})
	if !slices.Contains(got, "s3cr3t-value") {
		t.Fatalf("unset values must be collected, got %q", got)
	}
	for _, v := range got {
		if v == "1" {
			t.Fatalf("plain values must not be collected, got %q", got)
		}
	}
}
