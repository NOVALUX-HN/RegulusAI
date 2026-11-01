package portability

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("package.json not found from " + dir)
		}
		dir = parent
	}
}

func readScript(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "scripts", name))
	if err != nil {
		t.Fatalf("read scripts/%s: %v", name, err)
	}
	return string(data)
}

// G5: both bash entrypoints must resolve the version without node:
// explicit arg / node win when present, pure grep/sed otherwise,
// loud failure only when no version exists anywhere.
func TestScriptsDeclareNodeFallback(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{"build.bash", "install-devin.bash"} {
		t.Run(name, func(t *testing.T) {
			body := readScript(t, root, name)
			for _, want := range []string{
				"pkg_version_fallback()",
				"command -v node",
				`grep -o '"version"`,
				"head -n 1",
				"sed 's/.*\"version\"",
			} {
				if !strings.Contains(body, want) {
					t.Fatalf("scripts/%s must contain %q for the no-node fallback", name, want)
				}
			}
			if strings.Contains(body, "node not found on PATH") {
				t.Fatalf("scripts/%s must not hard-fail when node is missing", name)
			}
		})
	}
	if body := readScript(t, root, "build.bash"); strings.Contains(body, `version="${1:-$(node`) {
		t.Fatal("scripts/build.bash must not resolve the version through node unconditionally")
	}
}

// G5: the happy path is unchanged — explicit arg first, node when
// present, fallback otherwise, loud failure on empty version.
func TestScriptsKeepHappyPath(t *testing.T) {
	root := repoRoot(t)
	build := readScript(t, root, "build.bash")
	for _, want := range []string{`if [ "${1:-}" != "" ]`, `version="$1"`, "node -p"} {
		if !strings.Contains(build, want) {
			t.Fatalf("scripts/build.bash must keep happy-path marker %q", want)
		}
	}
	if !strings.Contains(build, "could not resolve a version from package.json") {
		t.Fatal("scripts/build.bash must fail loud when no version resolves")
	}
	install := readScript(t, root, "install-devin.bash")
	for _, want := range []string{"node -p", `fail "could not read a version from package.json."`} {
		if !strings.Contains(install, want) {
			t.Fatalf("scripts/install-devin.bash must keep marker %q", want)
		}
	}
}

// G5: PowerShell parity — the .ps1 entrypoints already resolve the
// version natively (ConvertFrom-Json), so no node fallback applies.
func TestPowerShellNeedsNoNode(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{"build.ps1", "install-devin.ps1"} {
		body := readScript(t, root, name)
		if strings.Contains(body, "node -p") || strings.Contains(body, "node.exe") {
			t.Fatalf("scripts/%s must not depend on node", name)
		}
		if !strings.Contains(body, "ConvertFrom-Json") {
			t.Fatalf("scripts/%s must resolve the version via ConvertFrom-Json", name)
		}
	}
}

func TestFallbackVersionTable(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"compact", `{"version":"0.10.0"}`, "0.10.0"},
		{"spaced", "{  \"version\"  :  \"1.2.3\"  }", "1.2.3"},
		{"multiline", "{\n  \"name\": \"x\",\n  \"version\": \"2.0.0-rc.1\"\n}", "2.0.0-rc.1"},
		{"tabbed", "{\n\t\"version\":\t\"3.1.0\"\n}", "3.1.0"},
		{"first wins", `{"version":"9.9.9","devDependencies":{"x":{"version":"1.0.0"}}}`, "9.9.9"},
		{"missing", `{"name":"x"}`, ""},
		{"empty", ``, ""},
		{"malformed", `{oops`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FallbackVersion([]byte(tc.body)); got != tc.want {
				t.Fatalf("FallbackVersion(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// G5: the mirror agrees with strict JSON on the real package.json.
func TestFallbackMatchesPackageJSON(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Version == "" {
		t.Fatalf("package.json must carry a version: %v", err)
	}
	if got := FallbackVersion(data); got != pkg.Version {
		t.Fatalf("FallbackVersion(package.json) = %q, want %q", got, pkg.Version)
	}
}

// extractFallback returns the shipped pkg_version_fallback body so the
// functional probe runs the script's own code, not a copy.
func extractFallback(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, "pkg_version_fallback() {")
	if start < 0 {
		t.Fatal("pkg_version_fallback not found")
	}
	rest := body[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatal("pkg_version_fallback closing brace not found")
	}
	return rest[:end+3]
}

// bashPosixPath translates a Windows path to the /mnt/<drive> form WSL
// bash understands; elsewhere it returns p unchanged.
func bashPosixPath(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}
	if len(p) >= 2 && p[1] == ':' {
		rest := strings.ReplaceAll(p[2:], `\`, "/")
		return "/mnt/" + strings.ToLower(string(p[0])) + rest
	}
	return strings.ReplaceAll(p, `\`, "/")
}

// hiddenNodeEnv keeps the host environment but swaps PATH for a minimal
// POSIX one: node (including Windows interop node.exe under WSL) is
// hidden while grep/head/sed stay available.
func hiddenNodeEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); !strings.EqualFold(k, "PATH") {
			out = append(out, kv)
		}
	}
	return append(out, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
}

// G5: run the shipped fallback in bash with node hidden from PATH:
// it resolves the fixture version, and empty when absent (the caller
// turns "" into the loud failure). Skips where bash is unavailable.
func TestShippedFallbackFunctional(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH — syntax (bash -n) and Go-mirror checks above still hold")
	}
	root := repoRoot(t)
	for _, tc := range []struct {
		name    string
		script  string
		fixture string
		want    string
	}{
		{"build resolves", "build.bash", `{"name":"x","version":"9.9.9-g5"}` + "\n", "9.9.9-g5"},
		{"install resolves", "install-devin.bash", "{\n  \"version\" : \"7.7.7\"\n}\n", "7.7.7"},
		{"missing resolves empty", "build.bash", `{"name":"x"}` + "\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := readScript(t, root, tc.script)
			fn := extractFallback(t, body)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tc.fixture), 0o644); err != nil {
				t.Fatal(err)
			}
			// NOTE: the probe goes through a file, not `bash -c`: the
			// WSL bash launcher pre-expands $vars/`$(..)` in -c args,
			// which would eat the very expansions under test.
			probe := fn + "\ncd '" + bashPosixPath(dir) + "'\n" +
				"command -v node >/dev/null 2>&1 && { echo 'node leaked into probe PATH'; exit 99; }\n" +
				"version=\"$(pkg_version_fallback)\"; printf '%s' \"$version\"\n"
			probePath := filepath.Join(dir, "probe.bash")
			if err := os.WriteFile(probePath, []byte(probe), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", bashPosixPath(probePath))
			cmd.Env = hiddenNodeEnv()
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("shipped fallback failed: %v (stderr %q)", err, stderr.String())
			}
			if got := string(out); got != tc.want {
				t.Fatalf("shipped fallback = %q, want %q (posixdir %s) (stderr %q)",
					got, tc.want, bashPosixPath(dir), stderr.String())
			}
		})
	}
}

// G5: both bash scripts stay parseable (POSIX hosts run the same check
// as `bash -n`; here it also guards the suite on Windows with WSL bash).
func TestBashSyntax(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	root := repoRoot(t)
	for _, name := range []string{"build.bash", "install-devin.bash"} {
		cmd := exec.Command("bash", "-n", bashPosixPath(filepath.Join(root, "scripts", name)))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bash -n scripts/%s: %v\n%s", name, err, out)
		}
	}
}
