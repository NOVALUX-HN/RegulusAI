package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// initCmd implements `regulus init [--stack go|node|smoke]`: onboard a
// fresh repo (cross-project S8). Writes regulus.json (evolution budget
// + one stack preset), creates charts/, and ensures .gitignore covers
// .regulus/. Refuses when regulus.json already exists — init never
// overwrites a configured project.
func (c CLI) initCmd(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	stack := fs.String("stack", "smoke", "go | node | smoke")
	harnessFrom := fs.String("harness-from", "", "repo root whose .opencode harness config to copy")
	devin := fs.Bool("devin", false, "also scaffold .devin/mcp_config.json for Devin CLI/Desktop")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus init [--stack go|node|smoke] [--harness-from <repo>] [--devin]")
		return 2
	}
	if _, err := os.Stat("regulus.json"); err == nil {
		fmt.Fprintln(c.Stderr, "init: regulus.json already exists — refusing to overwrite")
		return 1
	}
	gates, ok := initPresets[*stack]
	if !ok {
		fmt.Fprintf(c.Stderr, "init: unknown stack %q (go | node | smoke)\n", *stack)
		return 2
	}
	cfg := struct {
		Evolution map[string]int   `json:"evolution"`
		Gates     []map[string]any `json:"gates"`
	}{
		Evolution: map[string]int{"max_per_day": 10},
		Gates:     gates,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(c.Stderr, "init: %v\n", err)
		return 1
	}
	if err := os.WriteFile("regulus.json", append(data, '\n'), 0o644); err != nil {
		fmt.Fprintf(c.Stderr, "init: %v\n", err)
		return 1
	}
	if err := os.MkdirAll("charts", 0o755); err != nil {
		fmt.Fprintf(c.Stderr, "init: %v\n", err)
		return 1
	}
	if err := ensureGitignore(); err != nil {
		fmt.Fprintf(c.Stderr, "init: %v\n", err)
		return 1
	}
	if *harnessFrom != "" {
		if err := copyHarnessConfig(*harnessFrom); err != nil {
			fmt.Fprintf(c.Stderr, "init: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "initialized: regulus.json, charts/, .gitignore, .opencode copied from %s\n", *harnessFrom)
		fmt.Fprintln(c.Stdout, "next: run bun install in .opencode/, run regulus doctor, open your first Chart")
		return 0
	}
	devinNote := ""
	if *devin {
		if err := scaffoldDevinMcpConfig(); err != nil {
			fmt.Fprintf(c.Stderr, "init: %v\n", err)
			return 1
		}
		devinNote = ", .devin/mcp_config.json"
	}
	fmt.Fprintf(c.Stdout, "initialized: regulus.json, charts/, .gitignore (.regulus/ ignored)%s\n", devinNote)
	fmt.Fprintln(c.Stdout, "next: copy the .opencode/ harness config in (or re-run with --harness-from <repo>), run bun install in .opencode/, run regulus doctor, open your first Chart")
	return 0
}

// devinMcpConfig is the `.devin/mcp_config.json` scaffold written by
// `regulus init --devin`: it wires the regulus stdio MCP server the same
// way this repo's own .devin/ pack does (command on PATH, `mcp` argv).
// Devin project permissions stay the enforcement point (see docs/devin.md
// MCP trust boundary) — this file only registers the server.
const devinMcpConfig = `{
  "mcpServers": {
    "regulus": {
      "command": "regulus",
      "args": ["mcp"],
      "env": {}
    }
  }
}
`

// scaffoldDevinMcpConfig writes the Devin MCP server registration for a
// fresh repo. It never overwrites: an existing .devin/mcp_config.json is
// a project-owned file and stays untouched.
func scaffoldDevinMcpConfig() error {
	dst := filepath.Join(".devin", "mcp_config.json")
	if _, err := os.Stat(dst); err == nil {
		return errors.New(".devin/mcp_config.json already exists — refusing to overwrite")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(devinMcpConfig), 0o644)
}

// copyHarnessConfig copies the harness configuration (discipline.md,
// tool-guidance.md, agents/, skills/, plugins/) from <from>/.opencode
// into the fresh repo. The environment (node_modules, package.json,
// lockfiles) is deliberately NOT copied — the target project installs
// its own. Refuses when the target already has an .opencode directory.
func copyHarnessConfig(from string) error {
	src := filepath.Join(from, ".opencode")
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return fmt.Errorf("no .opencode directory at %s", src)
	}
	if _, err := os.Stat(".opencode"); err == nil {
		return errors.New(".opencode already exists here — refusing to overwrite")
	}
	for _, item := range []string{"discipline.md", "tool-guidance.md", "agents", "skills", "plugins"} {
		s := filepath.Join(src, item)
		if _, err := os.Stat(s); err != nil {
			continue
		}
		if err := copyPath(s, filepath.Join(".opencode", item)); err != nil {
			return err
		}
	}
	return nil
}

// copyPath copies one file or directory tree.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	}
	return filepath.WalkDir(src, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if e.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// initPresets are the starter gates per stack. Small and honest: each
// command runs on a fresh checkout of that stack; replace them with the
// project's real gates as the harness takes over.
var initPresets = map[string][]map[string]any{
	"go": {
		{"name": "typecheck", "layer": 1, "command": "go build ./..."},
		{"name": "unit-tests", "layer": 2, "command": "go test ./..."},
	},
	"node": {
		{"name": "unit-tests", "layer": 1, "command": "npm test --if-present"},
	},
	"smoke": {
		{"name": "presence", "layer": 1, "command": "test -f README.md && echo repo-present"},
	},
}

// ensureGitignore appends the Regulus runtime-state block when the
// repo has no .gitignore or it lacks a .regulus/ entry.
func ensureGitignore() error {
	data, err := os.ReadFile(".gitignore")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == ".regulus/" {
				return nil
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(".gitignore", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	block := "\n# Regulus runtime state — per-project, never committed\n.regulus/\n"
	if err == nil && len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		block = "\n" + strings.TrimPrefix(block, "\n")
	}
	_, err = f.WriteString(block)
	return err
}
