package gates

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLintGatePinsExplicitAnalyzers guards the gap-sweep/G6 hardening: the
// lint gate must enumerate its stdlib vet analyzers explicitly instead of
// relying on the bare `go vet ./...` defaults. The explicit list (the full
// `go tool vet help` suite for the pinned toolchain: 35 analyzers, no
// -vettool, no shadow/fieldalignment which are not stdlib) keeps the gate
// reproducible across Go upgrades: a toolchain that changes the defaults
// cannot silently weaken or widen the gate, and any newly added analyzer
// stays off until it is consciously listed here and in regulus.json.
func TestLintGatePinsExplicitAnalyzers(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate this test file")
	}
	cfg, err := Load(filepath.Join(filepath.Dir(file), "..", "..", "regulus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cmd string
	for _, g := range cfg.Gates {
		if g.Name == "lint" {
			cmd = g.Command
		}
	}
	if cmd == "" {
		t.Fatal("regulus.json must declare a lint gate")
	}
	if !strings.HasPrefix(cmd, "go vet ") || !strings.HasSuffix(cmd, "./...") {
		t.Fatalf("lint gate must stay a go vet over ./..., got %q", cmd)
	}
	if strings.Contains(cmd, "-vettool") {
		t.Fatalf("lint gate must not use an external -vettool (zero-dep invariant), got %q", cmd)
	}
	// The full stdlib analyzer suite per `go tool vet help` (go 1.27).
	want := []string{
		"-appends", "-asmdecl", "-assign", "-atomic", "-bools", "-buildtag",
		"-cgocall", "-composites", "-copylocks", "-defers", "-directive",
		"-errorsas", "-framepointer", "-hostport", "-httpresponse",
		"-ifaceassert", "-loopclosure", "-lostcancel", "-nilfunc", "-printf",
		"-shift", "-sigchanyzer", "-slog", "-stdmethods", "-stdversion",
		"-stringintconv", "-structtag", "-testinggoroutine", "-tests",
		"-timeformat", "-unmarshal", "-unreachable", "-unsafeptr",
		"-unusedresult", "-waitgroup",
	}
	var missing []string
	for _, flag := range want {
		if !strings.Contains(cmd, flag+" ") && !strings.Contains(cmd, flag+"=") && !strings.HasSuffix(cmd, flag) {
			missing = append(missing, flag)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("lint gate must list every stdlib analyzer explicitly, missing %v in %q", missing, cmd)
	}
}
