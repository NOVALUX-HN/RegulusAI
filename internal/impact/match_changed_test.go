// matchChanged tests (NOV-119 CRAP paydown): table-driven coverage of
// the diff-to-node mapping per Kind — exact cmd match, go.mod/go.sum
// dependency fan-out, dirOf vs isDirID package resolution and the
// _test.go filter that keeps only test bundles. Pure: no filesystem.
package impact

import (
	"testing"
)

func matchChangedFixture() Graph {
	return Graph{Nodes: []Node{
		{ID: "pkg:internal/a", Kind: KindPackage, Path: "internal/a"},
		{ID: "pkg:internal/b", Kind: KindPackage, Path: "internal/b"},
		{ID: "pkg:internal/c", Kind: KindPackage, Path: "internal/c"},
		{ID: "test:internal/b", Kind: KindTest, Path: "internal/b", Tests: []string{"internal/b/b_test.go"}},
		{ID: "test:internal/c", Kind: KindTest, Path: "internal/c", Tests: []string{"internal/c/c_test.go", "internal/shared/helper_test.go"}},
		{ID: "wf:.github/workflows/ci.yml", Kind: KindWorkflow, Path: ".github/workflows/ci.yml"},
		{ID: "cfg:regulus.yaml", Kind: KindConfig, Path: "regulus.yaml"},
		{ID: "api:internal/a/a.go#Hello", Kind: KindAPI, Path: "internal/a/a.go"},
		{ID: "dep:example.com/mod", Kind: KindDependency, Path: "go.mod"},
		{ID: "dep:example.com/other", Kind: KindDependency, Path: "go.mod"},
		{ID: "cmd:deploy", Kind: KindCommand, Path: "internal/cli"},
	}}
}

func TestMatchChangedTable(t *testing.T) {
	g := matchChangedFixture()
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "cmd exact", in: "cmd:deploy", want: []string{"cmd:deploy"}},
		{name: "cmd missing", in: "cmd:missing", want: nil},
		{name: "go.mod fans out to dependencies", in: "go.mod", want: []string{"dep:example.com/mod", "dep:example.com/other"}},
		{name: "go.sum fans out to dependencies", in: "go.sum", want: []string{"dep:example.com/mod", "dep:example.com/other"}},
		{name: "package file via dirOf plus api", in: "internal/a/a.go", want: []string{"api:internal/a/a.go#Hello", "pkg:internal/a"}},
		{name: "package dir via isDirID", in: "internal/a", want: []string{"pkg:internal/a"}},
		{name: "package and test share dir", in: "internal/b", want: []string{"pkg:internal/b", "test:internal/b"}},
		{name: "test file keeps only test bundle", in: "internal/b/b_test.go", want: []string{"test:internal/b"}},
		{name: "test file via tests list", in: "internal/shared/helper_test.go", want: []string{"test:internal/c"}},
		{name: "test file without bundle falls back", in: "internal/a/a_test.go", want: []string{"pkg:internal/a"}},
		{name: "workflow", in: ".github/workflows/ci.yml", want: []string{"wf:.github/workflows/ci.yml"}},
		{name: "config", in: "regulus.yaml", want: []string{"cfg:regulus.yaml"}},
		{name: "unknown", in: "internal/unknown/z.go", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchChanged(g, tc.in)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("matchChanged(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("matchChanged(%q) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}
