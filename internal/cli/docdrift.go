// Doc-drift gate (NOV-63, S4): README, docs and CI must not
// contradict the repository's own sources (version, gate count, tool
// counts, supported runtimes, command surface). It extends `regulus
// manifest check`, so the existing manifest-check gate enforces it
// with no gate-count churn: the check derives every figure from its
// source (package.json, regulus.json, tool registries, adapter
// directories, CLI dispatch) and fails naming file:line.
//
// Point-in-time convention (H10): a .md file containing the H10 marker
// is a frozen historical record and is skipped. To mark a new one,
// add a line naming H10 (e.g. `> **Nota (H10):** ...`) at the top and
// document the freeze date; the gate picks it up automatically.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"regulus-ai/internal/mcp"
)

// h10Marker exempts frozen historical documents from drift checks.
const h10Marker = "H10"

// checkDocDrift compares documented claims against derived sources.
// Every problem names file:line; empty means docs square with the repo.
func checkDocDrift(root string, surface map[string]bool) []string {
	var problems []string
	problems = append(problems, checkDocVersion(root)...)
	problems = append(problems, checkDocGateCounts(root)...)
	problems = append(problems, checkDocToolCounts(root)...)
	problems = append(problems, checkDocRuntimes(root)...)
	cmdProblems, _ := checkDocCommands(root, surface)
	problems = append(problems, cmdProblems...)
	sort.Strings(problems)
	return problems
}

// docDriftNotes lists runnable-but-undocumented commands for the
// human-readable report; notes never fail the gate.

func driftFile(root, rel string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func driftLines(body string) []string { return strings.Split(body, "\n") }

// driftDocs lists the live .md claim sources: README plus docs,
// skipping H10 point-in-time records.
func driftDocs(root string) []string {
	out := []string{"README.md"}
	entries, err := os.ReadDir(filepath.Join(root, "docs"))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		body, ok := driftFile(root, "docs/"+e.Name())
		if !ok || strings.Contains(body, h10Marker) {
			continue
		}
		out = append(out, "docs/"+e.Name())
	}
	return out
}

func normVersion(v string) string {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	parts := strings.Split(v, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return strings.Join(parts[:3], ".")
}

// checkDocVersion pins one version across package.json,
// versioninfo.json (Fixed + String), the exe manifest, the CHANGELOG
// head and the README status line.
func checkDocVersion(root string) []string {
	var problems []string
	found := map[string]string{}
	if body, ok := driftFile(root, "package.json"); ok {
		var pkg struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal([]byte(body), &pkg); err != nil {
			problems = append(problems, "package.json: malformed")
		} else {
			found["package.json"] = normVersion(pkg.Version)
		}
	} else {
		problems = append(problems, "package.json: missing")
	}
	if body, ok := driftFile(root, "versioninfo.json"); ok {
		var vi struct {
			FixedFileInfo struct {
				FileVersion struct {
					Major, Minor, Patch int
				}
			}
			StringFileInfo struct {
				FileVersion    string
				ProductVersion string
			}
		}
		if err := json.Unmarshal([]byte(body), &vi); err != nil {
			problems = append(problems, "versioninfo.json: malformed")
		} else {
			fv := vi.FixedFileInfo.FileVersion
			found["versioninfo.json Fixed"] = normVersion(fmt.Sprintf("%d.%d.%d", fv.Major, fv.Minor, fv.Patch))
			found["versioninfo.json String"] = normVersion(vi.StringFileInfo.FileVersion)
			if normVersion(vi.StringFileInfo.ProductVersion) != normVersion(vi.StringFileInfo.FileVersion) {
				problems = append(problems, "versioninfo.json: ProductVersion != FileVersion")
			}
		}
	} else {
		problems = append(problems, "versioninfo.json: missing")
	}
	// The exe manifest carries several version attributes (XML
	// declaration, manifestVersion); the product version is the
	// four-part assemblyIdentity version.
	manifestRe := regexp.MustCompile(`version="(\d+\.\d+\.\d+\.\d+)"`)
	if body, ok := driftFile(root, "cmd/regulus/regulus.manifest"); ok {
		if m := manifestRe.FindStringSubmatch(body); m != nil {
			found["regulus.manifest"] = normVersion(m[1])
		} else {
			problems = append(problems, "cmd/regulus/regulus.manifest: no version attribute")
		}
	} else {
		problems = append(problems, "cmd/regulus/regulus.manifest: missing")
	}
	changeRe := regexp.MustCompile(`^## \[([0-9.]+)\]`)
	if body, ok := driftFile(root, "CHANGELOG.md"); ok {
		hit := ""
		for _, ln := range driftLines(body) {
			if m := changeRe.FindStringSubmatch(strings.TrimSpace(ln)); m != nil {
				hit = m[1]
				break
			}
		}
		if hit == "" {
			problems = append(problems, "CHANGELOG.md: no version head")
		} else {
			found["CHANGELOG.md"] = normVersion(hit)
		}
	}
	statusRe := regexp.MustCompile(`Status:\s*v([0-9.]+)`)
	if body, ok := driftFile(root, "README.md"); ok {
		if m := statusRe.FindString(body); m != "" {
			found["README.md Status"] = normVersion(statusRe.FindStringSubmatch(m)[1])
		} else {
			problems = append(problems, "README.md: no Status version line")
		}
	}
	ref := found["package.json"]
	for _, name := range []string{"versioninfo.json Fixed", "versioninfo.json String", "regulus.manifest", "CHANGELOG.md", "README.md Status"} {
		if v, ok := found[name]; ok && ref != "" && v != ref {
			problems = append(problems, fmt.Sprintf("version drift: %s is %s, package.json is %s", name, v, ref))
		}
	}
	return problems
}

// gateCount derives the gauntlet size from regulus.json.
func gateCount(root string) (int, bool) {
	body, ok := driftFile(root, "regulus.json")
	if !ok {
		return 0, false
	}
	var cfg struct {
		Gates []json.RawMessage `json:"gates"`
	}
	if err := json.Unmarshal([]byte(body), &cfg); err != nil {
		return 0, false
	}
	return len(cfg.Gates), true
}

var gateClaimRe = regexp.MustCompile(`(\d+)-gate`)
var layerClaimRe = regexp.MustCompile(`(\d+)\s+layers?`)

// checkDocGateCounts fails stale gate/layer numbers in README and CI
// workflows. Historical docs are H10-excluded by driftDocs.
func checkDocGateCounts(root string) []string {
	var problems []string
	n, ok := gateCount(root)
	if !ok {
		return []string{"regulus.json: missing or malformed gates"}
	}
	files := []string{"README.md"}
	entries, err := os.ReadDir(filepath.Join(root, ".github", "workflows"))
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yml") {
				files = append(files, ".github/workflows/"+e.Name())
			}
		}
	}
	for _, rel := range files {
		body, ok := driftFile(root, rel)
		if !ok {
			continue
		}
		for i, ln := range driftLines(body) {
			for _, m := range gateClaimRe.FindAllStringSubmatch(ln, -1) {
				if m[1] != fmt.Sprint(n) {
					problems = append(problems, fmt.Sprintf("%s:%d: claims %s-gate, regulus.json has %d gates", rel, i+1, m[1], n))
				}
			}
			for _, m := range layerClaimRe.FindAllStringSubmatch(ln, -1) {
				if m[1] != fmt.Sprint(n) {
					problems = append(problems, fmt.Sprintf("%s:%d: claims %s layers, regulus.json has %d gates", rel, i+1, m[1], n))
				}
			}
		}
	}
	return problems
}

var toolClaimRe = regexp.MustCompile(`(\d+)\s+tools\b`)

// pluginToolCount derives the OpenCode plugin surface: every tool
// registers through the 6-space `add({` call; commands use
// editor.add at a different indent.
func pluginToolCount(root string) (int, bool) {
	body, ok := driftFile(root, ".opencode/plugins/regulus.ts")
	if !ok {
		return 0, false
	}
	n := 0
	for _, ln := range driftLines(body) {
		if strings.HasPrefix(ln, "      add({") {
			n++
		}
	}
	return n, true
}

// checkDocToolCounts pins "N tools" claims: an MCP-flavored line must
// name the descriptor count, a plugin-flavored line the registration
// count. Mixed lines satisfy either side for their own number.
func checkDocToolCounts(root string) []string {
	var problems []string
	mcpN := len(mcp.ToolNames())
	plugN, ok := pluginToolCount(root)
	if !ok {
		problems = append(problems, ".opencode/plugins/regulus.ts: missing, plugin tool count unknown")
	}
	for _, rel := range driftDocs(root) {
		body, ok := driftFile(root, rel)
		if !ok {
			continue
		}
		for i, ln := range driftLines(body) {
			low := strings.ToLower(ln)
			for _, m := range toolClaimRe.FindAllStringSubmatch(ln, -1) {
				num := m[1]
				mcpSide := strings.Contains(low, "mcp")
				plugSide := strings.Contains(low, "plugin") || strings.Contains(low, "opencode") || strings.Contains(low, "agent surface")
				want := ""
				if num == fmt.Sprint(mcpN) && mcpSide {
					continue
				}
				if ok && num == fmt.Sprint(plugN) && plugSide {
					continue
				}
				if mcpSide && !plugSide {
					want = fmt.Sprintf("%d (MCP descriptors)", mcpN)
				} else if plugSide && !mcpSide {
					want = fmt.Sprintf("%d (plugin registrations)", plugN)
				} else if num != fmt.Sprint(mcpN) && (!ok || num != fmt.Sprint(plugN)) {
					want = fmt.Sprintf("%d MCP / %d plugin", mcpN, plugN)
				} else {
					continue
				}
				problems = append(problems, fmt.Sprintf("%s:%d: claims %s tools, want %s", rel, i+1, num, want))
			}
		}
	}
	return problems
}

// adapterDirs are the multi-host surfaces F6 committed.
var adapterDirs = []string{".agents/skills", ".claude/skills", ".codex", ".devin"}

// checkDocRuntimes fails "only supported runtime" claims once
// multi-host adapters exist on disk.
func checkDocRuntimes(root string) []string {
	var problems []string
	adapters := false
	for _, d := range adapterDirs {
		if fi, err := os.Stat(filepath.Join(root, d)); err == nil && fi.IsDir() {
			adapters = true
			break
		}
	}
	if !adapters {
		return nil
	}
	for _, rel := range driftDocs(root) {
		body, ok := driftFile(root, rel)
		if !ok {
			continue
		}
		for i, ln := range driftLines(body) {
			if strings.Contains(strings.ToLower(ln), "only supported runtime") {
				problems = append(problems, fmt.Sprintf("%s:%d: claims only supported runtime while multi-host adapters exist (see docs/conformity-matrix.md)", rel, i+1))
			}
		}
	}
	return problems
}

var readmeCmdRe = regexp.MustCompile("`regulus ([a-z][a-z-]*)")

// usageCmdRe reads command names off the user-facing usage text:
// lines opening with two spaces whose first token is a lowercase
// word (flags and prose never match). The drift check reads the
// documented surface instead of the routing table, so it cannot feed
// back into cliDispatch initialization.
// surfaceFromUsage extracts runnable top-level commands from usage
// text: dispatch names, serve and contract as the help prints them.
func surfaceFromUsage(text string) map[string]bool {
	out := map[string]bool{}
	cmdRe := regexp.MustCompile(`^  ([a-z][a-z-]*)\b`)
	for _, ln := range strings.Split(text, "\n") {
		if m := cmdRe.FindStringSubmatch(ln); m != nil && m[1] != "regulus" {
		// Example invocations ("  regulus doctor ...") are not
		// commands; the command list uses bare names.
		out[m[1]] = true
		}
	}
	return out
}

// checkDocCommands fails README rows that name runnable commands
// which do not exist. Undocumented commands print as notes on stdout
// by the caller; they do not fail (docs need not exhaust internals).
func checkDocCommands(root string, surface map[string]bool) (problems []string, notes []string) {
	body, ok := driftFile(root, "README.md")
	if !ok {
		return []string{"README.md: missing"}, nil
	}
	seen := map[string]bool{}
	for _, m := range readmeCmdRe.FindAllStringSubmatch(body, -1) {
		seen[m[1]] = true
	}
	for name := range seen {
		if !surface[name] {
			problems = append(problems, fmt.Sprintf("README.md: documents `regulus %s` which does not exist", name))
		}
	}
	for name := range surface {
		if name == "help" || name == "contract" {
			continue
		}
		if !seen[name] {
			notes = append(notes, fmt.Sprintf("note: `regulus %s` runnable but absent from the README table", name))
		}
	}
	sort.Strings(notes)
	return problems, notes
}
