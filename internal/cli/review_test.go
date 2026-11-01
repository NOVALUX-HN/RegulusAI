package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/sextant"
)

func reviewRunner(head, status string) *fakeGateRunner {
	return &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-parse --verify HEAD":                                {stdout: head + "\n"},
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {stdout: status},
	}}
}

func TestReviewUsage(t *testing.T) {
	for _, args := range [][]string{
		{"review", "extra"},
		{"review", "--lens", "bogus"},
		{"review", "--task", "../evil"},
		{"review", "--task", "a..b"},
	} {
		c, _, errb := newCLI(nil)
		if code := c.Run(args); code != 2 {
			t.Fatalf("args %v: exit = %d, want 2", args, code)
		}
		if errb.String() == "" {
			t.Fatalf("args %v: stderr must explain the usage error", args)
		}
	}
}

func TestReviewUnknownLensNamesTheLenses(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"review", "--lens", "nope"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	for _, l := range reviewLenses {
		if !strings.Contains(errb.String(), l) {
			t.Fatalf("stderr must name lens %q, got %q", l, errb.String())
		}
	}
}

func TestReviewCleanTreeIsInformational(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	c.Runner = reviewRunner("abc123def456", "")
	if code := c.Run([]string{"review"}); code != 0 {
		t.Fatalf("exit = %d, want 0 (review never blocks)", code)
	}
	got := out.String()
	for _, want := range []string{
		"frozen head=abc123de 0 file(s)",
		"lens=reviewer",
		"INFORMATIONAL",
		"nothing blocked, nothing committed",
		"gate run",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("review must report %q, got:\n%s", want, got)
		}
	}
}

func TestReviewNoGitEscalatesRiskHonestly(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	c.Runner = &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-parse --verify HEAD":                                {code: 1},
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {code: 1},
	}}
	if code := c.Run([]string{"review"}); code != 0 {
		t.Fatalf("exit = %d, want 0 (degrades, never fails)", code)
	}
	if got := out.String(); !strings.Contains(got, "nogit") || !strings.Contains(got, "risk=high") {
		t.Fatalf("git failure must freeze as nogit with high risk, got:\n%s", got)
	}
}

func TestReviewDocsOnlyPicksSpec(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll("charts", 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When regulus runs\n- Then exit 0\n"
	if err := os.WriteFile(filepath.Join("charts", "x.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	c.Runner = reviewRunner("abc123", " M charts/x.md\x00")
	if code := c.Run([]string{"review"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"lens=spec", "docs-only", "spec OK (1 scenario(s))"} {
		if !strings.Contains(got, want) {
			t.Fatalf("docs-only review must report %q, got:\n%s", want, got)
		}
	}
}

func TestReviewSpecLensReportsViolations(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll("charts", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("charts", "bad.md"), []byte("# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	c.Runner = reviewRunner("abc123", " M charts/bad.md\x00")
	if code := c.Run([]string{"review", "--lens", "spec"}); code != 0 {
		t.Fatalf("exit = %d, want 0 (violations inform, never block)", code)
	}
	if got := out.String(); !strings.Contains(got, "violation(s)") {
		t.Fatalf("spec lens must list violations, got:\n%s", got)
	}
}

func TestReviewGoChangeGetsReviewerLens(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll("pkg", 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package pkg\n\n// TODO: polish\nfunc Calc() int {\n\treturn 1\n}\n"
	if err := os.WriteFile(filepath.Join("pkg", "calc.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	c.Runner = reviewRunner("abc123", " M pkg/calc.go\x00")
	if code := c.Run([]string{"review"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{
		"risk=medium class=STANDARD lens=reviewer",
		"markers pkg/calc.go: TODO",
		"check: pkg/calc.go has no test companion",
		"never cut: validation",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("go review must report %q, got:\n%s", want, got)
		}
	}
}

func TestReviewDeletionIsHighRisk(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	c.Runner = reviewRunner("abc123", " D pkg/old.go\x00")
	if code := c.Run([]string{"review"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "risk=high") || !strings.Contains(got, "gate run") {
		t.Fatalf("destructive change must be high risk pointing at the gauntlet, got:\n%s", got)
	}
}

func TestReviewLensOverrides(t *testing.T) {
	t.Chdir(t.TempDir())
	for lens, want := range map[string]string{
		"gate":     "would face",
		"evidence": "across tasks",
		"mutation": "efficacy floor",
		"crap":     "no coverage profile",
	} {
		c, out, _ := newCLI(nil)
		c.Runner = reviewRunner("abc123", "")
		if code := c.Run([]string{"review", "--lens", lens}); code != 0 {
			t.Fatalf("lens %s: exit = %d, want 0", lens, code)
		}
		if got := out.String(); !strings.Contains(got, "explicit --lens") || !strings.Contains(got, want) {
			t.Fatalf("lens %s must honor the override with %q, got:\n%s", lens, want, got)
		}
	}
}

func TestReviewCrapLensScoresTouchedFunctions(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll("pkg", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.mod", []byte("module fixture\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("pkg", "calc.go"), []byte("package pkg\n\nfunc Simple() int {\n\treturn 1\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".regulus", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".regulus", "coverage.out"),
		[]byte("mode: set\nfixture/pkg/calc.go:3.19,5.2 1 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	c.Runner = reviewRunner("abc123", " M pkg/calc.go\x00")
	if code := c.Run([]string{"review", "--lens", "crap"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "functions scored: 1") {
		t.Fatalf("crap lens must score the fixture, got:\n%s", got)
	}
}

func TestReviewEvidenceLensScopesTask(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	taskDir := filepath.Join(".regulus", "evidence", "feat__T9")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "rec.json"),
		[]byte(`{"id":"rec_1","task":"feat/T9","gate":"typecheck","layer":1,"command":"c","exit_code":0,"verdict":"pass","started_at":"2026-01-01T00:00:00Z","duration_ms":1}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	c.Runner = reviewRunner("abc123", "")
	if code := c.Run([]string{"review", "--task", "feat/T9", "--lens", "evidence"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "task=feat/T9") || !strings.Contains(got, "1 pass / 0 non-pass") {
		t.Fatalf("evidence lens must scope the task, got:\n%s", got)
	}
}

func TestReviewRiskWords(t *testing.T) {
	cases := map[sextant.Class]string{
		sextant.ClassTrivial:  "low",
		sextant.ClassStandard: "medium",
		sextant.ClassComplex:  "elevated",
		sextant.ClassCritical: "high",
	}
	for class, want := range cases {
		if got := reviewRisk(class); got != want {
			t.Fatalf("reviewRisk(%s) = %q, want %q", class, got, want)
		}
	}
}

func TestReviewPickLensTable(t *testing.T) {
	cases := []struct {
		name     string
		files    []string
		override string
		want     string
	}{
		{"override wins", []string{"a.go"}, "gate", "gate"},
		{"clean tree", nil, "", "reviewer"},
		{"docs only", []string{"README.md", "docs/a.md"}, "", "spec"},
		{"gate config", []string{"regulus.json"}, "", "gate"},
		{"workflows", []string{".github/workflows/g.yml"}, "", "gate"},
		{"receipts", []string{".regulus/evidence/t/g.json"}, "", "evidence"},
		{"test heavy", []string{"a_test.go", "b_test.go", "a.go"}, "", "mutation"},
		{"code default", []string{"pkg/a.go"}, "", "reviewer"},
	}
	for _, tc := range cases {
		if got, _ := reviewPickLens(".", tc.files, tc.override); got != tc.want {
			t.Fatalf("%s: pick = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestReviewSignalsTable(t *testing.T) {
	s := reviewSignals([]string{"internal/cli/auth.go", "regulus.json"}, map[string]bool{}, true)
	if !s.TouchesCode || !s.Security || !s.Delivery {
		t.Fatalf("auth+config must vote code+security+delivery, got %+v", s)
	}
	s = reviewSignals([]string{"charts/a.md"}, map[string]bool{}, true)
	if s.TouchesCode || s.UnclearRisk {
		t.Fatalf("docs-only with git must stay trivial, got %+v", s)
	}
	s = reviewSignals([]string{"pkg/a.go"}, map[string]bool{"pkg/a.go": true}, true)
	if !s.Destructive {
		t.Fatalf("deleted files must vote destructive, got %+v", s)
	}
	s = reviewSignals([]string{"openapi/spec.yaml"}, map[string]bool{}, true)
	if !s.ContractConsumed {
		t.Fatalf("api surface must vote contracts, got %+v", s)
	}
}

func TestReviewParseStatus(t *testing.T) {
	files, deleted := reviewParseStatus(" M a.go\x00D  b.go\x00R  old.go\x00new.go\x00!! ignored\x00\x00")
	if len(files) != 3 || files[0] != "a.go" || files[1] != "b.go" || files[2] != "old.go" {
		t.Fatalf("files = %v", files)
	}
	if !deleted["b.go"] || deleted["a.go"] {
		t.Fatalf("deleted = %v", deleted)
	}
}

func TestReviewShortHead(t *testing.T) {
	if got := reviewShortHead("abc123def", true); got != "head=abc123de" {
		t.Fatalf("got %q", got)
	}
	if got := reviewShortHead("", true); got != "nogit" {
		t.Fatalf("empty head must be nogit, got %q", got)
	}
	if got := reviewShortHead("abc", false); got != "nogit" {
		t.Fatalf("failed git must be nogit, got %q", got)
	}
}

func TestReviewMutationLensBranches(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Policy branch: local harness config with a mutation gate.
	cfg := `{"gates":[{"name":"typecheck","layer":1,"command":"go build ./..."},{"name":"mutation","layer":6,"command":"gremlins unleash --threshold-efficacy 70"}]}`
	if err := os.WriteFile("regulus.json", []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	// Task with a mutation receipt carrying efficacy.
	taskDir := filepath.Join(".regulus", "evidence", "feat__M1")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := `{"id":"rec_mut1","task":"feat/M1","gate":"mutation","layer":6,"command":"gremlins unleash","exit_code":0,"verdict":"pass","started_at":"2026-01-01T00:00:00Z","duration_ms":9,"metric_value":87.5,"metric_threshold":70,"metric_operator":">="}`
	if err := os.WriteFile(filepath.Join(taskDir, "mutation-1.json"), []byte(rec), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	c.Runner = reviewRunner("abc123", "")
	if code := c.Run([]string{"review", "--task", "feat/M1", "--lens", "mutation"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"policy: gremlins unleash", "rec_mut1 verdict=pass", "efficacy: 87.5"} {
		if !strings.Contains(got, want) {
			t.Fatalf("mutation lens must report %q, got:\n%s", want, got)
		}
	}
	// Evidence-error branch: unresolvable task dir degrades honestly.
	c2, out2, _ := newCLI(nil)
	reviewRunMutation(c2, "../evil")
	if !strings.Contains(out2.String(), "evidence unavailable") {
		t.Fatalf("bad task must degrade honestly, got:\n%s", out2.String())
	}
	// No-config branch: without regulus.json there is simply no policy line.
	if err := os.Remove("regulus.json"); err != nil {
		t.Fatal(err)
	}
	c3, out3, _ := newCLI(nil)
	reviewRunMutation(c3, "feat/M1")
	if strings.Contains(out3.String(), "policy:") {
		t.Fatalf("without config no policy may print, got:\n%s", out3.String())
	}
	if !strings.Contains(out3.String(), "rec_mut1") {
		t.Fatalf("receipt lookup must not depend on config, got:\n%s", out3.String())
	}
}

func TestReviewValidLens(t *testing.T) {
	for _, l := range []string{"reviewer", "crap", "mutation", "spec", "evidence", "gate"} {
		if !reviewValidLens(l) {
			t.Fatalf("lens %q must be valid", l)
		}
	}
	if reviewValidLens("gauntlet") {
		t.Fatal("gauntlet is not a review lens")
	}
}
