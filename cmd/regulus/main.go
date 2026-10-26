// Command regulus is the Regulus AI core CLI: the compiled owner of gauntlet
// evidence, spec validation, lesson library and budget state.
package main

import (
	"context"
	"os"

	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	c := cli.CLI{
		Version: version,
		Probe: func() doctor.Result {
			return doctor.Probe(context.Background(), doctor.DefaultRunner())
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	os.Exit(c.Run(os.Args[1:]))
}
