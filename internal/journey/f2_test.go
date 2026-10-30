package journey

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
)

// findGremlins locates the mutation-testing binary (PATH, then ~/go/bin).
func findGremlins(t *testing.T) string {
	t.Helper()
	if path, err := exec.LookPath("gremlins"); err == nil {
		return filepath.ToSlash(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{"gremlins.exe", "gremlins"} {
		p := filepath.Join(home, "go", "bin", name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return filepath.ToSlash(p)
		}
	}
	return ""
}

// TestF2SevenLayers walks a real Go fixture project through the complete
// seven-layer gauntlet (F2 exit criterion): binary gates, the coverage
// floor, mutation testing and CRAP — in-process, with real shell gates and
// a freshly built regulus binary. Skips with a reason when gremlins is not
// installed (the mutation gate is verified live in the session dogfood,
// feature doc L17).
func TestF2SevenLayers(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	gremlinsBin := findGremlins(t)
	if gremlinsBin == "" {
		t.Skip("gremlins is not installed — the F2 mutation-layer journey skips; the mutation gate was verified live in the session dogfood (feature doc L17)")
	}

	dir := t.TempDir()
	regulusBin := filepath.ToSlash(filepath.Join(dir, "regulus.exe"))
	build := exec.Command("go", "build", "-o", regulusBin, "./cmd/regulus")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building regulus: %v\n%s", err, out)
	}

	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A small real Go module with mutation-killing tests.
	write("go.mod", "module fixture\n\ngo 1.27\n")
	write(".gitignore", ".regulus/\nbin/\nregulus.exe\n")
	write("pkg/calc/calc.go", `package calc

func Add(a, b int) int {
	return a + b
}

func Branchy(x int) string {
	if x > 0 {
		return "positive"
	}
	if x < 0 {
		return "negative"
	}
	return "zero"
}
`)
	write("pkg/calc/calc_test.go", `package calc

import "testing"

func TestAdd(t *testing.T) {
	cases := []struct{ a, b, want int }{
		{1, 2, 3},
		{0, 0, 0},
		{-1, 1, 0},
		{2, -5, -3},
	}
	for _, tc := range cases {
		if got := Add(tc.a, tc.b); got != tc.want {
			t.Fatalf("Add(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestBranchy(t *testing.T) {
	if Branchy(5) != "positive" {
		t.Fatal("Branchy(5) must be positive")
	}
	if Branchy(-3) != "negative" {
		t.Fatal("Branchy(-3) must be negative")
	}
	if Branchy(0) != "zero" {
		t.Fatal("Branchy(0) must be zero")
	}
}
`)
	write("cmd/fixture/main.go", `package main

import (
	"fmt"

	"fixture/pkg/calc"
)

func main() {
	fmt.Println(calc.Add(1, 2), calc.Branchy(0))
}
`)
	write("feature.md", `# Fixture

## Acceptance

### Scenario: gauntlet runs clean

- Given a real Go module with strong tests
- When the seven-layer gauntlet runs
- Then every layer passes and one receipt is written per layer
`)
	cfg := gates.Config{Gates: []gates.Gate{
		{Name: "typecheck", Layer: 1, Command: "go build ./..."},
		{Name: "lint", Layer: 2, Command: "go vet ./..."},
		{Name: "build", Layer: 3, Command: "go build -o bin/fixture.exe ./cmd/fixture"},
		{Name: "unit-tests", Layer: 4, Command: "go test ./... -coverprofile .regulus/coverage.out"},
		{Name: "coverage-floor", Layer: 5, Command: "go tool cover -func .regulus/coverage.out", Metric: &gates.Metric{Extract: gates.ExtractLastLinePercent, Threshold: 80, Operator: gates.OpAtLeast}},
		{Name: "mutation", Layer: 6, Command: "gremlins unleash ./pkg/calc --threshold-efficacy 70", TimeoutSec: 600},
		{Name: "crap", Layer: 7, Command: "./regulus.exe crap --profile .regulus/coverage.out --max 30 --top 5"},
	}}
	cfgData, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	write("regulus.json", string(cfgData))

	// The mutation gate exports HEAD: commit the fixture first.
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("add", "-A")
	git("-c", "user.email=journey@regulus", "-c", "user.name=journey", "commit", "-m", "fixture")

	t.Chdir(dir)

	var out, errb bytes.Buffer
	c := cli.CLI{
		Version: "test",
		Probe: func() doctor.Result {
			return doctor.Result{Detected: "opencode 2.1.4", Major: 2, Supported: true}
		},
		Stdout: &out,
		Stderr: &errb,
	}
	if code := c.Run([]string{"gate", "run", "fixture/T1"}); code != 0 {
		t.Fatalf("gate run exit = %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "gauntlet: 7/7 gates passed") {
		t.Fatalf("output = %q", out.String())
	}

	// One receipt per layer, with the coverage floor's metric recorded.
	paths, _ := filepath.Glob(filepath.Join(".regulus", "evidence", "fixture__T1", "*.json"))
	if len(paths) != 7 {
		t.Fatalf("want 7 receipts, got %v", paths)
	}
	byGate := map[string]receipts.Receipt{}
	for _, p := range paths {
		rec, err := receipts.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		byGate[rec.Gate] = rec
	}
	cov := byGate["coverage-floor"]
	if cov.MetricValue == nil || *cov.MetricValue < 80 {
		t.Fatalf("coverage-floor metric = %v, want >= 80", cov.MetricValue)
	}
	if byGate["mutation"].Verdict != receipts.VerdictPass {
		t.Fatalf("mutation receipt = %+v", byGate["mutation"])
	}
	if byGate["crap"].Verdict != receipts.VerdictPass {
		t.Fatalf("crap receipt = %+v", byGate["crap"])
	}
}
