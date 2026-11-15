package bench

// gauntlet-shape conformance and lean selection (NOV-67 split).

import (
	"encoding/json"
	"fmt"

	"regulus-ai/internal/gates"
)

// gauntletShapeGate is one row of the checked-in gauntlet shape: the
// selection-relevant fields of regulus.json. Commands never run in
// bench, so they are not part of the shape.
type gauntletShapeGate struct {
	Name     string   `json:"name"`
	Layer    int      `json:"layer"`
	Required bool     `json:"required"`
	Profiles []string `json:"profiles"`
}

// gauntletShape is the checked-in bench/gauntlet-shape.json: the real
// fifteen-gate gauntlet plus the six T5 --quick names, generated from
// regulus.json (the file's generation key holds the exact command).
type gauntletShape struct {
	Gates     []gauntletShapeGate `json:"gates"`
	QuickLean []string            `json:"quick_lean"`
}

// quickLeanExpected pins the T5 --quick preset by name: exactly the six
// gates gates.QuickLeanNames selects. B5 asserts the checked-in shape
// and the live selection both equal this list, so a renamed gate or a
// mutation leaking into --quick fails the suite instead of staying
// green.
var quickLeanExpected = []string{"typecheck", "lint", "build", "unit-tests", "spec", "gofmt"}

// parseGauntletShape parses bench/gauntlet-shape.json bytes. Only the
// suite tests read the file (from the repo); checks stay file-free and
// hermetic.
func parseGauntletShape(data []byte) (gauntletShape, error) {
	var s gauntletShape
	if err := json.Unmarshal(data, &s); err != nil {
		return gauntletShape{}, fmt.Errorf("gauntlet shape: %v", err)
	}
	if len(s.Gates) == 0 || len(s.QuickLean) == 0 {
		return gauntletShape{}, fmt.Errorf("gauntlet shape: empty gates or quick_lean")
	}
	return s, nil
}

// shapeFixtureConfig rebuilds a runnable gates.Config from the shape:
// selection fields carried over, commands synthetic (never executed).
func shapeFixtureConfig(shape gauntletShape) gates.Config {
	cfg := gates.Config{}
	for _, sg := range shape.Gates {
		g := gates.Gate{Name: sg.Name, Layer: sg.Layer, Command: "shape-only:" + sg.Name}
		if !sg.Required {
			g.Required = boolPtr(false)
		}
		g.Profiles = append([]string(nil), sg.Profiles...)
		cfg.Gates = append(cfg.Gates, g)
	}
	return cfg
}

// equalStrings reports element-wise equality (nil and empty both mean
// "no profiles" for the shape's purposes).
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// configGateNames lists gate names in selection order.
func configGateNames(gateList []gates.Gate) []string {
	names := make([]string, 0, len(gateList))
	for _, g := range gateList {
		names = append(names, g.Name)
	}
	return names
}

// assertGauntletShapeConformance fails when the hand-maintained
// realGateConfig mirror drifts from the checked-in shape: renamed
// gate, flipped required flag, changed profile membership, layer
// reorder, or gate count. The shape regenerates from regulus.json, so
// a regenerated shape with a stale mirror fails loudly here.
func assertGauntletShapeConformance(cfg gates.Config, shape gauntletShape) error {
	if len(cfg.Gates) != len(shape.Gates) {
		return fmt.Errorf("gauntlet shape drift: mirror has %d gates, shape has %d (regenerate the shape AND update the mirror)", len(cfg.Gates), len(shape.Gates))
	}
	for i, g := range cfg.Gates {
		want := shape.Gates[i]
		if g.Name != want.Name {
			return fmt.Errorf("gauntlet shape drift at position %d: mirror %q, shape %q (renamed gate?)", i, g.Name, want.Name)
		}
		if g.Layer != want.Layer {
			return fmt.Errorf("gauntlet shape drift for %q: mirror layer %d, shape layer %d", g.Name, g.Layer, want.Layer)
		}
		if g.IsRequired() != want.Required {
			return fmt.Errorf("gauntlet shape drift for %q: mirror required=%v, shape required=%v", g.Name, g.IsRequired(), want.Required)
		}
		if !equalStrings(g.Profiles, want.Profiles) {
			return fmt.Errorf("gauntlet shape drift for %q: mirror profiles=%v, shape profiles=%v", g.Name, g.Profiles, want.Profiles)
		}
	}
	return nil
}

// assertLeanSelection fails unless the live --quick selection over cfg
// is exactly the pinned six: a renamed gate drops out of the name-based
// selection, and an extra selected gate (mutation leaking into --quick)
// breaks the equality.
func assertLeanSelection(cfg gates.Config) error {
	lean, err := gates.SelectQuickLean(cfg)
	if err != nil {
		return err
	}
	if got := configGateNames(lean); !equalStrings(got, quickLeanExpected) {
		return fmt.Errorf("lean preset drift: selected %v, want %v", got, quickLeanExpected)
	}
	return nil
}

// realGateConfig mirrors the shipped fifteen-gate gauntlet in declaration
// order. The suite tests validate the mirror against the checked-in
// bench/gauntlet-shape.json (generated from regulus.json), so a renamed
// gate or a flipped required flag fails `go test` instead of staying
// green. Checks stay file-free and hermetic (no repo-root lookup, green
// in any temp dir) while pinning the real profile contract — full and
// quick select every required gate in order, unknown profiles refuse.
// Commands are shape-only; selection is what the check pins.
func realGateConfig() gates.Config {
	names := []string{"manifest-check", "typecheck", "lint", "build", "unit-tests", "coverage-floor", "mutation-receipts", "mutation-gates", "crap", "vulncheck", "spec", "ts-typecheck", "ts-unit-tests", "gofmt", "race-almanac"}
	cmds := []string{
		"regulus manifest check", "go build ./...", "go vet ./...", "go build -o bin/regulus ./cmd/regulus",
		"go test ./...", "go tool cover -func .regulus/coverage.out",
		"gremlins unleash ./internal/receipts", "gremlins unleash ./internal/gates", "regulus crap --max 30",
		"govulncheck ./...", "regulus spec check charts/",
		"bunx tsc --noEmit", "bun test ./.opencode/lib/", "gofmt -l .",
		"go test -race -count=1 ./internal/almanac/",
	}
	cfg := gates.Config{}
	for i, name := range names {
		g := gates.Gate{
			Name: name, Layer: i + 1, Command: cmds[i],
		}
		if name == "mutation-receipts" || name == "mutation-gates" || name == "vulncheck" || name == "race-almanac" {
			g.Required = boolPtr(false)
			g.Profiles = []string{"full"}
		}
		if name == "vulncheck" {
			g.Profiles = []string{"full", "security"}
		}
		cfg.Gates = append(cfg.Gates, g)
	}
	return cfg
}

// checkRealGateProfile pins the real gauntlet shape through profile
// selection: full keeps all fifteen gates in declaration order with halt
// policy, quick keeps the 11 required gates (mutation-receipts +
// mutation-gates + vulncheck + race-almanac are full-only), and an unknown profile refuses.
func checkRealGateProfile() (string, error) {
	cfg := realGateConfig()
	full, err := gates.SelectGates(cfg, "")
	if err != nil {
		return "", err
	}
	if len(full) != 15 {
		return "", fmt.Errorf("full profile: got %d gates, want 15", len(full))
	}
	for i, g := range full {
		if g.Name != cfg.Gates[i].Name {
			return "", fmt.Errorf("full profile order broke at %d: got %q, want %q", i, g.Name, cfg.Gates[i].Name)
		}
		if g.Command == "" || g.EffectiveOnFail() != gates.OnFailHalt {
			return "", fmt.Errorf("gate %q lost its command or halt policy", g.Name)
		}
	}
	quick, err := gates.SelectGates(cfg, "quick")
	if err != nil {
		return "", err
	}
	if len(quick) != 11 {
		return "", fmt.Errorf("quick profile: got %d gates, want 11 required", len(quick))
	}
	if _, err := gates.SelectGates(cfg, "nope"); err == nil {
		return "", fmt.Errorf("unknown profile accepted")
	}
	return "full=15, quick=11 required in order, unknown refused", nil
}
