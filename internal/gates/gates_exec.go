package gates

// output capture, native and shell runners (NOV-67 split; behavior unchanged).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"regulus-ai/internal/executil"
)

// outputCapError reports a gate killed for exceeding the output cap.
type outputCapError struct{ bytes int64 }

func (e *outputCapError) Error() string {
	return fmt.Sprintf("output exceeded %d bytes cap", e.bytes)
}

// capWriter buffers up to limit bytes, discards the rest, and kills the
// process once on overflow. It never returns a write error: a failing
// writer would wedge the pipe copy and deadlock the wait. Writes may
// arrive from concurrent pipe-copy goroutines (stdout/stderr streams),
// so total/buf/overflow are guarded by mu; kill fires once per writer
// via once and once globally via the shared killOnce in runShellCap.
type capWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	limit    int
	total    int64
	overflow bool
	once     sync.Once
	kill     func()
}

func (w *capWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.total += int64(len(p))
	if w.buf.Len() < w.limit {
		w.buf.Write(p[:min(len(p), w.limit-w.buf.Len())])
	}
	overflowed := w.total > int64(w.limit) && !w.overflow
	if overflowed {
		w.overflow = true
	}
	w.mu.Unlock()
	if overflowed {
		w.once.Do(func() {
			if w.kill != nil {
				w.kill()
			}
		})
	}
	return len(p), nil
}

// snapshot returns the buffered output, total bytes and overflow flag
// under the write mutex (race-safe read after Wait).
func (w *capWriter) snapshot() (string, int64, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String(), w.total, w.overflow
}

// CmdRunner executes one shell command. The real one goes through a POSIX
// sh with a PowerShell/cmd fallback; tests substitute fakes.
type CmdRunner interface {
	Run(ctx context.Context, command string) (stdout, stderr string, exitCode int, err error)
}

// ShRunner executes commands via `sh -c` when sh is on PATH, falling back
// to PowerShell (`pwsh`/`powershell -Command`) or `cmd /C` on machines
// without a POSIX shell (Windows without Git-bash on PATH). err is nil
// for any completed run (including non-zero exits); it is non-nil only
// when no verdict could be produced (spawn failure, kill, timeout).
type ShRunner struct {
	Dir      string
	EnvUnset []string
}

// StructuredCmd is one native invocation without shell (NOV-45):
// executable + args travel literally to exec.CommandContext, no
// sh -c / pwsh -Command / cmd /C, no expansion, no concatenation.
type StructuredCmd struct {
	Executable string
	Args       []string
	Env        []string
	Dir        string
}

// StructuredRunner executes StructuredCmd natively. It never resolves
// shellName/shellArgs and never spawns sh.exe/bash.exe/pwsh/cmd.
type StructuredRunner struct {
	Dir      string
	EnvUnset []string
}

// RunStructured runs cmd natively with output caps, HideWindow and
// ctx timeout. Empty executable is a no-verdict error.
func (s StructuredRunner) RunStructured(ctx context.Context, cmd StructuredCmd) (string, string, int, error) {
	if strings.TrimSpace(cmd.Executable) == "" {
		return "", "", -1, fmt.Errorf("structured runner: empty executable")
	}
	dir := cmd.Dir
	if dir == "" {
		dir = s.Dir
	}
	c := exec.CommandContext(ctx, cmd.Executable, cmd.Args...)
	if dir != "" {
		c.Dir = dir
	}
	if len(cmd.Env) > 0 {
		c.Env = append(os.Environ(), cmd.Env...)
	} else if len(s.EnvUnset) > 0 {
		unset := map[string]bool{}
		for _, n := range s.EnvUnset {
			if n != "" {
				unset[n] = true
			}
		}
		if len(unset) > 0 {
			filtered := []string{}
			for _, kv := range os.Environ() {
				if i := strings.IndexByte(kv, '='); i > 0 {
					if unset[kv[:i]] {
						continue
					}
				}
				filtered = append(filtered, kv)
			}
			c.Env = filtered
		}
	}
	executil.HideWindow(c)
	stdout, stderr, code, _, err := runShellCap(c, ctx, cmd.Executable, MaxOutputBytes)
	return stdout, stderr, code, err
}

// dockerProbe reports whether a docker daemon is reachable. Tests stub it;
// production runs `docker info` with a 5s deadline.
var dockerProbe = func(ctx context.Context) bool {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "docker", "info")
	executil.HideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func (s ShRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	stdout, stderr, code, _, _, err := s.runWithStats(ctx, command, s.Dir, "", "")
	return stdout, stderr, code, err
}

func (s ShRunner) runWithStats(ctx context.Context, command, root, dockerImage, isolate string) (stdout, stderr string, code int, cpuMs int64, isolation string, err error) {
	if isolate == "docker" && dockerImage != "" {
		if !dockerProbe(ctx) {
			reason := "docker daemon unavailable"
			stdout, stderr, code, cpuMs, err := runShell(ctx, shellName(command), shellArgs(command), s.Dir, s.EnvUnset)
			note := "isolation: docker requested but unavailable (" + reason + "); ran host-contained:degraded"
			if stderr == "" {
				stderr = note
			} else {
				stderr += "\n" + note
			}
			return stdout, stderr, code, cpuMs, IsolationHostDegraded, err
		}
		vol := root
		if vol == "" {
			vol = s.Dir
		}
		if vol == "" {
			if wd, werr := os.Getwd(); werr == nil {
				vol = wd
			} else {
				vol = "."
			}
		}
		if abs, aerr := filepath.Abs(vol); aerr == nil {
			vol = abs
		}
		args := []string{"run", "--rm", "-v", vol + ":/work", "-w", "/work", dockerImage, "sh", "-c", command}
		stdout, stderr, code, cpuMs, err := runShell(ctx, "docker", args, "", nil)
		return stdout, stderr, code, cpuMs, IsolationDocker, err
	}
	if isolate == "docker" && dockerImage == "" {
		reason := "no docker image configured"
		stdout, stderr, code, cpuMs, err := runShell(ctx, shellName(command), shellArgs(command), s.Dir, s.EnvUnset)
		note := "isolation: docker requested but unavailable (" + reason + "); ran host-contained:degraded"
		if stderr == "" {
			stderr = note
		} else {
			stderr += "\n" + note
		}
		return stdout, stderr, code, cpuMs, IsolationHostDegraded, err
	}
	stdout, stderr, code, cpuMs, err = runShell(ctx, shellName(command), shellArgs(command), s.Dir, s.EnvUnset)
	return stdout, stderr, code, cpuMs, IsolationHostContained, err
}

func shellName(command string) string {
	if _, err := exec.LookPath("sh"); err == nil {
		return "sh"
	}
	if _, err := exec.LookPath("pwsh"); err == nil {
		return "pwsh"
	}
	if _, err := exec.LookPath("powershell"); err == nil {
		return "powershell"
	}
	return "cmd"
}

func shellArgs(command string) []string {
	name := shellName(command)
	switch name {
	case "sh":
		return []string{"-c", command}
	case "pwsh", "powershell":
		return []string{"-NoProfile", "-NonInteractive", "-Command", command}
	default:
		return []string{"/C", command}
	}
}

// runShellFallback runs one command through a non-POSIX shell and maps the
// shell's own command-not-found report to the POSIX 127 convention, so a
// missing binary reads the same on every platform (sh reports 127; cmd
// reports 1/9009 and PowerShell reports 1 for the same condition).
func runShellFallback(ctx context.Context, name string, args []string, dir string, envUnset []string) (string, string, int, int64, error) {
	stdout, stderr, code, cpu, err := runShell(ctx, name, args, dir, envUnset)
	if err == nil && code != 0 && isShellNotFound(stdout, stderr) {
		return stdout, stderr, 127, cpu, nil
	}
	return stdout, stderr, code, cpu, err
}

// isShellNotFound reports whether the captured output is the shell itself
// refusing an unknown command (as opposed to the command running and
// failing). Markers cover cmd and PowerShell in English and Spanish; the
// PowerShell FullyQualifiedErrorId marker is unambiguous by construction.
func isShellNotFound(stdout, stderr string) bool {
	combined := stdout + "\n" + stderr
	for _, marker := range []string{
		"CommandNotFoundException",
		"not recognized as the name of a cmdlet",
		"no se reconoce como nombre de un cmdlet",
		"is not recognized as an internal or external command",
		"no se reconoce como un comando interno o externo",
	} {
		if strings.Contains(combined, marker) {
			return true
		}
	}
	return false
}

func cpuMillis(cmd *exec.Cmd) int64 {
	if cmd.ProcessState == nil {
		return 0
	}
	u := cmd.ProcessState.UserTime()
	s := cmd.ProcessState.SystemTime()
	return (u + s).Milliseconds()
}

func runShell(ctx context.Context, name string, args []string, dir string, envUnset []string) (string, string, int, int64, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(envUnset) > 0 {
		unset := map[string]bool{}
		for _, n := range envUnset {
			if n != "" {
				unset[n] = true
			}
		}
		if len(unset) > 0 {
			filtered := os.Environ()[:0:0]
			// rebuild without the unset names (exact match)
			filtered = filtered[:0]
			for _, kv := range os.Environ() {
				if i := strings.IndexByte(kv, '='); i > 0 {
					if unset[kv[:i]] {
						continue
					}
				}
				filtered = append(filtered, kv)
			}
			cmd.Env = filtered
		}
	}
	executil.HideWindow(cmd)
	return runShellCap(cmd, ctx, name, MaxOutputBytes)
}

// runShellCap starts cmd with per-stream output caps and waits. Extracted
// so tests can force the cap path with a tiny budget.
func runShellCap(cmd *exec.Cmd, ctx context.Context, name string, capBytes int) (string, string, int, int64, error) {
	outW := &capWriter{limit: capBytes}
	errbW := &capWriter{limit: capBytes}
	// Shared kill gate: each writer fires its own once, but the process
	// kill itself happens exactly once even when both streams overflow
	// concurrently.
	var killOnce sync.Once
	kill := func() {
		killOnce.Do(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
	}
	outW.kill, errbW.kill = kill, kill
	cmd.Stdout, cmd.Stderr = outW, errbW
	if err := cmd.Start(); err != nil {
		return "", "", -1, 0, err
	}
	werr := cmd.Wait()
	cpu := cpuMillis(cmd)
	out, outTotal, outOverflow := outW.snapshot()
	errb, errTotal, errOverflow := errbW.snapshot()
	if outOverflow || errOverflow {
		return out, errb, -1, cpu, &outputCapError{bytes: outTotal + errTotal}
	}
	if werr == nil {
		return out, errb, 0, cpu, nil
	}
	// A deadline kill surfaces as an ExitError on Windows (exit code 1)
	// and as a signal ExitError on Unix; the context check must come first
	// or a timeout masquerades as an ordinary failure.
	if ctx.Err() != nil {
		return out, errb, -1, cpu, fmt.Errorf("timed out: %w", ctx.Err())
	}
	var ee *exec.ExitError
	if errors.As(werr, &ee) {
		code := ee.ExitCode()
		if (name == "cmd" || name == "powershell" || name == "pwsh") && isShellNotFound(out, errb) {
			return out, errb, 127, cpu, nil
		}
		return out, errb, code, cpu, nil
	}
	return out, errb, -1, cpu, werr
}
