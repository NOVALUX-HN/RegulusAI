package cli

// T2 (pro-harness, ponytail-audit): `regulus audit` maps the repo first
// (entry points under cmd/) then ranks what to fix first over the CRAP
// report — must-fix over the gate max, should-fix for untested
// non-trivial logic, nice for the rest. The fixture module below is
// fixed (line numbers feed the cover profile), so the ranking is
// golden: MustFix (cx8, uncovered) first, Workhorse (cx5, uncovered)
// second, NiceHelper (cx2, covered) last.
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/crap"
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

func TestAuditCriticalThresholdDistinguishes(t *testing.T) {
	crit := crap.Scored{File: "internal/gates/run.go", Name: "Run", Complexity: 6, Coverage: 0, Measured: true, CRAP: 40}
	std := crap.Scored{File: "pkg/util/x.go", Name: "Help", Complexity: 6, Coverage: 0, Measured: true, CRAP: 40}
	if got := auditTierFor(crit, 30, 50, 100); got != "must-fix" {
		t.Fatalf("critical CRAP 40 with max-critical 30 = %q, want must-fix", got)
	}
	if got := auditTierFor(std, 30, 50, 100); got == "must-fix" {
		t.Fatalf("standard CRAP 40 with max-standard 50 = %q, want non-must-fix", got)
	}
}

func TestAuditHotspotReportHasSourceAndDate(t *testing.T) {
	dir := t.TempDir()
	writeAuditFixture(t, dir)
	t.Chdir(dir)
	hot := filepath.Join(dir, "hot.json")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"audit", "--profile", "profile.out", "--hotspot-report", hot}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	raw, err := os.ReadFile(hot)
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		GeneratedAt   string `json:"generated_at"`
		Profile       string `json:"profile"`
		ProfileSHA256 string `json:"profile_sha256"`
		Top           int    `json:"top"`
		Functions     []struct {
			File     string   `json:"file"`
			Func     string   `json:"func"`
			CX       int      `json:"cx"`
			Coverage *float64 `json:"coverage"`
			CRAP     float64  `json:"crap"`
			Tier     string   `json:"tier"`
		} `json:"functions"`
	}
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.GeneratedAt == "" || rep.Profile == "" || len(rep.ProfileSHA256) != 64 {
		t.Fatalf("hotspot must carry fuente+fecha, got %+v", rep)
	}
	if _, err := time.Parse(time.RFC3339, rep.GeneratedAt); err != nil {
		t.Fatalf("generated_at must be RFC3339, got %q", rep.GeneratedAt)
	}
	if len(rep.Functions) == 0 || rep.Functions[0].Tier == "" {
		t.Fatalf("hotspot must rank functions with tier, got %+v", rep.Functions)
	}
	if got := out.String(); !strings.Contains(got, "profile.out") || !strings.Contains(got, "sha256:") {
		t.Fatalf("stdout must print profile fuente (path+sha), got:\n%s", got)
	}
}

func TestAuditTopOneStopsEarly(t *testing.T) {
	dir := t.TempDir()
	writeAuditFixture(t, dir)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"audit", "--profile", "profile.out", "--top", "1"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "MustFix") {
		t.Fatalf("top-1 must rank the worst first, got:\n%s", got)
	}
	if strings.Contains(got, "Workhorse") {
		t.Fatalf("top-1 must stop after the first finding, got:\n%s", got)
	}
}

func TestAuditWaiverDiscountsRanking(t *testing.T) {
	dir := t.TempDir()
	writeAuditFixture(t, dir)
	exc := filepath.Join(dir, "exceptions.md")
	if err := os.WriteFile(exc, []byte("risky.go:MustFix legacy hotspot expira:2027-01-01 aprueba:tech-lead\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"audit", "--profile", "profile.out", "--exceptions", exc}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "waived") {
		t.Fatalf("waived function must be marked, got:\n%s", got)
	}
	if !strings.Contains(got, "fix Workhorse first") {
		t.Fatalf("verdict must skip the waived top, got:\n%s", got)
	}
}
