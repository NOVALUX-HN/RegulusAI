// NOV-52: registry sin duplicados, precedencia proyecto>global.
package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRegSkill(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: desc " + name + "\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryPrecedenceNoDuplicates(t *testing.T) {
	proj := t.TempDir()
	glob := t.TempDir()
	writeRegSkill(t, filepath.Join(proj, ".opencode", "skills"), "regulus")
	writeRegSkill(t, filepath.Join(proj, ".agents", "skills"), "regulus") // dup, pierde
	writeRegSkill(t, filepath.Join(glob, ".claude", "skills"), "extra")

	got := Registry(proj, glob)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (regulus+extra) got %v", len(got), got)
	}
	if got[0].Name != "extra" || got[1].Name != "regulus" {
		t.Fatalf("order = %v, want [extra regulus]", got)
	}
	if !contains(got[1].Source, ".opencode") {
		t.Fatalf("regulus source = %q, want proyecto .opencode (precedencia)", got[1].Source)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
