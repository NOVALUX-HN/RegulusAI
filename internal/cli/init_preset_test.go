package cli

import (
	"strings"
	"testing"
)

// The stack presets are generic starter gates: they must reference the
// target project's conventional layout, never RegulusAI's own internal
// paths (a node project has no .opencode/ directory — init used to
// scaffold .opencode/tsconfig.json and ./.opencode/lib/, forcing a
// hand-rewrite on every fresh repo).
func TestInitPresetsHaveNoRepoInternalPaths(t *testing.T) {
	for stack, gates := range initPresets {
		for _, g := range gates {
			cmd, _ := g["command"].(string)
			if strings.Contains(cmd, ".opencode") {
				t.Fatalf("stack %q gate %q pins a RegulusAI-internal path: %q", stack, g["name"], cmd)
			}
		}
	}
}

// Every gate's first token must be allowlisted for its stack, or the
// gauntlet refuses the gate as policy on a fresh checkout.
func TestInitPresetsFirstTokenAllowlisted(t *testing.T) {
	for stack, gates := range initPresets {
		allow := map[string]bool{}
		for _, tok := range initAllowlists[stack] {
			allow[tok] = true
		}
		for _, g := range gates {
			cmd, _ := g["command"].(string)
			first := strings.Fields(cmd)[0]
			if !allow[first] {
				t.Fatalf("stack %q gate %q runs %q, not in the %q allowlist", stack, g["name"], first, stack)
			}
		}
	}
}
