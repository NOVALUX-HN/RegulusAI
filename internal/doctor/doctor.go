// Package doctor probes the local environment and verifies that the only
// supported OpenCode runtime (V2) is present.
//
// Regulus targets OpenCode V2 exclusively: V1 and unknown runtimes are
// refused with an actionable message, never silently accepted.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"regulus-ai/internal/executil"
)

// SupportedMajor is the only OpenCode major version Regulus supports.
const SupportedMajor = 2

// ProbeTimeout bounds `opencode --version`, accommodating slow external CLI
// startup while keeping the probe bounded.
const ProbeTimeout = 15 * time.Second

// Runner runs a command and returns its combined output. exec.CommandContext
// backs the real one; tests substitute fakes.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// ExecRunner runs commands for real.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	executil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// DefaultRunner returns the real runner.
func DefaultRunner() Runner { return ExecRunner{} }

// Result is one probe outcome. Supported runtimes carry Major and Detected;
// refusals carry a Reason naming the failed condition.
type Result struct {
	Detected  string
	Major     int
	Supported bool
	Reason    string
}

var versionRe = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// Classify extracts the major version from `opencode --version` output.
func Classify(output string) (int, bool) {
	m := versionRe.FindStringSubmatch(strings.TrimSpace(output))
	if m == nil {
		return 0, false
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return major, true
}

// Probe runs `opencode --version` under ProbeTimeout and classifies the
// runtime. It runs at most once per invocation and never retries.
func Probe(ctx context.Context, r Runner) Result {
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()

	out, err := r.Run(ctx, "opencode", "--version")
	raw := strings.TrimSpace(out)

	if err != nil {
		if ctx.Err() != nil {
			return Result{Reason: "`opencode --version` did not answer within the 15s probe budget"}
		}
		var notFound *exec.Error
		if errors.As(err, &notFound) && errors.Is(notFound.Err, exec.ErrNotFound) {
			return Result{Reason: "opencode binary not found on PATH — install OpenCode V2"}
		}
		if raw != "" {
			return Result{Detected: raw, Reason: fmt.Sprintf("`opencode --version` failed: %v; output: %s", err, truncate(raw, 200))}
		}
		return Result{Reason: fmt.Sprintf("`opencode --version` failed: %v", err)}
	}

	major, ok := Classify(raw)
	if !ok {
		return Result{Detected: raw, Reason: fmt.Sprintf("could not parse a version from `opencode --version` output: %s", truncate(raw, 200))}
	}
	if major != SupportedMajor {
		return Result{
			Detected: raw,
			Major:    major,
			Reason:   fmt.Sprintf("OpenCode V%d is not supported — Regulus requires V%d. Install OpenCode V2.", major, SupportedMajor),
		}
	}
	return Result{Detected: raw, Major: major, Supported: true}
}

// truncate keeps refusal reasons bounded.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
