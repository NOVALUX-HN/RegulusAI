// F11.5 (NOV-91) CLI: correlation keys on record, agent/run filters and
// host grouping on cost, explicit-tariff money, host-status provenance.
// RED: new flags, filters, money lines and the host-status table.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceRecordAgentAndReasoningRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	args := []string{"trace", "record", "--session", "s1", "--message", "m1", "--call", "c1",
		"--tool", "read", "--status", "completed", "--task", "agent/T1", "--agent", "worker-a",
		"--input-tokens", "100", "--output-tokens", "33", "--reasoning-tokens", "22",
		"--model", "beam-free", "--host", "opencode", "--host-version", "2.2.5"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("record exit = %d, stderr %q", code, errb.String())
	}
	c2, out2, errb2 := newCLI(nil)
	if code := c2.Run([]string{"trace", "cost", "--agent", "worker-a"}); code != 0 {
		t.Fatalf("cost --agent exit = %d, stderr %q", code, errb2.String())
	}
	if got := out2.String(); !strings.Contains(got, "agent/T1") {
		t.Fatalf("cost --agent must attribute the bucket, got:\n%s", got)
	}
}

func TestTraceCostByAgentAndHost(t *testing.T) {
	t.Chdir(t.TempDir())
	rec := func(session, call, agent, host string) {
		t.Helper()
		c, _, errb := newCLI(nil)
		args := []string{"trace", "record", "--session", session, "--message", "m" + call,
			"--call", call, "--tool", "read", "--status", "completed",
			"--task", "T1", "--agent", agent, "--host", host,
			"--input-tokens", "10", "--output-tokens", "5"}
		if code := c.Run(args); code != 0 {
			t.Fatalf("record %s: exit = %d, stderr %q", call, code, errb.String())
		}
	}
	rec("s1", "c1", "worker-a", "opencode")
	rec("s2", "c2", "worker-b", "claude")

	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "cost", "--by", "agent"}); code != 0 {
		t.Fatalf("cost --by agent exit = %d", code)
	}
	if got := out.String(); !strings.Contains(got, "agent worker-a:") || !strings.Contains(got, "agent worker-b:") {
		t.Fatalf("--by agent must group per agent, got:\n%s", got)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"trace", "cost", "--by", "host"}); code != 0 {
		t.Fatalf("cost --by host exit = %d", code)
	}
	if got := out2.String(); !strings.Contains(got, "host opencode:") || !strings.Contains(got, "host claude:") {
		t.Fatalf("--by host must group per host, got:\n%s", got)
	}
	c3, _, errb3 := newCLI(nil)
	if code := c3.Run([]string{"trace", "cost", "--by", "bogus"}); code != 2 {
		t.Fatalf("--by bogus exit = %d, want 2", code)
	} else if !strings.Contains(errb3.String(), "must be task, agent or host") {
		t.Fatalf("--by bogus must name the rule, got %q", errb3.String())
	}
}

func TestTraceCostRunFilter(t *testing.T) {
	t.Chdir(t.TempDir())
	rec := func(call, exec string) {
		t.Helper()
		c, _, errb := newCLI(nil)
		args := []string{"trace", "record", "--session", "s", "--message", "m" + call,
			"--call", call, "--tool", "read", "--status", "completed", "--task", "T1",
			"--execution-id", exec, "--input-tokens", "10", "--output-tokens", "5",
			"--model", "m", "--host", "opencode"}
		if code := c.Run(args); code != 0 {
			t.Fatalf("record %s: exit = %d, stderr %q", call, code, errb.String())
		}
	}
	rec("c1", "run-9")
	rec("c2", "run-8")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "cost", "--run", "run-9"}); code != 0 {
		t.Fatalf("cost --run exit = %d", code)
	}
	if got := out.String(); !strings.Contains(got, "1 call(s)") || strings.Contains(got, "2 call(s)") {
		t.Fatalf("--run must scope to one execution, got:\n%s", got)
	}
}

func TestTraceCostRateMoney(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	rec := []string{"trace", "record", "--session", "s", "--message", "m", "--call", "c",
		"--tool", "read", "--status", "completed", "--task", "agent/T1",
		"--input-tokens", "1000", "--output-tokens", "500", "--reasoning-tokens", "100",
		"--model", "beam-free", "--host", "opencode", "--host-version", "2.2.5"}
	if code := c.Run(rec); code != 0 {
		t.Fatalf("record exit = %d, stderr %q", code, errb.String())
	}
	rate := `[{"model":"beam-free","host":"opencode","host_version":"2.2.5","effective_date":"2026-11-01","currency":"USD","input_per_1k":0.1,"output_per_1k":0.4,"reasoning_per_1k":0.4}]`
	ratePath := filepath.Join(t.TempDir(), "rates.json")
	if err := os.WriteFile(ratePath, []byte(rate), 0o644); err != nil {
		t.Fatal(err)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"trace", "cost", "--task", "agent/T1", "--rate", ratePath}); code != 0 {
		t.Fatalf("cost --rate exit = %d", code)
	}
	if got := out2.String(); !strings.Contains(got, "money") || !strings.Contains(got, "USD") || !strings.Contains(got, "2026-11-01") {
		t.Fatalf("cost --rate must cite money with currency and date, got:\n%s", got)
	}
	c3, _, errb3 := newCLI(nil)
	if code := c3.Run([]string{"trace", "cost", "--rate", filepath.Join(t.TempDir(), "nope.json")}); code != 1 {
		t.Fatalf("missing rate file exit = %d, want 1, stderr %q", code, errb3.String())
	}
	c4, out4, _ := newCLI(nil)
	if code := c4.Run([]string{"trace", "cost", "--task", "agent/T1", "--rate", ratePath, "--json-missing-ok"}); code != 2 {
		t.Fatalf("stray flag exit = %d, want 2", code)
		_ = out4
	}
}

func TestTraceHostStatusDeclaresEveryHost(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "host-status"}); code != 0 {
		t.Fatalf("host-status exit = %d", code)
	}
	got := out.String()
	for _, want := range []string{"opencode", "claude", "codex", "devin", "measured-host", "unknown"} {
		if !strings.Contains(got, want) {
			t.Fatalf("host-status missing %q:\n%s", want, got)
		}
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"trace", "host-status", "--json"}); code != 0 {
		t.Fatalf("host-status --json exit = %d", code)
	}
	var rows []struct {
		Host   string `json:"host"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(out2.String()), &rows); err != nil {
		t.Fatalf("host-status --json must parse: %v\n%s", err, out2.String())
	}
	if len(rows) != 4 {
		t.Fatalf("host-status rows = %d, want 4 (one declared state per host)", len(rows))
	}
	for _, r := range rows {
		if r.Host == "" || r.Source == "" || r.Reason == "" {
			t.Fatalf("every host must declare source and reason: %+v", r)
		}
	}
}
