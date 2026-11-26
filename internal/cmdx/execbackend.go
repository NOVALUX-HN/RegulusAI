// `regulus exec-backend` — the command that makes internal/execbackend real.
//
// It answers one question: where can this host actually run a freshly built
// binary? On a Windows host with application control in enforcement the answer
// is the WSL distro, and on a healthy host it is native. Until now the decision
// layer was a tested library nobody called.
package cmdx

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"

	"regulus-ai/internal/execbackend"
)

// execBackendReport is the --json shape.
type execBackendReport struct {
	Backend execbackend.Backend `json:"backend"`
	Reason  string              `json:"reason"`
	Distro  string              `json:"distro,omitempty"`
	Probe   string              `json:"probe"`
	Distros []string            `json:"distros,omitempty"`
}

// ExecBackendCmd implements `regulus exec-backend`.
//
// The probe compiles and launches a minimal program, which is the operation
// verification actually needs. That cost is fine for an explicit command and
// would be wrong inside doctor, so the probe lives here and nowhere else.
func (io IO) ExecBackendCmd(args []string) int {
	fs := flag.NewFlagSet("exec-backend", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the decision as JSON")
	goBin := fs.String("go", "go", "go toolchain the probe uses")
	distro := fs.String("distro", "", "preferred WSL distro (default: first installed)")
	override := fs.String("backend", "", "force auto|native|wsl|none")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, "usage: regulus exec-backend [--json] [--go <bin>] [--distro <name>] [--backend auto|native|wsl|none]")
		return 2
	}

	ctx := context.Background()
	blocked, probeReason := execbackend.HostProbe(ctx, nil, *goBin)
	distros := execbackend.DetectDistros(ctx, nil)
	decision, err := execbackend.Resolve(ctx, execbackend.Options{
		Override: *override,
		Probe:    func(context.Context) (bool, string) { return blocked, probeReason },
		Distros:  distros,
		Distro:   *distro,
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "exec-backend: %v\n", err)
		return 1
	}

	if *asJSON {
		data, merr := json.MarshalIndent(execBackendReport{
			Backend: decision.Backend, Reason: decision.Reason, Distro: decision.Distro,
			Probe: probeReason, Distros: distros,
		}, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "exec-backend: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(data))
	} else {
		fmt.Fprintf(io.Stdout, "exec-backend: %s\n", decision.Backend)
		fmt.Fprintf(io.Stdout, "  reason: %s\n", decision.Reason)
		fmt.Fprintf(io.Stdout, "  probe:  %s\n", probeReason)
		if decision.Distro != "" {
			fmt.Fprintf(io.Stdout, "  distro: %s\n", decision.Distro)
		}
		if len(distros) > 0 {
			fmt.Fprintf(io.Stdout, "  distros: %v\n", distros)
		}
	}

	// none means nothing can run here. Exiting non-zero keeps a script from
	// reading that as a usable host.
	if decision.Backend == execbackend.None {
		return 1
	}
	return 0
}
