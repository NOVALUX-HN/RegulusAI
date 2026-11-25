// Package execbackend decides where a verification command runs when
// the host cannot execute freshly built binaries.
//
// Windows application-control policy (Smart App Control, WDAC) can
// refuse every newly compiled executable: `go build` succeeds and
// running the result fails with "Access is denied". On such a host
// `go test` dies at fork/exec of the freshly linked test binary, so
// the whole suite is unrunnable — while the signed Go toolchain keeps
// working, which makes the failure look like a code defect instead of
// a policy refusal.
//
// The native path is never changed. A healthy host resolves to Native
// and behaves exactly as before; a sandbox backend is chosen only when
// an empirical probe proves a policy block. Every decision carries the
// reason it was made and every run carries the backend it ran on, so
// sandbox evidence is never presented as native or as CI evidence.
package execbackend

import (
	"context"
	"fmt"
	"runtime"
	"strings"
)

// Backend is the closed set of places a verification command can run.
// The vocabulary is closed on purpose: a new value means a new
// execution model, never a synonym.
type Backend string

const (
	// Native runs the command on the host, the way Regulus always has.
	Native Backend = "native"
	// WSL runs the command inside a Windows Subsystem for Linux
	// distro, where Windows application-control policy does not apply.
	WSL Backend = "wsl"
	// None means no backend can run the command here. The caller must
	// report BLOCKED; it must never fall back to claiming success.
	None Backend = "none"
)

// Valid reports whether b is one of the three defined backends.
func (b Backend) Valid() bool {
	switch b {
	case Native, WSL, None:
		return true
	}
	return false
}

// String implements fmt.Stringer so receipts and logs print the
// vocabulary instead of an empty string for unknown values.
func (b Backend) String() string { return string(b) }

// Decision is a resolved backend plus the evidence behind it. Reason
// is always populated: an unexplained backend switch is unreviewable.
type Decision struct {
	Backend Backend
	Reason  string
	Distro  string
}

// BlockProbe reports whether the host refuses to execute a freshly
// built binary, with a human-readable reason. It is injectable so the
// resolution rules stay pure and testable on any platform.
type BlockProbe func(ctx context.Context) (blocked bool, reason string)

// Options are the inputs of a resolution. Zero Options resolves the
// current host with no probe, which is the healthy-host default.
type Options struct {
	// Override forces a backend and skips probing. Accepted values are
	// the three backend names plus "" and "auto". Anything else is an
	// error: a typo must not silently select native.
	Override string
	// GOOS overrides runtime.GOOS (tests only).
	GOOS string
	// Probe reports a policy block. Nil means "no known block", which
	// keeps the native path untouched.
	Probe BlockProbe
	// Distros lists installed WSL distros. Empty means no sandbox
	// backend is available.
	Distros []string
	// Distro prefers a named distro. Empty picks the first installed.
	Distro string
}

// Resolve chooses the backend for this host. The rules, in order:
//
//  1. An explicit override is honoured (governance beats probing).
//  2. A non-Windows host is native: application-control policy is a
//     Windows mechanism and nothing else can block the launch.
//  3. A Windows host without a positive block signal stays native, so
//     the existing behaviour is preserved even when the probe is
//     inconclusive.
//  4. A Windows host with a proven block needs WSL; without an
//     installed distro the answer is None, never a silent skip.
func Resolve(ctx context.Context, o Options) (Decision, error) {
	override := strings.TrimSpace(strings.ToLower(o.Override))
	native, wsl, none := string(Native), string(WSL), string(None)
	if override != "" && override != "auto" && override != native && override != wsl && override != none {
		return Decision{}, fmt.Errorf("execbackend: unknown backend %q (want auto|native|wsl|none)", o.Override)
	}
	if override == native {
		return Decision{Backend: Native, Reason: "forced by override"}, nil
	}
	if override == none {
		return Decision{Backend: None, Reason: "forced by override"}, nil
	}
	if override == wsl {
		distro, ok := pickDistro(o.Distro, o.Distros)
		if !ok {
			return Decision{Backend: None, Reason: "wsl forced but no distro is installed"}, nil
		}
		return Decision{Backend: WSL, Distro: distro, Reason: "forced by override"}, nil
	}

	goos := o.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "windows" {
		return Decision{Backend: Native, Reason: "host is " + goos + "; no Windows application-control policy applies"}, nil
	}
	if o.Probe == nil {
		return Decision{Backend: Native, Reason: "no block signal on this host"}, nil
	}
	blocked, reason := o.Probe(ctx)
	if !blocked {
		if reason == "" {
			reason = "no block signal on this host"
		}
		return Decision{Backend: Native, Reason: reason}, nil
	}
	distro, ok := pickDistro(o.Distro, o.Distros)
	if !ok {
		return Decision{Backend: None, Reason: "policy blocks fresh binaries and no WSL distro is installed: " + reason}, nil
	}
	return Decision{Backend: WSL, Distro: distro, Reason: "policy blocks fresh binaries: " + reason}, nil
}

// pickDistro resolves the requested distro. An explicit pick must
// exist; otherwise the first installed distro wins.
func pickDistro(want string, have []string) (string, bool) {
	want = strings.TrimSpace(want)
	for _, d := range have {
		if d == "" {
			continue
		}
		if want == "" || d == want {
			return d, true
		}
	}
	return "", false
}

// Wrap returns the argv that runs argv on this decision's backend.
// Native returns argv unchanged — byte-identical behaviour, which is
// what keeps the healthy path safe. The caller stays responsible for
// the working directory: WSL translates an inherited Windows cwd to
// its /mnt/<drive>/ equivalent on its own.
func (d Decision) Wrap(argv []string) ([]string, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("execbackend: empty argv")
	}
	switch d.Backend {
	case Native:
		return argv, nil
	case WSL:
		if d.Distro == "" {
			return nil, fmt.Errorf("execbackend: wsl backend without a distro")
		}
		out := make([]string, 0, len(argv)+4)
		out = append(out, "wsl.exe", "-d", d.Distro, "-e")
		return append(out, argv...), nil
	case None:
		return nil, fmt.Errorf("execbackend: no backend can run %q here", argv[0])
	}
	return nil, fmt.Errorf("execbackend: unknown backend %q", d.Backend)
}
