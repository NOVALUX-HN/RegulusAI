package execbackend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"regulus-ai/internal/pxexec"
)

// CommandRunner runs argv and returns its combined output with the
// launch error. It is a seam: production injects the default runner,
// tests inject a fake so no suite ever launches a process.
type CommandRunner func(ctx context.Context, argv []string) (string, error)

// DefaultRun is the production runner. It goes through pxexec like
// every other spawn in Regulus: later process work owns console
// hiding, the minimal environment, output caps and the BLOCKED
// classification, so this package never re-implements any of it.
func DefaultRun(ctx context.Context, argv []string) (string, error) {
	if len(argv) == 0 {
		return "", fmt.Errorf("execbackend: empty argv")
	}
	res := pxexec.Run(ctx, pxexec.Spec{
		Name:    argv[0],
		Args:    argv[1:],
		EnvMode: "minimal",
		Timeout: 5 * time.Minute,
	})
	out := res.Combined
	if out == "" {
		out = res.Stdout + res.Stderr
	}
	if res.Outcome == pxexec.OutcomeBlocked {
		// The executor already proved the refusal; surface its own
		// diagnostic so IsPolicySignal matches without guessing.
		return out + " " + res.Detail + " " + res.BlockHint, fmt.Errorf("launch blocked by policy")
	}
	if res.Outcome == pxexec.OutcomePass {
		return out, nil
	}
	return out + " " + res.Detail, fmt.Errorf("outcome %s (exit %d)", res.Outcome, res.Exit)
}

// WSLExe is the launcher every WSL backend invocation goes through.
// It is a constant so the probe, the distro listing and Wrap can never
// disagree about which binary they mean.
const WSLExe = "wsl.exe"

// DetectDistros lists installed WSL distros. A non-Windows host has
// none by definition: the sandbox is a Windows-only workaround and
// claiming otherwise would hide a real policy block on Linux.
//
// The listing is parsed defensively because `wsl.exe -l -q` emits
// UTF-16-ish output on some builds: NUL bytes, CRLF line endings and a
// trailing blank line all appear in practice.
func DetectDistros(ctx context.Context, run CommandRunner) []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	if run == nil {
		run = DefaultRun
	}
	out, err := run(ctx, []string{WSLExe, "-l", "-q"})
	if err != nil && strings.TrimSpace(out) == "" {
		return nil
	}
	return parseDistros(out)
}

// parseDistros turns a `wsl.exe -l -q` listing into distro names.
// Split out so the defensive parsing is testable on every platform,
// including the ones where WSL does not exist.
func parseDistros(out string) []string {
	var distros []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.ReplaceAll(line, "\x00", "")
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		distros = append(distros, line)
	}
	return distros
}

// policyMarkers are the phrases an application-control refusal
// actually produces. They are matched case-insensitively against the
// launch error and its output.
//
// The list stays deliberately narrow: a false positive would switch a
// healthy host to a sandbox for no reason, which is worse than letting
// the native path fail loudly. Generic failures ("access is denied",
// "permission denied") are NOT markers — they can come from antivirus,
// filesystem permissions or a corrupt binary.
var policyMarkers = []string{
	"control de aplicaciones",
	"application control",
	"app control",
	"blocked this file",
	"code integrity",
	"smart app control",
	"policy blocked",
}

// IsPolicySignal reports whether the text carries an explicit
// application-control refusal, plus the marker that matched so the
// reason stays auditable.
func IsPolicySignal(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, m := range policyMarkers {
		if strings.Contains(lower, m) {
			return m, true
		}
	}
	return "", false
}

// HostProbe tests empirically whether this host can execute a freshly
// built binary: it compiles a minimal program and runs it.
//
// Empiricism is the point. Reading a registry key or inferring from a
// Windows version would guess; compiling and launching is exactly the
// operation `go test` needs, so a pass here means the suite can run.
//
// Conservative by design: only an explicit application-control marker
// reports a block. A build failure, a missing toolchain or an
// unrecognised launch error reports "not blocked" with the reason
// attached, so the native path keeps its existing behaviour and the
// real error surfaces where it happens.
func HostProbe(ctx context.Context, run CommandRunner, goBin string) (bool, string) {
	if runtime.GOOS != "windows" {
		return false, "host is " + runtime.GOOS + "; no Windows application-control policy applies"
	}
	if run == nil {
		run = DefaultRun
	}
	if goBin == "" {
		goBin = "go"
	}

	dir, err := os.MkdirTemp("", "regulus-probe-")
	if err != nil {
		return false, "probe skipped: " + err.Error()
	}
	defer func() { _ = os.RemoveAll(dir) }()

	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		return false, "probe skipped: " + err.Error()
	}
	exe := filepath.Join(dir, "probe.exe")
	if out, err := run(ctx, []string{goBin, "build", "-o", exe, src}); err != nil {
		return false, "probe inconclusive: go build failed: " + firstLine(out+err.Error())
	}

	out, runErr := run(ctx, []string{exe})
	if runErr == nil {
		return false, "freshly built binary executes on this host"
	}
	text := out + " " + runErr.Error()
	if marker, blocked := IsPolicySignal(text); blocked {
		return true, "application-control policy refused a freshly built binary (matched " + marker + ")"
	}
	return false, "probe inconclusive: " + firstLine(text)
}

// firstLine keeps diagnostics to one line so a reason field stays
// readable in a receipt.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
