package cli

// M4 `memory mine` at the CLI: dry run writes nothing, --apply admits
// facts and discards contradicted ones, --domain scopes the series,
// and re-running stays idempotent.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

func seedMineChart(t *testing.T, chart, domain string) {
	t.Helper()
	if err := os.MkdirAll("charts", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("charts", chart+".md"),
		[]byte("# "+chart+"\nDomain: "+domain+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func seedMineReceipt(t *testing.T, task, gate, verdict, runID string, at time.Time) {
	t.Helper()
	if _, err := receipts.Write(evidence.TaskDir(".", task), receipts.Receipt{
		Task: task, Gate: gate, Verdict: verdict, RunID: runID,
		StartedAt: at, Command: "cmd",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryMineDryRunWritesNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	seedMineChart(t, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		seedMineReceipt(t, "feat/T1", "crap", receipts.VerdictFail,
			"run_dry"+string(rune('0'+i)), base.Add(time.Duration(i)*time.Hour))
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "mine"}); code != 0 {
		t.Fatalf("mine dry run: exit = %d", code)
	}
	if !strings.Contains(out.String(), "would-mine envfact/sextant/gate/crap") {
		t.Fatalf("dry run must name the candidate, got %q", out.String())
	}
	all, err := almanac.All(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("dry run must write nothing, got %+v", all)
	}
}

func TestMemoryMineRejectsPositional(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "mine", "extra"}); code != 2 {
		t.Fatalf("positional must be a usage error, got %d", code)
	}
}

func TestMemoryMineApplyMinesAndStaysIdempotent(t *testing.T) {
	t.Chdir(t.TempDir())
	seedMineChart(t, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		seedMineReceipt(t, "feat/T1", "crap", receipts.VerdictFail,
			"run_app"+string(rune('0'+i)), base.Add(time.Duration(i)*time.Hour))
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "mine", "--apply"}); code != 0 {
		t.Fatalf("mine --apply: exit = %d", code)
	}
	if !strings.Contains(out.String(), "1 mined, 0 discarded") {
		t.Fatalf("apply must mine the candidate, got %q", out.String())
	}
	// The mined fact rides the existing domain injection search.
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"memory", "search", "--type", "lesson", "--domain", "sextant"}); code != 0 {
		t.Fatalf("search: exit = %d", code)
	}
	if !strings.Contains(out2.String(), "crap") {
		t.Fatalf("mined fact must surface via domain search, got %q", out2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"memory", "mine", "--apply"}); code != 0 {
		t.Fatalf("second mine --apply: exit = %d", code)
	}
	if !strings.Contains(out3.String(), "0 mined, 0 discarded, 1 skipped") {
		t.Fatalf("re-mine must skip the live fact, got %q", out3.String())
	}
}

func TestMemoryMineDomainFilter(t *testing.T) {
	t.Chdir(t.TempDir())
	seedMineChart(t, "feat", "sextant")
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		seedMineReceipt(t, "feat/T1", "crap", receipts.VerdictFail,
			"run_dom"+string(rune('0'+i)), base.Add(time.Duration(i)*time.Hour))
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "mine", "--domain", "nosuchdomain"}); code != 0 {
		t.Fatalf("mine --domain: exit = %d", code)
	}
	if strings.Contains(out.String(), "would-mine") {
		t.Fatalf("domain filter must hide other domains, got %q", out.String())
	}
}
