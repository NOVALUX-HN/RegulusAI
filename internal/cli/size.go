// sizeCmd implements `regulus size check [--exceptions E]`:
// the NOV-67 module-budget gate. No production Go file may exceed the
// file cap, no function the func cap, no package the coupling cap,
// with tighter caps for the security kernel and contract surface.
// Waivers reuse docs/quality-exceptions.md (file:func, optional
// file:* for whole files); expired or malformed waivers fail instead
// of silently passing.
package cli

import (
	"flag"
	"fmt"
	"sort"

	"regulus-ai/internal/size"
)

func (c CLI) sizeCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, "usage: regulus size check [--exceptions E]")
		return 0
	}
	fs := flag.NewFlagSet("size", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	exceptions := fs.String("exceptions", "docs/quality-exceptions.md", "waiver file (file:func [file:*] motivo expira aprueba)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 1 || rest[0] != "check" {
		fmt.Fprintln(c.Stderr, "usage: regulus size check [--exceptions E]")
		return 2
	}
	problems := size.Check(c.dir(), *exceptions)
	if len(problems) == 0 {
		fmt.Fprintf(c.Stdout, "size OK: files <= %d lines, funcs <= %d lines, coupling <= %d deps (critical: %d/%d/%d)\n",
			size.DefaultLimits.Files, size.DefaultLimits.Funcs, size.DefaultLimits.Coupling,
			size.CriticalLimits.Files, size.CriticalLimits.Funcs, size.CriticalLimits.Coupling)
		return 0
	}
	byKind := map[string]int{}
	for _, p := range problems {
		fmt.Fprintf(c.Stderr, "size: %s\n", p.String())
		byKind[p.Kind]++
	}
	var kinds []string
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(c.Stderr, "size: %d %s violation(s)\n", byKind[k], k)
	}
	return 1
}
