package gates

import "sort"

// QuickLeanNames is the lean daily-loop preset for `gate run --quick`:
// the cheap layers only (typecheck/lint/build/unit-tests) plus the spec
// proof gate and the gofmt hygiene gate. Everything expensive
// (mutation, vulncheck, coverage-floor, crap, the ts gates) stays
// full-only for pre-close. Selected by gate NAME in layer order so the
// preset survives layer renumbering; names absent from a custom config
// are simply absent from the selection.
var QuickLeanNames = []string{"typecheck", "lint", "build", "unit-tests", "spec", "gofmt"}

// SelectQuickLean resolves the lean daily-loop set: exactly the gates
// whose names are in QuickLeanNames, in ascending layer order. It
// validates first, exactly like SelectGates, and it never changes the
// --profile mechanism: "quick" still means required-only, "full" still
// means everything. --quick is an explicit opt-in preset, not a profile,
// so required gates outside the lean set (coverage-floor, crap, ts-*)
// are intentionally not selected here.
func SelectQuickLean(cfg Config) ([]Gate, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	keep := make(map[string]bool, len(QuickLeanNames))
	for _, n := range QuickLeanNames {
		keep[n] = true
	}
	var out []Gate
	for _, g := range cfg.Gates {
		if keep[g.Name] {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Layer < out[j].Layer })
	return out, nil
}
