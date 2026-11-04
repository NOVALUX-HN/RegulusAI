// Package cli dispatches the regulus core CLI.
package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
)

// CLI holds the injected dependencies of one regulus invocation.
// Dir scopes filesystem reads to one project root ("" means auto-detect:
// git top-level, then the nearest regulus.json ancestor, then the
// process cwd — preserving every existing caller). The HTTP adapter sets
// it to the resolved ?project= root so `status` reads that project's
// stores without a process-wide chdir.
type CLI struct {
	Version string
	Probe   func() doctor.Result
	Runner  gates.CmdRunner
	Stdout  io.Writer
	Stderr  io.Writer
	Dir     string
}

// dir resolves the project root this invocation reads: Dir when set,
// ResolveRoot("") (git top-level → regulus.json ancestor → cwd)
// otherwise. HTTP sets Dir per ?project= request; Run resolves the
// process cwd once per invocation, so this fallback only fires for
// direct method calls (tests, MCP sub-invocations).
func (c CLI) dir() string {
	if c.Dir != "" {
		return c.Dir
	}
	if r := ResolveRoot(""); r != "" {
		return r
	}
	return "."
}

// cliDispatch routes subcommand names to their handlers. Pure routing
// table: adding a command means adding one line here, Run stays short.
// doctor takes no arguments; its entry ignores the tail.
var cliDispatch = map[string]func(c CLI, args []string) int{
	"doctor":   func(c CLI, _ []string) int { return c.doctor() },
	"spec":     func(c CLI, args []string) int { return c.specCmd(args) },
	"status":   func(c CLI, args []string) int { return c.statusCmd(args) },
	"chart":    func(c CLI, args []string) int { return c.chartCmd(args) },
	"init":     func(c CLI, args []string) int { return c.initCmd(args) },
	"surface":  func(c CLI, args []string) int { return c.surfaceCmd(args) },
	"evidence": func(c CLI, args []string) int { return c.evidenceCmd(args) },
	"gate":     func(c CLI, args []string) int { return c.gateCmd(args) },
	"mcp":      func(c CLI, args []string) int { return c.mcpCmd(args) },
	"memory":   func(c CLI, args []string) int { return c.memoryCmd(args) },
	"budget":   func(c CLI, args []string) int { return c.budgetCmd(args) },
	"library":  func(c CLI, args []string) int { return c.libraryCmd(args) },
	"reflect":  func(c CLI, args []string) int { return c.reflectCmd(args) },
	"evolve":   func(c CLI, args []string) int { return c.evolveCmd(args) },
	"skills":   func(c CLI, args []string) int { return c.skillsCmd(args) },
	"probe":    func(c CLI, args []string) int { return c.probeCmd(args) },
	"trace":    func(c CLI, args []string) int { return c.traceCmd(args) },
	"task":     func(c CLI, args []string) int { return c.taskCmd(args) },
	"decide":   func(c CLI, args []string) int { return c.decideCmd(args) },
	"bench":    func(c CLI, args []string) int { return c.benchCmd(args) },
	"yolo":     func(c CLI, args []string) int { return c.yoloCmd(args) },
	"crap":     func(c CLI, args []string) int { return c.crapCmd(args) },
	"review":   func(c CLI, args []string) int { return c.reviewCmd(args) },
	"audit":    func(c CLI, args []string) int { return c.auditCmd(args) },
}

// Run executes one CLI invocation and returns the process exit code:
// 0 success, 1 refusal or failure, 2 usage error.
func (c CLI) Run(args []string) int {
	if len(args) == 0 {
		c.usage(c.Stderr)
		return 2
	}
	switch args[0] {
	case "--version":
		fmt.Fprintf(c.Stdout, "regulus %s\n", c.Version)
		return 0
	case "help", "--help", "-h":
		c.usage(c.Stdout)
		return 0
	case "contract":
		fmt.Fprintln(c.Stdout, contract.Version)
		return 0
	}
	if fn, ok := cliDispatch[args[0]]; ok {
		// Single entry point for root resolution (P4): an explicit Dir
		// (HTTP ?project= scoping, tests) wins; otherwise resolve once
		// per invocation so every c.dir() below — Almanac, receipts,
		// traces, budget, library — shares the project root instead of
		// splitting stores per subdirectory.
		if c.Dir == "" {
			if r := ResolveRoot(""); r != "" {
				c.Dir = r
			}
		}
		return fn(c, args[1:])
	}
	fmt.Fprintf(c.Stderr, "regulus: unknown command %q\n\n", args[0])
	c.usage(c.Stderr)
	return 2
}

var taskIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)

// validTaskID accepts task ids usable as receipt metadata without path
// traversal: alphanumeric start, conservative charset, no "..", bounded
// length.
func validTaskID(s string) bool {
	return taskIDRe.MatchString(s) && !strings.Contains(s, "..")
}

// runner returns the injected runner or the real shell runner.
func (c CLI) runner() gates.CmdRunner {
	if c.Runner != nil {
		return c.Runner
	}
	return gates.ShRunner{}
}

// checkProvenancePaths keeps Almanac provenance repo-relative: no absolute
// paths, no drive letters, no ".." segments, no empty entries. Provenance
// is metadata, not file access — the MCP trust boundary documents that the
// server never reads or writes these paths, it only stores them.
func checkProvenancePaths(values []string, flag string) error {
	for _, v := range values {
		p := strings.TrimSpace(v)
		if p == "" {
			return fmt.Errorf("invalid provenance %s %q: must be repo-relative, no absolute paths or ..", flag, v)
		}
		if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
			return fmt.Errorf("invalid provenance %s %q: must be repo-relative, no absolute paths or ..", flag, v)
		}
		if len(p) >= 2 && ((p[0] >= 'A' && p[0] <= 'Z' || p[0] >= 'a' && p[0] <= 'z') && p[1] == ':') {
			return fmt.Errorf("invalid provenance %s %q: must be repo-relative, no absolute paths or ..", flag, v)
		}
		for _, seg := range strings.Split(strings.ReplaceAll(p, "\\", "/"), "/") {
			if seg == ".." {
				return fmt.Errorf("invalid provenance %s %q: must be repo-relative, no absolute paths or ..", flag, v)
			}
		}
	}
	return nil
}

// splitList splits a comma-separated flag value, trimming spaces.
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (c CLI) usage(w io.Writer) {
	fmt.Fprintf(w, `regulus %s — the Regulus AI core CLI

Usage:
  regulus <command> [arguments]

Commands:
  audit [--profile <coverprofile>] [--top N]   repo-wide ranked audit: entry points, then what to fix first
  budget show|record   the evolution ledger (show; record a probe run)
  bench list|run|baseline|compare   the self-benchmark: offline suite, baseline, contrast (regulus bench --help)
  chart init <name> | list             scaffold a Chart, list every Chart
  init [--stack go|node|smoke] [--harness-from <repo>]   scaffold regulus.json, charts/, .gitignore (+ .opencode config) for a fresh repo
  contract           print the plugin<->core contract version
  crap               score functions with the CRAP formula (regulus crap --help)
  doctor             probe OpenCode and verify V2 runtime support
  evidence list [--task <t>] | show <file> | verify [--task <t>]   audit gauntlet receipts
  evolve propose --task <t> --domain <d> | evolve tick [--every N]   assemble the evidence pack a proposer needs; periodic proposer (every N traced sessions)
  gate run [--profile NAME] [--dry-run] <task> | gate evaluate ... | gate verdict <task>   run the gauntlet, record guardian assessments, consolidate verdicts
  help               show this help
  library list|candidates|show|domains|forget|retain [--domain <name>]  lessons, pending candidates, domain index, snapshot pruning
  library admit --domain <d> --candidate <id> --guardian <name>  admit a validated lesson
  library forget --domain <d> --task <t>  drop one task snapshot
  library retain --domain <d> --task <t>  keep tasks strictly before <t>
  mcp                serve the stdio MCP server (newline-delimited JSON-RPC 2.0)
  serve [--port N] [--bind ADDR]   serve the read-only dashboard (default 127.0.0.1:17817)
  memory note|show|save|search|history|context|prune|doctor|import|pin|unpin|relate|export|library   the Almanac — persistent memory
  probe contrast --task <t> --gate <g> --a <cmdA> --b <cmdB>   run one gate twice and diff the receipts
  probe compose --task <t> --gate <g> --base <cmd> --a <cmdA> --b <cmdB>   run one gate three times and merge the variants
  reflect            admit an evidence-gated lesson (regulus reflect --help)
  skills list        trigger registry: skills with their one-line triggers
  trace record|show|files|sessions|prune|cost   capture and inspect per-session tool traces, report per-task proxy cost (regulus trace --help)
  task check|status|close|classify   the Sextant discipline: contracts, delegation, progress, verified completion
  decide propose|assess|resolve|show|list   Decision Intelligence: structured decisions, assessed evidence, governed resolution
  review [--task <t>] [--lens NAME]   proportional review: freeze the diff, apply 1 lens (informative, opt-in; never blocks)
  spec check <file|dir>  validate the acceptance contract of feature documents
  status             one look at the project: runtime, gauntlet, evolution, receipts
  yolo [on|off|status]   flip safe auto-approve in opencode.json (deletes stay denied)
  --version          print the regulus version

Exit codes:
  0  success
  1  refusal or failure
  2  usage error
`, c.Version)
}
