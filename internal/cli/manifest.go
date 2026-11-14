package cli

// `regulus manifest check` (NOV-25): the canonical tool manifest is the
// single source of truth for MCP/plugin dispatch. This command loads
// internal/manifest/tools.json from the project root (c.dir()) and
// compares it against the two code registries that must never drift:
// mcpTools() in this package (the server dispatch table) and
// mcp.ToolNames() (the capability descriptors in internal/mcp).
// Required-param lists are compared too, and the 5 intentional
// plugin-vs-MCP gaps must match the known Devin-policy set with a
// reason each. Any divergence fails with exit 1 naming every problem;
// a clean tree prints one OK line and exits 0.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/mcp"
)

// knownIntentionalGaps is the pinned Devin-policy delta: the OpenCode
// plugin exposes 19 tools, MCP deliberately exposes 14. These 5 never
// transfer (harness self-modification, evolution probes, local skill
// registry); audit and evolve_report already ported because they are
// read-only like trace_show.
var knownIntentionalGaps = []string{
	"evolve_propose",
	"probe_contrast",
	"probe_compose",
	"skills_list",
	"evolve_tick",
}

// manifestToolsFile mirrors internal/manifest/tools.json (the canonical
// source, not the run-manifest in internal/manifest/manifest.go).
type manifestToolsFile struct {
	Version         int                 `json:"version"`
	MCP             []manifestToolEntry `json:"mcp"`
	IntentionalGaps []manifestGapEntry  `json:"intentional_gaps"`
}

// manifestToolEntry is one canonical MCP tool: name plus required params.
type manifestToolEntry struct {
	Name     string   `json:"name"`
	Required []string `json:"required"`
}

// manifestGapEntry is one intentional plugin-vs-MCP gap with its reason.
type manifestGapEntry struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// manifestCmd implements `regulus manifest check`.
func (c CLI) manifestCmd(args []string) int {
	if len(args) != 1 || args[0] != "check" {
		fmt.Fprintln(c.Stderr, "usage: regulus manifest check")
		return 2
	}
	return c.manifestCheck()
}

// manifestCheck loads tools.json from the project root and reports every
// divergence. Exit 0 prints one OK line; exit 1 lists each problem.
func (c CLI) manifestCheck() int {
	path := filepath.Join(c.dir(), "internal", "manifest", "tools.json")
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(c.Stderr, "manifest check: cannot read %s: %v\n", path, err)
		return 1
	}
	problems := checkToolManifest(data, mcpTools(), mcp.ToolNames())
	if len(problems) != 0 {
		for _, p := range problems {
			fmt.Fprintf(c.Stderr, "manifest check: %s\n", p)
		}
		return 1
	}
	var file manifestToolsFile
	_ = json.Unmarshal(data, &file)
	fmt.Fprintf(c.Stdout, "manifest OK: %d tools + %d intentional gaps\n", len(file.MCP), len(file.IntentionalGaps))
	return 0
}

// checkToolManifest compares one tools.json document against the server
// dispatch table (cliTools, same package) and the capability descriptors
// (mcpNames, internal/mcp). It returns every problem found, sorted for
// deterministic output; empty means the three registries agree and the
// intentional gaps match the known set with reasons.
func checkToolManifest(data []byte, cliTools []mcpTool, mcpNames []string) []string {
	var file manifestToolsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return []string{fmt.Sprintf("malformed tools.json: %v", err)}
	}
	var problems []string

	// NOV-37: tools.json version 1 maps to the supported contract, MCP
	// contract and wire protocol. Any other version — or an unsupported
	// pinned version — fails with the supported set named.
	if file.Version != 1 {
		problems = append(problems, fmt.Sprintf("unsupported manifest version %d: version 1 maps to contract %q, mcp contract %q, protocol %q", file.Version, contract.Version, mcp.ContractVersion, mcp.ProtocolVersion))
	} else {
		if !contract.Supports(contract.Version) {
			problems = append(problems, fmt.Sprintf("unsupported contract %q (supported %v)", contract.Version, contract.Supported))
		}
		if !mcp.SupportsContract(mcp.ContractVersion) {
			problems = append(problems, fmt.Sprintf("unsupported mcp contract %q (supported %v)", mcp.ContractVersion, mcp.SupportedContracts))
		}
		if !mcp.SupportsProtocol(mcp.ProtocolVersion) {
			problems = append(problems, fmt.Sprintf("unsupported protocol %q (supported %v)", mcp.ProtocolVersion, mcp.SupportedProtocols))
		}
	}

	jsonRequired := map[string][]string{}
	jsonSeen := map[string]int{}
	for _, e := range file.MCP {
		jsonSeen[e.Name]++
		if _, ok := jsonRequired[e.Name]; !ok {
			req := append([]string(nil), e.Required...)
			jsonRequired[e.Name] = req
		}
	}
	for name, n := range jsonSeen {
		if n > 1 {
			problems = append(problems, fmt.Sprintf("duplicate tool in manifest: %q x%d", name, n))
		}
		if strings.TrimSpace(name) == "" {
			problems = append(problems, "empty tool name in manifest")
		}
	}

	cliRequired := map[string][]string{}
	cliSeen := map[string]int{}
	for _, t := range cliTools {
		cliSeen[t.name]++
		if _, ok := cliRequired[t.name]; !ok {
			cliRequired[t.name] = append([]string(nil), t.required...)
		}
	}
	for name, n := range cliSeen {
		if n > 1 {
			problems = append(problems, fmt.Sprintf("duplicate tool in dispatch: %q x%d", name, n))
		}
	}

	mcpSeen := map[string]int{}
	for _, n := range mcpNames {
		mcpSeen[n]++
	}
	for name, n := range mcpSeen {
		if n > 1 {
			problems = append(problems, fmt.Sprintf("duplicate tool in descriptors: %q x%d", name, n))
		}
	}

	// JSON vs dispatch (same package).
	for name := range cliRequired {
		if _, ok := jsonRequired[name]; !ok {
			problems = append(problems, fmt.Sprintf("missing in manifest: %q (dispatches but not in tools.json)", name))
		}
	}
	for name := range jsonRequired {
		if _, ok := cliRequired[name]; !ok {
			problems = append(problems, fmt.Sprintf("extra in manifest: %q (in tools.json but no dispatch)", name))
		}
	}
	// JSON vs descriptors (internal/mcp).
	mcpSet := map[string]bool{}
	for _, n := range mcpNames {
		mcpSet[n] = true
	}
	for name := range jsonRequired {
		if !mcpSet[name] {
			problems = append(problems, fmt.Sprintf("missing in descriptors: %q (in tools.json but not in mcp.ToolNames())", name))
		}
	}
	for _, name := range mcpNames {
		if _, ok := jsonRequired[name]; !ok {
			problems = append(problems, fmt.Sprintf("missing in manifest: %q (in mcp.ToolNames() but not in tools.json)", name))
		}
	}

	// Required lists: manifest vs dispatch, order-insensitive.
	for name, want := range cliRequired {
		got, ok := jsonRequired[name]
		if !ok {
			continue
		}
		if !sameStringSet(got, want) {
			problems = append(problems, fmt.Sprintf("required mismatch for %q: manifest %s vs dispatch %s", name, formatStringList(got), formatStringList(want)))
		}
	}

	// Intentional gaps: exact known set, each with a reason.
	gapReason := map[string]string{}
	gapSeen := map[string]int{}
	for _, g := range file.IntentionalGaps {
		gapSeen[g.Name]++
		if _, ok := gapReason[g.Name]; !ok {
			gapReason[g.Name] = g.Reason
		}
	}
	for name, n := range gapSeen {
		if n > 1 {
			problems = append(problems, fmt.Sprintf("duplicate intentional gap: %q x%d", name, n))
		}
	}
	knownSet := map[string]bool{}
	for _, n := range knownIntentionalGaps {
		knownSet[n] = true
	}
	for _, n := range knownIntentionalGaps {
		if _, ok := gapReason[n]; !ok {
			problems = append(problems, fmt.Sprintf("missing intentional gap: %q", n))
		}
	}
	for name := range gapReason {
		if !knownSet[name] {
			problems = append(problems, fmt.Sprintf("unexpected intentional gap: %q", name))
		}
	}
	for name, reason := range gapReason {
		if strings.TrimSpace(reason) == "" {
			problems = append(problems, fmt.Sprintf("intentional gap %q needs a reason", name))
		}
	}

	sort.Strings(problems)
	return problems
}

// sameStringSet reports whether two string lists hold the same set
// (order-insensitive, duplicates collapsed — duplicates elsewhere are
// reported separately).
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		// Lengths may still match as sets with dupes; fall through to
		// the set comparison but keep the fast path for the common case.
	}
	set := map[string]int{}
	for _, s := range a {
		set[s]++
	}
	for _, s := range b {
		set[s]--
	}
	for _, n := range set {
		if n != 0 {
			return false
		}
	}
	return true
}

// formatStringList renders a required list deterministically for errors.
func formatStringList(ss []string) string {
	cp := append([]string(nil), ss...)
	sort.Strings(cp)
	return "[" + strings.Join(cp, ", ") + "]"
}
