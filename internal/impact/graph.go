// Graph (F12.8/NOV-103): the incremental dependency graph with
// verifiable edges.
//
// Every edge names its source (the check that reproduces it) and the
// commit/date that last touched its evidence file. BuildGraph takes
// the previous graph and reuses every edge whose evidence file
// fingerprint did not move — the commit/date travel with the reuse
// (Reused=true) instead of re-querying git. Fresh edges resolve
// through GitLookup; the default runs `git log -1 --format=%H %cI`
// per evidence file with a bounded timeout and falls back to
// commit "worktree" plus the evidence mtime, so an edge is never
// silently undated.
package impact

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/pxexec"
)

// GitLookup resolves the last-touch commit and ISO date of one
// repo-relative path. Tests inject a fake; production uses
// DefaultGitLookup.
type GitLookup func(rel string) (commit, date string, err error)

// DefaultGitLookup returns a lookup rooted at root: one bounded
// `git log -1` per evidence file, fallback worktree+mtime when git
// is absent or the file untracked.
func DefaultGitLookup(root string) GitLookup {
	return func(rel string) (string, string, error) {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res := pxexec.Run(ctx, pxexec.Spec{
			Name: "git", Args: []string{"log", "-1", "--format=%H%x00%cI", "--", rel},
			Dir: root, Timeout: 10 * time.Second,
		})
		if res.Outcome == pxexec.OutcomePass {
			if parts := strings.SplitN(strings.TrimSpace(res.Stdout), "\x00", 2); len(parts) == 2 && parts[0] != "" {
				return parts[0], parts[1], nil
			}
		}
		return fallbackStamp(abs)
	}
}

// fallbackStamp dates an edge without git: commit "worktree" (the
// same version context.Inventory prints) and the evidence mtime.
func fallbackStamp(abs string) (string, string, error) {
	if fi, err := os.Stat(abs); err == nil {
		return "worktree", fi.ModTime().UTC().Format(time.RFC3339), nil
	}
	return "worktree", time.Now().UTC().Format(time.RFC3339), fmt.Errorf("sin evidencia %s", abs)
}

// dispatchRe finds cliDispatch["name"] entries (evidence for
// cli-route edges and command nodes).
var dispatchRe = regexp.MustCompile(`cliDispatch\["([a-z0-9][a-z0-9-]*)"\]`)

// dispatchNames lists every command registered in one Go source.
func dispatchNames(src string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range dispatchRe.FindAllStringSubmatch(src, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// internalRefRe finds internal/<pkg> mentions (evidence for
// workflow-ref and config-ref edges).
var internalRefRe = regexp.MustCompile(`internal/([a-z0-9][a-z0-9_/]*)`)

// regulusCmdRe finds `regulus <cmd>` invocations in workflows.
var regulusCmdRe = regexp.MustCompile(`regulus\s+([a-z][a-z0-9-]*)`)

// BuildGraph builds the graph over root, reusing prev edges whose
// evidence fingerprint did not move. It fails only on an unreadable
// root; git failures degrade to worktree+mtime with a warning.
func BuildGraph(root string, prev *Graph, lookup GitLookup) (Graph, error) {
	inv, err := Census(root)
	if err != nil {
		return Graph{}, err
	}
	if lookup == nil {
		lookup = DefaultGitLookup(root)
	}
	g := Graph{
		Module: inv.Module, Nodes: inv.Nodes,
		Files: map[string]string{}, Warnings: append([]string{}, inv.Warnings...),
		BuiltAt: time.Now().UTC(),
	}
	g.Head = headOf(root)
	byID := map[string]bool{}
	for _, n := range g.Nodes {
		byID[n.ID] = true
	}
	ensure := func(n Node) {
		if !byID[n.ID] {
			byID[n.ID] = true
			g.Nodes = append(g.Nodes, n)
		}
	}
	var fresh []Edge
	addEdge := func(from, to, source, file string, line int) {
		if from == to {
			return
		}
		for _, e := range fresh {
			if e.From == from && e.To == to && e.Source == source {
				return
			}
		}
		fresh = append(fresh, Edge{From: from, To: to, Source: source, File: file, Line: line})
	}
	walkGo(root, &g, byID, ensure, addEdge)
	walkRefs(root, &g, byID, ensure, addEdge, inv)
	reused := 0
	prevByKey := map[string]Edge{}
	prevFiles := map[string]string{}
	if prev != nil {
		prevFiles = prev.Files
		for _, e := range prev.Edges {
			prevByKey[e.From+"\x00"+e.To+"\x00"+e.Source] = e
		}
	}
	for i, e := range fresh {
		fp := fingerprint(root, e.File)
		g.Files[e.File] = fp
		if pe, ok := prevByKey[e.From+"\x00"+e.To+"\x00"+e.Source]; ok && prevFiles[e.File] == fp && fp != "" {
			fresh[i].Commit, fresh[i].Date, fresh[i].Reused = pe.Commit, pe.Date, true
			reused++
			continue
		}
		commit, date, err := lookup(e.File)
		if err != nil {
			g.Warnings = append(g.Warnings, fmt.Sprintf("git: %s sin commit (%v): worktree+mtime", e.File, err))
		}
		fresh[i].Commit, fresh[i].Date = commit, date
	}
	sort.Slice(fresh, func(i, j int) bool {
		if fresh[i].From != fresh[j].From {
			return fresh[i].From < fresh[j].From
		}
		if fresh[i].To != fresh[j].To {
			return fresh[i].To < fresh[j].To
		}
		return fresh[i].Source < fresh[j].Source
	})
	g.Edges = fresh
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	if reused > 0 {
		g.Warnings = append(g.Warnings, fmt.Sprintf("incremental: %d de %d aristas reutilizadas (evidencia sin mover)", reused, len(fresh)))
	}
	return g, nil
}

// headOf best-efforts the HEAD sha (empty when git is absent or the
// probe times out).
func headOf(root string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res := pxexec.Run(ctx, pxexec.Spec{
		Name: "git", Args: []string{"rev-parse", "HEAD"},
		Dir: root, Timeout: 10 * time.Second,
	})
	if res.Outcome != pxexec.OutcomePass {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

// fingerprint identifies an evidence file state (mtime+size): equal
// fingerprints mean the edge evidence did not move.
func fingerprint(root, rel string) string {
	fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	return fi.ModTime().UTC().Format(time.RFC3339Nano) + ":" + itoa(int(fi.Size()))
}

// goFile lists one parsed Go source: its package dir and imports.
type goFile struct {
	rel     string
	dir     string
	test    bool
	imports []fileImport
}

type fileImport struct {
	path string
	line int
}

// walkGo parses every non-skipped .go file: internal imports become
// go-import (test-import from _test.go) edges, external imports
// become dependency edges (dep nodes are created on the fly).
func walkGo(root string, g *Graph, byID map[string]bool, ensure func(Node), addEdge func(from, to, source, file string, line int)) {
	module := g.Module
	if module == "" {
		module = "regulus-ai"
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		gf, ok := parseGoFile(path, rel)
		if !ok {
			g.Warnings = append(g.Warnings, fmt.Sprintf("sin parsear %s: omitido (sintaxis)", rel))
			return nil
		}
		from := "pkg:" + gf.dir
		source := SourceGoImport
		if gf.test {
			from = "test:" + gf.dir
			source = SourceTestImport
		}
		for _, imp := range gf.imports {
			to, isInternal := importTarget(module, imp.path)
			if !isInternal {
				if !strings.Contains(strings.Split(imp.path, "/")[0], ".") {
					continue // stdlib: no es dependencia verificable del repo
				}
				dep := "dep:" + imp.path
				ensure(Node{ID: dep, Kind: KindDependency, Path: imp.path, Detail: "imported by " + rel})
				addEdge(from, dep, source, rel, imp.line)
				continue
			}
			ensure(Node{ID: "pkg:" + to, Kind: KindPackage, Path: to, Critical: isCriticalPkg(to)})
			addEdge(from, "pkg:"+to, source, rel, imp.line)
		}
		return nil
	})
}

// parseGoFile parses one file imports-only (positioned, so the
// evidence line is exact).
func parseGoFile(abs, rel string) (goFile, bool) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, abs, nil, parser.ImportsOnly)
	if err != nil {
		return goFile{}, false
	}
	gf := goFile{rel: rel, dir: dirOf(rel), test: strings.HasSuffix(rel, "_test.go")}
	for _, spec := range f.Imports {
		p := strings.Trim(spec.Path.Value, "\"")
		if p == "" {
			continue
		}
		gf.imports = append(gf.imports, fileImport{path: p, line: fset.Position(spec.Pos()).Line})
	}
	return gf, true
}

// importTarget maps an import path onto a package node: regulus-ai/
// <rest> (or the local module prefix) folds to the package dir,
// anything with a dot in the first segment is external.
func importTarget(module, imp string) (string, bool) {
	if imp == module || strings.HasPrefix(imp, module+"/") {
		rest := strings.TrimPrefix(strings.TrimPrefix(imp, module), "/")
		if strings.HasPrefix(rest, "internal/") || strings.HasPrefix(rest, "cmd/") {
			return rest, true
		}
		if rest == "" {
			return ".", true
		}
		return rest, true
	}
	if strings.HasPrefix(imp, "regulus-ai/") {
		return strings.TrimPrefix(imp, "regulus-ai/"), true
	}
	return "", false
}

// walkRefs derives cli-route, workflow-ref and config-ref edges:
//   - cli-route: cmd:X -> pkg:internal/cli (cliDispatch entry) and
//     pkg:internal/cli -> pkg:internal/cmdx (forwarder files calling
//     cmdx.New — one edge per file, first occurrence);
//   - workflow-ref: wf -> pkg/cmd mentions in .github/workflows;
//   - config-ref: cfg -> pkg/cmd mentions in root configs.
func walkRefs(root string, g *Graph, byID map[string]bool, ensure func(Node), addEdge func(from, to, source, file string, line int), inv Inventory) {
	walkCliRefs(root, addEdge)
	walkWorkflowRefs(root, ensure, addEdge, inv, collectCmds(inv))
}

// collectCmds indexes the command names in the inventory. Pure (no
// FS): the walkers consume the map, so unit tests pin it directly.
func collectCmds(inv Inventory) map[string]bool {
	cmds := map[string]bool{}
	for _, n := range inv.Nodes {
		if n.Kind == KindCommand {
			cmds[strings.TrimPrefix(n.ID, "cmd:")] = true
		}
	}
	return cmds
}

// scanCliLine extracts one line's cli evidence without touching the
// filesystem: the cliDispatch command names plus whether the line
// forwards to cmdx. Pure: unit tests pin it line by line.
func scanCliLine(line string) (cmds []string, forward bool) {
	for _, m := range dispatchRe.FindAllStringSubmatch(line, -1) {
		cmds = append(cmds, m[1])
	}
	return cmds, strings.Contains(line, "cmdx.New(")
}

// scanRefLine extracts one line's workflow/config evidence without
// touching the filesystem: internal package mentions plus `regulus
// <cmd>` invocations. Pure: unit tests pin it line by line (the
// caller filters commands against the inventory).
func scanRefLine(line string) (pkgs []string, cmds []string) {
	for _, m := range internalRefRe.FindAllStringSubmatch(line, -1) {
		pkgs = append(pkgs, strings.TrimSuffix(m[1], "/"))
	}
	for _, m := range regulusCmdRe.FindAllStringSubmatch(line, -1) {
		cmds = append(cmds, m[1])
	}
	return pkgs, cmds
}

// isWorkflowFile reports whether a ref file carries `regulus <cmd>`
// invocations (workflows do; root configs do not).
func isWorkflowFile(rel string) bool {
	return strings.HasSuffix(rel, ".yml") || strings.HasSuffix(rel, ".yaml")
}

// refFrom maps a ref file onto its graph node: workflows to wf:,
// configs to cfg:.
func refFrom(rel, source string) string {
	if source == SourceConfig {
		return "cfg:" + rel
	}
	return "wf:" + rel
}

// sortedRefSources lists the workflow/config evidence files in
// stable order with their edge source.
func sortedRefSources(inv Inventory) ([]string, map[string]string) {
	refFiles := map[string]string{}
	for _, n := range inv.Nodes {
		switch n.Kind {
		case KindWorkflow:
			refFiles[n.Path] = SourceWorkflow
		case KindConfig:
			refFiles[n.Path] = SourceConfig
		}
	}
	var rels []string
	for rel := range refFiles {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	return rels, refFiles
}

// scanCliFile folds one cli source's edges: one cli-route edge per
// cliDispatch entry plus one forwarder edge on the first cmdx.New
// line (one per file, first occurrence).
func scanCliFile(rel, src string, addEdge func(from, to, source, file string, line int)) {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		cmds, _ := scanCliLine(line)
		for _, c := range cmds {
			addEdge("cmd:"+c, "pkg:internal/cli", SourceCLIRoute, rel, i+1)
		}
	}
	for i, line := range lines {
		if _, forward := scanCliLine(line); forward {
			addEdge("pkg:internal/cli", "pkg:internal/cmdx", SourceCLIRoute, rel, i+1)
			break
		}
	}
}

// walkCliRefs scans internal/cli/*.go: every cliDispatch entry is a
// cmd:X -> pkg:internal/cli edge and every forwarder file gains one
// pkg:internal/cli -> pkg:internal/cmdx edge.
func walkCliRefs(root string, addEdge func(from, to, source, file string, line int)) {
	cliDir := filepath.Join(root, "internal", "cli")
	entries, _ := os.ReadDir(cliDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		rel := "internal/cli/" + e.Name()
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		scanCliFile(rel, string(raw), addEdge)
	}
}

// scanRefFile folds one workflow/config file's edges: one edge per
// internal package mention plus, in workflows only, one edge per
// known `regulus <cmd>` invocation.
func scanRefFile(rel, source string, src string, cmds map[string]bool, ensure func(Node), addEdge func(from, to, source, file string, line int)) {
	from := refFrom(rel, source)
	workflow := isWorkflowFile(rel)
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		pkgs, refs := scanRefLine(line)
		for _, pkg := range pkgs {
			ensure(Node{ID: "pkg:" + pkg, Kind: KindPackage, Path: pkg, Critical: isCriticalPkg(pkg)})
			addEdge(from, "pkg:"+pkg, source, rel, i+1)
		}
		if workflow {
			for _, c := range refs {
				if cmds[c] {
					addEdge(from, "cmd:"+c, source, rel, i+1)
				}
			}
		}
	}
}

// walkWorkflowRefs scans the workflow/config evidence files:
// wf/cfg -> pkg/cmd mentions with exact file:line evidence.
// Missing files are skipped (the inventory warns, never fails).
func walkWorkflowRefs(root string, ensure func(Node), addEdge func(from, to, source, file string, line int), inv Inventory, cmds map[string]bool) {
	rels, refFiles := sortedRefSources(inv)
	for _, rel := range rels {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		scanRefFile(rel, refFiles[rel], string(raw), cmds, ensure, addEdge)
	}
}
