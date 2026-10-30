package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, root, dir, frontmatter string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".opencode", "skills", dir), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\n" + frontmatter + "\n---\n\n## What I do\nThings.\n"
	if err := os.WriteFile(filepath.Join(root, ".opencode", "skills", dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListParsesNameAndDescription(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "wf", "name: my-flow\ndescription: Fires when work starts.")
	writeSkill(t, root, "zz", "name: zz-late\ndescription: Late skill.")
	all, err := List(root)
	if err != nil || len(all) != 2 || all[0].Name != "my-flow" || all[1].Name != "zz-late" {
		t.Fatalf("list = %+v (%v)", all, err)
	}
	if all[0].Description != "Fires when work starts." {
		t.Fatalf("description = %q", all[0].Description)
	}
}

func TestListMissingDirIsEmpty(t *testing.T) {
	all, err := List(t.TempDir())
	if err != nil || len(all) != 0 {
		t.Fatalf("missing dir = %+v (%v)", all, err)
	}
}

func TestListSkipsUnparseableAndKeepsDirName(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".opencode", "skills", "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".opencode", "skills", "plain", "SKILL.md"), []byte("# no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := List(root)
	if err != nil || len(all) != 1 || all[0].Name != "plain" || all[0].Description != "" {
		t.Fatalf("fallback = %+v (%v)", all, err)
	}
}
