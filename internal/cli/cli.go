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
type CLI struct {
	Version string
	Probe   func() doctor.Result
	Runner  gates.CmdRunner
	Stdout  io.Writer
	Stderr  io.Writer
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
	case "doctor":
		return c.doctor()
	case "spec":
		return c.specCmd(args[1:])
	case "status":
		return c.statusCmd(args[1:])
	case "chart":
		return c.chartCmd(args[1:])
	case "init":
		return c.initCmd(args[1:])
	case "surface":
		return c.surfaceCmd(args[1:])
	case "evidence":
		return c.evidenceCmd(args[1:])
	case "gate":
		return c.gateCmd(args[1:])
	case "mcp":
		return c.mcpCmd(args[1:])
	case "memory":
		return c.memoryCmd(args[1:])
	case "budget":
		return c.budgetCmd(args[1:])
	case "library":
		return c.libraryCmd(args[1:])
	case "reflect":
		return c.reflectCmd(args[1:])
	case "evolve":
		return c.evolveCmd(args[1:])
	case "skills":
		return c.skillsCmd(args[1:])
	case "probe":
		return c.probeCmd(args[1:])
	case "trace":
		return c.traceCmd(args[1:])
	case "yolo":
		return c.yoloCmd(args[1:])
	case "crap":
		return c.crapCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "regulus: unknown command %q\n\n", args[0])
		c.usage(c.Stderr)
		return 2
	}
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
  budget show|record   the evolution ledger (show; record a probe run)
  chart init <name> | list             scaffold a Chart, list every Chart
  init [--stack go|node|smoke] [--harness-from <repo>]   scaffold regulus.json, charts/, .gitignore (+ .opencode config) for a fresh repo
  contract           print the plugin<->core contract version
  crap               score functions with the CRAP formula (regulus crap --help)
  doctor             probe OpenCode and verify V2 runtime support
  evidence list [--task <t>] | show <file>   audit gauntlet receipts
  evolve propose --task <t> --domain <d>   assemble the evidence pack a proposer needs
  gate run <task>    run the layered gauntlet and write receipts
  help               show this help
  library list|candidates|show|domains|forget|retain [--domain <name>]  lessons, pending candidates, domain index, snapshot pruning
  library admit --domain <d> --candidate <id> --guardian <name>  admit a validated lesson
  library forget --domain <d> --task <t>  drop one task snapshot
  library retain --domain <d> --task <t>  keep tasks strictly before <t>
  mcp                serve the stdio MCP server (newline-delimited JSON-RPC 2.0)
  memory note|show|save|search|history|context|prune|doctor|import|pin|unpin|relate|export|library   the Almanac — persistent memory
  probe contrast --task <t> --gate <g> --a <cmdA> --b <cmdB>   run one gate twice and diff the receipts
  probe compose --task <t> --gate <g> --base <cmd> --a <cmdA> --b <cmdB>   run one gate three times and merge the variants
  reflect            admit an evidence-gated lesson (regulus reflect --help)
  skills list        trigger registry: skills with their one-line triggers
  trace record|show|files|sessions|prune   capture and inspect per-session tool traces (regulus trace --help)
  spec check <file>  validate the acceptance contract of a feature document
  status             one look at the project: runtime, gauntlet, evolution, receipts
  yolo [on|off|status]   flip safe auto-approve in opencode.json (deletes stay denied)
  --version          print the regulus version

Exit codes:
  0  success
  1  refusal or failure
  2  usage error
`, c.Version)
}
