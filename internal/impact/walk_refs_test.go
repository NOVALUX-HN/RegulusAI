// Tests for the walkRefs split (collectCmds/scanCliLine/scanRefLine
// plus the scan*/walk* folders): same edges as the monolith, pinned
// through pure line scanners, a file fixture and BuildGraph.
package impact

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// edgeCollector records ensure/addEdge calls for assertions.
type edgeCollector struct {
	calls []edgeCall
	byID  map[string]bool
	nodes []Node
}

type edgeCall struct {
	from, to, source, file string
	line                   int
}

func (c *edgeCollector) ensure(n Node) {
	if c.byID == nil {
		c.byID = map[string]bool{}
	}
	if !c.byID[n.ID] {
		c.byID[n.ID] = true
		c.nodes = append(c.nodes, n)
	}
}

func (c *edgeCollector) addEdge(from, to, source, file string, line int) {
	c.calls = append(c.calls, edgeCall{from, to, source, file, line})
}

func (c *edgeCollector) has(from, to, source, file string, line int) bool {
	for _, e := range c.calls {
		if e == (edgeCall{from, to, source, file, line}) {
			return true
		}
	}
	return false
}

func (c *edgeCollector) countTo(to string) int {
	n := 0
	for _, e := range c.calls {
		if e.to == to {
			n++
		}
	}
	return n
}

func TestCollectCmds(t *testing.T) {
	inv := Inventory{Nodes: []Node{
		{ID: "cmd:deploy", Kind: KindCommand},
		{ID: "cmd:gate", Kind: KindCommand},
		{ID: "pkg:internal/cli", Kind: KindPackage},
		{ID: "wf:.github/workflows/ci.yml", Kind: KindWorkflow},
	}}
	want := map[string]bool{"deploy": true, "gate": true}
	if got := collectCmds(inv); !reflect.DeepEqual(got, want) {
		t.Fatalf("collectCmds = %v, want %v", got, want)
	}
	if got := collectCmds(Inventory{}); len(got) != 0 {
		t.Fatalf("collectCmds(empty) = %v, want empty", got)
	}
}

func TestScanCliLine(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		cmds    []string
		forward bool
	}{
		{"plain code", `x := 1`, nil, false},
		{"single dispatch", `cliDispatch["deploy"] = runDeploy`, []string{"deploy"}, false},
		{"two dispatches", `a := cliDispatch["a"]; b := cliDispatch["b-c"]`, []string{"a", "b-c"}, false},
		{"forwarder", `e := cmdx.New(nil)`, nil, true},
		{"dispatch and forward", `cliDispatch["gate"]; _ = cmdx.New(x)`, []string{"gate"}, true},
		{"underscore name ignored", `cliDispatch["x_test"]`, nil, false},
		{"uppercase ignored", `cliDispatch["Deploy"]`, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmds, forward := scanCliLine(tc.line)
			if !reflect.DeepEqual(cmds, tc.cmds) || forward != tc.forward {
				t.Fatalf("scanCliLine(%q) = (%v, %v), want (%v, %v)",
					tc.line, cmds, forward, tc.cmds, tc.forward)
			}
		})
	}
}

func TestScanRefLine(t *testing.T) {
	cases := []struct {
		name string
		line string
		pkgs []string
		cmds []string
	}{
		{"empty", ``, nil, nil},
		{"internal pkg", `import "regulus-ai/internal/cli"`, []string{"internal/cli"}, nil},
		{"trailing slash trimmed", `see internal/foo/ docs`, []string{"internal/foo"}, nil},
		{"regulus cmd", `run: regulus deploy --fast`, nil, []string{"deploy"}},
		{"hyphen cmd", `run: regulus gate-run now`, nil, []string{"gate-run"}},
		{"uppercase ignored", `run: regulus Deploy`, nil, nil},
		{"both", `internal/gates plus regulus gate`, []string{"internal/gates"}, []string{"gate"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkgs, cmds := scanRefLine(tc.line)
			if !reflect.DeepEqual(pkgs, tc.pkgs) || !reflect.DeepEqual(cmds, tc.cmds) {
				t.Fatalf("scanRefLine(%q) = (%v, %v), want (%v, %v)",
					tc.line, pkgs, cmds, tc.pkgs, tc.cmds)
			}
		})
	}
}

func TestRefFromIsWorkflowFile(t *testing.T) {
	if got := refFrom(".github/workflows/ci.yml", SourceWorkflow); got != "wf:.github/workflows/ci.yml" {
		t.Fatalf("refFrom workflow = %q", got)
	}
	if got := refFrom("regulus.json", SourceConfig); got != "cfg:regulus.json" {
		t.Fatalf("refFrom config = %q", got)
	}
	for rel, want := range map[string]bool{
		".github/workflows/ci.yml": true, "hook.yaml": true,
		"regulus.json": false, "ci.yml.bak": false,
	} {
		if got := isWorkflowFile(rel); got != want {
			t.Fatalf("isWorkflowFile(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestScanCliFileEdges(t *testing.T) {
	src := "package cli\n\n" +
		"var _ = cliDispatch[\"deploy\"]\n" +
		"var _ = cmdx.New(a) // forward\n" +
		"var _ = cmdx.New(b) // second, ignored\n" +
		"var _ = cliDispatch[\"gate\"]\n"
	c := &edgeCollector{}
	scanCliFile("internal/cli/dispatch.go", src, c.addEdge)
	const rel = "internal/cli/dispatch.go"
	if !c.has("cmd:deploy", "pkg:internal/cli", SourceCLIRoute, rel, 3) {
		t.Fatalf("missing deploy edge: %+v", c.calls)
	}
	if !c.has("cmd:gate", "pkg:internal/cli", SourceCLIRoute, rel, 6) {
		t.Fatalf("missing gate edge: %+v", c.calls)
	}
	if n := c.countTo("pkg:internal/cmdx"); n != 1 {
		t.Fatalf("forwarder edges = %d, want exactly 1 (first occurrence): %+v", n, c.calls)
	}
	if !c.has("pkg:internal/cli", "pkg:internal/cmdx", SourceCLIRoute, rel, 4) {
		t.Fatalf("forwarder edge must point at line 4: %+v", c.calls)
	}
	if len(c.calls) != 3 {
		t.Fatalf("calls = %+v, want exactly 3 edges", c.calls)
	}
}

func TestScanRefFileEdges(t *testing.T) {
	cmds := map[string]bool{"deploy": true}
	t.Run("workflow pkg and known cmd", func(t *testing.T) {
		c := &edgeCollector{}
		src := "uses internal/gates here\nrun: regulus deploy now\nrun: regulus unknown now\n"
		scanRefFile(".github/workflows/ci.yml", SourceWorkflow, src, cmds, c.ensure, c.addEdge)
		const rel = ".github/workflows/ci.yml"
		from := "wf:" + rel
		if !c.has(from, "pkg:internal/gates", SourceWorkflow, rel, 1) {
			t.Fatalf("missing pkg edge: %+v", c.calls)
		}
		if !c.has(from, "cmd:deploy", SourceWorkflow, rel, 2) {
			t.Fatalf("missing cmd edge: %+v", c.calls)
		}
		if c.countTo("cmd:unknown") != 0 {
			t.Fatalf("unknown cmd must be ignored: %+v", c.calls)
		}
		if !c.byID["pkg:internal/gates"] {
			t.Fatalf("ensure not called for pkg:internal/gates: %+v", c.nodes)
		}
	})
	t.Run("config has no cmd edges", func(t *testing.T) {
		c := &edgeCollector{}
		src := "see internal/evidence\nrun: regulus deploy\n"
		scanRefFile("regulus.json", SourceConfig, src, cmds, c.ensure, c.addEdge)
		if !c.has("cfg:regulus.json", "pkg:internal/evidence", SourceConfig, "regulus.json", 1) {
			t.Fatalf("missing cfg pkg edge: %+v", c.calls)
		}
		for _, e := range c.calls {
			if strings.HasPrefix(e.to, "cmd:") {
				t.Fatalf("config must not gain cmd edges: %+v", c.calls)
			}
		}
	})
}

func TestSortedRefSources(t *testing.T) {
	inv := Inventory{Nodes: []Node{
		{ID: "cfg:regulus.json", Kind: KindConfig, Path: "regulus.json"},
		{ID: "wf:.github/workflows/ci.yml", Kind: KindWorkflow, Path: ".github/workflows/ci.yml"},
		{ID: "pkg:internal/cli", Kind: KindPackage, Path: "internal/cli"},
	}}
	rels, byRel := sortedRefSources(inv)
	want := []string{".github/workflows/ci.yml", "regulus.json"}
	if !reflect.DeepEqual(rels, want) {
		t.Fatalf("sortedRefSources rels = %v, want %v", rels, want)
	}
	if byRel[".github/workflows/ci.yml"] != SourceWorkflow || byRel["regulus.json"] != SourceConfig {
		t.Fatalf("sortedRefSources sources = %v", byRel)
	}
}

// writeFixture creates one file under a temp root, making parents.
func writeFixture(t *testing.T, root, rel, body string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalkRefsFixture(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "internal/cli/dispatch.go",
		"package cli\n\nvar _ = cliDispatch[\"deploy\"]\nvar _ = cmdx.New(nil)\n")
	writeFixture(t, root, ".github/workflows/ci.yml",
		"uses internal/gates here\nrun: regulus deploy now\n")
	writeFixture(t, root, "regulus.json", "{\"see\": \"internal/evidence\"}\n")
	inv := Inventory{Nodes: []Node{
		{ID: "cmd:deploy", Kind: KindCommand, Path: "internal/cli/dispatch.go"},
		{ID: "wf:.github/workflows/ci.yml", Kind: KindWorkflow, Path: ".github/workflows/ci.yml"},
		{ID: "cfg:regulus.json", Kind: KindConfig, Path: "regulus.json"},
		{ID: "wf:.github/workflows/missing.yml", Kind: KindWorkflow, Path: ".github/workflows/missing.yml"},
	}}
	c := &edgeCollector{}
	walkRefs(root, nil, nil, c.ensure, c.addEdge, inv)
	const cliRel = "internal/cli/dispatch.go"
	if !c.has("cmd:deploy", "pkg:internal/cli", SourceCLIRoute, cliRel, 3) {
		t.Fatalf("missing cli-route edge: %+v", c.calls)
	}
	if !c.has("pkg:internal/cli", "pkg:internal/cmdx", SourceCLIRoute, cliRel, 4) {
		t.Fatalf("missing forwarder edge: %+v", c.calls)
	}
	const wfRel = ".github/workflows/ci.yml"
	if !c.has("wf:"+wfRel, "pkg:internal/gates", SourceWorkflow, wfRel, 1) {
		t.Fatalf("missing workflow-ref pkg edge: %+v", c.calls)
	}
	if !c.has("wf:"+wfRel, "cmd:deploy", SourceWorkflow, wfRel, 2) {
		t.Fatalf("missing workflow-ref cmd edge: %+v", c.calls)
	}
	if !c.has("cfg:regulus.json", "pkg:internal/evidence", SourceConfig, "regulus.json", 1) {
		t.Fatalf("missing config-ref edge: %+v", c.calls)
	}
	for _, e := range c.calls {
		if e.file == ".github/workflows/missing.yml" {
			t.Fatalf("missing file must be skipped silently: %+v", c.calls)
		}
	}
}

// TestBuildGraphWalkRefs covers walkRefs through the public entry:
// dispatch + forwarder + workflow/pkg/cmd edges land in the graph
// with evidence stamps.
func TestBuildGraphWalkRefs(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module regulus-ai\n\ngo 1.27\n")
	writeFixture(t, root, "internal/cli/dispatch.go",
		"package cli\n\nvar _ = cliDispatch[\"deploy\"]\n\nfunc fwd() { _ = cmdx.New(nil) }\n")
	writeFixture(t, root, ".github/workflows/ci.yml",
		"jobs:\n  x:\n    steps:\n      - run: echo internal/gates\n      - run: regulus deploy\n")
	g, err := BuildGraph(root, nil, func(rel string) (string, string, error) {
		return "abc123", "2026-01-01T00:00:00Z", nil
	})
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	has := func(from, to, source string) bool {
		for _, e := range g.Edges {
			if e.From == from && e.To == to && e.Source == source {
				return true
			}
		}
		return false
	}
	for _, want := range [][3]string{
		{"cmd:deploy", "pkg:internal/cli", SourceCLIRoute},
		{"pkg:internal/cli", "pkg:internal/cmdx", SourceCLIRoute},
		{"wf:.github/workflows/ci.yml", "pkg:internal/gates", SourceWorkflow},
		{"wf:.github/workflows/ci.yml", "cmd:deploy", SourceWorkflow},
	} {
		if !has(want[0], want[1], want[2]) {
			t.Fatalf("missing edge %v in %+v", want, g.Edges)
		}
	}
	for _, e := range g.Edges {
		if e.Commit == "" || e.Date == "" {
			t.Fatalf("edge without stamp: %+v", e)
		}
	}
}
