package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decideFixture(t *testing.T, contract string) string {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("charts", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("charts", "c.md"), []byte("# c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("internal", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("internal", "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "d.json")
	if err := os.WriteFile(path, []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const decideContract = `{
	"problem": "gate runs repeat unchanged work on every invocation",
	"scope": "gate execution",
	"context": "full profile runs all gates even when nothing changed",
	"proposal": "add when-conditions so optional gates skip on unchanged trees",
	"alternatives": [{"name": "keep all-always", "tradeoff": "simpler but slower"}],
	"tradeoffs": ["one git status per run, cheap and simple"],
	"constraints": ["mandatory: required gates always run"],
	"risks": ["stale skip output -> fail open with a receipt note"],
	"consequences": "faster no-op runs",
	"evidence": ["chart:c"],
	"confidence": "medium",
	"reversibility": "easy",
	"rollback": "",
	"artifacts": ["internal/x.go"],
	"commits": [],
	"task": "",
	"related": [],
	"critical": false
}`

func TestDecideUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"decide"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus decide") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestDecideProposeShowList(t *testing.T) {
	path := decideFixture(t, decideContract)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"decide", "propose", "--file", path}); code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "PROPOSED") {
		t.Fatalf("output = %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"decide", "list"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "PROPOSED") || !strings.Contains(out2.String(), "gate execution") {
		t.Fatalf("list must show state and scope, got:\n%s", out2.String())
	}
}

func TestDecideProposeRejectsBadContract(t *testing.T) {
	t.Chdir(t.TempDir())
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte(`{"problem": ""}`), 0o644)
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"decide", "propose", "--file", bad}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

func TestDecideAssessAndResolve(t *testing.T) {
	path := decideFixture(t, decideContract)
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"decide", "propose", "--file", path}); code != 0 {
		t.Fatal("propose must succeed")
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"decide", "list"}); code != 0 {
		t.Fatal("list must succeed")
	}
	var id string
	for _, line := range strings.Split(out2.String(), "\n") {
		if strings.HasPrefix(line, "dec_") {
			id = strings.Fields(line)[0]
		}
	}
	if id == "" {
		t.Fatalf("list must print the id, got:\n%s", out2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"decide", "assess", id}); code != 0 {
		t.Fatalf("assess exit = %d, output:\n%s", code, out3.String())
	}
	if !strings.Contains(out3.String(), "PERCEPTION") || !strings.Contains(out3.String(), "RECOMMENDATION") {
		t.Fatalf("assess must show pipeline stages, got:\n%s", out3.String())
	}
	c4, out4, _ := newCLI(nil)
	if code := c4.Run([]string{"decide", "resolve", id, "--accept"}); code != 0 {
		t.Fatalf("accept exit = %d, output:\n%s", code, out4.String())
	}
	if !strings.Contains(out4.String(), "ACCEPTED") {
		t.Fatalf("output = %q", out4.String())
	}
	c5, out5, _ := newCLI(nil)
	if code := c5.Run([]string{"decide", "show", id}); code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	if !strings.Contains(out5.String(), "ACCEPTED") || !strings.Contains(out5.String(), "history") {
		t.Fatalf("show must print state and history, got:\n%s", out5.String())
	}
}

func TestDecideResolveGovernance(t *testing.T) {
	path := decideFixture(t, decideContract)
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"decide", "propose", "--file", path}); code != 0 {
		t.Fatal("propose must succeed")
	}
	c2, out2, _ := newCLI(nil)
	c2.Run([]string{"decide", "list"})
	var id string
	for _, line := range strings.Split(out2.String(), "\n") {
		if strings.HasPrefix(line, "dec_") {
			id = strings.Fields(line)[0]
		}
	}
	// Critical decision without human approval is refused.
	crit := strings.Replace(decideContract, `"critical": false`, `"critical": true`, 1)
	critPath := filepath.Join(t.TempDir(), "crit.json")
	os.WriteFile(critPath, []byte(crit), 0o644)
	c3, _, _ := newCLI(nil)
	if code := c3.Run([]string{"decide", "propose", "--file", critPath}); code != 0 {
		t.Fatal("critical propose must succeed")
	}
	c4, out4, _ := newCLI(nil)
	c4.Run([]string{"decide", "list"})
	var critID string
	for _, line := range strings.Split(out4.String(), "\n") {
		if f := strings.Fields(line); len(f) > 0 && strings.HasPrefix(f[0], "dec_") && f[0] != id {
			critID = f[0]
		}
	}
	if critID == "" {
		t.Fatalf("list must show the critical id, got:\n%s", out4.String())
	}
	_ = id
	c5, out5, _ := newCLI(nil)
	if code := c5.Run([]string{"decide", "resolve", critID, "--accept"}); code != 1 {
		t.Fatalf("critical accept without human must fail, output:\n%s", out5.String())
	}
	c6, out6, _ := newCLI(nil)
	if code := c6.Run([]string{"decide", "resolve", critID, "--accept", "--approve-by", "ana: reviewed, risk accepted"}); code != 0 {
		t.Fatalf("human-approved critical must accept, exit %d output:\n%s", code, out6.String())
	}
}
