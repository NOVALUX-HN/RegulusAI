// Evidence pruning with a fail-closed CLOSED guard (retention-gc R1, S1).
//
// Prune deletes old receipts under one task dir, keeping the N newest by
// mtime, never touching manifests (manifest-*.json) or corrupt/. Without
// --yes the caller only plans (dry run). Tasks marked CLOSED in charts/
// refuse without --force.
package evidence

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"regulus-ai/internal/receipts"
)

// candidate is one deletable receipt file with its mtime for ordering.
type candidate struct {
	path    string
	name    string
	modTime int64
	nano    int64
}

// SplitSlug splits a normalized task slug into chart base and task id.
// "win-hardening__H1" -> ("win-hardening", "H1"). A slug without "__"
// yields ("<slug>", "").
func SplitSlug(slug string) (chart, id string) {
	parts := strings.Split(slug, "__")
	if len(parts) == 1 {
		return slug, ""
	}
	return parts[0], parts[len(parts)-1]
}

// Plan lists what `prune --task <task> --keep N` would delete. Receipts
// are ordered by mtime (oldest first, name tie-break); the N newest are
// kept. Manifests (manifest-*.json) are counted but never listed for
// deletion; corrupt/ is never touched. A missing task dir is a clean
// error.
func Plan(root, task string, keep int) (toDelete, toKeep []string, manifests int, err error) {
	if keep < 0 {
		return nil, nil, 0, fmt.Errorf("keep must be >= 0")
	}
	slug := receipts.Slug(task)
	dir, err := TaskDirSafe(root, task)
	if err != nil {
		return nil, nil, 0, err
	}
	fi, statErr := os.Stat(dir)
	if statErr != nil || !fi.IsDir() {
		return nil, nil, 0, fmt.Errorf("no evidence for task %s", slug)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, 0, err
	}
	var cands []candidate
	for _, e := range entries {
		if e.IsDir() {
			// corrupt/ and any other subdir: never touched.
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "manifest-") && strings.HasSuffix(name, ".json") {
			manifests++
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, nil, 0, err
		}
		mt := info.ModTime()
		cands = append(cands, candidate{
			path:    filepath.Join(dir, name),
			name:    name,
			modTime: mt.UnixNano(),
			nano:    mt.UnixNano(),
		})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].modTime != cands[j].modTime {
			return cands[i].modTime < cands[j].modTime
		}
		return cands[i].name < cands[j].name
	})
	paths := make([]string, len(cands))
	for i, c := range cands {
		paths[i] = c.path
	}
	if keep >= len(paths) {
		return nil, paths, manifests, nil
	}
	cut := len(paths) - keep
	if keep == 0 {
		return paths, nil, manifests, nil
	}
	return paths[:cut], paths[cut:], manifests, nil
}

// Delete removes every path (os.Remove, in order) and returns how many
// were removed. It never touches manifests or corrupt/: callers must pass
// only Plan's toDelete list.
func Delete(paths []string) (int, error) {
	n := 0
	for _, p := range paths {
		base := filepath.Base(p)
		if strings.HasPrefix(base, "manifest-") {
			return n, fmt.Errorf("refusing to delete manifest %s", p)
		}
		if strings.Contains(filepath.ToSlash(p), "/corrupt/") {
			return n, fmt.Errorf("refusing to delete quarantined file %s", p)
		}
		if err := os.Remove(p); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

var (
	closedWordRe  = regexp.MustCompile(`(?i)\bclosed\b`)
	chartClosedRe = regexp.MustCompile(`(?i)\bchart\b[^\n]{0,80}\b(cerrado|closed)\b`)
	rangeRe       = regexp.MustCompile(`([A-Za-z]+)(\d+)\s*-\s*([A-Za-z]*)(\d+)`)
	idRe          = regexp.MustCompile(`^([A-Za-z]+)(\d+)$`)
)

// IsClosed reports whether charts/ marks the task's id as CLOSED.
// slugInput accepts "chart/ID" or "chart__ID" (normalized via
// receipts.Slug). The chart base (before "__") selects relevant files:
// a file is relevant when its basename matches the chart base or its
// content mentions it. Within relevant files, a chart-wide
// "Chart ... CERRADO/CLOSED" closes every id, otherwise the id must be
// named ("H1 CLOSED") or covered by a range ("H1-H4 CLOSED", en/em
// dashes normalized). A missing charts/ dir means not closed.
func IsClosed(root, task string) (bool, error) {
	slug := receipts.Slug(task)
	chart, id := SplitSlug(slug)
	chartsDir := filepath.Join(root, "charts")
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(chartsDir, e.Name()))
		if err != nil {
			continue
		}
		content := string(data)
		base := strings.TrimSuffix(e.Name(), ".md")
		if base != chart && !strings.Contains(content, chart) {
			continue
		}
		if chartClosedRe.MatchString(content) {
			return true, nil
		}
		if id != "" && taskMarkedClosed(content, id) {
			return true, nil
		}
	}
	return false, nil
}

// directReCache memoizes the per-id word-boundary regexp so the pattern
// compiles once per distinct id (S3 hygiene): behavior is identical to
// compiling per call, without paying regexp.Compile in the CLOSED loop.
var directReCache sync.Map // map[string]*regexp.Regexp

// directReFor returns the cached `\b<id>\b` regexp, compiling once.
func directReFor(id string) *regexp.Regexp {
	if re, ok := directReCache.Load(id); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + `\b`)
	actual, _ := directReCache.LoadOrStore(id, re)
	return actual.(*regexp.Regexp)
}

// taskMarkedClosed checks the current-line window before every CLOSED
// for a direct mention of id or a range covering it.
func taskMarkedClosed(content, id string) bool {
	norm := strings.ReplaceAll(content, "–", "-")
	norm = strings.ReplaceAll(norm, "—", "-")
	norm = strings.ReplaceAll(norm, "−", "-")
	locs := closedWordRe.FindAllStringIndex(norm, -1)
	if len(locs) == 0 {
		return false
	}
	directRe := directReFor(id)
	var idPrefix string
	var idNum int
	idParts := idRe.FindStringSubmatch(id)
	if len(idParts) == 3 {
		idPrefix = idParts[1]
		fmt.Sscanf(idParts[2], "%d", &idNum)
	}
	for _, loc := range locs {
		s := loc[0]
		lineStart := strings.LastIndex(norm[:s], "\n") + 1
		window := norm[lineStart:s]
		if len(window) > 200 {
			window = window[len(window)-200:]
		}
		if directRe.MatchString(window) {
			return true
		}
		if idPrefix == "" {
			continue
		}
		for _, m := range rangeRe.FindAllStringSubmatch(window, -1) {
			startPrefix, startNumS, endPrefix, endNumS := m[1], m[2], m[3], m[4]
			if endPrefix == "" {
				endPrefix = startPrefix
			}
			if startPrefix != idPrefix || endPrefix != idPrefix {
				continue
			}
			var startNum, endNum int
			fmt.Sscanf(startNumS, "%d", &startNum)
			fmt.Sscanf(endNumS, "%d", &endNum)
			lo, hi := startNum, endNum
			if lo > hi {
				lo, hi = hi, lo
			}
			if idNum >= lo && idNum <= hi {
				return true
			}
		}
	}
	return false
}
