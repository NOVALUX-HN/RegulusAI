package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"regulus-ai/internal/spec"
)

// specCmd implements `regulus spec check <file|dir> [...]`: validate the
// acceptance-criteria contract of one feature document, several files, or
// every `*.md` inside a directory (PRD R-G1). A directory keeps the gate
// portable: no shell glob or loop is needed and every failure names the
// broken file.
func (c CLI) specCmd(args []string) int {
	if len(args) < 2 || args[0] != "check" {
		fmt.Fprintln(c.Stderr, "usage: regulus spec check <file|dir> [more...]")
		return 2
	}
	files, err := collectSpecFiles(args[1:])
	if err != nil {
		fmt.Fprintf(c.Stderr, "spec check: %s\n", err.Error())
		return 1
	}
	if len(files) == 1 {
		if len(args) == 2 {
			if info, statErr := os.Stat(args[1]); statErr == nil && !info.IsDir() {
				return c.specCheckOne(files[0])
			}
		}
	}
	failed := false
	total := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(c.Stderr, "spec check: %s: cannot read: %v\n", f, err)
			failed = true
			continue
		}
		n, violations := spec.Check(string(data))
		if len(violations) > 0 {
			for _, v := range violations {
				fmt.Fprintf(c.Stderr, "spec check: %s: %s\n", f, v)
			}
			failed = true
			continue
		}
		total += n
	}
	if failed {
		return 1
	}
	fmt.Fprintf(c.Stdout, "spec OK: %d scenario(s) in %d file(s)\n", total, len(files))
	return 0
}

// collectSpecFiles expands each target to chart files: a directory yields
// its sorted `*.md` entries, a file yields itself. It fails naming the
// target when nothing usable is found.
func collectSpecFiles(targets []string) ([]string, error) {
	var files []string
	for _, t := range targets {
		info, err := os.Stat(t)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %v", t, err)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(t)
			if err != nil {
				return nil, fmt.Errorf("cannot read %s: %v", t, err)
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				files = append(files, filepath.Join(t, e.Name()))
			}
			continue
		}
		files = append(files, t)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no feature documents in %s", strings.Join(targets, ", "))
	}
	sort.Strings(files)
	return files, nil
}

// specCheckOne preserves the single-file contract: violations without a
// filename prefix (the filename is the only argument) and the historical
// `spec OK: N scenario(s)` summary.
func (c CLI) specCheckOne(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(c.Stderr, "spec check: cannot read %s: %v\n", path, err)
		return 1
	}
	n, violations := spec.Check(string(data))
	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Fprintf(c.Stderr, "spec check: %s\n", v)
		}
		return 1
	}
	fmt.Fprintf(c.Stdout, "spec OK: %d scenario(s)\n", n)
	return 0
}
