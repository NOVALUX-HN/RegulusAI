package gates

import (
	"testing"
)

// T5 RED: --quick selects exactly the lean daily-loop set
// (typecheck/lint/build/unit-tests + spec + gofmt) while full stays
// complete and --profile quick keeps its required-only contract.
func TestSelectQuickLeanExact(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "typecheck", Layer: 1, Command: "go build ./..."},
		{Name: "lint", Layer: 2, Command: "go vet ./..."},
		{Name: "build", Layer: 3, Command: "go build -o bin/regulus ./cmd/regulus"},
		{Name: "unit-tests", Layer: 4, Command: "go test ./... -coverprofile .regulus/coverage.out"},
		{Name: "coverage-floor", Layer: 5, Command: "go tool cover -func .regulus/coverage.out", Required: boolPtr(false), Profiles: []string{"full"}},
		{Name: "mutation", Layer: 6, Command: "gremlins unleash ./internal/receipts", Required: boolPtr(false), Profiles: []string{"full"}},
		{Name: "crap", Layer: 7, Command: "go run ./cmd/regulus crap --profile .regulus/coverage.out --max 30 --top 5"},
		{Name: "vulncheck", Layer: 8, Command: "govulncheck ./...", Required: boolPtr(false), Profiles: []string{"full", "security"}},
		{Name: "spec", Layer: 9, Command: "go run ./cmd/regulus spec check charts/"},
		{Name: "ts-typecheck", Layer: 10, Command: "bunx tsc --noEmit -p .opencode/tsconfig.json"},
		{Name: "ts-unit-tests", Layer: 11, Command: "bun test ./.opencode/lib/"},
		{Name: "gofmt", Layer: 12, Command: "go test ./internal/gates/ -run TestGofmtClean -count=1"},
	}}
	lean, err := SelectQuickLean(cfg)
	if err != nil {
		t.Fatalf("SelectQuickLean: %v", err)
	}
	want := []string{"typecheck", "lint", "build", "unit-tests", "spec", "gofmt"}
	if len(lean) != len(want) {
		t.Fatalf("lean set = %d gates %v, want %d %v", len(lean), gateNames(lean), len(want), want)
	}
	for i, n := range want {
		if lean[i].Name != n {
			t.Fatalf("lean[%d] = %q, want %q (full order %v)", i, lean[i].Name, n, gateNames(lean))
		}
	}
	full, err := SelectGates(cfg, "full")
	if err != nil || len(full) != 12 {
		t.Fatalf("full must stay complete: got %v/%v", gateNames(full), err)
	}
	quick, err := SelectGates(cfg, "quick")
	if err != nil {
		t.Fatalf("profile quick: %v", err)
	}
	for _, g := range quick {
		if !g.IsRequired() {
			t.Fatalf("profile quick must keep required-only, kept optional %q", g.Name)
		}
	}
}

func gateNames(gates []Gate) []string {
	out := make([]string, 0, len(gates))
	for _, g := range gates {
		out = append(out, g.Name)
	}
	return out
}
