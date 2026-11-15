// NOV-69: journey levels, matrix rendering and probe classification.
package bench

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeHostClassifies(t *testing.T) {
	ok := HostProbe{ID: "t", Version: func(ctx context.Context) (string, error) { return "v1", nil }}
	if lvl, _ := ProbeHost(context.Background(), ok); lvl != LevelContract {
		t.Fatalf("present binary = %q, want contrato-probado", lvl)
	}
	missing := HostProbe{ID: "t", Version: func(ctx context.Context) (string, error) { return "", errors.New("no such binary") }}
	if lvl, reason := ProbeHost(context.Background(), missing); lvl != LevelNotRun || reason == "" {
		t.Fatalf("missing host = %q %q, want no-ejecutado with reason", lvl, reason)
	}
}

func TestRenderMatrixNeverPassesUnexecuted(t *testing.T) {
	got := RenderMatrix([]string{"opencode", "codex"}, map[string]HostJourneyOutcome{
		"opencode": {Host: "opencode", Level: LevelHost, Version: "v2.2.5", Status: AgentPass, Reason: "journey green"},
	})
	if !strings.Contains(got, "| opencode | host-probado |") {
		t.Fatalf("executed host must show its level:\n%s", got)
	}
	found := false
	for _, ln := range strings.Split(got, "\n") {
		if !strings.Contains(ln, "codex") {
			continue
		}
		found = true
		if !strings.Contains(ln, "no-ejecutado") {
			t.Fatalf("codex row must render no-ejecutado:\n%s", ln)
		}
		if strings.Contains(ln, "pass") {
			t.Fatalf("codex row must not claim a pass:\n%s", ln)
		}
	}
	if !found {
		t.Fatalf("codex row missing:\n%s", got)
	}
}

func TestJourneyFixtureIsSelfContained(t *testing.T) {
	dir := t.TempDir()
	if err := JourneyFixture(dir); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"charts/demo.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("fixture missing %s: %v", p, err)
		}
	}
}
