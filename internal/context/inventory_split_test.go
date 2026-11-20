// Split directo del inventario (CRAP>30 sin cambiar comportamiento):
// helpers puros más tabla del orquestador delgado. Todo en proceso
// sobre t.TempDir(), portable Linux+Windows: sin red, shell ni git.
package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// splitWrite guarda un fichero bajo root, creando los padres.
func splitWrite(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func splitNow() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }

func TestSplitNormalizeConfigDefaults(t *testing.T) {
	cfg, now, version := normalizeConfig(InventoryConfig{})
	def := DefaultInventoryConfig()
	if cfg.MaxFiles != def.MaxFiles || cfg.MaxFileBytes != def.MaxFileBytes ||
		cfg.MaxSymbolsPerFile != def.MaxSymbolsPerFile || cfg.MaxIssues != def.MaxIssues {
		t.Fatalf("defaults drifted: %+v", cfg)
	}
	if version != "worktree" {
		t.Fatalf("version = %q, want worktree", version)
	}
	if now.IsZero() {
		t.Fatal("zero Now must resolve to now UTC")
	}
}

func TestSplitNormalizeConfigKeepsExplicit(t *testing.T) {
	fixed := splitNow()
	in := InventoryConfig{
		GitHead: "abc1234", MaxFiles: 7, MaxFileBytes: 11,
		MaxSymbolsPerFile: 3, MaxIssues: 5, Now: fixed,
	}
	cfg, now, version := normalizeConfig(in)
	if cfg.MaxFiles != 7 || cfg.MaxFileBytes != 11 || cfg.MaxSymbolsPerFile != 3 || cfg.MaxIssues != 5 {
		t.Fatalf("explicit bounds must survive: %+v", cfg)
	}
	if !now.Equal(fixed) {
		t.Fatalf("explicit now must survive: %v", now)
	}
	if version != "abc1234" {
		t.Fatalf("explicit head must survive: %q", version)
	}
}

func TestSplitClassifyPathTable(t *testing.T) {
	for _, tc := range []struct {
		rel, name, want string
	}{
		{"hello.go", "hello.go", pathGo},
		{"pkg/a.go", "a.go", pathGo},
		{"hello_test.go", "hello_test.go", pathSkip},
		{"go.mod", "go.mod", pathMod},
		{"docs/guide.md", "guide.md", pathDoc},
		{"charts/c.md", "c.md", pathDoc},
		{"docs/adr/001.md", "001.md", pathDoc},
		{"README.md", "README.md", pathSkip},
		{"docs/notes.txt", "notes.txt", pathSkip},
		{"pkg/a.go", "A.GO", pathGo},
		{"docs/guide.md", "GUIDE.MD", pathDoc},
	} {
		if got := classifyPath(tc.rel, tc.name); got != tc.want {
			t.Errorf("classifyPath(%q,%q) = %q, want %q", tc.rel, tc.name, got, tc.want)
		}
	}
}

func TestSplitAppendFileFragmentGoDocMod(t *testing.T) {
	root := t.TempDir()
	splitWrite(t, root, "hello.go", "package p\n\nfunc Hello() {}\n")
	splitWrite(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n\nrequire github.com/foo/bar v1.2.3\n")
	splitWrite(t, root, "docs/guide.md", "# Title\n\nBody.\n")
	cfg, _, version := normalizeConfig(InventoryConfig{IncludeMemory: false})
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]os.DirEntry{}
	for _, e := range entries {
		byName[e.Name()] = e
	}
	// docs/guide.md vive en subdir: se lista aparte.
	docEntries, err := os.ReadDir(filepath.Join(root, "docs"))
	if err != nil {
		t.Fatal(err)
	}
	byName["guide.md"] = docEntries[0]

	var frags []Fragment
	var warnings []string
	truncated := 0
	appendFileFragment(cfg, version, "hello.go", filepath.Join(root, "hello.go"), pathGo, byName["hello.go"], &frags, &warnings, &truncated)
	appendFileFragment(cfg, version, "go.mod", filepath.Join(root, "go.mod"), pathMod, byName["go.mod"], &frags, &warnings, &truncated)
	appendFileFragment(cfg, version, "docs/guide.md", filepath.Join(root, "docs", "guide.md"), pathDoc, byName["guide.md"], &frags, &warnings, &truncated)
	byID := map[string]Fragment{}
	for _, f := range frags {
		byID[f.ID] = f
	}
	if _, ok := byID["code/hello.go"]; !ok {
		t.Fatalf("go code missing: %v", byID)
	}
	if _, ok := byID["symbol/hello.go#Hello"]; !ok {
		t.Fatalf("go symbol missing: %v", byID)
	}
	if dep, ok := byID["dependency/github.com/foo/bar"]; !ok || !strings.Contains(dep.Content, "from go.mod") {
		t.Fatalf("go.mod dependency missing: %v", byID)
	}
	if doc, ok := byID["doc/docs/guide.md"]; !ok || doc.Title != "Title" {
		t.Fatalf("doc fragment missing: %v", byID)
	}
	if len(warnings) != 0 || truncated != 0 {
		t.Fatalf("clean files must not warn/truncate: %v %d", warnings, truncated)
	}
}

func TestSplitAppendFileFragmentTruncatesAndWarns(t *testing.T) {
	root := t.TempDir()
	splitWrite(t, root, "big.go", "package big\n\n// "+strings.Repeat("x", 200)+"\n")
	cfg, _, version := normalizeConfig(InventoryConfig{MaxFileBytes: 16, IncludeMemory: false})
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var frags []Fragment
	var warnings []string
	truncated := 0
	appendFileFragment(cfg, version, "big.go", filepath.Join(root, "big.go"), pathGo, entries[0], &frags, &warnings, &truncated)
	if truncated != 1 {
		t.Fatalf("truncated = %d, want 1", truncated)
	}
	for _, f := range frags {
		if f.ID == "code/big.go" && int64(len(f.Content)) > 16 {
			t.Fatalf("body = %d bytes, want <= 16", len(f.Content))
		}
	}
}

func TestSplitFinalizeCorpusSortsMeasures(t *testing.T) {
	cfg, now, version := normalizeConfig(InventoryConfig{IncludeMemory: false})
	frags := []Fragment{
		{ID: "b", Kind: KindDoc, Source: "docs/b.md", Title: "b", Content: "body b"},
		{ID: "a", Kind: KindCode, Source: "a.go", Title: "a", Content: "body a"},
	}
	out, warnings, err := finalizeCorpus(t.TempDir(), cfg, version, now, frags, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "truncado") {
		t.Fatalf("truncation must warn, got %v", warnings)
	}
	if len(out.Fragments) != 2 || out.Fragments[0].ID != "a" || out.Fragments[1].ID != "b" {
		t.Fatalf("corpus must sort by id: %v", out.Fragments)
	}
	for _, f := range out.Fragments {
		if f.Tokens <= 0 || f.Trust != TrustTrusted {
			t.Fatalf("fragments must measure trust/tokens: %+v", f)
		}
	}
}

func TestSplitFinalizeCorpusIssuesStrict(t *testing.T) {
	root := t.TempDir()
	issuesPath := filepath.Join(root, "issues.json")
	if err := os.WriteFile(issuesPath, []byte(`[{"id":"i1","title":"First","body":"details"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, now, version := normalizeConfig(InventoryConfig{IssuesFile: issuesPath, IncludeMemory: false})
	out, _, err := finalizeCorpus(root, cfg, version, now, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range out.Fragments {
		if f.ID == "issue/i1" && f.Title == "First" {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues must fold: %v", out.Fragments)
	}
	badPath := filepath.Join(root, "bad.json")
	if err := os.WriteFile(badPath, []byte(`[{"id":"1","title":"T","bogus":"x"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	badCfg, _, badVersion := normalizeConfig(InventoryConfig{IssuesFile: badPath, IncludeMemory: false})
	if _, _, err := finalizeCorpus(root, badCfg, badVersion, now, nil, nil, 0); err == nil {
		t.Fatal("strict issues must refuse unknown fields")
	}
}

func TestSplitInventoryThinOrchestratorTable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root string)
		cfg   func() InventoryConfig
		check func(t *testing.T, corpus Corpus, warnings []string)
	}{
		{
			name:  "empty",
			setup: func(t *testing.T, root string) {},
			cfg:   func() InventoryConfig { return InventoryConfig{IncludeMemory: false} },
			check: func(t *testing.T, corpus Corpus, warnings []string) {
				if len(corpus.Fragments) != 0 {
					t.Fatalf("empty root must inventory nothing, got %v", corpus.Fragments)
				}
			},
		},
		{
			name: "go",
			setup: func(t *testing.T, root string) {
				splitWrite(t, root, "hello.go", "package p\n\nfunc Hello() {}\n")
			},
			cfg: func() InventoryConfig { return InventoryConfig{IncludeMemory: false} },
			check: func(t *testing.T, corpus Corpus, warnings []string) {
				found := false
				for _, f := range corpus.Fragments {
					if f.ID == "code/hello.go" && f.Version == "worktree" {
						found = true
					}
				}
				if !found {
					t.Fatalf("thin orchestrator must fold go: %v", corpus.Fragments)
				}
			},
		},
		{
			name: "cap",
			setup: func(t *testing.T, root string) {
				splitWrite(t, root, "a.go", "package a\n\nfunc A() {}\n")
				splitWrite(t, root, "b.go", "package b\n\nfunc B() {}\n")
			},
			cfg: func() InventoryConfig { return InventoryConfig{MaxFiles: 1, IncludeMemory: false} },
			check: func(t *testing.T, corpus Corpus, warnings []string) {
				if !strings.Contains(strings.Join(warnings, "\n"), "inventario acotado a 1 ficheros") {
					t.Fatalf("cap must warn, got %v", warnings)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(t, root)
			corpus, warnings, err := Inventory(root, tc.cfg())
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, corpus, warnings)
		})
	}
}
