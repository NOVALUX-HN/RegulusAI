package crap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCRAPFormula(t *testing.T) {
	cases := []struct {
		comp int
		cov  float64
		want float64
	}{
		{1, 1, 1},
		{10, 1, 10},
		{10, 0, 110},
		{4, 0.5, 6},
	}
	for _, tc := range cases {
		if got := CRAP(tc.comp, tc.cov); got != tc.want {
			t.Fatalf("CRAP(%d, %.2f) = %.4f, want %.4f", tc.comp, tc.cov, got, tc.want)
		}
	}
}

func TestParseProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cover.out")
	content := "mode: set\nfixture/pkg/calc.go:4.9,4.17 1 1\nfixture/pkg/calc.go:9.3,9.12 1 0\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	blocks, err := ParseProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %+v", blocks)
	}
	b := blocks[0]
	if b.File != "fixture/pkg/calc.go" || b.StartLine != 4 || b.EndLine != 4 || b.NumStmts != 1 || b.Hits != 1 {
		t.Fatalf("first block = %+v", b)
	}
	if blocks[1].Hits != 0 {
		t.Fatalf("second block hits = %d, want 0", blocks[1].Hits)
	}
}

func TestParseProfileRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.out")
	if err := os.WriteFile(bad, []byte("this is not a profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseProfile(bad); err == nil || !strings.Contains(err.Error(), "not a go cover profile") {
		t.Fatalf("want not-a-profile refusal, got %v", err)
	}
	if _, err := ParseProfile(filepath.Join(dir, "missing.out")); err == nil {
		t.Fatal("a missing profile must be an error")
	}
}

// fixture: module "fixture" with pkg/calc.go — Simple (complexity 1, fully
// covered) and Branchy (complexity 4: if + && + for, half covered).
func writeFixture(t *testing.T) (root, profilePath string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package pkg

func Simple() int {
	return 1
}

func Branchy(x int) int {
	if x > 0 && x < 10 {
		return 1
	}
	for i := 0; i < x; i++ {
		x--
	}
	return 0
}
`
	if err := os.WriteFile(filepath.Join(dir, "pkg", "calc.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(dir, "cover.out")
	content := "mode: set\n" +
		"fixture/pkg/calc.go:4.9,4.17 1 1\n" +
		"fixture/pkg/calc.go:9.3,9.12 1 0\n" +
		"fixture/pkg/calc.go:14.2,14.10 1 1\n"
	if err := os.WriteFile(profile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, profile
}

func TestAnalyzeScoresFunctions(t *testing.T) {
	root, profile := writeFixture(t)
	rep, err := Analyze(root, profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Functions) != 2 {
		t.Fatalf("want 2 scored functions, got %+v", rep.Functions)
	}
	// Sorted by CRAP descending: Branchy first (CRAP 6), Simple last (CRAP 1).
	branchy := rep.Functions[0]
	if branchy.Name != "Branchy" {
		t.Fatalf("worst function = %+v", branchy)
	}
	if branchy.Complexity != 4 {
		t.Fatalf("Branchy complexity = %d, want 4", branchy.Complexity)
	}
	if branchy.Coverage != 0.5 {
		t.Fatalf("Branchy coverage = %.4f, want 0.5", branchy.Coverage)
	}
	if branchy.CRAP != 6 {
		t.Fatalf("Branchy CRAP = %.4f, want 6", branchy.CRAP)
	}
	simple := rep.Functions[1]
	if simple.Name != "Simple" || simple.Complexity != 1 || simple.Coverage != 1 || simple.CRAP != 1 {
		t.Fatalf("Simple = %+v", simple)
	}
	if rep.Max != 6 {
		t.Fatalf("Max = %.4f, want 6", rep.Max)
	}
}

func TestAnalyzeRequiresGoModule(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "cover.out")
	if err := os.WriteFile(profile, []byte("mode: set\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Analyze(dir, profile)
	if err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Fatalf("want missing-go.mod refusal, got %v", err)
	}
}

func TestAnalyzeNamesReceiverQualified(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `package pkg

type calc struct{}

func (c calc) Add(a, b int) int {
	return a + b
}

func (c *calc) Sub(a, b int) int {
	return a - b
}

type wrapper[T any] struct{ v T }

func (w wrapper[T]) Get() int {
	return 1
}
`
	if err := os.WriteFile(filepath.Join(dir, "pkg", "recv.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(dir, "cover.out")
	content := "mode: set\n" +
		"fixture/pkg/recv.go:6.3,6.14 1 1\n" +
		"fixture/pkg/recv.go:10.3,10.15 1 1\n" +
		"fixture/pkg/recv.go:16.3,16.11 1 0\n"
	if err := os.WriteFile(profile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Analyze(dir, profile)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]Scored{}
	for _, f := range rep.Functions {
		names[f.Name] = f
	}
	if _, ok := names["calc.Add"]; !ok {
		t.Fatalf("want calc.Add, got %+v", rep.Functions)
	}
	sub, ok := names["(*calc).Sub"]
	if !ok || sub.Coverage != 1 || sub.Complexity != 1 || sub.CRAP != 1 {
		t.Fatalf("(*calc).Sub = %+v", sub)
	}
	get, ok := names["wrapper[T].Get"]
	if !ok || get.Coverage != 0 {
		t.Fatalf("wrapper[T].Get = %+v", get)
	}
}
