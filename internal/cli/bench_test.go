package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

func TestBenchUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"bench"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus bench") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"bench", "frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand: exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("stderr = %q", errb2.String())
	}
}

func TestBenchList(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "list"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, want := range []string{"B1", "B7", "offline, hidden checks"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("list must show %q, got:\n%s", want, out.String())
		}
	}
	// The list never prints hidden answers.
	for _, leak := range []string{"formula holds", "byte-identical"} {
		if strings.Contains(out.String(), leak) {
			t.Fatalf("list leaks hidden check detail %q", leak)
		}
	}
}

func TestBenchListJSON(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "list", "--json"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var tasks []map[string]string
	if err := json.Unmarshal([]byte(out.String()), &tasks); err != nil {
		t.Fatalf("list --json is not JSON: %v\n%s", err, out.String())
	}
	if len(tasks) < 5 || len(tasks) > 12 {
		t.Fatalf("suite band broken: %d tasks", len(tasks))
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"bench", "list", "extra"}); code != 2 {
		t.Fatalf("stray positional: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus bench list") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestBenchRun(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "run", "--surface", "tree"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, want := range []string{"B1", "PASS", "B11", "bench: 11/11 passed"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("run must show %q, got:\n%s", want, out.String())
		}
	}
}

func TestBenchRunJSONAndSave(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	save := filepath.Join(dir, "rep.json")
	if code := c.Run([]string{"bench", "run", "--surface", "tree", "--save", save, "--json"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var rep bench.Report
	if err := json.Unmarshal([]byte(out.String()), &rep); err != nil {
		t.Fatalf("run --json is not a report: %v", err)
	}
	if rep.Passed != len(rep.Tasks) {
		t.Fatalf("report = %+v", rep)
	}
	raw, err := os.ReadFile(save)
	if err != nil {
		t.Fatalf("saved report missing: %v", err)
	}
	var saved bench.Report
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("saved report is not JSON: %v", err)
	}
	if saved.Passed != rep.Passed {
		t.Fatalf("saved report diverges from stdout")
	}
}

func TestBenchRunRejectsBadSurface(t *testing.T) {
	for _, surface := range []string{"../evil", "a/b..c", ""} {
		c, _, errb := newCLI(nil)
		if code := c.Run([]string{"bench", "run", "--surface", surface}); code != 2 {
			t.Fatalf("surface %q: exit = %d, want 2", surface, code)
		}
		if !strings.Contains(errb.String(), "usage: regulus bench run") {
			t.Fatalf("surface %q: stderr = %q", surface, errb.String())
		}
	}
}

func TestBenchBaselineSetAndShow(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "baseline", "--set"}); code != 0 {
		t.Fatalf("set: exit = %d", code)
	}
	if !strings.Contains(out.String(), "baseline recorded to bench/baseline.json") {
		t.Fatalf("set output = %q", out.String())
	}
	raw, err := os.ReadFile(filepath.Join("bench", "baseline.json"))
	if err != nil {
		t.Fatalf("baseline file missing: %v", err)
	}
	var rep bench.Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("baseline is not a report: %v", err)
	}
	if len(rep.Tasks) != 11 || rep.Passed != 11 {
		t.Fatalf("baseline = %+v", rep)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "baseline"}); code != 0 {
		t.Fatalf("show: exit = %d", code)
	}
	if !strings.Contains(out2.String(), "11/11 passed") {
		t.Fatalf("show output = %q", out2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"bench", "baseline", "--json"}); code != 0 {
		t.Fatalf("json: exit = %d", code)
	}
	var back bench.Report
	if err := json.Unmarshal([]byte(out3.String()), &back); err != nil {
		t.Fatalf("baseline --json is not a report: %v", err)
	}
	c4, _, errb := newCLI(nil)
	if code := c4.Run([]string{"bench", "baseline", "extra"}); code != 2 {
		t.Fatalf("stray positional: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus bench baseline") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestBenchBaselineMissing(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"bench", "baseline"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "bench baseline:") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func writeBenchReport(t *testing.T, path string, flipFirst bool) {
	t.Helper()
	rep := bench.Run("tree", "abc")
	if flipFirst {
		rep.Tasks[0].Pass = false
		rep.Passed--
		rep.Failed++
	}
	if err := bench.Save(path, rep); err != nil {
		t.Fatal(err)
	}
}

func TestBenchCompareAdopt(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	base, cand := filepath.Join(dir, "base.json"), filepath.Join(dir, "cand.json")
	writeBenchReport(t, base, false)
	writeBenchReport(t, cand, false)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "compare", "--base", base, "--candidate", cand}); code != 0 {
		t.Fatalf("identical green: exit = %d, want 0\n%s", code, out.String())
	}
	for _, want := range []string{"bench contrast:", "bench verdict: ADOPT"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("compare must show %q, got:\n%s", want, out.String())
		}
	}
}

func TestBenchCompareReject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	base, cand := filepath.Join(dir, "base.json"), filepath.Join(dir, "cand.json")
	writeBenchReport(t, base, false)
	writeBenchReport(t, cand, true)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "compare", "--base", base, "--candidate", cand}); code != 1 {
		t.Fatalf("regressed: exit = %d, want 1\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "bench verdict: REJECT") {
		t.Fatalf("compare must reject, got:\n%s", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "compare", "--base", base, "--candidate", cand, "--json"}); code != 1 {
		t.Fatalf("json regressed: exit = %d, want 1", code)
	}
	var cmp bench.Comparison
	if err := json.Unmarshal([]byte(out2.String()), &cmp); err != nil {
		t.Fatalf("compare --json is not a comparison: %v", err)
	}
	if cmp.Verdict != "REJECT" || cmp.Regressions != 1 {
		t.Fatalf("comparison = %+v", cmp)
	}
}

func TestBenchCompareUsageAndMissing(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"bench", "compare", "--base", "a.json"}); code != 2 {
		t.Fatalf("missing candidate: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus bench compare") {
		t.Fatalf("stderr = %q", errb.String())
	}
	dir := t.TempDir()
	t.Chdir(dir)
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"bench", "compare", "--base", "nope.json", "--candidate", "nope2.json"}); code != 1 {
		t.Fatalf("missing files: exit = %d, want 1", code)
	}
	if !strings.Contains(errb2.String(), "bench compare:") {
		t.Fatalf("stderr = %q", errb2.String())
	}
}
