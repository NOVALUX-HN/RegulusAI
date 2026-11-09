package cli

import (
	"strings"
	"testing"
)

func twelveGateConfig() string {
	return `{"gates":[
		{"name":"typecheck","layer":1,"command":"echo typecheck"},
		{"name":"lint","layer":2,"command":"echo lint"},
		{"name":"build","layer":3,"command":"echo build"},
		{"name":"unit-tests","layer":4,"command":"echo unit-tests"},
		{"name":"coverage-floor","layer":5,"command":"echo coverage-floor"},
		{"name":"mutation","layer":6,"command":"echo mutation","required":false,"profiles":["full"]},
		{"name":"crap","layer":7,"command":"echo crap"},
		{"name":"vulncheck","layer":8,"command":"echo vulncheck","required":false,"profiles":["full"]},
		{"name":"spec","layer":9,"command":"echo spec"},
		{"name":"ts-typecheck","layer":10,"command":"echo ts-typecheck"},
		{"name":"ts-unit-tests","layer":11,"command":"echo ts-unit-tests"},
		{"name":"gofmt","layer":12,"command":"echo gofmt"}
	]}`
}

// T5 RED: `gate run --quick` runs exactly the lean daily-loop set.
func TestGateRunQuickFlagLean(t *testing.T) {
	dir := writeGatesProject(t, twelveGateConfig())
	t.Chdir(dir)
	r := &fakeGateRunner{results: map[string]fakeGateResult{}}
	c, out, _ := newCLI(nil)
	c.Runner = r
	if code := c.Run([]string{"gate", "run", "--quick", "demo/T5"}); code != 0 {
		t.Fatalf("exit = %d, want 0, output:\n%s", code, out.String())
	}
	s := out.String()
	for _, want := range []string{"typecheck", "lint", "build", "unit-tests", "spec", "gofmt"} {
		if !strings.Contains(s, want) {
			t.Fatalf("quick output must contain %q, got:\n%s", want, s)
		}
	}
	for _, skip := range []string{"coverage-floor", "mutation", "crap", "vulncheck", "ts-typecheck", "ts-unit-tests"} {
		if strings.Contains(s, "layer ") && strings.Contains(s, skip) && !strings.Contains(s, "skipped") {
			t.Fatalf("quick must not run full-only gate %q, got:\n%s", skip, s)
		}
	}
	if !strings.Contains(s, "gauntlet: 6/6 gates passed") {
		t.Fatalf("counts must reflect the lean set, got:\n%s", s)
	}
}

// T5 RED: --quick and --profile are mutually exclusive.
func TestGateRunQuickRejectsProfile(t *testing.T) {
	dir := writeGatesProject(t, twelveGateConfig())
	t.Chdir(dir)
	c, _, errb := newCLI(nil)
	c.Runner = &fakeGateRunner{}
	if code := c.Run([]string{"gate", "run", "--quick", "--profile", "full", "demo/T5"}); code != 2 {
		t.Fatalf("--quick --profile: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "mutually exclusive") {
		t.Fatalf("stderr must name the conflict, got %q", errb.String())
	}
}
