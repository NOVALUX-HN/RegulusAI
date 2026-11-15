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

func TestOrderedHostIDs(t *testing.T) {
	if got := orderedHostIDs("all"); len(got) != 4 || got[0] != "opencode" || got[3] != "devin" {
		t.Fatalf("all = %v, want the four hosts in order", got)
	}
	if got := orderedHostIDs("opencode"); len(got) != 1 || got[0] != "opencode" {
		t.Fatalf("opencode filter = %v, want [opencode]", got)
	}
	if got := orderedHostIDs("nosuchhost"); len(got) != 0 {
		t.Fatalf("unknown filter = %v, want empty", got)
	}
}

func TestTallyHostJourneys(t *testing.T) {
	results := map[string]bench.HostJourneyOutcome{
		"a": {Status: bench.AgentPass},
		"b": {Status: bench.AgentPass},
		"c": {Status: bench.AgentFail},
		"d": {Status: bench.AgentNotRun},
	}
	if failed, ran := tallyHostJourneys(results); failed != 1 || ran != 2 {
		t.Fatalf("tally = (%d, %d), want (1, 2)", failed, ran)
	}
	if failed, ran := tallyHostJourneys(nil); failed != 0 || ran != 0 {
		t.Fatalf("empty tally = (%d, %d), want (0, 0)", failed, ran)
	}
}

func TestReportHostJourneys(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := reportHostJourneys(c, 2, 1); code != 1 || !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("fail verdict = %d %q, want 1 + FAIL", code, out.String())
	}
	out.Reset()
	if code := reportHostJourneys(c, 0, 0); code != 0 || !strings.Contains(out.String(), "INCONCLUSIVE") {
		t.Fatalf("inconclusive verdict = %d %q, want 0 + INCONCLUSIVE", code, out.String())
	}
	out.Reset()
	if code := reportHostJourneys(c, 0, 3); code != 0 || !strings.Contains(out.String(), "PASS") {
		t.Fatalf("pass verdict = %d %q, want 0 + PASS", code, out.String())
	}
}

func TestSaveHostJourneyResults(t *testing.T) {
	dir := t.TempDir()
	results := map[string]bench.HostJourneyOutcome{"opencode": {Status: bench.AgentNotRun}}
	if err := saveHostJourneyResults(filepath.Join(dir, "ev"), results); err != nil {
		t.Fatalf("save = %v, want nil", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "ev"))
	if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "host-journeys-") {
		t.Fatalf("evidence pack = %v, %v; want one host-journeys-*.json", entries, err)
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveHostJourneyResults(blocker, results); err == nil {
		t.Fatal("saving into a file path must fail")
	}
}

func TestBenchHostJourneysHostFilter(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir()) // no host binaries at all
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "host-journeys", "--host", "opencode"}); code != 0 {
		t.Fatalf("exit = %d, want 0 (INCONCLUSIVE)", code)
	}
	got := out.String()
	if !strings.Contains(got, "| opencode | no-ejecutado |") {
		t.Fatalf("filtered matrix must keep opencode:\n%s", got)
	}
	if strings.Contains(got, "| claude |") || strings.Contains(got, "| codex |") {
		t.Fatalf("filtered matrix must drop other hosts:\n%s", got)
	}
}
