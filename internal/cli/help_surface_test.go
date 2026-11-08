package cli

// Regression tests for the help-surface audit: every command family
// answers --help/-h with usage (exit 0), usage text matches the real
// dispatch (no phantom subcommands, none missing), and help never
// mutates the filesystem (chart init --help must not scaffold a file).
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTopHelpListsSurface(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"--help"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "surface propose|list|apply") {
		t.Fatalf("top help must list the surface command:\n%s", got)
	}
	if strings.Contains(got, "|library   the Almanac") {
		t.Fatalf("top help must not advertise a memory library subcommand:\n%s", got)
	}
}

func TestSurfaceHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		c, out, errb := newCLI(nil)
		if code := c.Run([]string{"surface", flag}); code != 0 {
			t.Fatalf("surface %s: exit = %d, want 0", flag, code)
		}
		if !strings.Contains(out.String(), "usage: regulus surface") {
			t.Fatalf("surface %s: stdout = %q", flag, out.String())
		}
		if strings.Contains(errb.String(), "unknown subcommand") {
			t.Fatalf("surface %s: must not say unknown: %q", flag, errb.String())
		}
	}
}

func TestChartHelp(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"chart", "--help"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage: regulus chart init") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestChartInitHelpCreatesNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, flag := range []string{"--help", "-h", "--list"} {
		c, _, _ := newCLI(nil)
		if code := c.Run([]string{"chart", "init", flag}); code != 2 {
			t.Fatalf("chart init %s: exit = %d, want 2", flag, code)
		}
		if _, err := os.Stat(filepath.Join("charts", flag+".md")); err == nil {
			t.Fatalf("chart init %s must not create charts/%s.md", flag, flag)
		}
	}
	if _, err := os.Stat("charts"); !os.IsNotExist(err) {
		t.Fatal("help invocations must not create a charts/ directory")
	}
}

func TestReviewHelp(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"review", "--help"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage: regulus review") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestMemoryHelp(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "--help"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"relate|export|import|context", "check|status|mine", "usage: regulus memory"} {
		if !strings.Contains(got, want) {
			t.Fatalf("memory --help must contain %q, got:\n%s", want, got)
		}
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"memory", "frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand: exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("unknown subcommand must still be refused: %q", errb2.String())
	}
}

func TestDashFlagsNeverCreateFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cases := [][]string{
		{"bench", "run", "--save", "--help"},
		{"evolve", "report", "--save", "--help"},
		{"evolve", "report", "--compare", "--help"},
		{"memory", "export", "--out", "--help"},
		{"init", "--harness-from", "--help"},
		{"task", "check", "--help"},
		{"spec", "check", "--help"},
		{"evidence", "show", "--help"},
		{"bench", "compare", "--base", "--help", "--candidate", "b.json"},
		{"decide", "propose", "--file", "--help"},
		{"chart", "init", "--help"},
	}
	for _, argv := range cases {
		c, _, _ := newCLI(nil)
		if code := c.Run(argv); code != 2 {
			t.Fatalf("%v: exit = %d, want 2", argv, code)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("help/flag-like args must not create files, got: %v", names)
	}
}

func TestYoloIgnoresNoExtraArgs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "on", "extra"}); code != 2 {
		t.Fatalf("yolo on extra: exit = %d, want 2", code)
	}
	if _, err := os.Stat("opencode.json"); !os.IsNotExist(err) {
		t.Fatal("yolo on extra must not create opencode.json")
	}
}

func TestFamilyHelpExitsZero(t *testing.T) {
	families := map[string]string{
		"audit":    "usage: regulus audit",
		"bench":    "usage: regulus bench",
		"budget":   "usage: regulus budget",
		"library":  "usage: regulus library",
		"evolve":   "usage: regulus evolve",
		"decide":   "usage: regulus decide",
		"evidence": "usage: regulus evidence",
		"gate":     "usage: regulus gate",
		"mcp":      "usage: regulus mcp",
		"probe":    "usage: regulus probe",
		"skills":   "usage: regulus skills",
		"spec":     "usage: regulus spec",
		"task":     "usage: regulus task",
		"trace":    "usage: regulus trace",
		"yolo":     "usage: regulus yolo",
		"init":     "usage: regulus init",
		"crap":     "usage: regulus crap",
		"reflect":  "usage: regulus reflect",
		"doctor":   "usage: regulus doctor",
		"status":   "usage: regulus status",
	}
	for family, want := range families {
		for _, flag := range []string{"--help", "-h"} {
			c, out, errb := newCLI(nil)
			if code := c.Run([]string{family, flag}); code != 0 {
				t.Fatalf("%s %s: exit = %d, want 0 (stderr: %q)", family, flag, code, errb.String())
			}
			if !strings.Contains(out.String(), want) {
				t.Fatalf("%s %s: stdout must contain %q, got %q", family, flag, want, out.String())
			}
		}
	}
}
