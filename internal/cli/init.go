package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"regulus-ai/internal/gates"
)

// initCmd implements `regulus init [--stack go|node|smoke] [--force]`: onboard a
// fresh repo (cross-project S8). Writes regulus.json (evolution budget
// + execution allowlist + one stack preset with floors), creates
// charts/, and ensures .gitignore covers .regulus/. Refuses when
// regulus.json already exists in the cwd or in any ancestor directory
// (running in a subdirectory would split-brain: the nested config in
// the cwd while every CLI path resolves stores upward via ResolveRoot)
// — pass --force to overwrite deliberately. Never chdirs to the
// resolved root: surprising writes outside the cwd are worse than a
// refusal. After a successful scaffold it prints a best-effort
// validation (doctor + gate run --dry-run summary lines) that never
// fails init: validation errors print and init still exits 0.
func (c CLI) initCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, "usage: regulus init [--stack go|node|smoke] [--force] [--harness-from <repo>] [--devin]")
		return 0
	}
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	stack := fs.String("stack", "smoke", "go | node | smoke")
	force := fs.Bool("force", false, "overwrite regulus.json even when one exists here or in an ancestor directory")
	harnessFrom := fs.String("harness-from", "", "repo root whose .opencode harness config to copy")
	devin := fs.Bool("devin", false, "also scaffold .devin/mcp_config.json for Devin CLI/Desktop")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus init [--stack go|node|smoke] [--force] [--harness-from <repo>] [--devin]")
		return 2
	}
	if _, err := os.Stat("regulus.json"); err == nil && !*force {
		if abs, aberr := filepath.Abs("regulus.json"); aberr == nil {
			fmt.Fprintf(c.Stderr, "init: regulus.json already exists at %s — refusing to overwrite (use --force)\n", abs)
		} else {
			fmt.Fprintln(c.Stderr, "init: regulus.json already exists — refusing to overwrite (use --force)")
		}
		return 1
	}
	// H9: refuse a nested scaffold even when the cwd itself is clean.
	// Every other CLI path resolves stores upward via ResolveRoot, so
	// writing sub/regulus.json + sub/charts/ here would leave a nested
	// config whose gates run against the ancestor's stores. Name the
	// ancestor path so the fix is obvious; --force scaffolds the cwd
	// deliberately and never chdirs.
	if !*force {
		if ancestor, ok := ancestorRegulusJSON(); ok {
			fmt.Fprintf(c.Stderr, "init: ancestor regulus.json at %s — refusing to scaffold a nested project (run in the project root or use --force)\n", ancestor)
			return 1
		}
	}
	if *harnessFrom != "" {
		if st, err := os.Stat(filepath.Join(*harnessFrom, ".opencode")); err != nil || !st.IsDir() {
			fmt.Fprintf(c.Stderr, "init: no .opencode directory at %s\n", filepath.Join(*harnessFrom, ".opencode"))
			return 2
		}
	}
	gates, ok := initPresets[*stack]
	if !ok {
		fmt.Fprintf(c.Stderr, "init: unknown stack %q (go | node | smoke)\n", *stack)
		return 2
	}
	allowlist, ok := initAllowlists[*stack]
	if !ok {
		fmt.Fprintf(c.Stderr, "init: unknown stack %q (go | node | smoke)\n", *stack)
		return 2
	}
	cfg := struct {
		Evolution map[string]int   `json:"evolution"`
		Execution map[string]any   `json:"execution"`
		Gates     []map[string]any `json:"gates"`
	}{
		Evolution: map[string]int{"max_per_day": 10},
		Execution: map[string]any{"allowlist": allowlist},
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
		c.printInitValidation()
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
	if *stack == "smoke" {
		fmt.Fprintln(c.Stdout, "next: replace the smoke gate with your real gates in regulus.json, run regulus doctor, open your first Chart in charts/")
	} else {
		fmt.Fprintln(c.Stdout, "next: point the gates at your real layout in regulus.json (see the --harness-from option for the full harness), run regulus doctor, open your first Chart in charts/")
	}
	c.printInitValidation()
	return 0
}

// ancestorRegulusJSON reports the nearest regulus.json strictly above
// the process cwd (the cwd itself is the caller's own check): the
// first regular regulus.json walking up from the cwd's parent to the
// filesystem root. The returned path is absolute so the refusal can
// name exactly which project the nested scaffold would split-brain
// against.
func ancestorRegulusJSON() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := filepath.Dir(filepath.Clean(wd))
	for {
		cand := filepath.Join(dir, "regulus.json")
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func (c CLI) printInitValidation() {
	func() {
		defer func() { _ = recover() }()
		if c.Probe == nil {
			fmt.Fprintln(c.Stdout, "validate: doctor skipped (no runtime probe)")
			return
		}
		res := c.Probe()
		if res.Supported {
			fmt.Fprintf(c.Stdout, "validate: doctor ok — %s\n", res.Detected)
		} else {
			fmt.Fprintf(c.Stdout, "validate: doctor unavailable — %s (init still complete)\n", res.Reason)
		}
	}()
	func() {
		defer func() { _ = recover() }()
		cfg, err := gates.Load("regulus.json")
		if err != nil {
			fmt.Fprintf(c.Stdout, "validate: gate run --dry-run skipped — %v (init still complete)\n", err)
			return
		}
		r := c.Runner
		if r == nil {
			r = gates.ShRunner{}
		}
		fmt.Fprintln(c.Stdout, "validate: gate run --dry-run plan:")
		if c.renderDryRunPlan(gateRunArgs{task: "init", cfg: cfg, runner: r}) != 0 {
			fmt.Fprintln(c.Stdout, "validate: gate run --dry-run unavailable (init still complete)")
			return
		}
		fmt.Fprintf(c.Stdout, "validate: gate run --dry-run ok — %d gate(s)\n", len(cfg.Gates))
	}()
}

// printInitValidation prints the post-init validation summary: a doctor
// line and the gate run --dry-run plan over the just-written
// regulus.json. Best-effort by contract: every failure path prints and
// continues, and init still exits 0 — validation must never turn a good
// scaffold into a failure (broken PATH, no runtime probe in tests, an
// unreadable config). It reads the cwd regulus.json explicitly, never
// c.dir(): with --force inside a nested checkout c.dir() may resolve to
// the ancestor project, but the validation must cover the config init
// just wrote.

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
// into the fresh repo, plus the repo-root codegen script
// scripts/sync-plugin-defaults.ts (the plugin fallback literals are
// generated from the .md sources — a fresh checkout needs the script
// for `bun scripts/sync-plugin-defaults.ts --check` to run). The
// environment (node_modules, package.json, lockfiles) is deliberately
// NOT copied — the target project installs its own. Refuses when the
// target already has an .opencode directory.
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
	// Repo-root codegen script (outside .opencode): copy when the source
	// harness repo carries it, skip when it does not. Never overwrite a
	// target-owned file.
	if _, err := os.Stat(filepath.Join(from, "scripts", "sync-plugin-defaults.ts")); err == nil {
		if _, err := os.Stat(filepath.Join("scripts", "sync-plugin-defaults.ts")); err == nil {
			return errors.New("scripts/sync-plugin-defaults.ts already exists here — refusing to overwrite")
		}
		data, err := os.ReadFile(filepath.Join(from, "scripts", "sync-plugin-defaults.ts"))
		if err != nil {
			return err
		}
		if err := os.MkdirAll("scripts", 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join("scripts", "sync-plugin-defaults.ts"), data, 0o644); err != nil {
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

// initAllowlists are the execution allowlists per stack: every gate
// command's first token must be listed, otherwise the gauntlet refuses
// the gate as policy (see internal/gates/policy.go allowlistRefused).
// Small and closed: each stack lists only the tools its gates use.
var initAllowlists = map[string][]string{
	"go":    {"go", "gofmt", "regulus"},
	"node":  {"bun", "bunx", "regulus"},
	"smoke": {"echo"},
}

// initPresets are the starter gates per stack. Small and honest: each
// command runs on a fresh checkout of that stack; replace them with the
// project's real gates as the harness takes over. Every stack carries
// floors from birth: coverage with a threshold, the CRAP and spec gates
// where they apply, and gofmt for Go — so a new project never starts
// without a quality floor.
var initPresets = map[string][]map[string]any{
	"go": {
		{"name": "typecheck", "layer": 1, "command": "go build ./..."},
		{"name": "lint", "layer": 2, "command": "go vet ./..."},
		{"name": "build", "layer": 3, "command": "go build -o bin/app ./..."},
		{"name": "unit-tests", "layer": 4, "command": "go test ./... -coverprofile .regulus/coverage.out"},
		{"name": "coverage-floor", "layer": 5, "command": "go tool cover -func .regulus/coverage.out", "metric": map[string]any{"extract": "last-line-percent", "threshold": 80, "operator": ">="}},
		{"name": "crap", "layer": 6, "command": "go run ./cmd/regulus crap --profile .regulus/coverage.out --max 30 --top 5"},
		{"name": "spec", "layer": 7, "command": "regulus spec check charts/"},
		{"name": "gofmt", "layer": 8, "command": "gofmt -l ."},
	},
	"node": {
		{"name": "ts-typecheck", "layer": 1, "command": "bunx tsc --noEmit -p tsconfig.json"},
		{"name": "ts-unit-tests", "layer": 2, "command": "bun test ./"},
		{"name": "spec", "layer": 3, "command": "regulus spec check charts/"},
	},
	"smoke": {
		{"name": "presence", "layer": 1, "command": "echo repo-present"},
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
