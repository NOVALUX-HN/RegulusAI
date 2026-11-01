package almanac

// M3 portable domain bundles: ExportDomain writes the same bundle shape
// as Export (almanac.jsonl + pins.jsonl + relations.jsonl + status.jsonl
// + INDEX.md) filtered to one domain, so a team can commit a domain slice
// — Engram's `.engram/` pattern: portable and committable — and restore
// it on another machine with `memory import`, which merges idempotently
// by id (re-importing skips what is already present; events pointing at
// observations outside the bundle skip with a warning). An empty domain
// exports everything, so `--out` alone relocates the full bundle.
//
// The stores are only read, never written: the JSONL log stays the
// single source of truth, and the store format is untouched (M2 owns it).
// Lines no domain can be attributed to (no parseable id) are preserved
// verbatim, exactly as Export copies the log; import judges malformed
// lines strictly in both forms.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExportDomain writes a portable bundle of one domain ("" = everything)
// into outDir and returns the written paths. Missing store files export
// as empty bundle files — the bundle shape is stable, and `memory import`
// of a bundle without the extra files still restores the observations.
func ExportDomain(root, domain, outDir string) ([]string, error) {
	scoped, keep, pins, rels, err := domainScope(root, domain)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	return writeDomainBundle(root, outDir, domain, scoped, keep, pins, rels)
}

// domainScope reads the log and returns the observations of one domain
// ("" = everything) with their id set, the pin set, and the relations
// wholly inside the scope.
func domainScope(root, domain string) (scoped []Observation, keep map[string]bool, pins map[string]bool, rels []Relation, err error) {
	all, err := All(root)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	keep = map[string]bool{}
	for _, o := range all {
		if domain == "" || o.Domain == domain {
			keep[o.ID] = true
			scoped = append(scoped, o)
		}
	}
	pins, err = PinnedIDs(root)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	allRels, err := Relations(root)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	for _, r := range allRels {
		if keep[r.From] && keep[r.To] {
			rels = append(rels, r)
		}
	}
	return scoped, keep, pins, rels, nil
}

// writeDomainBundle writes the five bundle files into outDir.
func writeDomainBundle(root, outDir, domain string, scoped []Observation, keep, pins map[string]bool, rels []Relation) ([]string, error) {
	jsonl := filepath.Join(outDir, "almanac.jsonl")
	if err := writeFilteredLines(File(root), jsonl, func(line string) bool {
		id := lineID(line)
		return id == "" || keep[id]
	}); err != nil {
		return nil, err
	}
	index := filepath.Join(outDir, "INDEX.md")
	if err := os.WriteFile(index, []byte(renderDomainIndex(ProjectName(root), domain, scoped, pins, rels)), 0o644); err != nil {
		return nil, err
	}
	paths := []string{jsonl, index}
	filtered := []struct {
		src  string
		name string
		keep func(line string) bool
	}{
		{pinsFile(root), "pins.jsonl", func(line string) bool { return keep[lineID(line)] }},
		{relationsFile(root), "relations.jsonl", keepRelationLine(keep)},
		{StatusFile(root), "status.jsonl", keepStatusLine(keep)},
	}
	for _, s := range filtered {
		dst := filepath.Join(outDir, s.name)
		data, err := os.ReadFile(s.src)
		if os.IsNotExist(err) {
			data = []byte("")
		} else if err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, selectLines(data, s.keep), 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, dst)
	}
	return paths, nil
}

// renderDomainIndex builds the bundle INDEX.md for the scope.
func renderDomainIndex(project, domain string, scoped []Observation, pins map[string]bool, rels []Relation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Almanac export — %s\n\n", project)
	if domain == "" {
		fmt.Fprintf(&b, "%d observation(s), %d pinned, %d relation(s).\n\n", len(scoped), countPinned(scoped, pins), len(rels))
	} else {
		fmt.Fprintf(&b, "domain %q: %d observation(s), %d pinned, %d relation(s).\n\n", domain, len(scoped), countPinned(scoped, pins), len(rels))
	}
	b.WriteString("## Pinned\n\n")
	if !writePinnedLines(&b, scoped, pins) {
		b.WriteString("_(none)_\n")
	}
	b.WriteString("\n## Relations\n\n")
	if len(rels) == 0 {
		b.WriteString("_(none)_\n")
	} else {
		for _, r := range rels {
			fmt.Fprintf(&b, "- `%s` **%s** `%s`\n", r.From, r.Relation, r.To)
		}
	}
	b.WriteString("\n## Observations\n\n")
	for _, o := range scoped {
		fmt.Fprintf(&b, "### `%s` — %s: %s\n\n- saved: %s\n", o.ID, o.Type, o.Title, o.SavedAt.Format(time.RFC3339))
		if o.Lesson != nil {
			fmt.Fprintf(&b, "- cue: %s\n- lesson: %s\n- confidence: %s\n", o.Lesson.Cue, o.Lesson.Lesson, o.Lesson.Confidence)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// writePinnedLines renders the scoped pinned observations; it reports
// whether any line was written.
func writePinnedLines(b *strings.Builder, scoped []Observation, pins map[string]bool) bool {
	any := false
	for _, o := range scoped {
		if pins[o.ID] {
			any = true
			fmt.Fprintf(b, "- `%s` %s: %s\n", o.ID, o.Type, o.Title)
		}
	}
	return any
}

// countPinned counts the scoped observations whose last pin event pins them.
func countPinned(scoped []Observation, pins map[string]bool) int {
	n := 0
	for _, o := range scoped {
		if pins[o.ID] {
			n++
		}
	}
	return n
}

// lineID extracts the observation id of one JSONL line; "" when the line
// carries no parseable id.
func lineID(line string) string {
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(line), &v); err != nil {
		return ""
	}
	return strings.TrimSpace(v.ID)
}

// keepRelationLine keeps a relations.jsonl line when both endpoints
// survive in the bundle. A line without parseable endpoints is preserved
// verbatim — Export copies stores verbatim, and import judges malformed
// lines strictly.
func keepRelationLine(keep map[string]bool) func(string) bool {
	return func(line string) bool {
		var r Relation
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return true
		}
		return keep[r.From] && keep[r.To]
	}
}

// keepStatusLine keeps a status.jsonl line when its observation survives
// in the bundle. Unparseable lines are preserved verbatim, as above.
func keepStatusLine(keep map[string]bool) func(string) bool {
	return func(line string) bool {
		var ev StatusEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return true
		}
		return keep[strings.TrimSpace(ev.ID)]
	}
}

// selectLines keeps the non-empty lines the predicate accepts. Lines the
// predicate cannot attribute (no parseable id) are kept by the caller's
// predicate returning true — a filtered bundle never silently drops
// history it cannot read.
func selectLines(data []byte, include func(line string) bool) []byte {
	var b strings.Builder
	for _, raw := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if include(raw) {
			b.WriteString(raw + "\n")
		}
	}
	return []byte(b.String())
}

// writeFilteredLines copies a store file keeping only attributed lines.
// A missing source writes an empty bundle file, never an omission.
func writeFilteredLines(src, dst string, include func(line string) bool) error {
	data, err := os.ReadFile(src)
	if os.IsNotExist(err) {
		data = []byte("")
	} else if err != nil {
		return err
	}
	return os.WriteFile(dst, selectLines(data, include), 0o644)
}
