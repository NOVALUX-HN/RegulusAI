package cli

import (
	"strings"
	"testing"
)

func TestGateRunQuickProfile(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"req","layer":1,"command":"echo req"},
		{"name":"opt","layer":2,"command":"echo opt","required":false,"profiles":["full"]}
	]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "--profile", "quick", "demo/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "layer 1 req") || strings.Contains(s, "layer 2 opt") {
		t.Fatalf("quick must run required only, got:\n%s", s)
	}
	if !strings.Contains(s, "gauntlet: 1/1 gates passed") {
		t.Fatalf("counts must reflect the profile, got:\n%s", s)
	}
}

func TestGateRunUnknownProfileRejected(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[{"name":"a","layer":1,"command":"echo hi"}]}`)
	t.Chdir(dir)
	c, _, errb := newCLI(nil)
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"gate", "run", "--profile", "nope", "demo/T1"}); code != 1 {
		t.Fatalf("unknown profile must fail, got %d", code)
	}
	if !strings.Contains(errb.String(), "unknown profile") {
		t.Fatalf("stderr must name the problem, got %q", errb.String())
	}
}

func TestGateRunAdvisoryFooter(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"a","layer":1,"command":"ok-a"},
		{"name":"b","layer":2,"command":"fail-b","on_fail":"advisory"}
	]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"fail-b": {code: 1, stderr: "soft"},
	}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "demo/T1"}); code != 0 {
		t.Fatalf("advisory-only failure must exit 0, got %d, output:\n%s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "advisory failure(s)") {
		t.Fatalf("footer must name advisory failures, got:\n%s", s)
	}
	if !strings.Contains(s, "verdict: PASS (ok)") {
		t.Fatalf("overall verdict stays PASS, got:\n%s", s)
	}
}

func TestGateRunSkipLine(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"go-check","layer":1,"command":"check-go"},
		{"name":"docs-check","layer":2,"command":"check-docs","required":false,"when":["*.md"]}
	]}`)
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {stdout: " M main.go\x00"},
	}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "demo/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "docs-check") || !strings.Contains(s, "skipped") {
		t.Fatalf("skip must be visible, got:\n%s", s)
	}
	if !strings.Contains(s, "1 gate(s) skipped") {
		t.Fatalf("footer must count skips, got:\n%s", s)
	}
}
