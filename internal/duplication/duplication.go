// Package duplication finds structurally repeated code.
//
// F20.2 (NOV-151) asked to evaluate duplication, complexity and dead-code
// detection. Complexity and CRAP are already measured and gated (internal/crap, the
// required `crap` gate, the required coverage floor and the required size gate), so
// this package exists only for the part that was missing.
//
// It is deliberately report-only and deliberately dependency-free. The module has no
// external requires at all, and adding golang.org/x/tools to obtain a clone detector
// would be the first one - a delivery decision, not a quality one. So the analysis is
// stdlib: parse with go/parser, scan with go/scanner, normalise identifiers and
// literals to placeholders, and hash sliding windows of tokens. Two windows that hash
// equal are the same code modulo names and values, which is the definition of a
// structural clone worth quoting.
//
// Nothing here decides anything. It reports; a human decides whether to remediate.
package duplication

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultMinTokens is the window size in normalised tokens.
//
// Forty is chosen so that a clone has to be a whole meaningful unit, not a shared
// three-line prologue: below that, every file looks like every other file and the
// report becomes noise nobody reads.
const DefaultMinTokens = 40

// Options controls a run.
type Options struct {
	// MinTokens is the normalised window size. Zero means DefaultMinTokens.
	MinTokens int
	// IncludeTests includes _test.go files. Tests repeat fixtures on purpose, so
	// the default is false.
	IncludeTests bool
	// Limit caps the number of reported pairs. Zero means 50.
	Limit int
}

// Location is one end of a clone.
type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Func string `json:"func"`
}

// Clone is a pair of locations sharing at least one normalised window.
type Clone struct {
	A       Location `json:"a"`
	B       Location `json:"b"`
	Windows int      `json:"windows"`
	Tokens  int      `json:"tokens"`
}

// Report is the analysis result.
type Report struct {
	Root      string  `json:"root"`
	Files     int     `json:"files"`
	Functions int     `json:"functions"`
	MinTokens int     `json:"min_tokens"`
	Clones    []Clone `json:"clones"`
	// Truncated reports that Limit hid further pairs, so a zero-clone result is
	// never confused with a capped one.
	Truncated bool `json:"truncated"`
}

// skipDir names directories that never carry project code.
var skipDir = map[string]bool{
	".git": true, "vendor": true, "testdata": true, "node_modules": true,
	".regulus": true, "bin": true, "dist": true,
}

// funcKey identifies a function in the report.
type funcKey struct {
	file string
	line int
	name string
}

// window is one occurrence of a hashed normalised window.
type window struct {
	fn funcKey
}

// Analyze walks root and reports structural clones.
//
// It never writes and never fails on unparsable files: a file the parser rejects is
// skipped, because a report that aborts on one generated file is a report nobody
// can run in CI.
func Analyze(root string, opts Options) (Report, error) {
	if opts.MinTokens <= 0 {
		opts.MinTokens = DefaultMinTokens
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Report{}, err
	}
	if !info.IsDir() {
		return Report{}, fmt.Errorf("%s is not a directory", root)
	}

	rep := Report{Root: abs, MinTokens: opts.MinTokens}
	seen := make(map[uint64][]window)

	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, never fatal
		}
		if d.IsDir() {
			if path != abs && skipDir[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !opts.IncludeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if perr != nil || file == nil {
			return nil
		}
		tf := fset.File(file.Pos())
		if tf == nil {
			return nil
		}
		toks := scanTokens(tf, src)

		rep.Files++
		rel := relative(abs, path)
		ast.Inspect(file, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				return true
			}
			start, end := tf.Offset(fd.Body.Lbrace), tf.Offset(fd.Body.Rbrace)
			norm := normalizeRange(toks, start, end)
			if len(norm) < opts.MinTokens {
				return true
			}
			key := funcKey{file: rel, line: fset.Position(fd.Pos()).Line, name: funcName(fd)}
			rep.Functions++
			for i := 0; i+opts.MinTokens <= len(norm); i++ {
				h := hashWindow(norm[i : i+opts.MinTokens])
				seen[h] = append(seen[h], window{fn: key})
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		return Report{}, walkErr
	}

	rep.Clones, rep.Truncated = collect(seen, opts)
	return rep, nil
}

// tok is one normalised token with its byte offset inside the file.
type tok struct {
	off int
	val string
}

// scanTokens normalises every token in the file.
func scanTokens(tf *token.File, src []byte) []tok {
	var s scanner.Scanner
	s.Init(tf, src, nil, 0)
	out := make([]tok, 0, len(src)/6)
	for {
		pos, tk, _ := s.Scan()
		if tk == token.EOF {
			break
		}
		out = append(out, tok{off: tf.Offset(pos), val: normalize(tk)})
	}
	return out
}

// normalize collapses names and values so a clone differs only if its structure
// differs. Keywords and operators keep their identity.
func normalize(tk token.Token) string {
	switch tk {
	case token.IDENT:
		return "id"
	case token.INT, token.FLOAT, token.IMAG, token.CHAR, token.STRING:
		return "lit"
	}
	return tk.String()
}

// normalizeRange selects the tokens inside [start,end).
func normalizeRange(toks []tok, start, end int) []string {
	out := make([]string, 0, 64)
	for _, t := range toks {
		if t.off < start {
			continue
		}
		if t.off >= end {
			break
		}
		out = append(out, t.val)
	}
	return out
}

// hashWindow hashes a normalised window. The separator prevents
// ["if","id"] from colliding with ["ifid",""].
func hashWindow(w []string) uint64 {
	h := fnv.New64a()
	for _, s := range w {
		_, _ = h.Write([]byte(s))
		_, _ = h.Write([]byte{0x1f})
	}
	return h.Sum64()
}

// collect turns hashed windows into reported pairs.
//
// A window that occurs twice inside the SAME function is excluded: intra-function
// repetition is usually a loop the author already wrote, and reporting it buries the
// finding that matters.
func collect(seen map[uint64][]window, opts Options) ([]Clone, bool) {
	pairs := make(map[[2]funcKey]int)
	for _, occ := range seen {
		if len(occ) < 2 {
			continue
		}
		for i := 0; i < len(occ); i++ {
			for j := i + 1; j < len(occ); j++ {
				a, b := occ[i].fn, occ[j].fn
				if a == b {
					continue
				}
				key := [2]funcKey{a, b}
				if less(b, a) {
					key = [2]funcKey{b, a}
				}
				pairs[key]++
			}
		}
	}

	clones := make([]Clone, 0, len(pairs))
	for key, count := range pairs {
		clones = append(clones, Clone{
			A:       Location{File: key[0].file, Line: key[0].line, Func: key[0].name},
			B:       Location{File: key[1].file, Line: key[1].line, Func: key[1].name},
			Windows: count,
			Tokens:  opts.MinTokens,
		})
	}
	sort.Slice(clones, func(i, j int) bool {
		if clones[i].A.File != clones[j].A.File {
			return clones[i].A.File < clones[j].A.File
		}
		if clones[i].A.Line != clones[j].A.Line {
			return clones[i].A.Line < clones[j].A.Line
		}
		if clones[i].B.File != clones[j].B.File {
			return clones[i].B.File < clones[j].B.File
		}
		if clones[i].B.Line != clones[j].B.Line {
			return clones[i].B.Line < clones[j].B.Line
		}
		return clones[i].Windows > clones[j].Windows
	})

	limit := opts.Limit
	if len(clones) > limit {
		return clones[:limit], true
	}
	return clones, false
}

// less orders two keys so a pair has one canonical direction.
func less(a, b funcKey) bool {
	if a.file != b.file {
		return a.file < b.file
	}
	if a.line != b.line {
		return a.line < b.line
	}
	return a.name < b.name
}

// relative makes paths stable in a report.
func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// funcName is the report name of a function, including its receiver.
func funcName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	return recvName(fd.Recv.List[0].Type) + "." + fd.Name.Name
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	}
	return "recv"
}
