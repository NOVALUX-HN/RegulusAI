// Package skills is the machine-readable trigger registry for the
// harness skills (harness2 skill auto-trigger, minimal): name and one-line
// description parsed from each SKILL.md frontmatter, so agents and the
// context hook can list what each skill fires on without reading prose.
package skills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill is one skill's trigger entry.
type Skill struct {
	Name        string
	Description string
}

// Dir returns the skills directory under a project root.
func Dir(root string) string {
	return filepath.Join(root, ".opencode", "skills")
}

// List returns every skill with frontmatter, sorted by name. Skills
// without a parseable name keep their directory name; a missing
// description stays empty. A missing skills dir is empty.
func List(root string) ([]Skill, error) {
	entries, err := os.ReadDir(Dir(root))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(Dir(root), e.Name(), "SKILL.md"))
		if err != nil {
			continue
		}
		s := Skill{Name: e.Name()}
		for _, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(raw)
			if line == "---" {
				continue
			}
			if v, ok := strings.CutPrefix(line, "name:"); ok && strings.TrimSpace(v) != "" {
				s.Name = strings.TrimSpace(v)
			}
			if v, ok := strings.CutPrefix(line, "description:"); ok {
				s.Description = strings.TrimSpace(v)
			}
			if strings.HasPrefix(line, "## ") {
				break
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
