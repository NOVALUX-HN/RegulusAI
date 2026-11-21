// Inventory (F12.8/NOV-103): modules, packages, APIs, config,
// workflows and dependencies.
//
// The code/symbol/dependency folds reuse context.Inventory (F12.3:
// the single corpus walk with its bounds, skip dirs and git head) —
// impact never walks a second corpus for those. Packages derive
// from code fragment sources, APIs from exported symbol fragments,
// dependencies from dependency fragments. What the corpus does not
// carry (test files are excluded from the corpus by design,
// workflows/configs/commands are not code) is walked here with the
// same skip-dir discipline and warning-instead-of-silence rule.
package impact

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"regulus-ai/internal/context"
)

// skipDirs mirrors the corpus discipline: history, caches and
// vendored code are never inventoried.
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true,
	".regulus": true, "bin": true, "dist": true, "target": true,
}

// configCandidates are the repo-root configs that count as config
// nodes when present (repo-relative paths).
var configCandidates = []string{
	"regulus.json", "opencode.json", "opencode.jsonc",
	".codex/config.toml",
}

// Census walks root and folds the closed census. The context
// corpus supplies code packages, exported API symbols and go.mod
// dependencies; this walk adds test bundles, workflows, configs and
// commands. It fails only on an unreadable root; everything else is
// a warning.
func Census(root string) (Inventory, error) {
	cfg := context.DefaultInventoryConfig()
	cfg.IncludeMemory = false
	corpus, warnings, err := context.Inventory(root, cfg)
	if err != nil {
		return Inventory{}, err
	}
	inv := Inventory{Warnings: warnings}
	inv.Module = moduleName(root)
	pkgs := map[string]bool{}
	for _, f := range corpus.Fragments {
		switch f.Kind {
		case context.KindCode:
			dir := dirOf(f.Source)
			pkgs[dir] = true
		case context.KindSymbol:
			if name := symbolName(f.Title); isExported(name) {
				inv.Nodes = append(inv.Nodes, Node{
					ID:   "api:" + f.Source + "#" + name,
					Kind: KindAPI, Path: f.Source,
					Detail: "exported symbol " + name,
				})
			}
		case context.KindDependency:
			mod := strings.TrimPrefix(f.ID, "dependency/")
			inv.Nodes = append(inv.Nodes, Node{
				ID:   "dep:" + mod,
				Kind: KindDependency, Path: "go.mod",
				Detail: strings.TrimPrefix(f.Content, " "),
			})
		}
	}
	var pkgList []string
	for p := range pkgs {
		pkgList = append(pkgList, p)
	}
	sort.Strings(pkgList)
	for _, p := range pkgList {
		inv.Nodes = append(inv.Nodes, Node{
			ID: "pkg:" + p, Kind: KindPackage, Path: p,
			Critical: isCriticalPkg(p),
		})
	}
	if inv.Module != "" {
		inv.Nodes = append(inv.Nodes, Node{
			ID: "module:" + inv.Module, Kind: KindModule, Path: "go.mod",
			Detail: "go module root",
		})
	}
	tests, w := inventoryTests(root)
	inv.Warnings = append(inv.Warnings, w...)
	inv.Nodes = append(inv.Nodes, tests...)
	wf, w := inventoryWorkflows(root)
	inv.Warnings = append(inv.Warnings, w...)
	inv.Nodes = append(inv.Nodes, wf...)
	for _, rel := range configCandidates {
		if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil && !st.IsDir() {
			inv.Nodes = append(inv.Nodes, Node{
				ID: "cfg:" + rel, Kind: KindConfig, Path: rel,
			})
		}
	}
	cmds, w := inventoryCommands(root)
	inv.Warnings = append(inv.Warnings, w...)
	inv.Nodes = append(inv.Nodes, cmds...)
	sort.Slice(inv.Nodes, func(i, j int) bool { return inv.Nodes[i].ID < inv.Nodes[j].ID })
	attachTests(&inv)
	return inv, nil
}

// moduleName reads the go.mod module line; empty when absent.
func moduleName(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(mod)
		}
	}
	return ""
}

// dirOf returns the package dir of a repo-relative source file.
func dirOf(rel string) string {
	if i := strings.LastIndex(rel, "/"); i > 0 {
		return rel[:i]
	}
	return "."
}

// symbolName takes the declared name from a "symbol Name in file"
// title.
func symbolName(title string) string {
	t := strings.TrimPrefix(title, "symbol ")
	if i := strings.Index(t, " "); i >= 0 {
		return t[:i]
	}
	return t
}

// isExported reports the Go export rule: first rune uppercase.
func isExported(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}

// inventoryTests walks *_test.go files into one test node per dir
// (the corpus excludes tests by design, so this walk owns them).
func inventoryTests(root string) ([]Node, []string) {
	byDir := map[string][]string{}
	var warnings []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		byDir[dirOf(rel)] = append(byDir[dirOf(rel)], rel)
		return nil
	})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("tests: recorrido parcial (%v)", err))
	}
	var out []Node
	for dir, files := range byDir {
		sort.Strings(files)
		out = append(out, Node{
			ID: "test:" + dir, Kind: KindTest, Path: dir,
			Detail: fmt.Sprintf("%d test file(s)", len(files)),
			Tests:  files,
		})
	}
	return out, warnings
}

// inventoryWorkflows folds .github/workflows/*.yml into workflow
// nodes (spec: workflows are first-class inventory).
func inventoryWorkflows(root string) ([]Node, []string) {
	dir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var out []Node
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yml") && !strings.HasSuffix(e.Name(), ".yaml")) {
			continue
		}
		rel := ".github/workflows/" + e.Name()
		out = append(out, Node{ID: "wf:" + rel, Kind: KindWorkflow, Path: rel})
	}
	return out, nil
}

// inventoryCommands scans internal/cli/*.go for cliDispatch["name"]
// entries into command nodes (no cli import: the graph must not
// depend on the package it describes; grep reproduces every node).
func inventoryCommands(root string) ([]Node, []string) {
	dir := filepath.Join(root, "internal", "cli")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	seen := map[string]string{}
	var warnings []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		rel := "internal/cli/" + e.Name()
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("ilegible %s: omitido (%v)", rel, err))
			continue
		}
		for _, name := range dispatchNames(string(raw)) {
			if _, ok := seen[name]; !ok {
				seen[name] = rel
			}
		}
	}
	var out []Node
	for name, rel := range seen {
		out = append(out, Node{
			ID: "cmd:" + name, Kind: KindCommand, Path: rel,
			Detail: "cliDispatch[\"" + name + "\"]",
		})
	}
	return out, warnings
}

// attachTests links every package node to the test bundle covering
// its dir (the mapping the suite selection consumes).
func attachTests(inv *Inventory) {
	tests := map[string][]string{}
	for _, n := range inv.Nodes {
		if n.Kind == KindTest {
			tests[n.Path] = n.Tests
		}
	}
	for i, n := range inv.Nodes {
		if n.Kind == KindPackage {
			if files, ok := tests[n.Path]; ok {
				inv.Nodes[i].Tests = files
			}
		}
	}
}

// NodeByID returns the node or nil.
func (inv Inventory) NodeByID(id string) *Node {
	for i, n := range inv.Nodes {
		if n.ID == id {
			return &inv.Nodes[i]
		}
	}
	return nil
}

// TestIDs lists every test bundle id, sorted.
func (inv Inventory) TestIDs() []string {
	var out []string
	for _, n := range inv.Nodes {
		if n.Kind == KindTest {
			out = append(out, n.ID)
		}
	}
	sort.Strings(out)
	return out
}
