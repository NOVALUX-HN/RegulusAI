package cli

// T3 (pro-harness, token-economizer): `trace usage` reports the
// measured per-tool usage the plugin's toolset pruning reads — one
// row per called tool (calls + sessions) plus the session count that
// carries the evidence floor. Human text by default, --json for the
// plugin (which parses it without scraping prose).
import (
	"encoding/json"
	"strings"
	"testing"
)

func recordUsageCall(t *testing.T, session, call, tool string) {
	t.Helper()
	c, _, errb := newCLI(nil)
	args := []string{"trace", "record", "--session", session, "--message", "m" + call,
		"--call", call, "--tool", tool, "--status", "completed"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("record %s: exit = %d, stderr %q", call, code, errb.String())
	}
}

func TestTraceUsageReportsRowsAndSessions(t *testing.T) {
	t.Chdir(t.TempDir())
	recordUsageCall(t, "s1", "c1", "read")
	recordUsageCall(t, "s1", "c2", "read")
	recordUsageCall(t, "s2", "c3", "edit")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "usage"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"sessions: 2", "read", "2 call(s)", "edit", "1 call(s)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage output missing %q:\n%s", want, got)
		}
	}
}

func TestTraceUsageJSON(t *testing.T) {
	t.Chdir(t.TempDir())
	recordUsageCall(t, "s1", "c1", "read")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "usage", "--json"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var rep struct {
		Sessions int `json:"sessions"`
		Tools    []struct {
			Tool     string   `json:"tool"`
			Calls    int      `json:"calls"`
			Sessions []string `json:"sessions"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out.String()), &rep); err != nil {
		t.Fatalf("usage --json is not a report: %v\n%s", err, out.String())
	}
	if rep.Sessions != 1 || len(rep.Tools) != 1 || rep.Tools[0].Tool != "read" || rep.Tools[0].Calls != 1 {
		t.Fatalf("usage report = %+v", rep)
	}
}

func TestTraceUsageEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "usage"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "no sessions traced") {
		t.Fatalf("empty usage must say so, got %q", out.String())
	}
}
