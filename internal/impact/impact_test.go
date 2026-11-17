// Engine tests (F12.8/NOV-103): known change finds consumers and
// tests, every edge carries source+commit/date, precision/recall
// measure the focus, uncertainty widens, criticals are never
// omitted by weak inference.
package impact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/size"
)

func fakeLookup(rel string) (string, string, error) {
	return "abc123def456", "2026-01-02T03:04:05Z", nil
}

// fixtureRepo builds a small repo: internal/a (exported Hello),
// internal/b importing a, internal/b test, critical internal/gates,
// one workflow mentioning internal/b.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/demo\n\ngo 1.27\n")
	write("internal/a/a.go", "package a\n\n// Hello is exported.\nfunc Hello() string { return \"hi\" }\n")
	write("internal/b/b.go", "package b\n\nimport \"example.com/demo/internal/a\"\n\nfunc Shout() string { return a.Hello() }\n")
	write("internal/b/b_test.go", "package b\n\nimport (\n\t\"example.com/demo/internal/a\"\n\t\"testing\"\n)\n\nfunc TestShout(t *testing.T) {\n\tif a.Hello() == \"\" {\n\t\tt.Fatal(\"empty\")\n\t}\n}\n")
	write("internal/gates/g.go", "package gates\n\nfunc Check() bool { return true }\n")
	write("internal/gates/g_test.go", "package gates\n\nimport \"testing\"\n\nfunc TestCheck(t *testing.T) {\n\tif !Check() {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")
	write(".github/workflows/ci.yml", "name: ci\non: [push]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go test ./internal/b/...\n")
	return dir
}

func mustGraph(t *testing.T, dir string) Graph {
	t.Helper()
	g, err := BuildGraph(dir, nil, fakeLookup)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	return g
}

func TestCriticalMirrorsSize(t *testing.T) {
	if len(CriticalPackages) != len(size.CriticalPackages) {
		t.Fatalf("CriticalPackages = %v, want mirror of %v", CriticalPackages, size.CriticalPackages)
	}
	for i, c := range CriticalPackages {
		if c != size.CriticalPackages[i] {
			t.Fatalf("CriticalPackages = %v, want mirror of %v", CriticalPackages, size.CriticalPackages)
		}
	}
}

func TestInventoryCensus(t *testing.T) {
	dir := fixtureRepo(t)
	inv, err := Census(dir)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	ids := map[string]bool{}
	for _, n := range inv.Nodes {
		ids[n.ID] = true
	}
	for _, want := range []string{
		"pkg:internal/a", "pkg:internal/b", "pkg:internal/gates",
		"test:internal/b", "test:internal/gates",
		"wf:.github/workflows/ci.yml",
		"api:internal/a/a.go#Hello",
	} {
		if !ids[want] {
			t.Errorf("inventory omits %q (got %d nodes)", want, len(inv.Nodes))
		}
	}
	if n := inv.NodeByID("pkg:internal/gates"); n == nil || !n.Critical {
		t.Errorf("pkg:internal/gates must be critical")
	}
}

func TestGraphEdgesVerified(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	foundImport, foundTest := false, false
	for _, e := range g.Edges {
		if e.Source == "" || e.Commit == "" || e.Date == "" {
			t.Fatalf("edge %+v lacks source/commit/date", e)
		}
		if e.File == "" || e.Line <= 0 {
			t.Fatalf("edge %+v lacks evidence locator", e)
		}
		if e.From == "pkg:internal/b" && e.To == "pkg:internal/a" && e.Source == SourceGoImport {
			foundImport = true
		}
		if e.From == "test:internal/b" && e.To == "pkg:internal/a" && e.Source == SourceTestImport {
			foundTest = true
		}
	}
	if !foundImport {
		t.Errorf("missing go-import pkg:internal/b -> pkg:internal/a")
	}
	if !foundTest {
		t.Errorf("missing test-import test:internal/b -> pkg:internal/a")
	}
}

func TestMapDiffKnownChange(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	imp := MapDiff(g, []string{"internal/a/a.go"})
	has := func(list []string, want string) bool {
		for _, s := range list {
			if s == want {
				return true
			}
		}
		return false
	}
	if !has(imp.Direct, "pkg:internal/a") {
		t.Errorf("direct must hold pkg:internal/a, got %v", imp.Direct)
	}
	if !has(imp.Consumers, "pkg:internal/b") {
		t.Errorf("consumers must hold pkg:internal/b, got %v", imp.Consumers)
	}
	if !has(imp.Tests, "test:internal/b") {
		t.Errorf("tests must hold test:internal/b, got %v", imp.Tests)
	}
	if imp.Widened {
		t.Errorf("known change must not widen")
	}
}

func TestWidenUnknown(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	imp := MapDiff(g, []string{"zzz/unknown.go"})
	if !imp.Widened {
		t.Fatalf("unknown file must widen, got %+v", imp)
	}
	sel := SelectSuite(g, imp, nil)
	if !sel.Widened || len(sel.Focused) != len(sel.Full) {
		t.Fatalf("widened suite must equal full, got %+v", sel)
	}
}

func TestCriticalNeverOmitted(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	imp := MapDiff(g, []string{"internal/gates/g.go"})
	if len(imp.Critical) == 0 {
		t.Fatalf("gates change must be critical, got %+v", imp)
	}
	sel := SelectSuite(g, imp, nil)
	found := false
	for _, f := range sel.Focused {
		if f == "test:internal/gates" {
			found = true
		}
	}
	if !found {
		t.Fatalf("focused suite omits critical test:internal/gates: %v", sel.Focused)
	}
	if r := AssessRisk(imp, 0); r.Level != "high" {
		t.Fatalf("critical risk level = %q, want high", r.Level)
	}
}

func TestMeasurePrecisionRecall(t *testing.T) {
	m := MeasureSelection([]string{"t1", "t2"}, []string{"t2", "t3"}, []string{"t1", "t2", "t3"})
	if m.Precision != 0.5 || m.Recall != 0.5 {
		t.Fatalf("want P=R=0.5, got %+v", m)
	}
	m = MeasureSelection([]string{"t1"}, []string{}, []string{"t1", "t2"})
	if m.Precision != 1 || m.Recall != 0.5 {
		t.Fatalf("empty actuals fall back to full (P=1 R=0.5), got %+v", m)
	}
	m = MeasureSelection(nil, nil, nil)
	if m.Precision != 1 || m.Recall != 1 {
		t.Fatalf("nothing predicted, nothing actual must score 1/1, got %+v", m)
	}
	m = MeasureSelection(nil, []string{"t1"}, []string{"t1"})
	if m.Recall != 0 {
		t.Fatalf("empty prediction with actuals must recall 0, got %+v", m)
	}
}

func TestIncrementalReuse(t *testing.T) {
	dir := fixtureRepo(t)
	g1 := mustGraph(t, dir)
	g2, err := BuildGraph(dir, &g1, func(rel string) (string, string, error) {
		t.Fatalf("reuse must not call git for %s", rel)
		return "", "", nil
	})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if len(g2.Edges) != len(g1.Edges) {
		t.Fatalf("edge count moved %d -> %d", len(g1.Edges), len(g2.Edges))
	}
	for _, e := range g2.Edges {
		if !e.Reused {
			t.Fatalf("edge %+v was not reused", e)
		}
	}
}

func TestDetectCycles(t *testing.T) {
	g := Graph{Nodes: []Node{
		{ID: "pkg:a"}, {ID: "pkg:b"}, {ID: "pkg:c"},
	}, Edges: []Edge{
		{From: "pkg:a", To: "pkg:b", Source: SourceGoImport, File: "a", Line: 1},
		{From: "pkg:b", To: "pkg:a", Source: SourceGoImport, File: "b", Line: 1},
		{From: "pkg:c", To: "pkg:a", Source: SourceGoImport, File: "c", Line: 1},
	}}
	cycles := DetectCycles(g)
	if len(cycles) != 1 {
		t.Fatalf("want 1 cycle, got %v", cycles)
	}
	if !strings.Contains(strings.Join(cycles[0], " "), "pkg:a") {
		t.Fatalf("cycle must name pkg:a: %v", cycles)
	}
	if len(DetectCycles(Graph{})) != 0 {
		t.Fatalf("empty graph must have no cycles")
	}
}

func TestBreakingChanges(t *testing.T) {
	before := Inventory{Nodes: []Node{
		{ID: "pkg:internal/a", Kind: KindPackage, Path: "internal/a"},
		{ID: "api:internal/a/a.go#Hello", Kind: KindAPI, Path: "internal/a/a.go"},
	}}
	after := Graph{Nodes: []Node{{ID: "pkg:internal/a", Kind: KindPackage}}}
	got := BreakingChanges(before, after)
	if len(got) != 1 || !strings.Contains(got[0], "Hello") {
		t.Fatalf("want removed API reported, got %v", got)
	}
}

func TestHiddenDeps(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	hidden := HiddenDeps(g)
	if len(hidden) == 0 {
		t.Fatalf("workflow mentioning internal/b must surface a hidden dep")
	}
	for _, e := range hidden {
		if e.Source == SourceGoImport || e.Source == SourceTestImport {
			t.Fatalf("hidden dep carries import source: %+v", e)
		}
	}
}

func TestRunBenchmark(t *testing.T) {
	rep, err := RunBenchmark(fixtureRepo(t), fakeLookup)
	if err != nil {
		t.Fatalf("RunBenchmark: %v", err)
	}
	if rep.Nodes == 0 || rep.Edges == 0 {
		t.Fatalf("benchmark must count nodes/edges: %+v", rep)
	}
	if !strings.Contains(rep.Decision, "grafo propio") {
		t.Fatalf("decision must keep the own graph: %q", rep.Decision)
	}
}

func TestImportTargetDotlessModule(t *testing.T) {
	// Regression: the stdlib filter once dropped every import of a
	// dot-less module (regulus-ai/...) as "stdlib", leaving zero
	// go-import edges on the real repo. The module prefix always wins.
	to, ok := importTarget("demo", "demo/internal/a")
	if !ok || to != "internal/a" {
		t.Fatalf("importTarget(demo, demo/internal/a) = %q, %v", to, ok)
	}
	if _, ok := importTarget("demo", "fmt"); ok {
		t.Fatalf("stdlib fmt must not map to an internal package")
	}
}

func TestRiskExplanation(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	imp := MapDiff(g, []string{"internal/a/a.go"})
	r := AssessRisk(imp, 0.2)
	if r.Score <= 0 || r.Score > 100 {
		t.Fatalf("score out of range: %+v", r)
	}
	if len(r.Reasons) < 4 {
		t.Fatalf("risk must explain every term: %+v", r)
	}
	joined := strings.Join(r.Reasons, "\n")
	if !strings.Contains(joined, "F12.4") {
		t.Fatalf("risk must cite the F12.4 severity input: %q", joined)
	}
}
