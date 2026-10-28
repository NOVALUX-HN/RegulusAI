package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePkg(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVersionLine(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		bin     string
		wantSub string
	}{
		{"match", func(t *testing.T, dir string) { writePkg(t, dir, `{"version":"0.6.0"}`) }, "0.6.0", "matches package.json"},
		{"dev vs known", func(t *testing.T, dir string) { writePkg(t, dir, `{"version":"0.6.0"}`) }, "dev", "WARNING — unstamped dev build"},
		{"mismatch", func(t *testing.T, dir string) { writePkg(t, dir, `{"version":"0.7.0"}`) }, "0.6.0", "WARNING — binary says 0.6.0 but package.json says 0.7.0"},
		{"no package.json", func(t *testing.T, dir string) {}, "0.6.0", "no package.json — nothing to compare"},
		{"malformed package.json", func(t *testing.T, dir string) { writePkg(t, dir, `{oops`) }, "dev", "no version — nothing to compare"},
		{"no version field", func(t *testing.T, dir string) { writePkg(t, dir, `{"name":"x"}`) }, "dev", "no version — nothing to compare"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)
			got := VersionLine(dir, tc.bin)
			if !strings.Contains(got, tc.wantSub) {
				t.Fatalf("got %q, want substring %q", got, tc.wantSub)
			}
		})
	}
}
