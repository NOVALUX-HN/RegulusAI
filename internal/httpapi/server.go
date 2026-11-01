// Server is the minimal read-only HTTP adapter (Phase 7, S5): a versioned
// subset reusing CLI core paths via buffers — the same buffers-in,
// buffers-out pattern as the MCP server in internal/cli/mcp.go, which
// never shells out so HTTP semantics match CLI stderr semantics exactly.
//
// Exposed read-only core paths:
//   - GET /v1/status  reuses `regulus status` (the project readout).
//   - GET /v1/projects lists the startup allowlist ([{name, root}],
//     first entry is the default served when ?project= is absent).
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
	"path/filepath"
	"strings"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/mcp"
	"regulus-ai/internal/observability"
)

// DefaultDashboardPort is the fixed visual port (S1 "puerto especial",
// S2 always-on visibility). Loopback-only by default; override with
// `regulus serve --port N`.
const DefaultDashboardPort = 17817

// Project is one servable project: the display name clients send as
// ?project= plus the filesystem root that name is allowlisted to. The
// name is almanac.ProjectName(dir) computed once at startup; the query
// value is never interpreted as a path, so traversal dies in the
// lookup before any disk access.
type Project struct {
	Name string `json:"name"`
	Root string `json:"root"`
}

// Server is the read-only HTTP adapter. Version, Probe and Runner mirror
// cli.CLI's injected dependencies so each request can run a core path
// with private buffers inheriting the server's configuration.
// Projects is the startup allowlist: every ?project= value resolves by
// NAME against it, never as a path. Empty (or a server built without
// roots) means exactly today's behavior: the single current project.
type Server struct {
	Version  string
	Probe    func() doctor.Result
	Runner   gates.CmdRunner
	Projects []Project
}

// New builds a Server with the given core dependencies. Each root joins
// the project allowlist under its almanac.ProjectName; with no roots
// the allowlist is just the current project ("."), identical to today.
func New(version string, probe func() doctor.Result, runner gates.CmdRunner, roots ...string) Server {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	seen := map[string]bool{}
	projects := make([]Project, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		name := almanac.ProjectName(root)
		if seen[name] {
			continue
		}
		seen[name] = true
		projects = append(projects, Project{Name: name, Root: root})
	}
	if len(projects) == 0 {
		projects = []Project{{Name: almanac.ProjectName("."), Root: "."}}
	}
	return Server{Version: version, Probe: probe, Runner: runner, Projects: projects}
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

// defaultRoot is the project served when ?project= is absent: the
// first allowlisted root (the current project when no --projects flag
// was given, preserving today's behavior).
func (s Server) defaultRoot() string {
	if len(s.Projects) == 0 {
		return "."
	}
	return s.Projects[0].Root
}

// isProjectPathAttempt reports query values that look like paths rather
// than project names: slashes, backslashes, "..", absolute forms
// (unix "/abs", Windows drive "C:\x"/"C:/x"/"C:x") and the self/parent
// shorthands. Such values are usage failures (400), never lookups.
func isProjectPathAttempt(name string) bool {
	if name == "." || name == ".." || name == "~" {
		return true
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return true
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return true
	}
	if len(name) >= 2 && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')) && name[1] == ':' {
		return true
	}
	if strings.HasPrefix(name, "~/") || strings.HasPrefix(name, `~\`) {
		return true
	}
	return false
}

// jailed reports whether root resolves inside the startup allowlist
// (defensive backstop: resolved roots come from the allowlist by
// construction, but every disk access must stay jailed even if a
// future caller derived a root elsewhere).
func (s Server) jailed(root string) bool {
	abs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	allow := s.Projects
	if len(allow) == 0 {
		allow = []Project{{Root: "."}}
	}
	for _, p := range allow {
		a, err := filepath.Abs(p.Root)
		if err != nil {
			continue
		}
		if abs == a || strings.HasPrefix(abs, a+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// projectRoot resolves ?project= to its allowlisted root by NAME. An
// absent value means the default project. A path-shaped value is a 400
// (fail-closed before any disk access); an unknown plain name is a 404.
// The allowlist gates NAMES (P2 jail, checked before any disk access);
// the returned root is then P4-resolved (git top-level → regulus.json
// ancestor → input) so serving from a subdirectory shares that
// checkout's stores instead of splitting .regulus/ per subdir. Outside
// git/regulus.json the resolution is the identity and behavior is
// exactly today's.
func (s Server) projectRoot(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := strings.TrimSpace(r.URL.Query().Get("project"))
	if name == "" {
		root := s.defaultRoot()
		if !s.jailed(root) {
			writeEnvelope(w, http.StatusBadRequest, InvalidParams("project: default root escapes the allowlist"))
			return "", false
		}
		return cli.ResolveRoot(root), true
	}
	// Exact name match first: allowlisted names are computed from the
	// trusted startup dirs (almanac.ProjectName, often "owner/repo"
	// with a slash), so a value that IS an allowlisted name is by
	// definition servable — the query is never interpreted as a path.
	for _, p := range s.Projects {
		if p.Name == name {
			if !s.jailed(p.Root) {
				writeEnvelope(w, http.StatusBadRequest, InvalidParams("project: "+observability.RedactText(name)))
				return "", false
			}
			return cli.ResolveRoot(p.Root), true
		}
	}
	// Not allowlisted: path-shaped values are usage failures (400,
	// fail-closed before any disk access); unknown plain names are 404.
	if isProjectPathAttempt(name) {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("project: "+observability.RedactText(name)))
		return "", false
	}
	writeEnvelope(w, http.StatusNotFound, NotFound("project "+name))
	return "", false
}

// runCLI runs one CLI core path with private buffers, inheriting the
// server's version, probe and runner — the HTTP analogue of the MCP
// server's mcpExec. root scopes the invocation to one allowlisted
// project (cli.CLI.Dir, never a process-wide chdir, so concurrent
// requests stay isolated).
func (s Server) runCLI(root string, args ...string) (stdout, stderr string, exit int) {
	var ob, eb bytes.Buffer
	c := cli.CLI{Version: s.Version, Probe: s.probeFunc(), Runner: s.Runner, Stdout: &ob, Stderr: &eb, Dir: root}
	code := c.Run(args)
	return ob.String(), eb.String(), code
}

// statusData is the verbatim `regulus status` readout (both buffers and
// the exit code, forwarded without transformation) plus the serving
// project identity: ProjectName(root) — git remote or dirname — so
// the dashboard header names the real project without a new endpoint.
type statusData struct {
	Stdout  string `json:"stdout"`
	Stderr  string `json:"stderr"`
	Exit    int    `json:"exit"`
	Project string `json:"project"`
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
	mux.HandleFunc("/v1/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/almanac/summary", s.handleAlmanacSummary)
	mux.HandleFunc("/v1/receipts", s.handleReceipts)
	mux.HandleFunc("/v1/observations", s.handleObservations)
	mux.HandleFunc("/v1/lessons", s.handleLessons)
	mux.HandleFunc("/v1/traces", s.handleTraces)
	mux.HandleFunc("/v1/budget", s.handleBudget)
	mux.HandleFunc("/v1/projects", s.handleProjects)
	mux.HandleFunc("/", s.handleDashboard)
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
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	stdout, stderr, exit := s.runCLI(root, "status")
	stdout = observability.RedactText(stdout)
	stderr = observability.RedactText(stderr)
	project := observability.RedactText(almanac.ProjectName(root))
	status, env := MapResult(statusData{Stdout: stdout, Stderr: stderr, Exit: exit, Project: project}, observability.RedactText(stderr), exit)
	writeEnvelope(w, status, env)
}

func (s Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	versionOut, versionErr, versionExit := s.runCLI(root, "--version")
	contractOut, contractErr, contractExit := s.runCLI(root, "contract")
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
	status, env := MapResult(versionData{Version: observability.RedactText(versionOut), Contract: observability.RedactText(contractOut)}, "", 0)
	writeEnvelope(w, status, env)
}

func (s Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	// The allowlist itself is the data: names are what clients echo
	// back as ?project=, so they go out verbatim (no RedactText —
	// redaction would break the name round-trip). Roots go out
	// verbatim as declared at startup: they ARE the operator's own
	// allowlist configuration (informative, never derived from user
	// input), served loopback-only by default with an explicit
	// warning on non-loopback binds (cmd/regulus/main.go). No store
	// is read and no ?project= scoping applies — the list is the
	// allowlist, identical for every caller; P2 jail still guards
	// every data handler that consumes ?project=. Order is allowlist
	// order: entries[0] is the default served when ?project= is
	// absent (the current project when no --projects flag was given).
	items := make([]Project, 0, len(s.Projects))
	items = append(items, s.Projects...)
	if len(items) == 0 {
		items = []Project{{Name: almanac.ProjectName("."), Root: "."}}
	}
	status, env := MapResult(items, "", 0)
	writeEnvelope(w, status, env)
}

func (s Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeEnvelope(w, http.StatusNotFound, NotFound(r.URL.Path))
}
