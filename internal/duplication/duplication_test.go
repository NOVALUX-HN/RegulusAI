package duplication

import (
	"os"
	"path/filepath"
	"testing"
)

// collectBody is a body of about 44 normalised tokens: comfortably above
// DefaultMinTokens, so a duplicate of it is a clone and not a coincidence.
const collectBody = `	total := 0
	for i := 0; i < len(xs); i++ {
		if xs[i] > 0 {
			total += xs[i]
		} else {
			total -= 1
		}
	}
	return total
`

func writeGo(t *testing.T, dir, name, src string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func analyze(t *testing.T, opts Options, files map[string]string) Report {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		writeGo(t, dir, name, src)
	}
	rep, err := Analyze(dir, opts)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return rep
}

func TestFindsAStructuralClone(t *testing.T) {
	rep := analyze(t, Options{}, map[string]string{
		"a.go": "package p\n\nfunc alpha(xs []int) int {\n" + collectBody + "}\n",
		"b.go": "package p\n\nfunc beta(xs []int) int {\n" + collectBody + "}\n",
	})
	if len(rep.Clones) != 1 {
		t.Fatalf("clones = %d, want 1: %+v", len(rep.Clones), rep.Clones)
	}
	got := rep.Clones[0]
	if got.A.Func != "alpha" || got.B.Func != "beta" {
		t.Errorf("pair = %s/%s, want alpha/beta", got.A.Func, got.B.Func)
	}
	if got.Windows < 1 {
		t.Errorf("windows = %d, want >= 1", got.Windows)
	}
}

// A clone that differs only in names and values is still a clone: that is the
// normalisation the whole report depends on.
func TestNormalisesNamesAndLiterals(t *testing.T) {
	renamed := `	total := 0
	for i := 0; i < len(values); i++ {
		if values[i] > 0 {
			total += values[i]
		} else {
			total -= 7
		}
	}
	return total
`
	rep := analyze(t, Options{}, map[string]string{
		"a.go": "package p\n\nfunc alpha(xs []int) int {\n" + collectBody + "}\n",
		"b.go": "package p\n\nfunc gamma(values []int) int {\n" + renamed + "}\n",
	})
	if len(rep.Clones) != 1 {
		t.Fatalf("clones = %d, want 1 (names and literals must normalise): %+v", len(rep.Clones), rep.Clones)
	}
}

// Different structure is not a clone, even with the same names.
func TestDifferentStructureIsNotAClone(t *testing.T) {
	rep := analyze(t, Options{}, map[string]string{
		"a.go": "package p\n\nfunc alpha(xs []int) int {\n" + collectBody + "}\n",
		"b.go": `package p

func delta(xs []int) int {
	defer func() { _ = xs }()
	switch len(xs) {
	case 0:
		return -1
	default:
		return xs[0] * 2
	}
}
`,
	})
	if len(rep.Clones) != 0 {
		t.Fatalf("clones = %d, want 0: %+v", len(rep.Clones), rep.Clones)
	}
}

// Repetition inside one function is a loop the author already wrote. Reporting it
// would bury the finding that matters.
func TestIntraFunctionRepetitionIsExcluded(t *testing.T) {
	rep := analyze(t, Options{}, map[string]string{
		"a.go": `package p

func twice(xs []int) int {
	total := 0
	for i := 0; i < len(xs); i++ {
		if xs[i] > 0 {
			total += xs[i]
		} else {
			total -= 1
		}
	}
	for i := 0; i < len(xs); i++ {
		if xs[i] > 0 {
			total += xs[i]
		} else {
			total -= 1
		}
	}
	return total
}
`,
	})
	if len(rep.Clones) != 0 {
		t.Fatalf("clones = %d, want 0 for intra-function repetition: %+v", len(rep.Clones), rep.Clones)
	}
}

// A body shorter than the window is noise, not a finding.
func TestShortBodiesAreNotReported(t *testing.T) {
	rep := analyze(t, Options{}, map[string]string{
		"a.go": "package p\n\nfunc one(xs []int) int { return len(xs) + 1 }\n",
		"b.go": "package p\n\nfunc two(xs []int) int { return len(xs) + 1 }\n",
	})
	if len(rep.Clones) != 0 {
		t.Fatalf("clones = %d, want 0 for bodies under MinTokens: %+v", len(rep.Clones), rep.Clones)
	}
}

// Tests repeat fixtures on purpose, so they are excluded unless asked for.
func TestTestsAreExcludedUnlessRequested(t *testing.T) {
	files := map[string]string{
		"a.go":      "package p\n\nfunc alpha(xs []int) int {\n" + collectBody + "}\n",
		"a_test.go": "package p\n\nfunc helper(xs []int) int {\n" + collectBody + "}\n",
	}
	if rep := analyze(t, Options{}, files); rep.Functions != 1 || len(rep.Clones) != 0 {
		t.Fatalf("without IncludeTests: functions = %d clones = %d, want 1 and 0", rep.Functions, len(rep.Clones))
	}
	if rep := analyze(t, Options{IncludeTests: true}, files); len(rep.Clones) != 1 {
		t.Fatalf("with IncludeTests: clones = %d, want 1", len(rep.Clones))
	}
}

// An unreadable file is skipped, never fatal: a report that aborts on one generated
// file is a report nobody can run in CI.
func TestUnparsableFileIsSkipped(t *testing.T) {
	rep := analyze(t, Options{}, map[string]string{
		"bad.go": "package p\n\nfunc broken( {\n",
		"a.go":   "package p\n\nfunc alpha(xs []int) int {\n" + collectBody + "}\n",
	})
	if rep.Files != 1 || rep.Functions != 1 {
		t.Fatalf("files = %d functions = %d, want 1 and 1", rep.Files, rep.Functions)
	}
}

func TestTruncationIsReported(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"a", "b", "c"} {
		files[name+".go"] = "package p\n\nfunc " + name + "f(xs []int) int {\n" + collectBody + "}\n"
	}
	rep := analyze(t, Options{Limit: 1}, files)
	if len(rep.Clones) != 1 {
		t.Fatalf("clones = %d, want 1 with Limit 1", len(rep.Clones))
	}
	if !rep.Truncated {
		t.Fatal("Truncated = false, want true: a capped report must never look complete")
	}
}

// The report is sorted, so two runs agree and a diff of two reports is meaningful.
func TestReportIsDeterministic(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"a", "b", "c", "d"} {
		files[name+".go"] = "package p\n\nfunc " + name + "f(xs []int) int {\n" + collectBody + "}\n"
	}
	first := analyze(t, Options{}, files)
	second := analyze(t, Options{}, files)
	if len(first.Clones) != len(second.Clones) {
		t.Fatalf("lengths differ: %d vs %d", len(first.Clones), len(second.Clones))
	}
	for i := range first.Clones {
		if first.Clones[i] != second.Clones[i] {
			t.Fatalf("clone %d differs between runs: %+v vs %+v", i, first.Clones[i], second.Clones[i])
		}
	}
}
