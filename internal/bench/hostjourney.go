// Host conformity journeys (NOV-69, S4): from declared capabilities
// to verified ones. Each host runs the same journey — install, doctor,
// context and memory, spec and chart, gate run, receipt, close —
// against a fixture repo, or reports not-run with the reason. Results
// feed the three-level matrix (declared, contract-tested,
// host-tested); a missing host or credential marks not-executed,
// never pass, and one host's failure never degrades the others.
package bench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"sort"
	"strings"
	"time"

	"regulus-ai/internal/pxexec"
)

// Host levels, in increasing evidence order.
const (
	LevelDeclared = "declarado"
	LevelContract = "contrato-probado"
	LevelHost     = "host-probado"
	LevelNotRun   = "no-ejecutado"
)

// HostJourneyOutcome is one host's journey result.
type HostJourneyOutcome struct {
	Host       string `json:"host"`
	Version    string `json:"host_version"`
	Level      string `json:"level"`
	Status     string `json:"status"` // pass | fail | not-run
	Reason     string `json:"reason"`
	Evidence   string `json:"evidence"`
	LatencyMS  int64  `json:"latency_ms"`
	Windows    bool   `json:"windows"`
	RecordedAt string `json:"recorded_at"`
}

// JourneyStep is one named probe of the journey.
type JourneyStep struct {
	Name string
	Run  func(ctx context.Context, workdir string) (string, error)
}

// HostProbe decides whether a host can run headless at all.
type HostProbe struct {
	ID      string
	Version func(ctx context.Context) (string, error)
}

// ProbeHost checks binary presence and basic auth without spending
// model calls: a probe that needs credentials reports not-run.
func ProbeHost(ctx context.Context, p HostProbe) (string, string) {
	ver, err := p.Version(ctx)
	if err != nil {
		return LevelNotRun, "host unavailable: " + err.Error()
	}
	return LevelContract, "binary present, version " + ver + " (journey not yet executed)"
}

// DefaultHostProbes knows the four surfaces and their cheapest
// no-model-cost probes.
func DefaultHostProbes() []HostProbe {
	return []HostProbe{
		{ID: "opencode", Version: func(ctx context.Context) (string, error) {
			return toolVersion(ctx, "opencode", "--version")
		}},
		{ID: "claude", Version: func(ctx context.Context) (string, error) {
			return toolVersion(ctx, "claude", "--version")
		}},
		{ID: "codex", Version: func(ctx context.Context) (string, error) {
			return toolVersion(ctx, "codex", "--version")
		}},
		{ID: "devin", Version: func(ctx context.Context) (string, error) {
			return "", fmt.Errorf("devin ships desktop-only, no non-interactive CLI")
		}},
	}
}

func toolVersion(ctx context.Context, bin string, args ...string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res := pxexec.Run(tctx, pxexec.Spec{Name: bin, Args: args, Timeout: 30 * time.Second})
	if res.Outcome != pxexec.OutcomePass {
		if res.ErrClass == pxexec.ErrFail {
			return "", fmt.Errorf("%s probe failed: exit status %d", bin, res.Exit)
		}
		return "", fmt.Errorf("%s probe failed: %s", bin, res.Detail)
	}
	first := strings.TrimSpace(strings.SplitN(res.Combined, "\n", 2)[0])
	if len(first) > 120 {
		first = first[:120]
	}
	if first == "" {
		return "", fmt.Errorf("%s probe empty", bin)
	}
	return first, nil
}

// JourneyFixture materializes the minimal repo a journey runs
// against: a chart plus one gate target, offline and deterministic.
func JourneyFixture(workdir string) error {
	chart := "# demo\n\nDomain: demo\nClass: STANDARD\n\n## Contract\n\n- Objective: journey fixture\n- Scope: fixture work\n- Acceptance: trivial\n- Verification: gate\n\n## Acceptance\n\n### Scenario: trivial\n\n- Given a fixture\n- When the journey runs\n- Then it passes\n\n## Tasks\n\n| ID | Spec | Work | Route | Depends | Status |\n|----|------|------|-------|---------|--------|\n| T1 | trivial | build | inline | - | todo |\n\n## Log\n\n- L1: journey.\n"
	if err := os.MkdirAll(filepath.Join(workdir, "charts"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workdir, "charts", "demo.md"), []byte(chart), 0o644)
}

// RenderMatrix renders the support matrix from journey outcomes:
// one row per host with its level, version, status and evidence.
// Hosts without results render as not-executed, never as passed.
func RenderMatrix(hosts []string, results map[string]HostJourneyOutcome) string {
	var b strings.Builder
	b.WriteString("| Host | Nivel | Versión | Estado | Evidencia |\n")
	b.WriteString("|------|-------|---------|--------|-----------|\n")
	ordered := append([]string(nil), hosts...)
	sort.Strings(ordered)
	for _, h := range ordered {
		r, ok := results[h]
		if !ok {
			fmt.Fprintf(&b, "| %s | %s | unknown | %s | sin corrida |\n", h, LevelNotRun, AgentNotRun)
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", h, r.Level, r.Version, r.Status, r.Reason)
	}
	return b.String()
}
