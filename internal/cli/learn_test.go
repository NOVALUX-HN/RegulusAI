package cli

// F12.6 cross-agent learning (NOV-101) CLI tests: verified demands
// evidence, hypotheses never read as facts, retrieval respects
// permissions, invalidation audits.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/mcp"
)

func learnSave(t *testing.T, args ...string) {
	t.Helper()
	c, _, errb := newCLI(nil)
	full := append([]string{"learn", "save"}, args...)
	if code := c.Run(full); code != 0 {
		t.Fatalf("learn save %v: exit = %d (%s)", args, code, errb.String())
	}
}

func TestLearnScopeMirrorsMCPPolicy(t *testing.T) {
	if almanac.MemoryReadScope != mcp.ScopeMemoryRead {
		t.Fatalf("MemoryReadScope = %q, want mcp.ScopeMemoryRead %q", almanac.MemoryReadScope, mcp.ScopeMemoryRead)
	}
}

func TestLearnSaveAndSearch(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	learnSave(t, "--kind", "verified-solution", "--title", "pool fix", "--author", "agent-alpha",
		"--scope", "project", "--confidence", "high", "--evidence", "go test ./... passes", "pooling speeds reads")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"learn", "search", "--as", "agent-alpha", "pooling"}); code != 0 {
		t.Fatalf("search exit = %d", code)
	}
	if !strings.Contains(out.String(), "pool fix") {
		t.Fatalf("search must surface the saved fix, got:\n%s", out.String())
	}
}

func TestLearnSaveRefusals(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cases := [][]string{
		{"--kind", "pattern", "--title", "p", "--scope", "project", "content here"},                             // no author
		{"--kind", "pattern", "--title", "p", "--author", "a", "content here"},                                  // no scope
		{"--kind", "verified-solution", "--title", "v", "--author", "a", "--scope", "project", "no proof here"}, // no evidence
		{"--kind", "session-note", "--title", "s", "--author", "a", "--scope", "project", "wrong door"},         // not learn
		{"--kind", "pattern", "--title", "p", "--author", "a", "--scope", "galaxy", "content here"},             // bad scope
	}
	for i, args := range cases {
		c, _, _ := newCLI(nil)
		if code := c.Run(append([]string{"learn", "save"}, args...)); code == 0 {
			t.Fatalf("case %d %v: must refuse", i, args)
		}
	}
}

func TestLearnVerifiedOnlyExcludesHypotheses(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	learnSave(t, "--kind", "hypothesis", "--title", "pooling hunch", "--author", "agent-alpha",
		"--scope", "shared", "pooling may speed things")
	learnSave(t, "--kind", "verified-solution", "--title", "pooling fix", "--author", "agent-alpha",
		"--scope", "shared", "--evidence", "bench passes", "pooling speeds things")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"learn", "search", "pooling"}); code != 0 {
		t.Fatalf("search exit = %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "pooling hunch") || !strings.Contains(got, "[hipótesis — no es un hecho]") {
		t.Fatalf("hypothesis must show labeled, got:\n%s", got)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"learn", "search", "--verified-only", "pooling"}); code != 0 {
		t.Fatalf("verified-only exit = %d", code)
	}
	if strings.Contains(out2.String(), "pooling hunch") || strings.Contains(out2.String(), "pooling fix") {
		t.Fatalf("verified-only shows only VERIFIED knowledge, got:\n%s", out2.String())
	}
	if err := os.MkdirAll(filepath.Join(dir, ".regulus", "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".regulus", "evidence", "r.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := lastLearnID(t, dir, "pooling fix")
	cv, _, _ := newCLI(nil)
	if code := cv.Run([]string{"learn", "verify", id, "--receipt", ".regulus/evidence/r.json"}); code != 0 {
		t.Fatalf("verify exit = %d", code)
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"learn", "search", "--verified-only", "pooling"}); code != 0 {
		t.Fatalf("verified-only exit = %d", code)
	}
	if !strings.Contains(out3.String(), "pooling fix") || !strings.Contains(out3.String(), "[verificado]") {
		t.Fatalf("verified fix shows labeled, got:\n%s", out3.String())
	}
}

func TestLearnVerifyNeedsEvidenceFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	learnSave(t, "--kind", "pattern", "--title", "pool pattern", "--author", "agent-alpha",
		"--scope", "project", "--evidence", "bench passes", "pool everywhere")
	id := lastLearnID(t, dir, "pool pattern")
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"learn", "verify", id, "--receipt", ".regulus/evidence/missing.json"}); code != 1 {
		t.Fatalf("missing receipt must fail, got %d (%s)", code, errb.String())
	}
	if err := os.MkdirAll(filepath.Join(dir, ".regulus", "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".regulus", "evidence", "r.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"learn", "verify", id, "--receipt", ".regulus/evidence/r.json"}); code != 0 {
		t.Fatalf("verify exit = %d", code)
	}
	if !strings.Contains(out2.String(), "-> VERIFIED") {
		t.Fatalf("verify must promote, got:\n%s", out2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"learn", "search", "--verified-only", "--as", "agent-alpha", "pool"}); code != 0 {
		t.Fatalf("search exit = %d", code)
	}
	if !strings.Contains(out3.String(), "[verificado]") {
		t.Fatalf("verified knowledge must label verified, got:\n%s", out3.String())
	}
}

func TestLearnHypothesisNeverVerifies(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	learnSave(t, "--kind", "hypothesis", "--title", "maybe cache", "--author", "agent-alpha",
		"--scope", "shared", "caching may help")
	id := lastLearnID(t, dir, "maybe cache")
	if err := os.MkdirAll(filepath.Join(dir, ".regulus", "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".regulus", "evidence", "r.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"learn", "verify", id, "--receipt", ".regulus/evidence/r.json"}); code != 1 {
		t.Fatalf("hypothesis verify must fail, got %d", code)
	}
	if !strings.Contains(errb.String(), "hypothes") {
		t.Fatalf("refusal must name hypotheses, got %q", errb.String())
	}
}

func TestLearnSearchRespectsPermissions(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	learnSave(t, "--kind", "pattern", "--title", "shared pattern", "--author", "agent-alpha",
		"--scope", "shared", "for everyone")
	learnSave(t, "--kind", "pattern", "--title", "private pattern", "--author", "agent-alpha",
		"--scope", "private", "only for alpha")
	learnSave(t, "--kind", "pattern", "--title", "project pattern", "--author", "agent-alpha",
		"--scope", "project", "needs scope")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"learn", "search", "--scopes", ""}); code != 0 {
		t.Fatalf("lockdown search exit = %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "shared pattern") {
		t.Fatalf("lockdown still sees shared, got:\n%s", got)
	}
	if strings.Contains(got, "private pattern") || strings.Contains(got, "project pattern") {
		t.Fatalf("lockdown must hide private/project, got:\n%s", got)
	}
	if !strings.Contains(got, "denied by ACL") {
		t.Fatalf("denials must be reported, got:\n%s", got)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"learn", "search", "--as", "agent-beta", "pattern"}); code != 0 {
		t.Fatalf("beta search exit = %d", code)
	}
	if strings.Contains(out2.String(), "private pattern") {
		t.Fatalf("beta must not see alpha's private knowledge, got:\n%s", out2.String())
	}
}

func TestLearnInvalidateAndAudit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	learnSave(t, "--kind", "error", "--title", "nil map panic", "--author", "agent-alpha",
		"--scope", "project", "--evidence", "repro passes", "never write to nil maps")
	id := lastLearnID(t, dir, "nil map panic")
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"learn", "invalidate", id}); code != 2 {
		t.Fatalf("invalidate without reason is a usage error, got %d", code)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"learn", "invalidate", id, "--reason", "runtime changed", "--by", "reviewer"}); code != 0 {
		t.Fatalf("invalidate exit = %d", code)
	}
	if !strings.Contains(out2.String(), "REJECTED by reviewer") {
		t.Fatalf("invalidate must record actor, got:\n%s", out2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"learn", "search", "--as", "agent-alpha", "nil map"}); code != 0 {
		t.Fatalf("search exit = %d", code)
	}
	if strings.Contains(out3.String(), "nil map panic") {
		t.Fatalf("invalidated knowledge hides by default, got:\n%s", out3.String())
	}
	c4, out4, _ := newCLI(nil)
	if code := c4.Run([]string{"memory", "status", id}); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	if !strings.Contains(out4.String(), "REJECTED") || !strings.Contains(out4.String(), "reviewer") {
		t.Fatalf("audit must show the rejection trail, got:\n%s", out4.String())
	}
}

func TestLearnResolveContradiction(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	learnSave(t, "--kind", "pattern", "--title", "pool always", "--author", "agent-alpha",
		"--scope", "project", "pool every connection")
	learnSave(t, "--kind", "pattern", "--title", "never pool", "--author", "agent-alpha",
		"--scope", "project", "pooling hurts here")
	a := lastLearnID(t, dir, "pool always")
	b := lastLearnID(t, dir, "never pool")
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "relate", "--id", a, "--relation", "conflicts_with", "--to", b}); code != 0 {
		t.Fatalf("relate exit = %d", code)
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"learn", "resolve", "--winner", a, "--loser", b}); code != 0 {
		t.Fatalf("resolve exit = %d (%s)", code, errb.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"learn", "search", "--as", "agent-alpha"}); code != 0 {
		t.Fatalf("search exit = %d", code)
	}
	if !strings.Contains(out3.String(), "pool always") || strings.Contains(out3.String(), "never pool") {
		t.Fatalf("loser hides, winner stays, got:\n%s", out3.String())
	}
}

func TestLearnMetricsReport(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"learn", "metrics"}); code != 0 {
		t.Fatalf("metrics exit = %d", code)
	}
	for _, want := range []string{"total: 0", "precision: 1.00", "useful recall: 1.00", "obsolescence: 0.00", "noise: 0.00"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("empty metrics must read clean (%q), got:\n%s", want, out.String())
		}
	}
	learnSave(t, "--kind", "verified-solution", "--title", "fix one", "--author", "agent-alpha",
		"--scope", "shared", "--evidence", "bench", "pooling speeds reads")
	learnSave(t, "--kind", "hypothesis", "--title", "a hunch", "--author", "agent-alpha",
		"--scope", "shared", "pooling may speed deletes")
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"learn", "metrics"}); code != 0 {
		t.Fatalf("metrics exit = %d", code)
	}
	for _, want := range []string{"total: 2", "hypotheses: 1"} {
		if !strings.Contains(out2.String(), want) {
			t.Fatalf("metrics must count (%q), got:\n%s", want, out2.String())
		}
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"learn", "metrics", "--json"}); code != 0 {
		t.Fatalf("metrics --json exit = %d", code)
	}
	if !strings.Contains(out3.String(), `"total": 2`) {
		t.Fatalf("json metrics must carry totals, got:\n%s", out3.String())
	}
}

func TestLearnHelp(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"learn", "--help"}); code != 0 {
		t.Fatalf("learn --help exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage: regulus learn save|search|verify|invalidate|resolve|metrics") {
		t.Fatalf("help must document the surface, got:\n%s", out.String())
	}
}

func lastLearnID(t *testing.T, dir, title string) string {
	t.Helper()
	all, err := almanac.All(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Title == title {
			return all[i].ID
		}
	}
	t.Fatalf("no observation titled %q", title)
	return ""
}
