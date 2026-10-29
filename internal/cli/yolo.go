package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

// yoloTemplate is the safe-YOLO opencode.json created when the project
// has none: auto-approve everything except the destructive denies. It
// mirrors the committed opencode.json at the repo root; Go's sorted-key
// marshaling keeps "*" first, which the last-match-wins rule requires.
const yoloTemplate = `{
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "bash": {
      "*": "allow",
      "rm *": "deny",
      "rmdir *": "deny",
      "del *": "deny",
      "rd *": "deny",
      "git reset --hard*": "deny",
      "git clean *": "deny",
      "git push *force*": "deny",
      "git branch -D*": "deny",
      "git stash drop*": "deny",
      "format *": "deny",
      "mkfs*": "deny",
      "dd *": "deny"
    },
    "edit": "allow",
    "doom_loop": "ask"
  }
}
`

// yoloCmd implements `regulus yolo [on|off|status]`: flip the safe-YOLO
// switch in the project's opencode.json without leaving the editor.
// Bare invocation reports status (read-only). on/off rewrites only the
// bash "*" catch-all and preserves every deny and every other key;
// Go's sorted-key marshaling also repairs the catch-all-first order the
// last-match-wins rule requires. Config applies on /reload or restart.
func (c CLI) yoloCmd(args []string) int {
	mode := "status"
	if len(args) > 0 {
		mode = args[0]
	}
	switch mode {
	case "on", "off", "status":
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus yolo [on|off|status]")
		return 2
	}
	raw, err := os.ReadFile("opencode.json")
	if err != nil {
		if mode != "on" {
			fmt.Fprintln(c.Stdout, "yolo: off (no opencode.json in this project — opencode defaults apply)")
			return 0
		}
		if err := os.WriteFile("opencode.json", append([]byte(yoloTemplate), '\n'), 0o644); err != nil {
			fmt.Fprintf(c.Stderr, "yolo: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, "yolo: on (created opencode.json with safe defaults; deletes still denied)")
		fmt.Fprintln(c.Stdout, "next: run /reload in opencode (or restart) to apply")
		return 0
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Fprintf(c.Stderr, "yolo: opencode.json is not plain JSON (%v) — edit it by hand\n", err)
		return 1
	}
	perm, _ := cfg["permission"].(map[string]any)
	if perm == nil {
		fmt.Fprintln(c.Stderr, "yolo: opencode.json has no permission block — edit it by hand")
		return 1
	}
	bash, _ := perm["bash"].(map[string]any)
	if bash == nil {
		fmt.Fprintln(c.Stderr, "yolo: permission.bash is not a pattern map — edit it by hand")
		return 1
	}
	cur, _ := bash["*"].(string)
	if mode == "status" {
		fmt.Fprintf(c.Stdout, "yolo: %s\n", yoloWord(cur))
		return 0
	}
	want := map[string]string{"on": "allow", "off": "ask"}[mode]
	if cur == want {
		fmt.Fprintf(c.Stdout, "yolo: already %s\n", yoloWord(cur))
		return 0
	}
	bash["*"] = want
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(c.Stderr, "yolo: %v\n", err)
		return 1
	}
	if err := os.WriteFile("opencode.json", append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(c.Stderr, "yolo: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "yolo: %s (deletes still denied)\n", mode)
	fmt.Fprintln(c.Stdout, "next: run /reload in opencode (or restart) to apply")
	return 0
}

func yoloWord(v string) string {
	if v == "allow" {
		return "on"
	}
	return "off"
}
