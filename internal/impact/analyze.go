// Analysis (F12.8/NOV-103): diff mapping, explicable risk, focused
// suite with measured precision/recall, cycles, hidden dependencies
// and breaking changes.
//
// The conservative fallback is the load-bearing rule: any changed
// path that maps to no node (unknown file, unparsed evidence, empty
// graph) widens the impact to every test instead of guessing.
// Critical tests are never omitted by weak inference — a focused
// suite that drops them is a bug, pinned by test.
package impact

import (
	"fmt"
	"sort"
	"strings"
)

// MapDiff maps changed repo-relative paths (files or dirs, plus
// cmd:<name>) onto the graph: directly touched nodes, transitive
// consumers (reverse reachable), critical nodes on the path and the
// covering tests. Unknown entries widen instead of guessing.
func MapDiff(g Graph, changed []string) Impact {
	imp := Impact{}
	byID := map[string]Node{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	fwd := map[string][]string{}
	rev := map[string][]string{}
	for _, e := range g.Edges {
		fwd[e.From] = append(fwd[e.From], e.To)
		rev[e.To] = append(rev[e.To], e.From)
	}
	var direct []string
	for _, c := range changed {
		c = normalizePath(c)
		if c == "" {
			continue
		}
		imp.Changed = append(imp.Changed, c)
		matched := matchChanged(g, c)
		if len(matched) == 0 {
			imp.Widened = true
			imp.Reasons = append(imp.Reasons, fmt.Sprintf("sin nodo para %q: ampliación conservadora (mejor de más que omitir)", c))
			continue
		}
		for _, m := range matched {
			imp.Reasons = append(imp.Reasons, fmt.Sprintf("%s toca %s (%s)", c, m, byID[m].Kind))
		}
		direct = append(direct, matched...)
	}
	direct = sortedUnique(direct)
	imp.Direct = direct
	seen := map[string]bool{}
	for _, d := range direct {
		seen[d] = true
	}
	var queue []string
	queue = append(queue, direct...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range rev[cur] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	for id := range seen {
		isDirect := false
		for _, d := range direct {
			if d == id {
				isDirect = true
				break
			}
		}
		if !isDirect {
			imp.Consumers = append(imp.Consumers, id)
		}
	}
	sort.Strings(imp.Consumers)
	for id := range seen {
		if n, ok := byID[id]; ok && n.Kind == KindPackage && n.Critical {
			imp.Critical = append(imp.Critical, id)
		}
		if n, ok := byID[id]; ok && n.Critical && n.Kind != KindPackage {
			imp.Critical = append(imp.Critical, id)
		}
	}
	sort.Strings(imp.Critical)
	imp.Tests = coveringTests(g, seen)
	imp.Paths = criticalPaths(fwd, direct, byID)
	if imp.Widened {
		imp.Reasons = append(imp.Reasons, "impacto ampliado: la selección usa la suite completa")
	}
	return imp
}

// matchChanged resolves one changed path onto node ids.
func matchChanged(g Graph, c string) []string {
	if strings.HasPrefix(c, "cmd:") {
		for _, n := range g.Nodes {
			if n.ID == c {
				return []string{c}
			}
		}
		return nil
	}
	var out []string
	isTest := strings.HasSuffix(c, "_test.go")
	dir := c
	if !isDirID(g, c) {
		dir = dirOf(c)
	}
	for _, n := range g.Nodes {
		switch n.Kind {
		case KindPackage:
			if n.Path == dir || n.Path == c {
				out = append(out, n.ID)
			}
		case KindTest:
			if n.Path == dir || n.Path == c {
				out = append(out, n.ID)
			} else {
				for _, f := range n.Tests {
					if f == c {
						out = append(out, n.ID)
						break
					}
				}
			}
		case KindWorkflow, KindConfig:
			if n.Path == c {
				out = append(out, n.ID)
			}
		case KindAPI:
			if n.Path == c {
				out = append(out, n.ID)
			}
		case KindDependency:
			if c == "go.mod" || c == "go.sum" {
				out = append(out, n.ID)
			}
		}
	}
	if isTest {
		var only []string
		for _, id := range out {
			if strings.HasPrefix(id, "test:") {
				only = append(only, id)
			}
		}
		if len(only) > 0 {
			return sortedUnique(only)
		}
	}
	return sortedUnique(out)
}

// isDirID reports whether c names a package/test dir node.
func isDirID(g Graph, c string) bool {
	for _, n := range g.Nodes {
		if (n.Kind == KindPackage || n.Kind == KindTest) && n.Path == c {
			return true
		}
	}
	return false
}

// coveringTests collects test bundles inside the affected set, the
// same-dir bundle of every affected package (a _test.go in package
// p covers p without importing it) plus any test bundle with an
// edge into the affected set.
func coveringTests(g Graph, affected map[string]bool) []string {
	nodes := map[string]bool{}
	for _, n := range g.Nodes {
		nodes[n.ID] = true
	}
	var out []string
	for id := range affected {
		if strings.HasPrefix(id, "test:") {
			out = append(out, id)
		}
		if strings.HasPrefix(id, "pkg:") {
			if tb := "test:" + strings.TrimPrefix(id, "pkg:"); nodes[tb] {
				out = append(out, tb)
			}
		}
	}
	for _, e := range g.Edges {
		if strings.HasPrefix(e.From, "test:") && affected[e.To] && !affected[e.From] {
			out = append(out, e.From)
		}
	}
	return sortedUnique(out)
}

// criticalPaths traces forward paths from the change to critical
// nodes (depth 6, 10 paths max): the "rutas críticas" of the spec.
func criticalPaths(fwd map[string][]string, direct []string, byID map[string]Node) []string {
	var paths []string
	var walk func(cur string, trail []string, depth int)
	walk = func(cur string, trail []string, depth int) {
		if len(paths) >= 10 || depth > 6 {
			return
		}
		if n, ok := byID[cur]; ok && n.Critical && len(trail) > 1 {
			paths = append(paths, strings.Join(trail, " -> "))
			return
		}
		next := append([]string{}, fwd[cur]...)
		sort.Strings(next)
		for _, n := range next {
			loop := false
			for _, t := range trail {
				if t == n {
					loop = true
					break
				}
			}
			if !loop {
				walk(n, append(trail, n), depth+1)
			}
		}
	}
	for _, d := range direct {
		walk(d, []string{d}, 0)
	}
	sort.Strings(paths)
	return paths
}

// AssessRisk scores the impact explicably: every term names its
// weight, severity (the observed F12.4 rate 0..1) scales the base.
// Any critical node floors the level at high — the fallback never
// downgrades the kernel by inference.
func AssessRisk(imp Impact, severity float64) Risk {
	if severity < 0 {
		severity = 0
	}
	if severity > 1 {
		severity = 1
	}
	base := 5*len(imp.Direct) + 3*len(imp.Consumers) + 2*len(imp.Tests)
	reasons := []string{
		fmt.Sprintf("%d nodo(s) directo(s) (+%d)", len(imp.Direct), 5*len(imp.Direct)),
		fmt.Sprintf("%d consumidor(es) transitivo(s) (+%d)", len(imp.Consumers), 3*len(imp.Consumers)),
		fmt.Sprintf("%d test(s) afectado(s) (+%d)", len(imp.Tests), 2*len(imp.Tests)),
	}
	if len(imp.Critical) > 0 {
		base += 40
		reasons = append(reasons, fmt.Sprintf("%d paquete(s) crítico(s) %s (+40): gates/evidence/receipts/manifest/verdict/contract nunca se degradan",
			len(imp.Critical), strings.Join(imp.Critical, ", ")))
	}
	if imp.Widened {
		base += 15
		reasons = append(reasons, "+15 por ampliación conservadora (entrada sin nodo: la incertidumbre suma, nunca resta)")
	}
	score := float64(base) * (1 + severity)
	if score > 100 {
		score = 100
	}
	reasons = append(reasons, fmt.Sprintf("severidad observada F12.4 = %.2f (×%.2f)", severity, 1+severity))
	level := "low"
	switch {
	case score >= 60 || len(imp.Critical) > 0:
		level = "high"
	case score >= 25:
		level = "medium"
	}
	return Risk{Score: score, Level: level, Severity: severity, Reasons: reasons}
}

// SelectSuite maps the impact onto the focused test set over full
// (every test bundle when empty). The conservative fallback widens
// to full on unknown input; critical-covering tests are always kept
// (never omitted by weak inference). Tier follows F11.3: L0 smoke
// for a single package without consumers, L1 focused otherwise, L2
// when critical or widened, never L3 (periodic/release only).
func SelectSuite(g Graph, imp Impact, full []string) SuiteSelection {
	if len(full) == 0 {
		for _, n := range g.Nodes {
			if n.Kind == KindTest {
				full = append(full, n.ID)
			}
		}
	}
	sort.Strings(full)
	sel := SuiteSelection{Full: append([]string{}, full...)}
	fullSet := map[string]bool{}
	for _, f := range full {
		fullSet[f] = true
	}
	var focused []string
	for _, t := range imp.Tests {
		if fullSet[t] {
			focused = append(focused, t)
		}
	}
	byID := map[string]Node{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	for _, c := range imp.Critical {
		if n, ok := byID[c]; ok {
			for _, t := range n.Tests {
				if fullSet[t] {
					focused = append(focused, t)
				}
			}
			if len(n.Tests) == 0 {
				for _, e := range g.Edges {
					if e.To == c && strings.HasPrefix(e.From, "test:") && fullSet[e.From] {
						focused = append(focused, e.From)
					}
				}
			}
		}
	}
	focused = sortedUnique(focused)
	if imp.Widened {
		sel.Widened = true
		sel.Focused = append([]string{}, full...)
		sel.Reasons = append(sel.Reasons, "ampliación conservadora: entrada sin nodo verificado, se corre la suite completa")
	} else {
		sel.Focused = focused
		sel.Reasons = append(sel.Reasons, fmt.Sprintf("foco: %d test(s) con camino verificado desde el diff", len(focused)))
	}
	sel.Reasons = append(sel.Reasons, imp.Reasons...)
	for _, f := range full {
		found := false
		for _, s := range sel.Focused {
			if s == f {
				found = true
				break
			}
		}
		if !found {
			sel.Omitted = append(sel.Omitted, f)
		}
	}
	sel.Tier = "L1"
	switch {
	case sel.Widened || len(imp.Critical) > 0:
		sel.Tier = "L2"
	case len(imp.Direct) <= 1 && len(imp.Consumers) == 0:
		sel.Tier = "L0"
	}
	sel.Reasons = append(sel.Reasons, fmt.Sprintf("tier F11.3 sugerido: %s (L3 solo periódica/pre-release, nunca por foco)", sel.Tier))
	return sel
}

// MeasureSelection scores a focused selection: precision over what
// was predicted, recall over what actually failed (or the full set
// when nothing failed). Empty predictions with empty actuals score
// 1/1 — nothing to run, nothing missed; empty predictions with
// actuals score recall 0.
func MeasureSelection(predicted, actual, full []string) SelectionMetrics {
	pred := map[string]bool{}
	for _, p := range predicted {
		pred[p] = true
	}
	act := actual
	if len(act) == 0 {
		act = full
	}
	actSet := map[string]bool{}
	for _, a := range act {
		actSet[a] = true
	}
	hit := 0
	for p := range pred {
		if actSet[p] {
			hit++
		}
	}
	m := SelectionMetrics{Hit: hit, Predicted: len(pred), Actual: len(act)}
	switch {
	case len(pred) == 0 && len(act) == 0:
		m.Precision, m.Recall, m.F1 = 1, 1, 1
	case len(pred) == 0:
		m.Precision, m.Recall, m.F1 = 0, 0, 0
	case len(act) == 0:
		m.Precision, m.Recall, m.F1 = 1, 1, 1
	default:
		m.Precision = float64(hit) / float64(len(pred))
		m.Recall = float64(hit) / float64(len(act))
		if m.Precision+m.Recall > 0 {
			m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
		}
	}
	return m
}

// DetectCycles finds dependency cycles over package/test/command
// nodes (tarjan-free iterative DFS, deterministic order): a cycle
// is a hidden coupling the diff must treat as one unit.
func DetectCycles(g Graph) [][]string {
	adj := map[string][]string{}
	for _, e := range g.Edges {
		if pkgNode(e.From) && pkgNode(e.To) {
			adj[e.From] = append(adj[e.From], e.To)
		}
	}
	for k := range adj {
		sort.Strings(adj[k])
	}
	var keys []string
	for k := range adj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var cycles [][]string
	var visit func(n string)
	visit = func(n string) {
		color[n] = gray
		stack = append(stack, n)
		for _, m := range adj[n] {
			switch color[m] {
			case gray:
				start := 0
				for i, s := range stack {
					if s == m {
						start = i
						break
					}
				}
				cycles = append(cycles, append(append([]string{}, stack[start:]...), m))
			case white:
				visit(m)
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
	}
	for _, k := range keys {
		if color[k] == white {
			visit(k)
		}
	}
	sort.Slice(cycles, func(i, j int) bool {
		return strings.Join(cycles[i], "\x00") < strings.Join(cycles[j], "\x00")
	})
	return cycles
}

// pkgNode keeps cycle detection on first-class code nodes.
func pkgNode(id string) bool {
	return strings.HasPrefix(id, "pkg:") || strings.HasPrefix(id, "test:") || strings.HasPrefix(id, "cmd:")
}

// HiddenDeps lists edges no import declares: cli-route,
// workflow-ref and config-ref. They are the dependencies a pure
// import scan misses — the spec's "dependencias ocultas".
func HiddenDeps(g Graph) []Edge {
	var out []Edge
	for _, e := range g.Edges {
		if e.Source != SourceGoImport && e.Source != SourceTestImport {
			out = append(out, e)
		}
	}
	return out
}

// BreakingChanges diffs two inventories (before file, after graph):
// removed packages and removed exported APIs read as incompatible
// changes the focused suite must cover.
func BreakingChanges(before Inventory, after Graph) []string {
	afterIDs := map[string]bool{}
	for _, n := range after.Nodes {
		afterIDs[n.ID] = true
	}
	var out []string
	for _, n := range before.Nodes {
		if afterIDs[n.ID] {
			continue
		}
		switch n.Kind {
		case KindPackage:
			out = append(out, fmt.Sprintf("breaking: paquete %s eliminado (estaba en %s)", n.ID, n.Path))
		case KindAPI:
			out = append(out, fmt.Sprintf("breaking: API %s eliminada", n.ID))
		}
	}
	sort.Strings(out)
	return out
}
