// Cobertura T1 (coverage-80/S1-S4): ramas puras del runner
// (classifyGoTest, Verdict, firstLine/lastLine, saveReport) más los
// caminos con subprocesos toolchain firmados (discover, runPackage,
// goVersion/gitHead, Run focused): sin shells hardcodeados, solo
// `go`/`git` vía pxexec.
package testflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const bogusPkg = "example.invalid/nonexistent-pkg-xyz"

func TestClassifyAllBranches(t *testing.T) {
	cases := []struct {
		name    string
		outcome string
		out     string
		want    string
		detail  bool // exige detail no vacío
	}{
		{"pass real", "PASS", "ok  \tregulus-ai/internal/pxexec\t1.2s", OutcomePass, true},
		{"pass vacío", "PASS", "", OutcomePass, false},
		{"sin archivos", "PASS", "?   \tregulus-ai/internal/foo\t[no test files]", OutcomeNotRun, true},
		{"sin tests que correr", "PASS", "testing: warning: no tests to run\nFAIL", OutcomeNotRun, true},
		{"no tests + ok pasa", "PASS", "ok  \tpkg\t1.0s\ntesting: warning: no tests to run", OutcomePass, true},
		{"blocked", OutcomeBlocked, "anything", OutcomeBlocked, true},
		{"skipped", OutcomeSkipped, "whatever", OutcomeSkipped, true},
		{"fail", "FAIL", "--- FAIL: TestX (0.1s)", OutcomeFail, true},
		{"fail vacío", "FAIL", "", OutcomeFail, false},
		{"not_run cae a fail", "NOT_RUN", "boom", OutcomeFail, true},
		{"outcome raro cae a fail", "BOGUS", "boom", OutcomeFail, true},
	}
	for _, c := range cases {
		got, detail := classifyGoTest(c.outcome, c.out)
		if got != c.want {
			t.Fatalf("%s: got %q, want %q", c.name, got, c.want)
		}
		if c.detail && detail == "" {
			t.Fatalf("%s: detail vacío", c.name)
		}
	}
}

func TestVerdictCombinations(t *testing.T) {
	pass := PackageResult{Package: "a", Outcome: OutcomePass}
	checkOK := PackageResult{Package: "(check) build", Outcome: OutcomePass}
	cases := []struct {
		name    string
		rep     Report
		allowed map[string]bool
		want    bool
	}{
		{"todo verde", Report{Discovered: []string{"a"}, Results: []PackageResult{pass}, Checks: []PackageResult{checkOK}}, map[string]bool{}, true},
		{"checks nil también vale", Report{Discovered: []string{"a"}, Results: []PackageResult{pass}}, map[string]bool{}, true},
		{"not_run falla", Report{Discovered: []string{"a", "b"}, Results: []PackageResult{pass, {Package: "b", Outcome: OutcomeNotRun}}, Checks: []PackageResult{checkOK}}, map[string]bool{}, false},
		{"fail permitido sigue fallando", Report{Discovered: []string{"a"}, Results: []PackageResult{{Package: "a", Outcome: OutcomeFail}}, Checks: []PackageResult{checkOK}}, map[string]bool{"a": true}, false},
		{"ausente no permitido falla", Report{Discovered: []string{"a", "b"}, Results: []PackageResult{pass}, Checks: []PackageResult{checkOK}}, map[string]bool{}, false},
		{"ausente permitido pasa", Report{Discovered: []string{"a", "b"}, Results: []PackageResult{pass}, Checks: []PackageResult{checkOK}}, map[string]bool{"b": true}, true},
		{"todo permitido sin results", Report{Discovered: []string{"a"}, Skipped: []string{"a"}}, map[string]bool{"a": true}, true},
		{"rep.Skipped no autoriza solo", Report{Discovered: []string{"a", "b"}, Results: []PackageResult{pass}, Checks: []PackageResult{checkOK}, Skipped: []string{"zzz"}}, map[string]bool{}, false},
		{"check skipped falla", Report{Discovered: []string{"a"}, Results: []PackageResult{pass}, Checks: []PackageResult{{Package: "c", Outcome: OutcomeSkipped}}}, map[string]bool{}, false},
		{"vacío falla", Report{}, map[string]bool{}, false},
	}
	for _, c := range cases {
		if got := Verdict(c.rep, c.allowed); got != c.want {
			t.Fatalf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFirstLineVectors(t *testing.T) {
	cases := map[string]string{
		"a\nb\nc":        "a",
		"  spaced  \n x": "spaced",
		"no-newline  ":   "no-newline",
		"":               "",
		"\nabc":          "",
	}
	for in, want := range cases {
		if got := firstLine(in); got != want {
			t.Fatalf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLastLinesVectors(t *testing.T) {
	if got := lastLines("a\nb\nc", 5); got != "a\nb\nc" {
		t.Fatalf("menos que n: %q", got)
	}
	if got := lastLines("a\nb\nc\nd", 2); got != "c\nd" {
		t.Fatalf("cola 2: %q", got)
	}
	if got := lastLines("a\nb\n", 5); got != "a\nb" {
		t.Fatalf("newlines finales: %q", got)
	}
	if got := lastLines("", 3); got != "" {
		t.Fatalf("vacío: %q", got)
	}
	if got := lastLines("a", 1); got != "a" {
		t.Fatalf("una línea: %q", got)
	}
	if got := lastLines("a\nb\nc", 0); got != "" {
		t.Fatalf("n=0: %q", got)
	}
	var b strings.Builder
	for i := 0; i < 25; i++ {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("l" + strings.Repeat("0", i))
	}
	got := lastLines(b.String(), 20)
	lines := strings.Split(got, "\n")
	if len(lines) != 20 || !strings.HasPrefix(lines[0], "l") || lines[0] == "l" {
		t.Fatalf("cola 20 de 25: %d líneas, primera %q", len(lines), lines[0])
	}
}

func TestSaveReportRoundTrip(t *testing.T) {
	rep := Report{Version: ReportVersion, Mode: ModeFocused, Commit: "abc123", Host: "h",
		Discovered: []string{"a"}, Skipped: []string{"b"},
		Results: []PackageResult{{Package: "a", Outcome: OutcomePass, Detail: "ok", DurationMS: 7}},
		Checks:  []PackageResult{{Package: "(check) build", Outcome: OutcomePass}}}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := saveReport(path, rep); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back Report
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Version != ReportVersion || back.Mode != ModeFocused || back.Commit != "abc123" {
		t.Fatalf("cabecera corrupta: %+v", back)
	}
	if len(back.Results) != 1 || back.Results[0].Detail != "ok" || back.Results[0].DurationMS != 7 {
		t.Fatalf("results corruptos: %+v", back.Results)
	}
	if len(back.Skipped) != 1 || back.Skipped[0] != "b" || len(back.Checks) != 1 {
		t.Fatalf("skipped/checks corruptos: %+v", back)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatal("reporte debe terminar en newline")
	}
}

func TestSaveReportInvalidDir(t *testing.T) {
	// Padre inexistente: WriteFile falla y el error se reporta.
	path := filepath.Join(t.TempDir(), "no-such-dir", "report.json")
	if err := saveReport(path, Report{Version: ReportVersion}); err == nil {
		t.Fatal("guardar en dir inexistente debe fallar")
	}
}

func TestDiscoverListsPackages(t *testing.T) {
	pkgs, err := discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) == 0 {
		t.Fatal("discover vacío")
	}
	found := false
	for _, p := range pkgs {
		if p == "regulus-ai/internal/pxexec" {
			found = true
		}
	}
	if !found {
		t.Fatalf("pxexec debe estar descubierto: %v", pkgs)
	}
	if !sort.SliceIsSorted(pkgs, func(i, j int) bool { return pkgs[i] < pkgs[j] }) {
		t.Fatalf("no ordenado: %v", pkgs)
	}
}

func TestDiscoverCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := discover(ctx); err == nil {
		t.Fatal("discover cancelado debe fallar")
	}
}

func TestGoVersionAndGitHead(t *testing.T) {
	if v := goVersion(context.Background()); v == "" {
		t.Fatal("goVersion vacío")
	}
	if h := gitHead(context.Background()); h == "" {
		t.Fatal("gitHead vacío")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if v := goVersion(ctx); v != "unknown" {
		t.Fatalf("goVersion cancelado = %q, want unknown", v)
	}
	if h := gitHead(ctx); h != "unknown" {
		t.Fatalf("gitHead cancelado = %q, want unknown", h)
	}
}

func TestHostnameNonEmpty(t *testing.T) {
	if hostname() == "" {
		t.Fatal("hostname vacío")
	}
}

func TestRunPackageFailFast(t *testing.T) {
	pr := runPackage(context.Background(), bogusPkg)
	if pr.Outcome != OutcomeFail {
		t.Fatalf("paquete inexistente debe ser FAIL: %+v", pr)
	}
	if pr.Package != bogusPkg || pr.DurationMS < 0 {
		t.Fatalf("paquete/duración: %+v", pr)
	}
	if !strings.Contains(pr.Detail, "[process-fail]") {
		t.Fatalf("detail debe llevar ErrClass: %q", pr.Detail)
	}
}

func TestRunPackagePassPxexec(t *testing.T) {
	pr := runPackage(context.Background(), "regulus-ai/internal/pxexec")
	if pr.Outcome != OutcomePass || pr.Detail == "" {
		t.Fatalf("pxexec debe pasar con evidencia: %+v", pr)
	}
}

func TestRunUnknownMode(t *testing.T) {
	_, ok := Run(context.Background(), Request{Mode: "bogus"})
	if ok {
		t.Fatal("modo desconocido debe fallar")
	}
}

func TestRunFocusedEmpty(t *testing.T) {
	rep, ok := Run(context.Background(), Request{Mode: ModeFocused})
	if ok || len(rep.Results) != 0 {
		t.Fatalf("focused vacío debe fallar sin results: %+v %v", rep, ok)
	}
}

func TestRunFullDiscoveryError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rep, ok := Run(ctx, Request{Mode: ModeFull})
	if ok {
		t.Fatal("discovery cancelado debe fallar")
	}
	if len(rep.Results) != 1 || rep.Results[0].Package != "(discovery)" || rep.Results[0].Outcome != OutcomeNotRun {
		t.Fatalf("results discovery: %+v", rep.Results)
	}
	if len(rep.Checks) != 2 {
		t.Fatalf("checks: %+v", rep.Checks)
	}
}

func TestRunFocusedBogusAndSave(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	save := filepath.Join(t.TempDir(), "flow.json")
	rep, ok := Run(ctx, Request{Mode: ModeFocused,
		Packages: []string{bogusPkg, "example.allowed/skip"},
		Skipped:  []string{"example.allowed/skip"},
		Save:     save})
	if ok {
		t.Fatal("paquete inexistente debe fallar el veredicto")
	}
	if len(rep.Results) != 1 || rep.Results[0].Outcome == OutcomePass {
		t.Fatalf("results: %+v", rep.Results)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0] != "example.allowed/skip" {
		t.Fatalf("skipped: %+v", rep.Skipped)
	}
	if len(rep.Checks) != 2 {
		t.Fatalf("checks: %+v", rep.Checks)
	}
	if rep.Version != ReportVersion || rep.Mode != ModeFocused {
		t.Fatalf("cabecera: %+v", rep)
	}
	raw, err := os.ReadFile(save)
	if err != nil {
		t.Fatalf("save no escrito: %v", err)
	}
	var back Report
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Version != ReportVersion || len(back.Results) != 1 {
		t.Fatalf("save corrupto: %+v", back)
	}
}
