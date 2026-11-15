// NOV-69: host journeys. Probe-only mode marks every surface
// honestly; classification rules are unit-pinned.
package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/bench"
)

func TestBenchHostJourneysProbeOnlyMarksNotRun(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir()) // no host binaries at all
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "host-journeys"}); code != 0 {
		t.Fatalf("exit = %d, want 0 (INCONCLUSIVE, never fail without hosts)", code)
	}
	got := out.String()
	for _, want := range []string{"| opencode | no-ejecutado |", "| codex | no-ejecutado |", "INCONCLUSIVE"} {
		if !strings.Contains(got, want) {
			t.Fatalf("matrix must mark %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "PASS:") {
		t.Fatalf("nothing executed, must not report PASS:\n%s", got)
	}
}

func writeFakeRegulus(t *testing.T, failOn string) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, "regulus.cmd")
		body := "@echo off\r\n"
		if failOn != "" {
			// Match one exact argument: findstr treats the value as a
			// filename, so loop over %* instead.
			body += "for %%a in (%*) do if \"%%a\"==\"" + failOn + "\" (echo boom>&2 & exit /b 1)\r\n"
		}
		body += "echo fake-ok\r\nexit /b 0\r\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := filepath.Join(dir, "regulus")
	body := "#!/usr/bin/env bash\n"
	if failOn != "" {
		body += "for a in \"$@\"; do if [ \"$a\" = \"" + failOn + "\" ]; then echo boom >&2; exit 1; fi; done\n"
	}
	body += "echo fake-ok\nexit 0\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExecuteHostJourneyPassWithFakeBin(t *testing.T) {
	t.Setenv("REGULUS_BIN", writeFakeRegulus(t, ""))
	c, _, _ := newCLI(nil)
	r := c.executeHostJourney(context.Background(), "fakehost", "v0", "", time.Now().UTC().Format(time.RFC3339))
	if r.Status != bench.AgentPass {
		t.Fatalf("fake-green journey must pass, got %q (%s)", r.Status, r.Reason)
	}
}

func TestExecuteHostJourneyFailNamesStep(t *testing.T) {
	t.Setenv("REGULUS_BIN", writeFakeRegulus(t, "gate"))
	c, _, _ := newCLI(nil)
	r := c.executeHostJourney(context.Background(), "fakehost", "v0", "", time.Now().UTC().Format(time.RFC3339))
	if r.Status != bench.AgentFail {
		t.Fatalf("gate failure must fail, got %q (%s)", r.Status, r.Reason)
	}
	if !strings.Contains(r.Reason, "gate") {
		t.Fatalf("fail must name the step, got %q", r.Reason)
	}
}

func TestClassifyJourneyErr(t *testing.T) {
	if s, _ := classifyJourneyErr("gate", errors.New("boom")); s != bench.AgentFail {
		t.Fatalf("gate failure = %q, want fail", s)
	}
	if s, reason := classifyJourneyErr("gate", errors.New("Insufficient balance 402")); s != bench.AgentNotRun || reason == "" {
		t.Fatalf("quota = %q %q, want not-run with reason", s, reason)
	}
	if s, _ := classifyJourneyErr("doctor", errors.New("refused")); s != bench.AgentNotRun {
		t.Fatalf("doctor refusal = %q, want not-run (advisory)", s)
	}
	if _, reason := classifyJourneyErr("gate", errors.New("boom")); !strings.Contains(reason, "gate") {
		t.Fatalf("fail must name the step, got %q", reason)
	}
}
