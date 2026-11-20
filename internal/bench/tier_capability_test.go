// Tier capability cover (CRAP paydown sin cambiar comportamiento):
// tabla pura de extensiones/scripts/default para tierCapabilityForPath.
package bench

import "testing"

func TestTierCapabilityForPathTable(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{path: "main.go", want: "go"},
		{path: "internal/cli/bench.go", want: "go"},
		{path: "app.js", want: "node"},
		{path: "app.mjs", want: "node"},
		{path: "app.ts", want: "node"},
		{path: "app.tsx", want: "node"},
		{path: "run.sh", want: "shell"},
		{path: "scripts/deploy", want: "shell"},
		{path: "scripts/build.sh", want: "shell"},
		{path: "tool.py", want: "python"},
		{path: "Makefile", want: "go"},
		{path: "unknown.txt", want: "go"},
		{path: "internal/foo/bar", want: "go"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if got := tierCapabilityForPath(c.path); got != c.want {
				t.Fatalf("tierCapabilityForPath(%q) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}
