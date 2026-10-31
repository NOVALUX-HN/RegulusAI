// Server is the minimal read-only HTTP adapter (Phase 7, S5): a versioned
// subset reusing CLI core paths via buffers — the same buffers-in,
// buffers-out pattern as the MCP server in internal/cli/mcp.go, which
// never shells out so HTTP semantics match CLI stderr semantics exactly.
//
// Exposed read-only core paths:
//   - GET /v1/status  reuses `regulus status` (the project readout).
//   - GET /v1/version reuses `regulus --version` plus `regulus contract`
//     (the two version readouts), returned verbatim under the
//     "version"/"contract" fields.
//
// Non-GET verbs on known paths are refused with 405; unknown paths
// (including the unversioned /status and /version equivalents, which have
// no route by design) answer 404. Both refusals use the shared envelope.
// The adapter holds no policy: core outcomes map through
// envelope.MapResult, which applies mcp.ClassifyExit — the same mapping
// MCP uses.
package httpapi

import (
	"bytes"
	"context"
	"net/http"

	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/mcp"
)

// Server is the read-only HTTP adapter. Version, Probe and Runner mirror
// cli.CLI's injected dependencies so each request can run a core path
// with private buffers inheriting the server's configuration.
type Server struct {
	Version string
	Probe   func() doctor.Result
	Runner  gates.CmdRunner
}

// New builds a Server with the given core dependencies.
func New(version string, probe func() doctor.Result, runner gates.CmdRunner) Server {
	return Server{Version: version, Probe: probe, Runner: runner}
}

// probeFunc returns the injected probe, defaulting to the real OpenCode
// probe (the same wiring as cmd/regulus/main.go) when none is injected
// so the adapter stays total outside tests.
func (s Server) probeFunc() func() doctor.Result {
	if s.Probe != nil {
		return s.Probe
	}
	return func() doctor.Result {
		return doctor.Probe(context.Background(), doctor.DefaultRunner())
	}
}

// runCLI runs one CLI core path with private buffers, inheriting the
// server's version, probe and runner — the HTTP analogue of the MCP
// server's mcpExec.
func (s Server) runCLI(args ...string) (stdout, stderr string, exit int) {
	var ob, eb bytes.Buffer
	c := cli.CLI{Version: s.Version, Probe: s.probeFunc(), Runner: s.Runner, Stdout: &ob, Stderr: &eb}
	code := c.Run(args)
	return ob.String(), eb.String(), code
}

// statusData is the verbatim `regulus status` readout: both buffers and
// the exit code, forwarded without transformation.
type statusData struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Exit   int    `json:"exit"`
}

// versionData aggregates the two version readouts verbatim: the stdout
// of `regulus --version` and the stdout of `regulus contract`.
type versionData struct {
	Version  string `json:"version"`
	Contract string `json:"contract"`
}

// Handler routes the versioned subset. Unregistered paths (including the
// unversioned /status and /version equivalents) fall through to the
// catch-all 404; there are no unversioned equivalents by design.
func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/version", s.handleVersion)
	mux.HandleFunc("/", s.handleNotFound)
	return mux
}

// ServeHTTP lets a Server act as its own handler.
func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Handler().ServeHTTP(w, r)
}

func (s Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	stdout, stderr, exit := s.runCLI("status")
	status, env := MapResult(statusData{Stdout: stdout, Stderr: stderr, Exit: exit}, stderr, exit)
	writeEnvelope(w, status, env)
}

func (s Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	versionOut, versionErr, versionExit := s.runCLI("--version")
	contractOut, contractErr, contractExit := s.runCLI("contract")
	// Neither readout takes params so exit 2 is unexpected; if either
	// core path reports usage failure, surface the first through the
	// shared mapping. Any other exit is a success envelope per
	// mcp.ClassifyExit (these readouts never refuse in practice).
	if _, isErr := mcp.ClassifyExit(versionExit); isErr {
		status, env := MapResult(nil, versionErr, versionExit)
		writeEnvelope(w, status, env)
		return
	}
	if _, isErr := mcp.ClassifyExit(contractExit); isErr {
		status, env := MapResult(nil, contractErr, contractExit)
		writeEnvelope(w, status, env)
		return
	}
	status, env := MapResult(versionData{Version: versionOut, Contract: contractOut}, "", 0)
	writeEnvelope(w, status, env)
}

func (s Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeEnvelope(w, http.StatusNotFound, NotFound(r.URL.Path))
}
