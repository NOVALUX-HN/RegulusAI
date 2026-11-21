// Host-capabilities CRAP paydown (BenchHostCapabilitiesCmd split):
// probe-only plus run/out through fakes, flag refusals, and the
// extracted pure helpers. Portable: t.TempDir + emptied PATH (no
// host binaries), no network, no shell.
package cmdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

func TestBenchHostCapProbeOnlyPrintsTable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", t.TempDir()) // no host binaries at all
	io, out, errb := cmdIO(dir)
	if code := io.BenchHostCapabilitiesCmd(nil); code != 0 {
		t.Fatalf("probe exit = %d, want 0 stderr = %q", code, errb.String())
	}
	got := out.String()
	for _, want := range []string{
		"clase de ejecución probe",
		"host-capabilities OK:",
		"traza:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("probe output must contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "clase de ejecución journey") {
		t.Fatalf("probe must not claim a journey:\n%s", got)
	}
}

func TestBenchHostCapRunModesStayHermetic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", t.TempDir()) // no host binaries at all
	io, out, errb := cmdIO(dir)
	// Strategy hosts never execute: the run loop skips devin before
	// any probe, so no journey (and no 5-minute step) ever starts.
	if code := io.BenchHostCapabilitiesCmd([]string{"--run", "--host", "devin"}); code != 0 {
		t.Fatalf("run devin exit = %d, want 0 stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "clase de ejecución journey") {
		t.Fatalf("run must label the journey class:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "hoy: not-run (devin ships desktop-only") {
		t.Fatalf("devin must report not-run, never execute:\n%s", out.String())
	}
	// A missing binary fails the probe, and the run loop skips the
	// journey the same way: fast, no model, no shell.
	out.Reset()
	errb.Reset()
	if code := io.BenchHostCapabilitiesCmd([]string{"--run", "--host", "opencode"}); code != 0 {
		t.Fatalf("run opencode exit = %d, want 0 stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "hoy: not-run (") {
		t.Fatalf("missing binary must report not-run:\n%s", out.String())
	}
}

func TestBenchHostCapOutSealsEvidence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", t.TempDir()) // no host binaries at all
	io, _, errb := cmdIO(dir)
	evDir := filepath.Join(dir, "ev")
	if code := io.BenchHostCapabilitiesCmd([]string{"--out", evDir, "--host", "devin"}); code != 0 {
		t.Fatalf("out exit = %d, want 0 stderr = %q", code, errb.String())
	}
	entries, err := os.ReadDir(evDir)
	if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "host-capabilities-") {
		t.Fatalf("evidence pack = %v, %v; want one host-capabilities-*.json", entries, err)
	}
	raw, err := os.ReadFile(filepath.Join(evDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var pack map[string]any
	if err := json.Unmarshal(raw, &pack); err != nil {
		t.Fatal(err)
	}
	if pack["execution_class"] != bench.ClassProbe {
		t.Fatalf("execution_class = %v, want probe", pack["execution_class"])
	}
	devin, ok := pack["devin"].(map[string]any)
	if !ok {
		t.Fatalf("devin evidence missing: %v", pack)
	}
	if devin["class"] != bench.ClassStrategy {
		t.Fatalf("devin class = %v, want strategy", devin["class"])
	}
}

func TestBenchHostCapFlagRefusals(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	cases := []struct {
		name string
		args []string
		msg  string
	}{
		{name: "unknown flag", args: []string{"--bogus"}, msg: "flag provided but not defined"},
		{name: "positional", args: []string{"extra"}, msg: "usage:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			io, out, errb := cmdIO(dir)
			if code := io.BenchHostCapabilitiesCmd(c.args); code != 2 {
				t.Fatalf("exit = %d, want 2 (out=%q err=%q)", code, out.String(), errb.String())
			}
			if !strings.Contains(out.String()+errb.String(), c.msg) {
				t.Fatalf("must mention %q, got out=%q err=%q", c.msg, out.String(), errb.String())
			}
		})
	}
}

func TestHostCapHelperPurity(t *testing.T) {
	if got := hostCapExecClass(false); got != bench.ClassProbe {
		t.Fatalf("exec class without --run = %q, want probe", got)
	}
	if got := hostCapExecClass(true); got != bench.ClassJourney {
		t.Fatalf("exec class with --run = %q, want journey", got)
	}
	now := "2026-01-01T00:00:00Z"
	manifests := bench.DefaultHostManifests(now)
	applyHostJourneyOutcomes(manifests, map[string]bench.HostJourneyOutcome{}, "devin", now)
	if len(manifests) != 1 {
		t.Fatalf("only=devin must keep one manifest, got %d", len(manifests))
	}
	if manifests["devin"].Class != bench.ClassStrategy {
		t.Fatalf("devin class = %q, want strategy", manifests["devin"].Class)
	}
	// A probe outcome never promotes or demotes: the dated journey
	// class survives, only a probe note lands.
	manifests = bench.DefaultHostManifests(now)
	outcomes := map[string]bench.HostJourneyOutcome{
		"opencode": {Host: "opencode", Level: bench.LevelNotRun, Status: bench.AgentNotRun, Reason: "sin binario"},
	}
	applyHostJourneyOutcomes(manifests, outcomes, "all", now)
	if len(manifests) != 4 {
		t.Fatalf("only=all must keep every manifest, got %d", len(manifests))
	}
	if got := manifests["opencode"].ProbeNote; !strings.Contains(got, "sin binario") {
		t.Fatalf("probe note must carry the reason, got %q", got)
	}
	if manifests["opencode"].Class != bench.ClassJourney {
		t.Fatalf("probe must not demote the journey class, got %q", manifests["opencode"].Class)
	}
	stampHostTraceHints(manifests, "hint-1")
	for h, m := range manifests {
		if m.TraceHint != "hint-1" {
			t.Fatalf("host %s trace hint = %q, want hint-1", h, m.TraceHint)
		}
	}
}
