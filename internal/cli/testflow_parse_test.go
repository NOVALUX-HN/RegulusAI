// Parse paydown for testflowCmd: focused coverage for the extracted
// parse/print helpers. Messages and exit codes stay byte-identical;
// these tests pin the helper contracts so the orchestrator stays under
// the standard CRAP ceiling. Portable: no shell, only t.TempDir.
package cli

import (
	"bytes"
	"strings"
	"testing"

	"regulus-ai/internal/testflow"
)

func TestParseTestflowArgsHelp(t *testing.T) {
	for _, h := range []string{"--help", "-h"} {
		var out, errb bytes.Buffer
		_, _, code, done := parseTestflowArgs([]string{h}, &out, &errb, "test")
		if !done || code != 0 {
			t.Fatalf("%s = (done=%v, code=%d), want (true, 0)", h, done, code)
		}
		if !strings.Contains(out.String(), "usage: regulus testflow run") {
			t.Fatalf("%s must print usage to stdout, got %q", h, out.String())
		}
		if errb.Len() != 0 {
			t.Fatalf("%s must not write stderr, got %q", h, errb.String())
		}
	}
}

func TestParseTestflowArgsRequiresRun(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"status"}} {
		var out, errb bytes.Buffer
		_, _, code, done := parseTestflowArgs(args, &out, &errb, "test")
		if !done || code != 2 {
			t.Fatalf("args=%v = (done=%v, code=%d), want (true, 2)", args, done, code)
		}
		if !strings.Contains(errb.String(), "usage: regulus testflow run") {
			t.Fatalf("args=%v must print usage to stderr, got %q", args, errb.String())
		}
	}
}

func TestParseTestflowArgsRejectsTimeout(t *testing.T) {
	var out, errb bytes.Buffer
	_, _, code, done := parseTestflowArgs([]string{"run", "--timeout-s", "5"}, &out, &errb, "test")
	if !done || code != 2 {
		t.Fatalf("timeout-s=5 = (done=%v, code=%d), want (true, 2)", done, code)
	}
	if !strings.Contains(errb.String(), "usage: regulus testflow run") {
		t.Fatalf("timeout must print usage to stderr, got %q", errb.String())
	}
}

func TestParseTestflowArgsRejectsPositionals(t *testing.T) {
	var out, errb bytes.Buffer
	if _, _, code, done := parseTestflowArgs([]string{"run", "extra"}, &out, &errb, "test"); !done || code != 2 {
		t.Fatalf("positional = (done=%v, code=%d), want (true, 2)", done, code)
	}
}

func TestParseTestflowArgsOK(t *testing.T) {
	var out, errb bytes.Buffer
	req, asJSON, code, done := parseTestflowArgs(
		[]string{"run", "--mode", "focused", "--packages", "a,b", "--skip", "c", "--json"},
		&out, &errb, "v1")
	if done || code != 0 {
		t.Fatalf("ok = (done=%v, code=%d), want (false, 0)", done, code)
	}
	if !asJSON {
		t.Fatal("json flag must propagate")
	}
	if req.Mode != "focused" || req.Regulus != "v1" {
		t.Fatalf("req = %+v, want mode=focused regulus=v1", req)
	}
	if len(req.Packages) != 2 || req.Packages[0] != "a" || req.Packages[1] != "b" {
		t.Fatalf("packages = %v, want [a b]", req.Packages)
	}
	if len(req.Skipped) != 1 || req.Skipped[0] != "c" {
		t.Fatalf("skipped = %v, want [c]", req.Skipped)
	}
}

func TestSplitCSVCases(t *testing.T) {
	if got := splitCSV(""); len(got) != 0 {
		t.Fatalf("empty = %v, want []", got)
	}
	got := splitCSV("a,b , c")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("csv = %v, want [a b c]", got)
	}
	got = splitCSV(" a ,, b ")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("sparse = %v, want [a b]", got)
	}
}

func TestPrintTestflowReportText(t *testing.T) {
	var out, errb bytes.Buffer
	rep := testflow.Report{Mode: "smoke", Discovered: []string{"p"},
		Results: []testflow.PackageResult{{Package: "p", Outcome: "PASS", Detail: "ok"}}}
	if rc := printTestflowReport(&out, &errb, rep, false); rc != 0 {
		t.Fatalf("text rc = %d, want 0", rc)
	}
	if !strings.Contains(out.String(), "testflow smoke N=1") {
		t.Fatalf("text must quote the header, got %q", out.String())
	}
}

func TestPrintTestflowReportJSON(t *testing.T) {
	var out, errb bytes.Buffer
	rep := testflow.Report{Mode: "smoke", Discovered: []string{"p"}}
	if rc := printTestflowReport(&out, &errb, rep, true); rc != 0 {
		t.Fatalf("json rc = %d, want 0", rc)
	}
	if !strings.Contains(out.String(), `"Mode": "smoke"`) {
		t.Fatalf("json must carry the mode, got %q", out.String())
	}
}
