package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"regulus-ai/internal/spec"
)

// discover Charts (committed feature documents under charts).
func (c CLI) chartCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus chart init <name> | regulus chart list")
		return 2
	}
	switch args[0] {
	case "init":
		if len(args) != 2 {
			fmt.Fprintln(c.Stderr, "usage: regulus chart init <name>")
			return 2
		}
		name := strings.TrimSuffix(strings.TrimSuffix(args[1], ".md"), "/")
		if name == "" || strings.ContainsAny(name, `\\/:*?"<>| `) {
			fmt.Fprintln(c.Stderr, "chart init: invalid chart name")
			return 2
		}
		path := filepath.Join("charts", name+".md")
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(c.Stderr, "chart init: %s already exists\n", path)
			return 1
		}
		if err := os.MkdirAll("charts", 0o755); err != nil {
			fmt.Fprintf(c.Stderr, "chart init: %v\n", err)
			return 1
		}
		scaffold := "# " + name + "\n\nDomain: <name>\nClass: STANDARD\n\nWhat this feature is and why it exists.\n\n## Contract\n\n- Objective: what changes and why\n- Scope: what is in and out\n- Constraints: what must never happen\n- Acceptance: the happy path\n- Artifacts: files and commands the task produces\n- Dependencies: none\n- Risk: standard\n- Verification: gauntlet + receipts\n- Completion: scenarios PROVEN with receipt ids\n\n## Acceptance\n\n### Scenario: the happy path\n\n- Given a clean project\n- When the change runs\n- Then the exit code is 0\n\n## Tasks\n\n| ID | Spec | Work | Route | Depends | Status |\n|----|------|------|-------|---------|--------|\n| T1 | the happy path | implement | inline | - | todo |\n\n## Log\n\n- Chart scaffolded by regulus chart init.\n"
		if err := os.WriteFile(path, []byte(scaffold), 0o644); err != nil {
			fmt.Fprintf(c.Stderr, "chart init: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "chart: %s\n", path)
		fmt.Fprintln(c.Stdout, "set the Domain: line to the lesson domain, then fill the Acceptance scenarios before implementation (regulus spec check "+path+")")
		return 0
	case "list":
		paths, err := filepath.Glob(filepath.Join("charts", "*.md"))
		if err != nil {
			fmt.Fprintf(c.Stderr, "chart list: %v\n", err)
			return 1
		}
		if len(paths) == 0 {
			fmt.Fprintln(c.Stdout, "(no charts)")
			return 0
		}
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				fmt.Fprintf(c.Stdout, "  %-30s (unreadable: %v)\n", p, err)
				continue
			}
			scenarios, violations := spec.Check(string(data))
			status := fmt.Sprintf("OK (%d scenario%s)", scenarios, plural(scenarios))
			if len(violations) > 0 {
				status = fmt.Sprintf("INVALID (%d violation%s)", len(violations), plural(len(violations)))
			}
			fmt.Fprintf(c.Stdout, "  %-30s %s\n", p, status)
		}
		return 0
	default:
		fmt.Fprintf(c.Stderr, "chart: unknown subcommand %q\n", args[0])
		return 2
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
