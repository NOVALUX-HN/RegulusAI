package doctor

import (
	"os"
	"path/filepath"
	"runtime"
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

// H3 (S4): the unstamped-dev warning must name the documented,
// stamped-by-default rebuild flow — not just the script path.
func TestVersionLineDevNamesStampedRebuild(t *testing.T) {
	dir := t.TempDir()
	writePkg(t, dir, `{"version":"0.8.1"}`)
	got := VersionLine(dir, "dev")
	for _, want := range []string{"WARNING — unstamped dev build", "npm run build"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dev warning must contain %q, got %q", want, got)
		}
	}
}

func TestBuildScriptFor(t *testing.T) {
	cases := []struct {
		goos       string
		wantScript string
		wantBinary string
	}{
		{"windows", "scripts/build.ps1", "bin/regulus.exe"},
		{"linux", "scripts/build.bash", "bin/regulus"},
		{"darwin", "scripts/build.bash", "bin/regulus"},
	}
	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			script, binary := buildScriptFor(tc.goos)
			if script != tc.wantScript || binary != tc.wantBinary {
				t.Fatalf("buildScriptFor(%q) = (%q, %q), want (%q, %q)", tc.goos, script, binary, tc.wantScript, tc.wantBinary)
			}
		})
	}
}

// H3 (S3): VersionLine warnings are OS-aware — unstamped and mismatch both
// name the stamping script for the current OS via npm run build.
func TestVersionLineOSAware(t *testing.T) {
	script, binary := buildScriptFor(runtime.GOOS)
	for _, tc := range []struct {
		name string
		bin  string
		pkg  string
	}{
		{"unstamped dev", "dev", `{"version":"0.8.1"}`},
		{"mismatch", "0.6.0", `{"version":"0.7.0"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writePkg(t, dir, tc.pkg)
			got := VersionLine(dir, tc.bin)
			for _, want := range []string{"WARNING", "npm run build", script, binary} {
				if !strings.Contains(got, want) {
					t.Fatalf("VersionLine(%q) must contain %q, got %q", tc.bin, want, got)
				}
			}
		})
	}
}
