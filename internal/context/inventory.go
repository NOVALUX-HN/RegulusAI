// Corpus inventory (F12.3/NOV-98): code, symbols, dependencies,
// docs, ADRs, issues and memory — every fragment with source,
// version/date and a bounded body.
//
// Bounds: MaxFiles caps the walk, MaxFileBytes caps one file body,
// MaxSymbolsPerFile caps symbol extraction. Anything beyond warns
// instead of truncating silently. Memory comes from almanac.All
// (provenance, domain, expiry preserved); issues come from an
// explicit JSON file (strict, bounded) because the repo holds no
// local issue store.
package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
)

// InventoryConfig bounds one inventory pass over a project root.
type InventoryConfig struct {
	GitHead           string
	MaxFiles          int
	MaxFileBytes      int64
	MaxSymbolsPerFile int
	MaxIssues         int
	IssuesFile        string
	IncludeMemory     bool
	Now               time.Time
}

// DefaultInventoryConfig is the bounded default: 500 files, 8KB per
// file body, 50 symbols per file, 1000 issues.
func DefaultInventoryConfig() InventoryConfig {
	return InventoryConfig{
		MaxFiles:          500,
		MaxFileBytes:      8 * 1024,
		MaxSymbolsPerFile: 50,
		MaxIssues:         1000,
		IncludeMemory:     true,
	}
}

// issueFileEntry is one issue from the explicit JSON file.
type issueFileEntry struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Critical bool   `json:"critical"`
	Scope    string `json:"scope"`
}

// symbolRe extracts top-level Go declarations: `func Name` (with an
// optional (receiver)) and `type Name`.
var symbolRe = regexp.MustCompile(`(?m)^(?:func\s+(?:\([^)]*\)\s*)?([A-ZA-Za-z_][\w]*)|^type\s+([A-ZA-Za-z_][\w]*))`)

// requireRe matches one go.mod requirement line inside a require
// block or a single-line require: "<module> v<version>".
var requireRe = regexp.MustCompile(`^\s*([a-zA-Z0-9._~/-]+\.[a-zA-Z]+/[a-zA-Z0-9._~/+-]*[a-zA-Z0-9_+/-])\s+(v\d+\S*)`)

// skipDirs are never inventoried (history, caches, vendored code).
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true,
	".regulus": true, "bin": true, "dist": true, "target": true,
}

// Inventory walks root and folds the corpus: Go sources (one code
// fragment plus symbol fragments per file), go.mod requirements,
// docs and ADRs, the optional issues file and the Almanac memory.
// It returns the corpus plus warnings (skips, truncations,
// malformed lines) — never an error for a partially readable tree,
// only for an unreadable root, issues file or memory store.
func Inventory(root string, cfg InventoryConfig) (Corpus, []string, error) {
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = DefaultInventoryConfig().MaxFiles
	}
	if cfg.MaxFileBytes <= 0 {
		cfg.MaxFileBytes = DefaultInventoryConfig().MaxFileBytes
	}
	if cfg.MaxSymbolsPerFile <= 0 {
		cfg.MaxSymbolsPerFile = DefaultInventoryConfig().MaxSymbolsPerFile
	}
	if cfg.MaxIssues <= 0 {
		cfg.MaxIssues = DefaultInventoryConfig().MaxIssues
	}
	now := cfg.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	version := cfg.GitHead
	if version == "" {
		version = "worktree"
	}
	var frags []Fragment
	var warnings []string
	files := 0
	truncated := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		base := strings.ToLower(d.Name())
		isGo := strings.HasSuffix(base, ".go") && !strings.HasSuffix(base, "_test.go")
		isDoc := strings.HasSuffix(base, ".md") && (strings.HasPrefix(rel, "docs/") || strings.HasPrefix(rel, "charts/"))
		isMod := base == "go.mod"
		if !isGo && !isDoc && !isMod {
			return nil
		}
		if files >= cfg.MaxFiles {
			warnings = append(warnings, fmt.Sprintf("inventario acotado a %d ficheros: resto omitido (sube MaxFiles para ampliar)", cfg.MaxFiles))
			return filepath.SkipAll
		}
		files++
		info, ierr := d.Info()
		if ierr != nil {
			warnings = append(warnings, fmt.Sprintf("sin metadatos %s: omitido (%v)", rel, ierr))
			return nil
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			warnings = append(warnings, fmt.Sprintf("ilegible %s: omitido (%v)", rel, rerr))
			return nil
		}
		body := string(raw)
		if int64(len(body)) > cfg.MaxFileBytes {
			body = body[:cfg.MaxFileBytes]
			truncated++
		}
		date := info.ModTime().UTC()
		switch {
		case isMod:
			frags = append(frags, dependencyFragments(rel, version, date, body)...)
		case isGo:
			frags = append(frags, Fragment{
				ID: "code/" + rel, Kind: KindCode, Source: rel,
				Version: version, Date: date,
				Title:   "source " + rel,
				Content: body,
			})
			frags = append(frags, symbolFragments(rel, version, date, body, cfg.MaxSymbolsPerFile, &warnings)...)
		case isDoc:
			kind := KindDoc
			critical := false
			if strings.HasPrefix(rel, "docs/adr/") {
				kind, critical = KindADR, true
			}
			frags = append(frags, Fragment{
				ID: kind + "/" + rel, Kind: kind, Source: rel,
				Version: version, Date: date, Critical: critical,
				Title:   docTitle(body, rel),
				Content: body,
			})
		}
		return nil
	})
	if err != nil {
		return Corpus{}, warnings, fmt.Errorf("context: inventario de %s: %w", root, err)
	}
	if truncated > 0 {
		warnings = append(warnings, fmt.Sprintf("%d fichero(s) truncado(s) a %d bytes (cuerpo acotado, no transcript completo)", truncated, cfg.MaxFileBytes))
	}
	if cfg.IssuesFile != "" {
		issues, w, err := inventoryIssues(cfg.IssuesFile, version, cfg.MaxIssues)
		warnings = append(warnings, w...)
		if err != nil {
			return Corpus{}, warnings, err
		}
		frags = append(frags, issues...)
	}
	if cfg.IncludeMemory {
		mem, w, err := inventoryMemory(root, version, now)
		warnings = append(warnings, w...)
		if err != nil {
			return Corpus{}, warnings, err
		}
		frags = append(frags, mem...)
	}
	sort.Slice(frags, func(i, j int) bool { return frags[i].ID < frags[j].ID })
	for i := range frags {
		frags[i].Tokens = CountTokens(frags[i].Title + "\n" + frags[i].Content)
		if frags[i].Trust == "" {
			frags[i].Trust = TrustTrusted
		}
	}
	return Corpus{Fragments: frags}, warnings, nil
}

// symbolFragments extracts one fragment per top-level Go
// declaration (name + file:line as content locator, not the body —
// symbols index, code carries the bytes).
func symbolFragments(rel, version string, date time.Time, body string, max int, warnings *[]string) []Fragment {
	locs := symbolRe.FindAllStringSubmatchIndex(body, -1)
	var out []Fragment
	seen := map[string]bool{}
	for _, loc := range locs {
		name := ""
		for g := 2; g <= 5; g += 2 {
			if loc[g] >= 0 {
				name = body[loc[g]:loc[g+1]]
				break
			}
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if len(out) >= max {
			*warnings = append(*warnings, fmt.Sprintf("símbolos de %s acotados a %d (resto omitido)", rel, max))
			break
		}
		line := 1 + strings.Count(body[:loc[0]], "\n")
		out = append(out, Fragment{
			ID: "symbol/" + rel + "#" + name, Kind: KindSymbol, Source: rel,
			Version: version, Date: date,
			Title:   "symbol " + name + " in " + rel,
			Content: fmt.Sprintf("%s:%d", rel, line),
		})
	}
	return out
}

// dependencyFragments folds one go.mod require line into a fragment
// (module@version with the go.mod locator).
func dependencyFragments(rel, version string, date time.Time, body string) []Fragment {
	var out []Fragment
	inRequire := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "require (") {
			inRequire = true
			continue
		}
		if inRequire && t == ")" {
			inRequire = false
			continue
		}
		if !inRequire && !strings.HasPrefix(t, "require ") {
			continue
		}
		m := requireRe.FindStringSubmatch(strings.TrimPrefix(t, "require "))
		if m == nil && inRequire {
			m = requireRe.FindStringSubmatch(t)
		}
		if m == nil {
			continue
		}
		out = append(out, Fragment{
			ID: "dependency/" + m[1], Kind: KindDependency, Source: rel,
			Version: m[2], Date: date,
			Title:   "dependency " + m[1],
			Content: fmt.Sprintf("%s %s (from %s)", m[1], m[2], rel),
		})
	}
	return out
}

// docTitle takes the first Markdown heading as the fragment title,
// falling back to the path.
func docTitle(body, rel string) string {
	for _, line := range strings.Split(body, "\n") {
		if t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#")); strings.HasPrefix(strings.TrimSpace(line), "#") && t != "" {
			return t
		}
	}
	return "doc " + rel
}

// inventoryIssues reads the explicit issues JSON file: strict shape
// (unknown fields refuse), bounded count and bytes — hostile input
// fails closed instead of polluting the corpus.
func inventoryIssues(path, version string, max int) ([]Fragment, []string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("context: issues %s: %w", path, err)
	}
	if len(raw) > 1024*1024 {
		return nil, nil, fmt.Errorf("context: issues %s too large (%d bytes, max 1MB)", path, len(raw))
	}
	var entries []issueFileEntry
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&entries); err != nil {
		return nil, nil, fmt.Errorf("context: malformed issues %s: %v", path, err)
	}
	if len(entries) > max {
		return nil, nil, fmt.Errorf("context: issues %s holds %d entries (max %d)", path, len(entries), max)
	}
	var out []Fragment
	for _, e := range entries {
		if strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.Title) == "" {
			return nil, nil, fmt.Errorf("context: issues %s: entry needs id and title", path)
		}
		out = append(out, Fragment{
			ID: "issue/" + e.ID, Kind: KindIssue, Source: "issues:" + e.ID,
			Version: version, Critical: e.Critical, RequiredScope: e.Scope,
			Title:   e.Title,
			Content: e.Body,
		})
	}
	return out, nil, nil
}

// inventoryMemory folds the Almanac into fragments: provenance
// (task, paths, receipts) joins the source locator, domain and
// expiry are preserved, high-confidence lessons read as critical,
// and imported observations read as untrusted (never laundered).
func inventoryMemory(root, version string, now time.Time) ([]Fragment, []string, error) {
	all, err := almanac.All(root)
	if err != nil {
		return nil, nil, fmt.Errorf("context: memoria: %w", err)
	}
	var out []Fragment
	for _, o := range all {
		source := "almanac:" + o.ID
		if o.Provenance != nil && len(o.Provenance.Paths) > 0 {
			source = "almanac:" + o.ID + " (" + strings.Join(o.Provenance.Paths, ", ") + ")"
		}
		trust := TrustTrusted
		if o.Source == almanac.SourceImport {
			trust = TrustUntrusted
		}
		critical := o.Type == almanac.TypeLesson && o.Lesson != nil && o.Lesson.Confidence == "high"
		out = append(out, Fragment{
			ID: "memory/" + o.ID, Kind: KindMemory, Source: source,
			Version: version, Date: o.SavedAt, Domain: o.Domain,
			Title: o.Title, Content: o.Content, Critical: critical,
			RequiredScope: RequiredScopeFor(KindMemory),
			Trust:         trust,
			Expired:       almanac.IsExpired(&o, now),
		})
	}
	return out, nil, nil
}
