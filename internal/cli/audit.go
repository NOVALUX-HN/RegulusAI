package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"regulus-ai/internal/crap"
)

// auditUntestedComplexityFloor bounds the should-fix tier: untested
// logic below it is trivial enough to ride as nice-to-have. Five
// means several decision points — the "branch, loop, parser" band the
// worker's Necessity Ladder calls non-trivial — not every uncovered
// getter.
const auditUntestedComplexityFloor = 5

// entryPoints maps the repo first (ponytail-audit order: map, then
// rank): every cmd/<name> directory holding Go sources is an entry
// point. Sorted, never empty-shaped: no cmd tree yields nil so the
// caller prints the honest "(none found)".
func entryPoints(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		srcs, err := filepath.Glob(filepath.Join(root, "cmd", e.Name(), "*.go"))
		if err != nil || len(srcs) == 0 {
			continue
		}
		out = append(out, "cmd/"+e.Name())
	}
	sort.Strings(out)
	return out
}

// auditTier ranks one scored function: over the gate max is must-fix
// (consistent with the gauntlet — the same code fails layer 7),
// untested non-trivial logic is should-fix, everything else in the
// top rides as nice-to-have.
func auditTier(f crap.Scored, max float64) string {
	if f.CRAP > max {
		return "must-fix"
	}
	if f.Coverage == 0 && f.Complexity >= auditUntestedComplexityFloor {
		return "should-fix"
	}
	return "nice"
}

// auditCmd implements `regulus audit --profile <coverprofile>
// [--top N] [--max M]`: the repo-wide ranked audit. Map first (entry
// points), then rank (CRAP tiers), then name what to fix first.
// Informational by design — exit 0 with a verdict, never a gate: the
// gauntlet owns failing, the audit owns prioritizing.
func (c CLI) auditCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, "usage: regulus audit --profile <coverprofile> [--max N] [--top N]")
		return 0
	}
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	profile := fs.String("profile", "", "go cover profile path (required)")
	max := fs.Float64("max", 30, "CRAP above this is must-fix (the layer-7 gate max)")
	top := fs.Int("top", 10, "how many worst functions to rank")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *profile == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus audit --profile <coverprofile> [--top N] [--max M]")
		return 2
	}
	rep, err := crap.Analyze(c.dir(), *profile)
	if err != nil {
		fmt.Fprintf(c.Stderr, "audit: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stdout, "Entry points:")
	for _, ep := range entryPoints(c.dir()) {
		fmt.Fprintf(c.Stdout, "  - %s\n", ep)
	}
	if len(entryPoints(c.dir())) == 0 {
		fmt.Fprintln(c.Stdout, "  (none found)")
	}
	fmt.Fprintln(c.Stdout, "Ranked findings:")
	var must, should, nice int
	first := ""
	for i, f := range rep.Functions {
		if i >= *top {
			break
		}
		tier := auditTier(f, *max)
		switch tier {
		case "must-fix":
			must++
		case "should-fix":
			should++
		default:
			nice++
		}
		if first == "" {
			first = f.Name
		}
		fmt.Fprintf(c.Stdout, "  - [%s] %s:%d %s (complexity %d, coverage %.1f%%) CRAP %.2f\n",
			tier, f.File, f.StartLine, f.Name, f.Complexity, f.Coverage*100, f.CRAP)
	}
	if first == "" {
		fmt.Fprintln(c.Stdout, "audit: 0 must-fix | 0 should-fix | 0 nice — clean")
		return 0
	}
	fmt.Fprintf(c.Stdout, "audit: %d must-fix | %d should-fix | %d nice — fix %s first\n",
		must, should, nice, first)
	return 0
}
