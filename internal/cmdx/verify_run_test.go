// Verification run CRAP paydown (VerifyRunCmd split): the --changed
// --no-toolchain --out evidence chain, the flag refusal table, and
// the extracted outcome helpers. Portable: IO{Dir:tmp} with a stub
// runner (no git, no shell) and local evidence only.
package cmdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/verify"
)

func TestVerifyRunOutSealsBlockedEvidence(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	planRepo(t, dir)
	runOut := filepath.Join(dir, "run-evidence")
	// A host without a toolchain blocks every gate instead of
	// inventing passes: the report seals BLOCKED and exits 1.
	if code := io.VerifyRunCmd([]string{"--changed", "internal/demo/demo.go", "--no-toolchain", "--out", runOut}); code != 1 {
		t.Fatalf("no-toolchain run exit = %d, want 1 stderr = %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(runOut, "manifest.json")); err != nil {
		t.Fatalf("blocked run must still seal evidence: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("blocked run must still render the report")
	}
	// Mission scope builds the same way: the plan carries the
	// mission id and the missing toolchain still blocks.
	out.Reset()
	errb.Reset()
	if code := io.VerifyRunCmd([]string{"--changed", "internal/demo/demo.go", "--mission", "m1", "--no-toolchain"}); code != 1 {
		t.Fatalf("mission run exit = %d, want 1 stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "m1") {
		t.Fatalf("mission run must name the mission:\n%s", out.String())
	}
}

func TestVerifyRunFlagRefusalsTable(t *testing.T) {
	dir := t.TempDir()
	planRepo(t, dir)
	badResults := filepath.Join(dir, "bad-results.json")
	if err := os.WriteFile(badResults, []byte("{no json"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		want int
		msg  string
	}{
		{name: "sin args", args: nil, want: 2, msg: "usage:"},
		{name: "flag desconocido", args: []string{"--bogus"}, want: 2, msg: "flag provided but not defined"},
		{name: "sin changed", args: []string{"--task", "t1"}, want: 2, msg: "usage:"},
		{name: "severity fuera", args: []string{"--changed", "internal/demo/demo.go", "--severity", "2"}, want: 2, msg: "severity"},
		{name: "gate desconocido", args: []string{"--changed", "internal/demo/demo.go", "--fail", "bogus:x"}, want: 2, msg: "verify run:"},
		{name: "results roto", args: []string{"--changed", "internal/demo/demo.go", "--results", badResults}, want: 1, msg: "verify run:"},
		{name: "results ausente", args: []string{"--changed", "internal/demo/demo.go", "--results", filepath.Join(dir, "no-existe.json")}, want: 1, msg: "verify run:"},
		{name: "baseline sin window", args: []string{"--changed", "internal/demo/demo.go", "--baseline", "base"}, want: 1, msg: "travel together"},
		{name: "window sin baseline", args: []string{"--changed", "internal/demo/demo.go", "--window", "win"}, want: 1, msg: "travel together"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			io, out, errb := cmdIO(dir)
			if code := io.VerifyRunCmd(c.args); code != c.want {
				t.Fatalf("run %s exit = %d, want %d (out=%q err=%q)", c.name, code, c.want, out.String(), errb.String())
			}
			if !strings.Contains(out.String()+errb.String(), c.msg) {
				t.Fatalf("run %s must mention %q, got out=%q err=%q", c.name, c.msg, out.String(), errb.String())
			}
		})
	}
}

func TestVerifyRunJSONRendersBlockedResult(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	planRepo(t, dir)
	if code := io.VerifyRunCmd([]string{"--changed", "internal/demo/demo.go", "--no-toolchain", "--json"}); code != 1 {
		t.Fatalf("json run exit = %d, want 1 stderr = %q", code, errb.String())
	}
	var res struct {
		Gates []struct {
			Kind    string `json:"kind"`
			Verdict string `json:"verdict"`
		} `json:"gates"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || len(res.Gates) == 0 {
		t.Fatalf("json run must decode a gated result: %v\n%s", err, out.String())
	}
	for _, g := range res.Gates {
		if g.Verdict != verify.VerdictBlocked {
			t.Fatalf("gate %s verdict = %q, want BLOCKED (no toolchain)", g.Kind, g.Verdict)
		}
	}
}

func TestVerifyRunRecordedHelpers(t *testing.T) {
	m, err := verifyRunFlagOutcomes(verifyOutcomeFlags{fail: "unit:roto", skip: "static:pendiente"})
	if err != nil {
		t.Fatal(err)
	}
	if m["unit"].Verdict != verify.VerdictFail || m["unit"].Cause != "roto" {
		t.Fatalf("fail flag must survive: %+v", m["unit"])
	}
	if m["static"].Verdict != verify.VerdictSkipped {
		t.Fatalf("skip flag must survive: %+v", m["static"])
	}
	if _, err := verifyRunFlagOutcomes(verifyOutcomeFlags{fail: "bogus:x"}); err == nil {
		t.Fatal("unknown gate must fail")
	}
	// The results file fills gaps without clobbering flags.
	results := filepath.Join(t.TempDir(), "results.json")
	raw, _ := json.Marshal([]map[string]string{
		{"kind": "unit", "verdict": "FAIL", "cause": "desde fichero"},
		{"kind": "static", "verdict": "SKIPPED", "cause": "desde fichero"},
	})
	if err := os.WriteFile(results, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := verifyRunFlagOutcomes(verifyOutcomeFlags{fail: "unit:desde flag"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mergeVerifyRunFileOutcomes(rec, results); err != nil {
		t.Fatal(err)
	}
	if rec["unit"].Cause != "desde flag" {
		t.Fatalf("flag must win over file: %+v", rec["unit"])
	}
	if rec["static"].Cause != "desde fichero" {
		t.Fatalf("file must fill the gap: %+v", rec["static"])
	}
	if err := mergeVerifyRunFileOutcomes(rec, ""); err != nil {
		t.Fatalf("empty path must be a no-op: %v", err)
	}
	if err := mergeVerifyRunFileOutcomes(rec, filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing results file must fail")
	}
	// The loader maps flag errors to 2 and file errors to 1.
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	if _, code, ok := io.loadVerifyRunRecorded(verifyOutcomeFlags{fail: "bogus:x"}); ok || code != 2 {
		t.Fatalf("bad flag must refuse with 2, got %d %v", code, ok)
	}
	if _, code, ok := io.loadVerifyRunRecorded(verifyOutcomeFlags{results: filepath.Join(dir, "missing.json")}); ok || code != 1 {
		t.Fatalf("missing file must refuse with 1, got %d %v", code, ok)
	}
	if _, code, ok := io.loadVerifyRunRecorded(verifyOutcomeFlags{}); !ok || code != 0 {
		t.Fatalf("empty outcomes must load clean, got %d %v", code, ok)
	}
	// Flag parsing refuses before any store work.
	if _, _, code, ok := io.parseVerifyRunArgs(nil); ok || code != 2 {
		t.Fatalf("missing --changed must refuse with 2, got %d %v", code, ok)
	}
	if _, _, code, ok := io.parseVerifyRunArgs([]string{"--changed", "x", "--severity", "9"}); ok || code != 2 {
		t.Fatalf("bad severity must refuse with 2, got %d %v", code, ok)
	}
	f, o, code, ok := io.parseVerifyRunArgs([]string{"--changed", "a.go", "--no-toolchain"})
	if !ok || code != 0 || f.changed != "a.go" || !o.noToolchain {
		t.Fatalf("valid args must parse: %+v %+v %d %v", f, o, code, ok)
	}
}
