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
      "git rm*": "deny",
      "git checkout --*": "deny",
      "git restore*": "deny",
      "git rebase*": "deny",
      "format *": "deny",
      "mkfs*": "deny",
      "dd *": "deny"
    },
    "edit": {
      "*": "allow",
      "*.env": "ask",
      "*.env.*": "ask",
      "*.env.example": "allow"
    },
    "doom_loop": "ask"
  }
}
`

// yoloSafeBashDenies are the destructive bash patterns safe-YOLO always
// keeps denied, even when flipping an existing project config on.
var yoloSafeBashDenies = []string{
	"rm *", "rmdir *", "del *", "rd *",
	"git reset --hard*", "git clean *", "git push *force*",
	"git branch -D*", "git stash drop*",
	"git rm*", "git checkout --*", "git restore*", "git rebase*",
	"format *", "mkfs*", "dd *",
}

// yoloEditEnvRules are the .env guardrails layered after the edit
// catch-all (last-match-wins): secrets ask, examples stay allowed.
var yoloEditEnvRules = [][2]string{
	{"*.env", "ask"},
	{"*.env.*", "ask"},
	{"*.env.example", "allow"},
}

// editStar reads the edit catch-all: a plain string, or the "*" entry
// of the granular object form. Unknown shapes report "ask" (safe).
func editStar(v any) string {
	switch t := v.(type) {
	case string:
		if t == "allow" {
			return "allow"
		}
		return "ask"
	case map[string]any:
		if s, _ := t["*"].(string); s == "allow" {
			return "allow"
		}
		return "ask"
	}
	return "ask"
}

// setEditStar flips only the edit catch-all, preserving every specific
// rule; a plain string becomes the granular object with the .env
// guardrails so secrets still ask when everything else auto-approves.
func setEditStar(perm map[string]any, want string) {
	switch t := perm["edit"].(type) {
	case string:
		if t == want && want == "ask" {
			return
		}
		m := map[string]any{"*": want}
		for _, r := range yoloEditEnvRules {
			m[r[0]] = r[1]
		}
		perm["edit"] = m
	case map[string]any:
		t["*"] = want
		for _, r := range yoloEditEnvRules {
			if _, ok := t[r[0]]; !ok {
				t[r[0]] = r[1]
			}
		}
	default:
		m := map[string]any{"*": want}
		for _, r := range yoloEditEnvRules {
			m[r[0]] = r[1]
		}
		perm["edit"] = m
	}
}

// ensureYoloDenies adds any missing destructive bash denies without
// touching the catch-all or unrelated keys.
func ensureYoloDenies(bash map[string]any) {
	for _, d := range yoloSafeBashDenies {
		if _, ok := bash[d]; !ok {
			bash[d] = "deny"
		}
	}
}

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
	curEdit := editStar(perm["edit"])
	if mode == "status" {
		fmt.Fprintf(c.Stdout, "yolo: %s (bash %s, edit %s)\n", yoloWord(cur), yoloWord(cur), yoloWord(curEdit))
		return 0
	}
	want := map[string]string{"on": "allow", "off": "ask"}[mode]
	if cur == want {
		if mode == "on" {
			ensureYoloDenies(bash)
		}
		setEditStar(perm, want)
		newEdit := editStar(perm["edit"])
		out, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "yolo: %v\n", err)
			return 1
		}
		if err := os.WriteFile("opencode.json", append(out, '\n'), 0o644); err != nil {
			fmt.Fprintf(c.Stderr, "yolo: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "yolo: already %s (bash %s, edit %s)\n", yoloWord(cur), yoloWord(cur), yoloWord(newEdit))
		return 0
	}
	bash["*"] = want
	if mode == "on" {
		ensureYoloDenies(bash)
	}
	setEditStar(perm, want)
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(c.Stderr, "yolo: %v\n", err)
		return 1
	}
	if err := os.WriteFile("opencode.json", append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(c.Stderr, "yolo: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "yolo: %s (bash %s, edit %s; deletes still denied)\n", mode, mode, mode)
	fmt.Fprintln(c.Stdout, "next: run /reload in opencode (or restart) to apply")
	return 0
}

func yoloWord(v string) string {
	if v == "allow" {
		return "on"
	}
	return "off"
}
