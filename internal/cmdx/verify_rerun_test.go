// Rerun integration tests (F13.4/NOV-107): happy rerun seals a new
// dir without touching prior evidence, plus FS/flag refusals.
// Portable: IO{Dir:tmp} with a stub runner (no git, no shell) and
// local evidence only — no network, sleeps or env reads.
package cmdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/verify"
)

func setupRerunEvidence(t *testing.T) (IO, string, string) {
	t.Helper()
	dir := t.TempDir()
	io, _, errb := cmdIO(dir)
	planRepo(t, dir)
	runOut := filepath.Join(dir, "run-evidence")
	if code := io.VerifyRunCmd([]string{"--changed", "internal/demo/demo.go", "--no-toolchain", "--out", runOut}); code != 1 {
		t.Fatalf("setup run exit = %d, want 1 stderr = %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(runOut, "manifest.json")); err != nil {
		t.Fatalf("setup must seal evidence: %v", err)
	}
	return io, dir, runOut
}

func TestVerifyRerunHappySealsNewDir(t *testing.T) {
	_, dir, runOut := setupRerunEvidence(t)
	io, out, errb := cmdIO(dir)
	rerunOut := filepath.Join(dir, "rerun-evidence")
	if code := io.VerifyRerunCmd([]string{"--dir", runOut, "--out", rerunOut}); code != 1 {
		t.Fatalf("rerun exit = %d, want 1 (UNKNOWN/BLOCKED blocks) stderr = %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(rerunOut, "manifest.json")); err != nil {
		t.Fatalf("rerun must seal new evidence: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runOut, "manifest.json")); err != nil {
		t.Fatalf("rerun must not consume prior evidence: %v", err)
	}
	if !strings.Contains(out.String(), "rerun: mismo plan") {
		t.Fatalf("rerun must compare configs, got %q", out.String())
	}
}

func TestVerifyRerunJSONAndRecordedOutcomes(t *testing.T) {
	_, dir, runOut := setupRerunEvidence(t)
	io, out, errb := cmdIO(dir)
	// Flag-recorded failure still seals new evidence.
	rerunFail := filepath.Join(dir, "rerun-fail")
	out.Reset()
	if code := io.VerifyRerunCmd([]string{"--dir", runOut, "--out", rerunFail, "--fail", "unit:roto por flag"}); code != 1 {
		t.Fatalf("rerun --fail exit = %d, want 1 stderr = %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(rerunFail, "manifest.json")); err != nil {
		t.Fatalf("failed rerun must still seal evidence: %v", err)
	}
	// Results file without clobbering flags: skipped gate seals too.
	results := filepath.Join(dir, "results.json")
	raw, _ := json.Marshal([]map[string]string{{"kind": "static", "verdict": "SKIPPED", "cause": "omitido en rerun"}})
	if err := os.WriteFile(results, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	rerunJSON := filepath.Join(dir, "rerun-json")
	out.Reset()
	errb.Reset()
	if code := io.VerifyRerunCmd([]string{"--dir", runOut, "--out", rerunJSON, "--results", results, "--json"}); code != 1 {
		t.Fatalf("rerun --results --json exit = %d, want 1 stderr = %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(rerunJSON, "manifest.json")); err != nil {
		t.Fatalf("json rerun must seal evidence: %v", err)
	}
	if !strings.Contains(out.String(), "rerun: mismo plan") {
		t.Fatalf("json rerun must still print the comparison, got %q", out.String())
	}
}

func TestVerifyRerunRefusalsTable(t *testing.T) {
	_, dir, runOut := setupRerunEvidence(t)
	badResults := filepath.Join(dir, "bad-results.json")
	if err := os.WriteFile(badResults, []byte("{no json"), 0o644); err != nil {
		t.Fatal(err)
	}
	rerunOut := filepath.Join(dir, "rerun-evidence")
	seed, _, _ := cmdIO(dir)
	if code := seed.VerifyRerunCmd([]string{"--dir", runOut, "--out", rerunOut}); code != 1 {
		t.Fatalf("first rerun exit = %d, want 1", code)
	}
	cases := []struct {
		name string
		args []string
		want int
		msg  string
	}{
		{name: "sin args", args: nil, want: 2, msg: "usage"},
		{name: "sin out", args: []string{"--dir", runOut}, want: 2, msg: "usage"},
		{name: "misma dir", args: []string{"--dir", runOut, "--out", runOut}, want: 2, msg: "must differ"},
		{name: "out ocupado", args: []string{"--dir", runOut, "--out", rerunOut}, want: 2, msg: "already holds evidence"},
		{name: "dir ausente", args: []string{"--dir", filepath.Join(dir, "missing"), "--out", filepath.Join(dir, "nuevo")}, want: 1, msg: "verify rerun:"},
		{name: "gate desconocido", args: []string{"--dir", runOut, "--out", filepath.Join(dir, "n1"), "--fail", "bogus:x"}, want: 2, msg: "verify rerun:"},
		{name: "results roto", args: []string{"--dir", runOut, "--out", filepath.Join(dir, "n2"), "--results", badResults}, want: 1, msg: "verify rerun:"},
		{name: "results ausente", args: []string{"--dir", runOut, "--out", filepath.Join(dir, "n3"), "--results", filepath.Join(dir, "no-existe.json")}, want: 1, msg: "verify rerun:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			io, out, errb := cmdIO(dir)
			if code := io.VerifyRerunCmd(c.args); code != c.want {
				t.Fatalf("rerun %s exit = %d, want %d (out=%q err=%q)", c.name, code, c.want, out.String(), errb.String())
			}
			combined := out.String() + errb.String()
			if !strings.Contains(combined, c.msg) {
				t.Fatalf("rerun %s must mention %q, got %q", c.name, c.msg, combined)
			}
		})
	}
}

func TestRerunFlagAndFileHelpers(t *testing.T) {
	m, err := rerunFlagOutcomes(rerunRequest{fail: "unit:roto", skip: "static:pendiente"})
	if err != nil {
		t.Fatal(err)
	}
	if m["unit"].Verdict != verify.VerdictFail || m["unit"].Cause != "roto" {
		t.Fatalf("fail flag must survive: %+v", m["unit"])
	}
	if m["static"].Verdict != verify.VerdictSkipped {
		t.Fatalf("skip flag must survive: %+v", m["static"])
	}
	if _, err := rerunFlagOutcomes(rerunRequest{fail: "bogus:x"}); err == nil {
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
	rec, err := rerunFlagOutcomes(rerunRequest{fail: "unit:desde flag"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mergeRerunFileOutcomes(rec, results); err != nil {
		t.Fatal(err)
	}
	if rec["unit"].Cause != "desde flag" {
		t.Fatalf("flag must win over file: %+v", rec["unit"])
	}
	if rec["static"].Cause != "desde fichero" {
		t.Fatalf("file must fill the gap: %+v", rec["static"])
	}
	if err := mergeRerunFileOutcomes(rec, ""); err != nil {
		t.Fatalf("empty path must be a no-op: %v", err)
	}
	if err := mergeRerunFileOutcomes(rec, filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing results file must fail")
	}
}
