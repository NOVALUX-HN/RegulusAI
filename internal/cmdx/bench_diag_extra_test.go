// Diag split cover (CRAP paydown sin cambiar comportamiento):
// 7 casos parse/thresholds/evidence/ids/vacio/Diagnose/json,
// con IO{Dir:tmp}, portables.
package cmdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/observability"
)

func seedDiagRun(t *testing.T, dir, run string) {
	t.Helper()
	now := time.Now().UTC()
	if _, _, err := observability.RecordExec(dir, observability.ExecEvent{
		RunID: run, Type: observability.ExecRunStart, Status: observability.ExecStarted,
	}, now); err != nil {
		t.Fatalf("seed start: %v", err)
	}
	if _, _, err := observability.RecordExec(dir, observability.ExecEvent{
		RunID: run, Type: observability.ExecRunEnd, Status: observability.ExecSucceeded,
	}, now); err != nil {
		t.Fatalf("seed end: %v", err)
	}
}

func TestResolveDiagThresholdsTable(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		var errb strings.Builder
		th, code, ok := resolveDiagThresholds(benchDiagOptions{}, &errb)
		if !ok || code != 0 || th != bench.DefaultDiagThresholds() {
			t.Fatalf("empty opts must keep defaults, got %+v %d %v", th, code, ok)
		}
	})
	t.Run("bad file", func(t *testing.T) {
		dir := t.TempDir()
		bad := filepath.Join(dir, "th.json")
		if err := os.WriteFile(bad, []byte("{oops"), 0o644); err != nil {
			t.Fatal(err)
		}
		var errb strings.Builder
		_, code, ok := resolveDiagThresholds(benchDiagOptions{thresholds: bad}, &errb)
		if ok || code != 1 {
			t.Fatalf("exit = %d ok = %v, want 1 false", code, ok)
		}
		if !strings.Contains(errb.String(), "bench diag:") {
			t.Fatalf("must name bench diag, got %q", errb.String())
		}
	})
	t.Run("ok file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "th.json")
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		var errb strings.Builder
		th, code, ok := resolveDiagThresholds(benchDiagOptions{thresholds: path}, &errb)
		if !ok || code != 0 || th != bench.DefaultDiagThresholds() {
			t.Fatalf("empty object must keep defaults, got %+v %d %v", th, code, ok)
		}
	})
}

func TestResolveDiagResultsTable(t *testing.T) {
	t.Run("sin evidence", func(t *testing.T) {
		var errb strings.Builder
		results, code, ok := resolveDiagResults("", &errb)
		if !ok || code != 0 || len(results) != 0 {
			t.Fatalf("empty dir must be trace-only, got %v %d %v", results, code, ok)
		}
	})
	t.Run("evidence-err", func(t *testing.T) {
		var errb strings.Builder
		_, code, ok := resolveDiagResults(filepath.Join(t.TempDir(), "missing"), &errb)
		if ok || code != 1 {
			t.Fatalf("exit = %d ok = %v, want 1 false", code, ok)
		}
		if !strings.Contains(errb.String(), "bench diag:") {
			t.Fatalf("must name bench diag, got %q", errb.String())
		}
	})
}

func TestResolveDiagIDsTable(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		var errb strings.Builder
		ids, code, ok := resolveDiagIDs([]string{"r1"}, nil, &errb)
		if !ok || code != 0 || len(ids) != 1 || ids[0] != "r1" {
			t.Fatalf("explicit runs must win, got %v %d %v", ids, code, ok)
		}
	})
	t.Run("backfill", func(t *testing.T) {
		var errb strings.Builder
		results := []bench.LabResult{{RunID: "b"}, {RunID: "a"}, {RunID: "a"}}
		ids, code, ok := resolveDiagIDs(nil, results, &errb)
		if !ok || code != 0 || len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
			t.Fatalf("must derive sorted ids, got %v %d %v", ids, code, ok)
		}
	})
	t.Run("vacio", func(t *testing.T) {
		var errb strings.Builder
		_, code, ok := resolveDiagIDs(nil, nil, &errb)
		if ok || code != 2 {
			t.Fatalf("exit = %d ok = %v, want 2 false", code, ok)
		}
		if !strings.Contains(errb.String(), "--run or --evidence-dir") {
			t.Fatalf("must name the required scope, got %q", errb.String())
		}
	})
}

func TestBenchDiagCmdTable(t *testing.T) {
	t.Run("parse", func(t *testing.T) {
		io, _, _ := cmdIO(t.TempDir())
		if code := io.BenchDiagCmd([]string{"--bogus"}); code != 2 {
			t.Fatalf("unknown flag exit = %d, want 2", code)
		}
	})
	t.Run("thresholds", func(t *testing.T) {
		dir := t.TempDir()
		bad := filepath.Join(dir, "th.json")
		if err := os.WriteFile(bad, []byte("{oops"), 0o644); err != nil {
			t.Fatal(err)
		}
		io, _, errb := cmdIO(dir)
		if code := io.BenchDiagCmd([]string{"--run", "r1", "--thresholds", bad}); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "bench diag:") {
			t.Fatalf("must name bench diag, got %q", errb.String())
		}
	})
	t.Run("evidence", func(t *testing.T) {
		io, _, errb := cmdIO(t.TempDir())
		if code := io.BenchDiagCmd([]string{"--run", "r1", "--evidence-dir", filepath.Join("no", "dir")}); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "bench diag:") {
			t.Fatalf("must name bench diag, got %q", errb.String())
		}
	})
	t.Run("ids", func(t *testing.T) {
		io, _, errb := cmdIO(t.TempDir())
		if code := io.BenchDiagCmd(nil); code != 2 {
			t.Fatalf("empty scope exit = %d, want 2", code)
		}
		if !strings.Contains(errb.String(), "--run or --evidence-dir") {
			t.Fatalf("must name the required scope, got %q", errb.String())
		}
	})
	t.Run("vacio", func(t *testing.T) {
		io, _, errb := cmdIO(t.TempDir())
		if code := io.BenchDiagCmd([]string{"--run", "ghost-run"}); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "nothing to diagnose") {
			t.Fatalf("must name nothing to diagnose, got %q", errb.String())
		}
	})
	t.Run("Diagnose", func(t *testing.T) {
		dir := t.TempDir()
		seedDiagRun(t, dir, "r1")
		io, out, _ := cmdIO(dir)
		if code := io.BenchDiagCmd([]string{"--run", "r1"}); code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out.String(), "perf-diagnosis") {
			t.Fatalf("must render the diagnosis, got %q", out.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		dir := t.TempDir()
		seedDiagRun(t, dir, "r1")
		io, out, _ := cmdIO(dir)
		if code := io.BenchDiagCmd([]string{"--run", "r1", "--json"}); code != 0 {
			t.Fatalf("exit = %d", code)
		}
		var rep bench.DiagReport
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("json must decode: %v %q", err, out.String())
		}
		if rep.Version != bench.DiagVersion {
			t.Fatalf("version = %q, want %q", rep.Version, bench.DiagVersion)
		}
	})
}

func TestRenderDiagReportTable(t *testing.T) {
	rep := bench.Diagnose(bench.DiagInput{}, bench.DefaultDiagThresholds())
	t.Run("texto", func(t *testing.T) {
		io, out, _ := cmdIO(t.TempDir())
		if code := io.renderDiagReport(rep, false); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "perf-diagnosis") {
			t.Fatalf("must render human report, got %q", out.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		io, out, _ := cmdIO(t.TempDir())
		if code := io.renderDiagReport(rep, true); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		var decoded bench.DiagReport
		if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
			t.Fatalf("json must decode: %v %q", err, out.String())
		}
	})
}
