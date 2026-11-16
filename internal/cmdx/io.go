package cmdx

import (
	"io"

	"regulus-ai/internal/gates"
)

// IO is the minimal invocation surface handlers need: output streams,
// the already-resolved project root, the binary version and the
// command runner. cli wrappers build it from cli.CLI (Stdout, Stderr,
// c.dir(), Version, runner) so behavior stays identical without
// cmdx importing internal/cli (which would be a cycle).
type IO struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Dir     string
	Version string
	Runner  gates.CmdRunner
}

// New builds the IO cli forwards on every invocation. Dir must be the
// resolved root (cli's c.dir()); an empty Dir falls back to "." so
// direct uses never crash on a missing root.
func New(stdout, stderr io.Writer, dir, version string, runner gates.CmdRunner) IO {
	if dir == "" {
		dir = "."
	}
	return IO{Stdout: stdout, Stderr: stderr, Dir: dir, Version: version, Runner: runner}
}

// Root returns the project root for store reads/writes.
func (io IO) Root() string {
	if io.Dir == "" {
		return "."
	}
	return io.Dir
}
