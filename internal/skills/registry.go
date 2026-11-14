// Skill Registry multiagente (NOV-52): índice sin duplicados con
// precedencia local>global y rutas correctas. Solo metadata.
package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RegistryEntry is one indexed skill with its source dir.
type RegistryEntry struct {
	Name   string
	Source string
	Skill  Skill
}

// Registry scans skill dirs in precedence order (first wins, no
// duplicates) and returns a sorted index. Candidate dirs, in order:
// .opencode/skills, .agents/skills, .claude/skills, .devin/skills,
// each under project root then global root.
func Registry(projectRoot, globalRoot string) []RegistryEntry {
	candidates := []string{}
	for _, base := range []string{projectRoot, globalRoot} {
		if base == "" {
			continue
		}
		for _, rel := range []string{
			filepath.Join(".opencode", "skills"),
			filepath.Join(".agents", "skills"),
			filepath.Join(".claude", "skills"),
			filepath.Join(".devin", "skills"),
		} {
			candidates = append(candidates, filepath.Join(base, rel))
		}
	}
	seen := map[string]bool{}
	var out []RegistryEntry
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name(), "SKILL.md"))
			if err != nil {
				continue
			}
			s := Skill{Name: e.Name()}
			name, desc := parseFrontmatter(string(data))
			if name != "" {
				s.Name = name
			}
			s.Description = desc
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			out = append(out, RegistryEntry{Name: s.Name, Source: filepath.Join(dir, e.Name()), Skill: s})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func parseFrontmatter(data string) (string, string) {
	var name, desc string
	for _, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(raw)
		if line == "---" {
			continue
		}
		if v, ok := strings.CutPrefix(line, "name:"); ok && strings.TrimSpace(v) != "" {
			name = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "description:"); ok {
			desc = strings.TrimSpace(v)
		}
		if strings.HasPrefix(line, "## ") {
			break
		}
	}
	return name, desc
}
