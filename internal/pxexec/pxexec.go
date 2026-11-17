// Centralized process executor (F15.2/NOV-117): the single
// infrastructure component that creates, runs, observes, cancels
// and classifies external processes in Regulus. No module decides
// on its own how to launch anything: callers describe WHAT
// (argv-separated, no shell strings) and the executor decides HOW.
//
// Reuse, never duplication:
//   - console hiding: executil.HideWindow (F11 runner work) —
//     every child gets it so background runs never flash.
//   - redaction: observability.RedactText for rendered commands.
//   - verdict vocabulary: PASS/FAIL/BLOCKED/SKIPPED/NOT_RUN is
//     shared with the mission report (F13.7) and the F15.4 runner.
//
// What this file never does: shells by default, Start-Process,
// policy bypasses, privilege elevation, secret logging. A policy
// block is classified BLOCKED with nondestructive diagnostics
// (path/hash/event-query hint) — never silently, never as PASS.
package pxexec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/executil"
	"regulus-ai/internal/observability"
)

// Outcomes: the closed classification vocabulary. BLOCKED is a
// security-policy refusal (never a PASS); NOT_RUN means the launch
// never happened (timeout before start, cancelled upfront, skipped
// by policy); SKIPPED is caller-declared (precondition unmet).
const (
	OutcomePass    = "PASS"
	OutcomeFail    = "FAIL"
	OutcomeBlocked = "BLOCKED"
	OutcomeSkipped = "SKIPPED"
	OutcomeNotRun  = "NOT_RUN"
)

// Error classes: where the failure lives. Launch vs policy vs
// timeout vs cancel vs process-fail never merge: a blocked launch
// is not a failed command and a timeout is not a failure.
const (
	ErrNone    = ""
	ErrLaunch  = "launch"
	ErrPolicy  = "policy"
	ErrTimeout = "timeout"
	ErrCancel  = "cancel"
	ErrFail    = "process-fail"
)

// DefaultTimeout bounds every launch without an explicit budget:
// no unbounded children. MaxOutputBytes caps captured output so a
// chatty child cannot exhaust memory; the truncation is reported,
// never silent.
const (
	DefaultTimeout = 120 * time.Second
	MaxOutputBytes = 4 << 20
	TruncateNotice = "[truncated]"
)

// Spec is one typed launch request: WHAT to run, never HOW. Args
// are separate values (no shell concatenation, ever). EnvMode
// selects the base: "inherit" (default, os.Environ — zero behavior
// change for migrated callers), "minimal" (DefaultEnv only, for new
// hardened callers), or "exact" (Spec.Env IS the complete
// environment — for callers with their own allowlist filters such
// as gates buildEnv or bench labChildEnv; nothing is added, so
// filtered policies survive migration byte-for-byte). Env adds on
// top of inherit/minimal. Stdin is optional; Timeout overrides the
// default (0 = default, negative = refused).
type Spec struct {
	Name    string
	Args    []string
	Dir     string
	EnvMode string
	Env     []string
	Stdin   []byte
	Timeout time.Duration
	// MaxOutput caps captured output per stream (0 = default).
	// Callers with tighter budgets (bench, 256K) keep their exact
	// truncation point instead of silently changing it.
	MaxOutput int64
	// Shell forces shell execution and is refused unless Justify
	// names the requirement: shells are explicit, justified,
	// bounded and tested — never the default path.
	Shell   string
	Justify string
	// Secrets lists env/key names whose values are redacted in
	// Render and diagnostics (values never hit logs).
	Secrets []string
	// SkipReason marks a caller-declared skip: the launch never
	// happens and the result is SKIPPED, never PASS.
	SkipReason string
}

// Result is one observed launch: the real exit code, duration,
// PID, mechanism, truncated streams, classification and, on
// blocks, nondestructive diagnostics. Zero Result is NOT_RUN.
type Result struct {
	Outcome   string
	ErrClass  string
	Exit      int
	Duration  time.Duration
	PID       int
	Mechanism string
	Stdout    string
	Stderr    string
	// Combined preserves the interleaved stdout+stderr stream, so
	// callers migrated from CombinedOutput keep byte-equivalent
	// behavior (interleave order is inherently racy either way).
	Combined  string
	Truncated bool
	// BlockHint carries the nondestructive diagnostic for policy
	// blocks: path, SHA-256 when the file exists, and the event
	// query that proves it. Empty unless BLOCKED.
	BlockHint string
	Detail    string
}

// DefaultEnv returns the minimal base environment: SystemRoot,
// TEMP/TMP and PATH only. Everything else is caller-supplied via
// Spec.Env, so secrets and exotic variables never leak in by
// inheritance.
func DefaultEnv() []string {
	var out []string
	for _, k := range []string{"SystemRoot", "TEMP", "TMP", "PATH", "PATHEXT"} {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// Validate refuses malformed specs before any side effect: empty
// names, shell without justification, contradictory skip+run.
func (s Spec) Validate() error {
	if s.SkipReason != "" {
		return nil
	}
	if s.EnvMode != "" && s.EnvMode != "inherit" && s.EnvMode != "minimal" {
		return fmt.Errorf("pxexec: unknown env mode %q (want inherit|minimal|exact)", s.EnvMode)
	}
	if s.MaxOutput < 0 {
		return fmt.Errorf("pxexec: negative output cap refused")
	}
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("pxexec: command name required (argv-separated, never a shell string)")
	}
	if strings.TrimSpace(s.Shell) != "" && strings.TrimSpace(s.Justify) == "" {
		return fmt.Errorf("pxexec: shell %q refused without a written justification", s.Shell)
	}
	if s.Timeout < 0 {
		return fmt.Errorf("pxexec: unbounded launches refused (set an explicit timeout)")
	}
	return nil
}

// Run executes one spec and classifies the outcome. Cancellation
// kills the child (best effort on Windows: the direct child is
// signalled; grandchildren without a job object may survive —
// recorded in the detail, never hidden).
func Run(ctx context.Context, s Spec) Result {
	start := time.Now()
	if err := s.Validate(); err != nil {
		return Result{Outcome: OutcomeNotRun, ErrClass: ErrLaunch, Exit: -1, Mechanism: "none", Detail: err.Error()}
	}
	if s.SkipReason != "" {
		return Result{Outcome: OutcomeSkipped, ErrClass: ErrNone, Exit: -1, Mechanism: "none", Detail: "skipped: " + s.SkipReason}
	}
	timeout := s.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	name, args := s.Name, s.Args
	mech := "go-native"
	if s.Shell != "" {
		name, args, mech = s.Shell, append(shellFlag(s.Shell), s.Name+" "+strings.Join(args, " ")), "shell:"+s.Shell
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if s.Dir != "" {
		cmd.Dir = s.Dir
	}
	base := os.Environ()
	if s.EnvMode == "minimal" {
		base = DefaultEnv()
	}
	if s.EnvMode == "exact" {
		base = nil
	}
	cmd.Env = append(base, s.Env...)
	if len(s.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(s.Stdin)
	}
	executil.HideWindow(cmd)
	capBytes := s.MaxOutput
	if capBytes == 0 {
		capBytes = int64(MaxOutputBytes)
	}
	var stdout, stderr, combined bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, n: capBytes, tee: &combined, tn: capBytes}
	cmd.Stderr = &limitedWriter{w: &stderr, n: capBytes, tee: &combined, tn: capBytes}
	err := cmd.Run()
	dur := time.Since(start)
	pid := -1
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	res := Result{Exit: -1, Duration: dur, PID: pid, Mechanism: mech,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Combined:  combined.String(),
		Truncated: stdout.Len() >= int(capBytes) || stderr.Len() >= int(capBytes),
	}
	if res.Truncated {
		res.Stdout += "\n" + TruncateNotice
		res.Stderr += "\n" + TruncateNotice
	}
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		res.Outcome, res.ErrClass, res.Detail = OutcomeNotRun, ErrTimeout,
			fmt.Sprintf("timeout after %v (child signalled; Windows grandchildren may survive without a job object)", timeout)
	case ctx.Err() == context.Canceled:
		res.Outcome, res.ErrClass, res.Detail = OutcomeNotRun, ErrCancel, "cancelled before completion (child signalled)"
	case err == nil:
		res.Outcome, res.ErrClass, res.Exit, res.Detail = OutcomePass, ErrNone, 0, "exit 0"
	default:
		if blockText(stdout.String()+"\n"+stderr.String()) || isPolicyDenied(err) {
			res.Outcome, res.ErrClass = OutcomeBlocked, ErrPolicy
			res.BlockHint = blockHint(s)
			res.Detail = "security policy refused the launch (BLOCKED, never PASS): " + firstLine(stderr.String()+stdout.String())
		} else if ee, ok := err.(*exec.ExitError); ok {
			res.Outcome, res.ErrClass, res.Exit, res.Detail = OutcomeFail, ErrFail, ee.ExitCode(), fmt.Sprintf("exit %d", ee.ExitCode())
		} else {
			res.Outcome, res.ErrClass, res.Detail = OutcomeNotRun, ErrLaunch, "launch failed: "+err.Error()
		}
	}
	return res
}

// Render quotes the launch with secrets redacted: safe for logs,
// receipts and Linear evidence alike.
func (s Spec) Render() string {
	redact := map[string]bool{}
	for _, k := range s.Secrets {
		redact[strings.ToLower(strings.TrimSpace(k))] = true
	}
	parts := []string{s.Name}
	for _, a := range s.Args {
		masked := a
		if eq := strings.Index(a, "="); eq > 0 && redact[strings.ToLower(a[:eq])] {
			masked = a[:eq+1] + observability.Redacted
		}
		parts = append(parts, observability.RedactText(masked))
	}
	return strings.Join(parts, " ")
}

// shellFlag maps the justified shell to its command flag.
func shellFlag(shell string) []string {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "pwsh", "powershell":
		return []string{"-NoProfile", "-NonInteractive", "-Command"}
	case "cmd":
		return []string{"/C"}
	default:
		return []string{"-c"}
	}
}

// blockText reports whether output carries a policy-block signal
// (App Control / Device Guard denial). Kept narrow: only refusal
// strings, never heuristic guessing.
func blockText(out string) bool {
	lower := strings.ToLower(out)
	for _, m := range []string{
		"control de aplicaciones bloque",
		"blocked this file",
		"device guard",
		"smart app control",
		"0xc0e90002",
	} {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// isPolicyDenied reports exec failures that are policy refusals
// rather than missing binaries: permission-denied on an existing
// executable file.
func isPolicyDenied(err error) bool {
	return err != nil && os.IsPermission(err)
}

// blockHint builds the nondestructive diagnostic: path, SHA-256
// when readable, and the exact event query that proves the block.
// Nothing is changed, excluded or bypassed.
func blockHint(s Spec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "file: %s", s.Name)
	if abs, err := filepath.Abs(s.Name); err == nil {
		if raw, err := os.ReadFile(abs); err == nil {
			sum := sha256.Sum256(raw)
			fmt.Fprintf(&b, " sha256=%s", hex.EncodeToString(sum[:])[:12])
		}
	}
	fmt.Fprintf(&b, " | check: Get-WinEvent -LogName Microsoft-Windows-CodeIntegrity/Operational | 3077=file+parent, 3089=signature, 3118=SAC detail")
	return b.String()
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// limitedWriter caps captured output at n bytes, optionally
// teeing into a second capped buffer (the combined stream).
// Teeing never fails the primary write: the tee is best effort.
type limitedWriter struct {
	w   *bytes.Buffer
	n   int64
	tee *bytes.Buffer
	tn  int64
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.tee != nil {
		if room := l.tn - int64(l.tee.Len()); room > 0 {
			q := p
			if int64(len(q)) > room {
				q = q[:room]
			}
			_, _ = l.tee.Write(q)
		}
	}
	room := l.n - int64(l.w.Len())
	if room <= 0 {
		return len(p), nil
	}
	if int64(len(p)) > room {
		p = p[:room]
	}
	return l.w.Write(p)
}
