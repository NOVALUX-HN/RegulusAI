// Package impact is Impact Analysis and the repository dependency
// graph (F12.8/NOV-103): predict affected components, contracts and
// tests before changing code.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - inventory: context.Inventory (F12.3) owns the code/symbol/
//     dependency corpus walk (bounds, skip dirs, git head, memory);
//     impact folds its fragments into packages, APIs and
//     dependencies and adds what the corpus does not carry (test
//     files, workflows, configs, commands) — never a second corpus;
//   - tiers: bench.TierL0..L3 (F11.3) name the selection levels the
//     focused suite maps onto; impact never redefines them;
//   - severity: the F12.4 quality diagnosis feeds the risk as an
//     observed input (alert/failure rate), never recomputed here;
//   - trace: the CLI records F11.1 exec events (run.start +
//     task.end + run.end); the engine stays pure (no clock, no disk,
//     no network beyond its inputs);
//   - critical packages mirror size.CriticalPackages (the security
//     kernel + contract surface); a test pins the mirror so the two
//     can never drift apart.
//
// Every edge carries its verification source (go-import, test-
// import, cli-route, workflow-ref, config-ref) plus the commit and
// date that last touched its evidence file: `git log -1
// --format=%H %cI -- <file>` reproduces them, `grep -n` reproduces
// the evidence line. Without git the edge falls back to
// commit "worktree" and the evidence mtime — still sourced, dated
// and flagged, never silently undated. The conservative fallback
// widens instead of omitting: unknown changed files select the full
// suite, critical tests are never omitted by weak inference.
package impact

import (
	"sort"
	"strings"
	"time"
)

// Node kinds: the closed inventory vocabulary (spec: modules,
// packages, APIs, config, workflows, dependencies, plus the tests
// and commands the diff maps onto).
const (
	KindModule     = "module"
	KindPackage    = "package"
	KindAPI        = "api"
	KindConfig     = "config"
	KindWorkflow   = "workflow"
	KindDependency = "dependency"
	KindTest       = "test"
	KindCommand    = "command"
)

// Edge sources: the only verifiable origins. Each one names the
// exact check that reproduces it (see docs/impact-analysis.md).
const (
	SourceGoImport   = "go-import"
	SourceTestImport = "test-import"
	SourceCLIRoute   = "cli-route"
	SourceWorkflow   = "workflow-ref"
	SourceConfig     = "config-ref"
)

// CriticalPackages mirrors size.CriticalPackages (security kernel +
// contract surface): touching one always widens the risk to high and
// keeps its tests in every focused suite. Pinned by TestCriticalMirrorsSize.
var CriticalPackages = []string{"gates", "evidence", "receipts", "manifest", "verdict", "contract"}

// Node is one inventoried unit: a package, API, config, workflow,
// dependency, test bundle, command or the module itself.
type Node struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Path     string   `json:"path"`
	Detail   string   `json:"detail,omitempty"`
	Critical bool     `json:"critical,omitempty"`
	Tests    []string `json:"tests,omitempty"`
}

// Edge is one verified dependency: From depends on To. Source names
// the check, File:Line the evidence, Commit/Date the last touch
// (`git log -1 --format=%H %cI -- <file>`, fallback worktree+mtime).
type Edge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Source string `json:"source"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Commit string `json:"commit"`
	Date   string `json:"date"`
	Reused bool   `json:"reused,omitempty"`
}

// Evidence renders the verifiable locator.
func (e Edge) Evidence() string {
	return e.File + ":" + itoa(e.Line)
}

// VerifyHint renders the two commands that reproduce the edge.
func (e Edge) VerifyHint() string {
	return "grep -n . " + e.File + " # línea " + itoa(e.Line) +
		" ; git log -1 --format='%H %cI' -- " + e.File
}

// Inventory is the closed census: modules, packages, APIs, config,
// workflows and dependencies with warnings instead of silent skips.
type Inventory struct {
	Module   string   `json:"module"`
	Nodes    []Node   `json:"nodes"`
	Warnings []string `json:"warnings,omitempty"`
}

// Graph is the incremental dependency graph: nodes plus verified
// edges, the per-file fingerprints the next build reuses and the
// HEAD it was built at.
type Graph struct {
	Module   string            `json:"module"`
	Nodes    []Node            `json:"nodes"`
	Edges    []Edge            `json:"edges"`
	Files    map[string]string `json:"files,omitempty"`
	Head     string            `json:"head,omitempty"`
	BuiltAt  time.Time         `json:"built_at"`
	Warnings []string          `json:"warnings,omitempty"`
}

// Impact maps one diff onto the graph: the directly touched nodes,
// their transitive consumers, the critical path and the covering
// tests. Widened reports the conservative fallback (unknown input
// selects everything); every inclusion carries its reason.
type Impact struct {
	Changed   []string `json:"changed"`
	Direct    []string `json:"direct"`
	Consumers []string `json:"consumers"`
	Critical  []string `json:"critical"`
	Tests     []string `json:"tests"`
	Paths     []string `json:"paths,omitempty"`
	Widened   bool     `json:"widened,omitempty"`
	Reasons   []string `json:"reasons"`
}

// Risk is the explicable score: every term names its weight so the
// number audits itself. Severity is the observed F12.4 input
// (alert/failure rate 0..1); unknown stays 0 and widens elsewhere,
// never here.
type Risk struct {
	Score    float64  `json:"score"`
	Level    string   `json:"level"`
	Severity float64  `json:"severity"`
	Reasons  []string `json:"reasons"`
}

// SuiteSelection is the focused suite: what to run, what the focus
// omits (with its recall measured against full), and why.
type SuiteSelection struct {
	Focused []string         `json:"focused"`
	Full    []string         `json:"full"`
	Omitted []string         `json:"omitted,omitempty"`
	Widened bool             `json:"widened,omitempty"`
	Tier    string           `json:"tier"`
	Metrics SelectionMetrics `json:"metrics,omitempty"`
	Reasons []string         `json:"reasons"`
}

// SelectionMetrics measures the focus against the full suite:
// precision = predicted∩actual / predicted, recall =
// predicted∩actual / actual (empty actual recalls 1 — nothing to
// miss). Actual comes from --failed (observed failures) or the full
// set when nothing failed.
type SelectionMetrics struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	Hit       int     `json:"hit"`
	Predicted int     `json:"predicted"`
	Actual    int     `json:"actual"`
}

// BenchmarkReport documents the CodeGraph decision: the own graph
// stays the dependency (verified edges, no daemon), the external
// tool is evaluated on cost/benefit only.
type BenchmarkReport struct {
	Version      string   `json:"version"`
	GraphBuildMs int64    `json:"graph_build_ms"`
	NaiveScanMs  int64    `json:"naive_scan_ms"`
	Nodes        int      `json:"nodes"`
	Edges        int      `json:"edges"`
	CodeGraph    string   `json:"codegraph"`
	Decision     string   `json:"decision"`
	Notes        []string `json:"notes"`
}

// BenchmarkVersion pins the benchmark document shape.
const BenchmarkVersion = "impact-benchmark-v1"

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// normalizePath cleans one user-supplied changed path: slashes,
// leading ./, trailing /.
func normalizePath(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\\", "/"))
	s = strings.TrimPrefix(s, "./")
	s = strings.TrimSuffix(s, "/")
	return s
}

// isCriticalPkg reports whether a package dir belongs to the
// security kernel or contract surface.
func isCriticalPkg(pkgDir string) bool {
	base := pkgDir
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	for _, c := range CriticalPackages {
		if base == c {
			return true
		}
	}
	return false
}

// sortedUnique returns the sorted deduplicated copy.
func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
