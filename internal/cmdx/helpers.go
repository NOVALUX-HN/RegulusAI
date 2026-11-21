package cmdx

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/pxexec"
)

// splitList splits a comma-separated flag value, trimming spaces.
// Duplicated trivially from internal/cli (cli.go) so cmdx stays free
// of the cli import cycle; behavior is byte-identical.
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

var taskIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)

// validTaskID accepts task ids usable as receipt metadata without path
// traversal: alphanumeric start, conservative charset, no "..", bounded
// length. Duplicated trivially from internal/cli (cli.go).
func validTaskID(s string) bool {
	return taskIDRe.MatchString(s) && !strings.Contains(s, "..")
}

// gitHeadBounded resolves HEAD with the S3 git bound (30s, same as
// gates.changedFiles): a wedged git must never hang a CLI command.
// Duplicated from internal/cli (githead.go) so moved handlers keep
// their evidence behavior without importing cli.
func gitHeadBounded(r gates.CmdRunner) string {
	ctx, cancel := context.WithTimeout(context.Background(), gates.GitHeadTimeout)
	defer cancel()
	return gates.GitHead(ctx, r)
}

// probeOpenCodeVersion best-efforts `opencode --version` ("opencode
// X.Y.Z" -> "X.Y.Z"); anything else means unknown, never an error.
// Duplicated from internal/cli (bench_agent.go).
func probeOpenCodeVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res := pxexec.Run(ctx, pxexec.Spec{Name: "opencode", Args: []string{"--version"}, Timeout: 15 * time.Second})
	if res.Outcome != pxexec.OutcomePass {
		return ""
	}
	fields := strings.Fields(strings.TrimSpace(res.Stdout))
	if len(fields) == 2 && fields[0] == "opencode" {
		return fields[1]
	}
	return ""
}

// resolveAgentRunner materializes the requested headless host. Without
// a usable host it returns the NOT RUN message, its exit code and
// ok=false: the suite never passes in false and never fails tasks the
// agent never attempted. Duplicated from internal/cli (bench_agent.go).
func resolveAgentRunner(host string) (bench.HeadlessAgent, string, int, bool) {
	switch host {
	case "opencode":
		r, err := bench.DefaultOpenCodeRunner()
		if err != nil {
			return nil, fmt.Sprintf("agent-tasks NOT RUN: %v (suite skipped, nothing passed or failed)", err), 3, false
		}
		return r, "", 0, true
	default:
		return nil, fmt.Sprintf("agent-tasks NOT RUN: unknown host %q (suite skipped, nothing passed or failed)", host), 3, false
	}
}

// benchAgentSplitsPath resolves the splits file: the
// REGULUS_AGENT_SPLITS override wins, otherwise the tracked path,
// otherwise the project-root join for callers running from a
// subdirectory. Duplicated from internal/cli (bench_agent.go).
func benchAgentSplitsPath(dir string) string {
	if env := os.Getenv("REGULUS_AGENT_SPLITS"); env != "" {
		return env
	}
	if _, err := os.Stat(bench.AgentSplitsPath); err != nil {
		return dir + "/" + bench.AgentSplitsPath
	}
	return bench.AgentSplitsPath
}

// loadBenchAgentSplits resolves the splits path and validates it
// against the corpus. On failure it prints the byte-identical refusal
// and reports exit 1 with ok=false. Duplicated from internal/cli
// (bench_agent.go); the prefix stays "bench agent-tasks:" even when
// called from lab/tiers paths to preserve UX.
func loadBenchAgentSplits(io IO, corpus []bench.AgentTask) (bench.AgentSplits, int, bool) {
	splits, err := bench.LoadAgentSplits(benchAgentSplitsPath(io.Dir), corpus)
	if err != nil {
		fmt.Fprintf(io.Stderr, "bench agent-tasks: %v\n", err)
		return bench.AgentSplits{}, 1, false
	}
	return splits, 0, true
}
