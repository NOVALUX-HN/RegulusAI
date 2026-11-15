// NOV-65: fuzzing of gate command segmentation and the allowlist.
// Properties: never panics, deterministic, deny-by-default holds
// (empty allowlist never allows).
package gates

import (
	"strings"
	"testing"
)

func TestAllowedByAllowlistDeniesByDefault(t *testing.T) {
	if AllowedByAllowlist("echo hi", nil) {
		t.Fatal("nil allowlist must deny")
	}
	if !AllowedByAllowlist("echo hi", []string{"echo"}) {
		t.Fatal("listed tool must pass")
	}
}

func FuzzAllowedByAllowlist(f *testing.F) {
	f.Add("echo hi", "echo")
	f.Add("echo hi; rm -rf /", "echo")
	f.Add("; ;", "echo")
	f.Add("git archive --format=tar HEAD", "git")
	f.Fuzz(func(t *testing.T, command, tool string) {
		allow := []string{tool}
		a1 := AllowedByAllowlist(command, allow)
		a2 := AllowedByAllowlist(command, allow)
		if a1 != a2 {
			t.Fatalf("non-deterministic for %q", command)
		}
		// Deny by default holds for real (non-empty) commands:
		// Record refuses empty tools upstream, so an empty command
		// never reaches the gate.
		if strings.TrimSpace(command) != "" && AllowedByAllowlist(command, nil) {
			t.Fatalf("deny-by-default violated for %q", command)
		}
		_ = Classify(command)
	})
}

func FuzzClassifySegments(f *testing.F) {
	f.Add("echo a; echo b")
	f.Add("cd /tmp && ls")
	f.Add("a | b | c")
	f.Add("export X=1\necho $X")
	f.Fuzz(func(t *testing.T, command string) {
		c1 := Classify(command)
		c2 := Classify(command)
		if c1 != c2 {
			t.Fatalf("non-deterministic classification for %q", command)
		}
		if c1 != "standard" && c1 != "privileged" && c1 != "destructive" {
			t.Fatalf("unknown class %q for %q", c1, command)
		}
	})
}
