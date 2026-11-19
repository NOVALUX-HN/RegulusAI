// BuildPlan directo puro: Graph+changed+opts+head sin disco ni reloj.
// Tabla ok/empty/severity/override/widen/breaking con mensajes
// preservados. Portable: solo t.TempDir().
package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/impact"
)

func bptRepo(t *testing.T) string {
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
	write("internal/a/a.go", "package a\n\nfunc Hello() string { return \"hi\" }\n")
	write("internal/b/b.go", "package b\n\nimport \"example.com/demo/internal/a\"\n\nfunc Shout() string { return a.Hello() }\n")
	write("internal/b/b_test.go", "package b\n\nimport \"testing\"\n\nfunc TestShout(t *testing.T) {}\n")
	write("internal/gates/g.go", "package gates\n\nfunc Check() bool { return true }\n")
	write("internal/gates/g_test.go", "package gates\n\nimport \"testing\"\n\nfunc TestCheck(t *testing.T) {}\n")
	return dir
}

func bptGraph(t *testing.T, dir string) impact.Graph {
	t.Helper()
	g, err := impact.BuildGraph(dir, nil, fakeLookup)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	return g
}

func bptProv(run string) Provenance {
	return Provenance{
		Commit: "abc123def456", Toolchain: "go1.27",
		Host: "ci-1", Agent: "atlas", Version: "test", RunID: run,
	}
}

func TestBuildPlanTable(t *testing.T) {
	dir := bptRepo(t)
	g := bptGraph(t, dir)
	cases := []struct {
		name      string
		changed   []string
		opts      PlanOptions
		head      string
		wantErr   string
		wantRisk  string
		wantWiden bool
		wantTier  string
	}{
		{name: "ok", changed: []string{"internal/b/b.go"}, opts: PlanOptions{Provenance: bptProv("r-ok")}, head: "abc123", wantRisk: "low"},
		{name: "empty", changed: nil, opts: PlanOptions{Provenance: bptProv("r-empty")}, head: "h", wantErr: "--changed"},
		{name: "severity", changed: []string{"internal/b/b.go"}, opts: PlanOptions{Severity: 2, Provenance: bptProv("r-sev")}, head: "h", wantErr: "severity"},
		{name: "override L3", changed: []string{"internal/b/b.go"}, opts: PlanOptions{TierOverride: "L3", Provenance: bptProv("r-l3")}, head: "h", wantErr: "L3"},
		{name: "override debil con criticos", changed: []string{"internal/gates/g.go"}, opts: PlanOptions{TierOverride: "L0", Provenance: bptProv("r-weak")}, head: "h", wantErr: "foco débil"},
		{name: "widen desconocido", changed: []string{"zzz/desconocido.go"}, opts: PlanOptions{Provenance: bptProv("r-widen")}, head: "", wantWiden: true},
		{name: "breaking", changed: []string{"internal/b/b.go"}, opts: PlanOptions{Contracts: []string{"breaking:api/demo/Shout"}, Provenance: bptProv("r-breaking")}, head: "h", wantRisk: "critical", wantTier: "L2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := BuildPlan(g, tc.changed, tc.opts, tc.head)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildPlan: %v", err)
			}
			if tc.wantRisk != "" && p.RiskLevel != tc.wantRisk {
				t.Fatalf("riesgo = %s, want %s", p.RiskLevel, tc.wantRisk)
			}
			if tc.wantWiden && !p.Widened {
				t.Fatalf("widen esperado: %+v", p)
			}
			if tc.wantTier != "" && p.Tier != tc.wantTier {
				t.Fatalf("tier = %s, want %s", p.Tier, tc.wantTier)
			}
			if p.DiffHash == "" || p.ConfigID == "" {
				t.Fatalf("sin trazabilidad: %+v", p)
			}
		})
	}
}
