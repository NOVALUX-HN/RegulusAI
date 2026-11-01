package gates

import (
	"context"
	"strings"
	"testing"
)

// S1: allowlist multi-segmento — cada segmento de splitSegments valida su
// primer token contra la allowlist, con los mismos mensajes `policy:`.

func runOneAllowlist(t *testing.T, command string, allowlist []string, allowDestructive bool) (Result, *fakeRunner) {
	t.Helper()
	cfg := Config{
		Gates: []Gate{{Name: "g", Layer: 1, Command: command}},
		Execution: &ExecutionConfig{
			Allowlist:        allowlist,
			AllowDestructive: allowDestructive,
		},
	}
	r := &fakeRunner{}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	return res, r
}

func mustRefusePolicy(t *testing.T, command string, allowlist []string, allowDestructive bool, wantNote string) {
	t.Helper()
	res, r := runOneAllowlist(t, command, allowlist, allowDestructive)
	if !res.Failed {
		t.Fatalf("%q must be refused (allowlist %v), got %+v", command, allowlist, res)
	}
	if len(r.calls) != 0 {
		t.Fatalf("%q refused gate must execute NOTHING, calls=%v", command, r.calls)
	}
	if res.Status != "BLOCKED" || res.Reason != "policy_violation" {
		t.Fatalf("%q result = %q/%q, want BLOCKED/policy_violation", command, res.Status, res.Reason)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.ExitCode != -1 {
		t.Fatalf("%q receipt exit = %d, want -1", command, rec.ExitCode)
	}
	if !strings.Contains(rec.Stderr, wantNote) {
		t.Fatalf("%q receipt stderr must contain %q, got %q", command, wantNote, rec.Stderr)
	}
}

func mustPassAllowlist(t *testing.T, command string, allowlist []string) {
	t.Helper()
	res, r := runOneAllowlist(t, command, allowlist, false)
	if res.Failed {
		t.Fatalf("%q must pass (allowlist %v), got %+v", command, allowlist, res)
	}
	if len(r.calls) != 1 || r.calls[0] != command {
		t.Fatalf("%q must execute once, calls=%v", command, r.calls)
	}
}

func TestAllowlistMultiSegmentRefuses(t *testing.T) {
	allow := []string{"echo", "go"}
	// Spec pair: each refuses with the existing `policy:` message.
	// `echo hi; rm -rf /` is destructive even with opt-in off; force
	// AllowDestructive so the verdict exercises the per-segment allowlist
	// path (`rm` outside the allowlist) instead of the destructive gate.
	mustRefusePolicy(t, "echo hi; rm -rf /", allow, true, "policy: command not in allowlist")
	mustRefusePolicy(t, "go test || chmod -R /", allow, true, "policy: command not in allowlist")
	// Without opt-in both still refuse (destructive message preserved).
	mustRefusePolicy(t, "echo hi; rm -rf /", allow, false, "policy:")
	mustRefusePolicy(t, "go test || chmod -R /", allow, false, "policy:")
}

func TestAllowlistMultiSegmentNonDestructiveBypass(t *testing.T) {
	allow := []string{"echo", "go"}
	mustRefusePolicy(t, "echo hi; whoami", allow, false, "policy: command not in allowlist")
	mustRefusePolicy(t, "go test && whoami", allow, false, "policy: command not in allowlist")
	mustRefusePolicy(t, "echo hi | whoami", allow, false, "policy: command not in allowlist")
	mustRefusePolicy(t, "echo hi & whoami", allow, false, "policy: command not in allowlist")
}

func TestAllowlistSingleSegmentPass(t *testing.T) {
	allow := []string{"echo", "go"}
	mustPassAllowlist(t, "echo hi", allow)
	mustPassAllowlist(t, "go test ./...", allow)
}

func TestAllowlistQuotedStaysPass(t *testing.T) {
	// Quoted payloads are not segments: stripQuoted already covers them.
	mustPassAllowlist(t, `echo "rm -rf /"`, []string{"echo"})
}

func TestAllowlistVariableRefuses(t *testing.T) {
	// `$RM -rf /` refuses closed (here via the destructive pattern, which
	// lowercases before matching, so the variable form still trips it).
	// Documented limit: the policy cannot resolve expansions, so even
	// `$ECHO hi` with ECHO=echo refuses (fail closed, both paths `policy:`).
	mustRefusePolicy(t, "$RM -rf /", []string{"echo", "go", "rm"}, false, "policy:")
	mustRefusePolicy(t, "${RM} -rf /", []string{"echo", "go", "rm"}, false, "policy:")
	// Non-destructive variable form exercises the allowlist path itself.
	mustRefusePolicy(t, "$WHOAMI", []string{"echo", "go"}, false, "policy: command not in allowlist")
}

func TestAllowlistSubstitutionRefuses(t *testing.T) {
	allow := []string{"echo", "go"}
	mustRefusePolicy(t, "echo $(whoami)", allow, false, "policy: command not in allowlist")
	mustRefusePolicy(t, "echo `whoami`", allow, false, "policy: command not in allowlist")
	mustPassAllowlist(t, "echo $(echo hi)", []string{"echo"})
}

func TestAllowlistEmptySegmentsIgnored(t *testing.T) {
	mustPassAllowlist(t, "echo hi;", []string{"echo"})
	mustPassAllowlist(t, "echo hi || ", []string{"echo"})
}

func TestNoAllowlistEchoOkPasses(t *testing.T) {
	// Journey fixtures (`echo ok` without an execution allowlist) stay green:
	// no allowlist configured means no allowlist enforcement.
	cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "echo ok"}}}
	r := &fakeRunner{}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("echo ok without allowlist must pass: %+v", res)
	}
}

func TestAllowlistSubstitutionQuoting(t *testing.T) {
	echo := []string{"echo"}
	// Single-quoted substitutions are literal: no inner execution.
	mustPassAllowlist(t, "echo '$(whoami)'", echo)
	mustPassAllowlist(t, "echo '`whoami`'", echo)
	// Double-quoted substitutions DO execute.
	mustRefusePolicy(t, `echo "$(whoami)"`, echo, false, "policy: command not in allowlist")
	// Parameter expansion and arithmetic are not commands.
	mustPassAllowlist(t, "echo ${HOME}", echo)
	mustPassAllowlist(t, "echo $((1+1))", echo)
	// Nested substitutions recurse.
	mustRefusePolicy(t, "echo $(echo $(whoami))", echo, false, "policy: command not in allowlist")
	// Unclosed substitution is a shell syntax error, not a hidden command.
	mustPassAllowlist(t, "echo $(whoami", echo)
	// Fully-quoted command name still refuses when outside the allowlist.
	mustRefusePolicy(t, `"whoami"`, echo, false, "policy: command not in allowlist")
}

func TestAllowlistSubstitutionInsideQuotes(t *testing.T) {
	echo := []string{"echo"}
	// Quoted parens/escapes/nesting inside `$(...)` scan without splitting.
	mustPassAllowlist(t, `echo $(echo "a)b")`, echo)
	mustPassAllowlist(t, `echo $(echo 'a)b')`, echo)
	mustPassAllowlist(t, `echo $(echo a\(b)`, echo)
	mustPassAllowlist(t, `echo $(echo (sub))`, echo)
	// Empty substitution is not a command.
	mustPassAllowlist(t, `echo $()`, echo)
	// Escaped opener is literal, not a substitution.
	mustPassAllowlist(t, `echo \$(whoami)`, echo)
	// Unclosed backtick is a shell syntax error, not a hidden command.
	mustPassAllowlist(t, "echo `whoami", echo)
}
