package cli

// T2 (pro-harness, ponytail-audit): `regulus audit` maps the repo first
// (entry points under cmd/) then ranks what to fix first over the CRAP
// report — must-fix over the gate max, should-fix for untested
// non-trivial logic, nice for the rest. The fixture module below is
// fixed (line numbers feed the cover profile), so the ranking is
// golden: MustFix (cx8, uncovered) first, Workhorse (cx5, uncovered)
// second, NiceHelper (cx2, covered) last.
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const auditFixtureMod = "module fixture.example/audit\n\ngo 1.24\n"

const auditFixtureRisky = `package audit

func MustFix(x int) int {
	if x > 0 {
		if x > 1 {
			if x > 2 {
				if x > 3 {
					if x > 4 {
						if x > 5 {
							if x > 6 {
								return 7
							}
							return 6
						}
						return 5
					}
					return 4
				}
				return 3
			}
			return 2
		}
		return 1
	}
	return 0
}

func Workhorse(x int) int {
	s := 0
	if x > 0 {
		s++
	}
	if x > 1 {
		s += 2
	}
	if x > 2 {
		s += 4
	}
	if x > 3 {
		s += 8
	}
	return s
}

func NiceHelper(x int) int {
	if x > 0 {
		return 1
	}
	return 0
}
`

const auditFixtureMain = `package main

func main() {}
`

const auditFixtureProfile = `mode: set
fixture.example/audit/risky.go:3.24,26.2 8 0
fixture.example/audit/risky.go:28.24,43.2 5 0
fixture.example/audit/risky.go:45.24,50.2 2 1
`

func writeAuditFixture(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"go.mod":             auditFixtureMod,
		"risky.go":           auditFixtureRisky,
		"profile.out":        auditFixtureProfile,
		"cmd/server/main.go": auditFixtureMain,
	}
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAuditUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"audit"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus audit --profile") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestAuditMissingProfile(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"audit", "--profile", "nope.out"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if errb.String() == "" {
		t.Fatal("missing profile must explain itself on stderr")
	}
}

func TestAuditRanksFixture(t *testing.T) {
	dir := t.TempDir()
	writeAuditFixture(t, dir)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"audit", "--profile", "profile.out"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{
		"Entry points:", "cmd/server",
		"must-fix", "MustFix",
		"should-fix", "Workhorse",
		"nice", "NiceHelper",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("audit output must contain %q, got:\n%s", want, got)
		}
	}
	must := strings.Index(got, "MustFix")
	should := strings.Index(got, "Workhorse")
	nice := strings.Index(got, "NiceHelper")
	if must < 0 || should < 0 || nice < 0 || !(must < should && should < nice) {
		t.Fatalf("ranking order broke (must < should < nice), got:\n%s", got)
	}
	if !strings.Contains(got, "fix MustFix first") {
		t.Fatalf("verdict must name the top item first, got:\n%s", got)
	}
}

func TestAuditNoEntryPoints(t *testing.T) {
	dir := t.TempDir()
	writeAuditFixture(t, dir)
	if err := os.RemoveAll(filepath.Join(dir, "cmd")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"audit", "--profile", "profile.out"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(none found)") {
		t.Fatalf("audit without cmd/ must say so, got:\n%s", out.String())
	}
}
