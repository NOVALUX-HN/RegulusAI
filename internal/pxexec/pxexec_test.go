// Executor tests (F15.2/NOV-117): contract, classification,
// redaction, timeout/cancel and the block classifier. Every test
// launches only inbox-signed or toolchain binaries; nothing fresh
// or unsigned is spawned, so the suite is deterministic under SAC.
package pxexec

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	if err := (Spec{}).Validate(); err == nil {
		t.Fatal("empty name must be refused")
	}
	if err := (Spec{Name: "x", Shell: "sh"}).Validate(); err == nil {
		t.Fatal("shell without justification must be refused")
	}
	if err := (Spec{Name: "x", Shell: "sh", Justify: "needs glob"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Spec{Name: "x", Timeout: -1}).Validate(); err == nil {
		t.Fatal("unbounded launches must be refused")
	}
}

func TestSkipNeverPasses(t *testing.T) {
	r := Run(context.Background(), Spec{Name: "x", SkipReason: "no toolchain"})
	if r.Outcome != OutcomeSkipped || r.ErrClass != ErrNone {
		t.Fatalf("skip must be SKIPPED, got %+v", r)
	}
}

func TestGoVersionPasses(t *testing.T) {
	r := Run(context.Background(), Spec{Name: "go", Args: []string{"version"}, Timeout: 30 * time.Second})
	if r.Outcome != OutcomePass || r.Exit != 0 || r.PID <= 0 {
		t.Fatalf("go version must PASS with a PID, got %+v", r)
	}
	if !strings.Contains(r.Stdout, "go version") {
		t.Fatalf("stdout must carry the version, got %q", r.Stdout)
	}
}

func TestMissingBinaryIsLaunchNotFail(t *testing.T) {
	r := Run(context.Background(), Spec{Name: "definitely-not-a-binary-xyz", Timeout: 10 * time.Second})
	if r.Outcome != OutcomeNotRun || r.ErrClass != ErrLaunch {
		t.Fatalf("missing binary must be NOT_RUN/launch, got %+v", r)
	}
}

func TestExitCodeIsFail(t *testing.T) {
	r := Run(context.Background(), Spec{Name: "go", Args: []string{"badflag-xyz"}, Timeout: 30 * time.Second})
	if r.Outcome != OutcomeFail || r.ErrClass != ErrFail || r.Exit == 0 {
		t.Fatalf("bad flag must be FAIL with nonzero exit, got %+v", r)
	}
}

func TestTimeoutIsNotRun(t *testing.T) {
	r := Run(context.Background(), Spec{Name: "ping", Args: []string{"-n", "6", "127.0.0.1"}, Timeout: 500 * time.Millisecond})
	if r.Outcome != OutcomeNotRun || r.ErrClass != ErrTimeout {
		t.Fatalf("expired launch must be NOT_RUN/timeout, got %+v", r)
	}
}

func TestCancelIsNotRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Run(ctx, Spec{Name: "ping", Args: []string{"-n", "6", "127.0.0.1"}, Timeout: 30 * time.Second})
	if r.Outcome != OutcomeNotRun || (r.ErrClass != ErrCancel && r.ErrClass != ErrTimeout) {
		t.Fatalf("cancelled launch must be NOT_RUN, got %+v", r)
	}
}

func TestBlockSignalClassifiesBlocked(t *testing.T) {
	// Real blocks never exit 0 (the process never starts): fail with
	// the policy marker on stderr, mirroring a SAC denial.
	r := Run(context.Background(), Spec{Name: "powershell", Args: []string{"-NoProfile", "-Command", "Write-Error 'blocked this file by policy'; exit 1"}, Timeout: 30 * time.Second})
	if r.Outcome != OutcomeBlocked || r.ErrClass != ErrPolicy || r.BlockHint == "" {
		t.Fatalf("policy signal must be BLOCKED with a hint, got %+v", r)
	}
}

func TestPassOutputIsNotBlocked(t *testing.T) {
	// Exit 0 with block-like words in stdout stays PASS: classifying it
	// BLOCKED would be a false positive on diagnostic chatter.
	r := Run(context.Background(), Spec{Name: "powershell", Args: []string{"-NoProfile", "-Command", "Write-Output 'blocked this file by policy (simulated log line)'"}, Timeout: 30 * time.Second})
	if r.Outcome != OutcomePass {
		t.Fatalf("exit-0 output must stay PASS, got %+v", r)
	}
}

func TestArgsWithSpacesPassThrough(t *testing.T) {
	r := Run(context.Background(), Spec{Name: "powershell", Args: []string{"-NoProfile", "-Command", "Write-Output 'a b c'"}, Timeout: 30 * time.Second})
	if r.Outcome != OutcomePass || !strings.Contains(r.Stdout, "a b c") {
		t.Fatalf("spaced args must pass through intact, got %+v", r)
	}
}

func TestScrubParity(t *testing.T) {
	// Pinned against internal/observability.RedactText vectors (kept
	// as literals: importing observability here would cycle via
	// manifest once callers migrate. If observability/redact.go
	// patterns change, update this copy deliberately.)
	vectors := map[string]string{
		"bearer abc123":        "bearer [redacted]",
		`{"api_key": "xyz"}`:   `{"api_key": "[redacted]"}`,
		"token=abc123":         "token=[redacted]",
		"nothing to hide here": "nothing to hide here",
	}
	for in, want := range vectors {
		if got := scrubCredentials(in); got != want {
			t.Fatalf("scrub drift for %q: got %q, want %q", in, got, want)
		}
	}
}

func TestRenderRedactsSecrets(t *testing.T) {
	s := Spec{Name: "deploy", Args: []string{"--token=abc123", "--env=prod"}, Secrets: []string{"token"}}
	out := s.Render()
	if strings.Contains(out, "abc123") || !strings.Contains(out, "prod") {
		t.Fatalf("secrets must be redacted, benign kept: %q", out)
	}
}
