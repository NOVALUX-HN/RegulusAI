// F11.7: host-capabilities. Probe mode never claims a journey,
// strategy hosts never execute, evidence is attributable and
// redacted, and every run links its F11.1 trace.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

func TestBenchHostCapabilitiesProbeNeverClaimsJourney(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir()) // no host binaries at all
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "host-capabilities"}); code != 0 {
		t.Fatalf("exit = %d, want 0 (capabilities report, never fails without hosts)", code)
	}
	got := out.String()
	if !strings.Contains(got, "clase de ejecución probe") {
		t.Fatalf("probe run must label its execution class:\n%s", got)
	}
	if strings.Contains(got, "clase de ejecución journey") {
		t.Fatalf("probe run must not claim a journey execution:\n%s", got)
	}
	for _, want := range []string{
		bench.CapVerified, bench.CapUnsupported, bench.CapNotExecuted, bench.CapUnknown,
		"| opencode |", "| devin |", "traza: regulus trace timeline --run run_",
		"host-capabilities OK:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output must contain %q:\n%s", want, got)
		}
	}
}

func TestBenchHostCapabilitiesEvidencePack(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", t.TempDir()) // no host binaries at all
	c, _, _ := newCLI(nil)
	evDir := filepath.Join(dir, "ev")
	if code := c.Run([]string{"bench", "host-capabilities", "--out", evDir, "--host", "devin"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
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
		t.Fatalf("execution_class = %v, want probe (never journey)", pack["execution_class"])
	}
	devin, ok := pack["devin"].(map[string]any)
	if !ok {
		t.Fatalf("devin evidence missing: %v", pack)
	}
	if devin["class"] != bench.ClassStrategy {
		t.Fatalf("devin class = %v, want strategy", devin["class"])
	}
	if _, ok := devin["config"]; !ok {
		t.Fatal("evidence must record non-sensitive config refs")
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "host-capabilities", "--out", blocker, "--host", "devin"}); code == 0 {
		t.Fatal("saving into a file path must fail")
	}
}

func TestBenchHostCapabilitiesStrategyNeverExecutes(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("REGULUS_BIN", writeFakeRegulus(t, ""))
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "host-capabilities", "--run", "--host", "devin"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "clase de ejecución journey") {
		t.Fatalf("run mode must label its execution class:\n%s", got)
	}
	if !strings.Contains(got, "hoy: not-run (devin ships desktop-only") {
		t.Fatalf("devin must report not-run with its reason, never execute:\n%s", got)
	}
}

func TestBenchHostCapabilitiesRunUpgradesGreenHosts(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("REGULUS_BIN", writeFakeRegulus(t, ""))
	// Fake only fails when a host binary exists; with an empty PATH
	// every host probes not-run and keeps its declared state.
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "host-capabilities", "--run", "--host", "opencode"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "hoy: not-run") {
		t.Fatalf("missing binary must report not-run today:\n%s", out.String())
	}
}

func TestBenchHostCapabilitiesUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"bench", "host-capabilities", "--bogus"}); code != 2 {
		t.Fatalf("bogus flag exit = %d, want 2", code)
	}
	_ = errb
	c, _, errb = newCLI(nil)
	if code := c.Run([]string{"bench", "host-capabilities", "extra"}); code != 2 {
		t.Fatalf("extra arg exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus bench host-capabilities") {
		t.Fatalf("usage must print:\n%s", errb.String())
	}
}
