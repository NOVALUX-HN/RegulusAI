// Package crap scores Go functions with Uncle Bob's CRAP formula:
// complexity^2 x (1 - coverage)^3 + complexity — the layer-7 gauntlet gate
// (PRD R-G2): uncovered and complex code is dangerous code, however green
// the tests look.
//
// It reads a `go test -coverprofile` profile, parses each covered source
// file with go/ast to compute cyclomatic complexity, maps profile blocks
// onto functions, and reports the worst offenders. Nested function
// literals count toward their enclosing function.
package crap

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Block is one coverage block from a go cover profile.
type Block struct {
	File      string
	StartLine int
	EndLine   int
	NumStmts  int
	Hits      int
}

// Scored is one function's CRAP analysis.
type Scored struct {
	File       string
	Name       string
	StartLine  int
	Complexity int
	Coverage   float64 // 0..1
	CRAP       float64
}

// Report is the analysis of one coverage profile.
type Report struct {
	Functions []Scored // sorted by CRAP descending, ties by file then name
	Max       float64
}

// CRAP computes the formula: comp^2 x (1 - coverage)^3 + comp.
func CRAP(complexity int, coverage float64) float64 {
	c := float64(complexity)
	return c*c*math.Pow(1-coverage, 3) + c
}

// ParseProfile reads a go cover profile ("mode: …" header, then one block
// per line: "file:startLine.startCol,endLine.endCol numStmts hitCount").
func ParseProfile(path string) ([]Block, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "mode:") {
		return nil, fmt.Errorf("%s is not a go cover profile (missing mode header)", path)
	}
	var blocks []Block
	for _, raw := range lines[1:] {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		b, err := parseBlockLine(line)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

func parseBlockLine(line string) (Block, error) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return Block{}, fmt.Errorf("bad profile block line: %q", line)
	}
	colon := strings.LastIndex(fields[0], ":")
	if colon < 0 {
		return Block{}, fmt.Errorf("bad profile block line: %q", line)
	}
	posStart, posEnd, ok := strings.Cut(fields[0][colon+1:], ",")
	if !ok {
		return Block{}, fmt.Errorf("bad profile block position in %q", line)
	}
	b := Block{File: fields[0][:colon]}
	var err error
	if b.StartLine, err = strconv.Atoi(strings.Split(posStart, ".")[0]); err != nil {
		return Block{}, fmt.Errorf("bad profile block position in %q", line)
	}
	if b.EndLine, err = strconv.Atoi(strings.Split(posEnd, ".")[0]); err != nil {
		return Block{}, fmt.Errorf("bad profile block position in %q", line)
	}
	if b.NumStmts, err = strconv.Atoi(fields[1]); err != nil {
		return Block{}, fmt.Errorf("bad profile block statements in %q", line)
	}
	if b.Hits, err = strconv.Atoi(fields[2]); err != nil {
		return Block{}, fmt.Errorf("bad profile block hits in %q", line)
	}
	return b, nil
}

// Analyze parses the profile and the Go sources of the module rooted at
// root, scoring every function: complexity via AST, coverage via the
// profile blocks, CRAP via the formula.
func Analyze(root, profilePath string) (Report, error) {
	blocks, err := ParseProfile(profilePath)
	if err != nil {
		return Report{}, err
	}
	module, err := modulePath(root)
	if err != nil {
		return Report{}, err
	}

	files := map[string]bool{}
	for _, b := range blocks {
		files[b.File] = true
	}

	var scored []Scored
	for file := range files {
		rel := strings.TrimPrefix(file, module+"/")
		src := filepath.Join(root, filepath.FromSlash(rel))
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, src, nil, 0)
		if err != nil {
			return Report{}, fmt.Errorf("parsing %s: %w", src, err)
		}
		for _, decl := range parsed.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			startLine := fset.Position(fd.Pos()).Line
			endLine := fset.Position(fd.End()).Line
			fn := Scored{
				File:       rel,
				Name:       qualName(fd),
				StartLine:  startLine,
				Complexity: complexity(fd),
			}
			var total, covered int
			for _, b := range blocks {
				if b.File != file || b.EndLine < startLine || b.StartLine > endLine {
					continue
				}
				total += b.NumStmts
				if b.Hits > 0 {
					covered += b.NumStmts
				}
			}
			if total > 0 {
				fn.Coverage = float64(covered) / float64(total)
			}
			fn.CRAP = CRAP(fn.Complexity, fn.Coverage)
			scored = append(scored, fn)
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].CRAP != scored[j].CRAP {
			return scored[i].CRAP > scored[j].CRAP
		}
		if scored[i].File != scored[j].File {
			return scored[i].File < scored[j].File
		}
		return scored[i].Name < scored[j].Name
	})
	rep := Report{Functions: scored}
	if len(scored) > 0 {
		rep.Max = scored[0].CRAP
	}
	return rep, nil
}

// modulePath reads the module path from the go.mod at root.
func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("reading go.mod at %s: %w", root, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("go.mod at %s declares no module path", root)
}

// complexity counts cyclomatic decisions: 1 + branch statements plus each
// short-circuit boolean operator.
func complexity(fd *ast.FuncDecl) int {
	c := 1
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			c++
		}
		if be, ok := n.(*ast.BinaryExpr); ok && (be.Op == token.LAND || be.Op == token.LOR) {
			c++
		}
		return true
	})
	return c
}

// receiverName renders a receiver type for qualified function names.
func receiverName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	return exprName(fd.Recv.List[0].Type)
}

// qualName renders a function's qualified name, parenthesizing pointer
// receivers the way Go tooling does: (*calc).Sub, calc.Add, wrapper[T].Get.
func qualName(fd *ast.FuncDecl) string {
	name := fd.Name.Name
	recv := receiverName(fd)
	if recv == "" {
		return name
	}
	if strings.HasPrefix(recv, "*") {
		return "(" + recv + ")." + name
	}
	return recv + "." + name
}

func exprName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprName(t.X)
	case *ast.IndexExpr:
		return exprName(t.X) + "[" + exprName(t.Index) + "]"
	case *ast.IndexListExpr:
		return exprName(t.X)
	}
	return ""
}
