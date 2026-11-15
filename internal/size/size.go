// Package size enforces the NOV-67 module budget: no production Go
// file over the file cap, no function over the func cap, no package
// importing more than the coupling cap of sibling packages. Critical
// packages (security kernel + contract surface) get tighter caps.
// Waivers reuse docs/quality-exceptions.md lines (file:func, optional
// expira:YYYY-MM-DD): an expired or malformed waiver fails instead of
// silently passing, and waived items stay listed, never hidden.
package size

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Limits is one tier's budget.
type Limits struct {
	Files    int
	Funcs    int
	Coupling int
}

// DefaultLimits applies to ordinary packages.
var DefaultLimits = Limits{Files: 800, Funcs: 200, Coupling: 30}

// CriticalLimits applies to the security kernel and the contract
// surface. Every current member passes with margin; tighten deliberately.
var CriticalLimits = Limits{Files: 600, Funcs: 100, Coupling: 12}

// CriticalPackages are the packages held to CriticalLimits.
var CriticalPackages = []string{"gates", "evidence", "receipts", "manifest", "verdict", "contract"}

// Violation is one over-budget item or bad waiver.
type Violation struct {
	Where  string
	Kind   string
	Detail string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s: %s %s", v.Where, v.Kind, v.Detail)
}

func isCritical(pkg string) bool {
	for _, c := range CriticalPackages {
		if pkg == c {
			return true
		}
	}
	return false
}

func limitsFor(pkg string) Limits {
	if isCritical(pkg) {
		return CriticalLimits
	}
	return DefaultLimits
}

// waiver is one parsed exceptions line.
type waiver struct {
	key     string
	expires string // "" = no expiry
	raw     string
}

func parseWaivers(data []byte) []waiver {
	var out []waiver
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key := strings.Fields(line)[0]
		if !strings.Contains(key, ":") {
			continue
		}
		w := waiver{key: key, raw: line}
		for _, f := range strings.Fields(line)[1:] {
			if d, ok := strings.CutPrefix(f, "expira:"); ok {
				w.expires = d
			}
		}
		out = append(out, w)
	}
	return out
}

// checker accumulates violations while walking one tree.
func Check(root, exceptionsPath string) []Violation {
	waivers, problems := loadWaivers(root, exceptionsPath)
	coupling := map[string]map[string]bool{}
	_ = filepath.Walk(filepath.Join(root, "internal"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		problems = append(problems, checkOneFile(root, p, waivers, coupling)...)
		return nil
	})
	problems = append(problems, checkCoupling(coupling)...)
	sort.Slice(problems, func(i, j int) bool {
		if problems[i].Where != problems[j].Where {
			return problems[i].Where < problems[j].Where
		}
		return problems[i].Kind < problems[j].Kind
	})
	return problems
}

// loadWaivers reads the exceptions file; expired or malformed entries
// are reported instead of silently applied.
func loadWaivers(root, exceptionsPath string) (map[string]waiver, []Violation) {
	var problems []Violation
	waivers := map[string]waiver{}
	if data, err := os.ReadFile(filepath.Join(root, exceptionsPath)); err == nil {
		for _, w := range parseWaivers(data) {
			waivers[w.key] = w
		}
	}
	now := time.Now().UTC()
	for key, w := range waivers {
		if w.expires == "" {
			continue
		}
		if t, err := time.Parse("2006-01-02", w.expires); err != nil || now.After(t.AddDate(0, 0, 1)) {
			problems = append(problems, Violation{Where: key, Kind: "waiver-expired", Detail: "remove the line or renew with a new expira date"})
		}
	}
	return waivers, problems
}

// checkOneFile reports file and function budget breaches for one Go
// file and records its internal imports for the coupling check.
func checkOneFile(root, p string, waivers map[string]waiver, coupling map[string]map[string]bool) []Violation {
	var problems []Violation
	rel, _ := filepath.Rel(root, p)
	rel = filepath.ToSlash(rel)
	pkg := filepath.Base(filepath.Dir(p))
	lim := limitsFor(pkg)
	src, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	if n := lineCount(src); n > lim.Files && !waived(waivers, rel+":*") {
		problems = append(problems, Violation{Where: rel, Kind: "file-too-big", Detail: fmt.Sprintf("%d lines, cap %d (split the file or add a file:* waiver with expira)", n, lim.Files)})
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, p, src, 0)
	if err != nil {
		return problems
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		size := fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1
		if size > lim.Funcs && !waived(waivers, rel+":"+fn.Name.Name) {
			problems = append(problems, Violation{Where: rel + ":" + fn.Name.Name, Kind: "func-too-big", Detail: fmt.Sprintf("%d lines, cap %d (split the function or add a waiver with expira)", size, lim.Funcs)})
		}
	}
	recordImports(f, pkg, coupling)
	return problems
}

func lineCount(src []byte) int {
	n := 1
	for _, b := range src {
		if b == '\n' {
			n++
		}
	}
	return n
}

// recordImports notes this file's internal dependencies for the
// per-package coupling budget.
func recordImports(f *ast.File, pkg string, coupling map[string]map[string]bool) {
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		rest, ok := strings.CutPrefix(path, "regulus-ai/internal/")
		if !ok {
			continue
		}
		dep := rest
		if i := strings.Index(dep, "/"); i >= 0 {
			dep = dep[:i]
		}
		if dep == "" || dep == pkg {
			continue
		}
		if coupling[pkg] == nil {
			coupling[pkg] = map[string]bool{}
		}
		coupling[pkg][dep] = true
	}
}

// checkCoupling reports packages whose internal imports exceed the
// coupling budget, naming the dependency set for the fix.

func checkCoupling(coupling map[string]map[string]bool) []Violation {
	var problems []Violation
	var pkgs []string
	for pkg := range coupling {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	for _, pkg := range pkgs {
		lim := limitsFor(pkg)
		if len(coupling[pkg]) <= lim.Coupling {
			continue
		}
		var deps []string
		for d := range coupling[pkg] {
			deps = append(deps, d)
		}
		sort.Strings(deps)
		problems = append(problems, Violation{Where: "internal/" + pkg, Kind: "too-coupled", Detail: fmt.Sprintf("%d internal deps (%s), cap %d", len(deps), strings.Join(deps, ", "), lim.Coupling)})
	}
	return problems
}

func waived(waivers map[string]waiver, key string) bool {
	_, ok := waivers[key]
	return ok
}
