package almanac

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"
)

// Health is the `regulus memory doctor` report: the log's shape, its
// unreadable lines, and the provenance events (pins, relations) that
// point at observations that no longer exist.
type Health struct {
	Total           int
	ByType          map[string]int
	Oldest          time.Time
	Newest          time.Time
	MalformedLines  []int
	OrphanPins      []string
	OrphanRelations []Relation
}

// Doctor scans the Almanac tolerantly: unlike All it never fails on a
// malformed line — it records the line number instead — and reports the
// pins and relations whose targets are missing from the log.
func Doctor(root string) (Health, error) {
	h := Health{ByType: map[string]int{}}
	ids := map[string]bool{}
	f, err := os.Open(File(root))
	if os.IsNotExist(err) {
		// An empty Almanac still gets its provenance events checked.
		if ferr := finishHealth(root, &h, ids); ferr != nil {
			return h, ferr
		}
		return h, nil
	}
	if err != nil {
		return h, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	ln := 0
	for sc.Scan() {
		ln++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var o Observation
		if err := json.Unmarshal([]byte(line), &o); err != nil {
			h.MalformedLines = append(h.MalformedLines, ln)
			continue
		}
		h.Total++
		h.ByType[o.Type]++
		ids[o.ID] = true
		if h.Oldest.IsZero() || o.SavedAt.Before(h.Oldest) {
			h.Oldest = o.SavedAt
		}
		if o.SavedAt.After(h.Newest) {
			h.Newest = o.SavedAt
		}
	}
	if err := sc.Err(); err != nil {
		return h, err
	}
	if err := finishHealth(root, &h, ids); err != nil {
		return h, err
	}
	return h, nil
}

// finishHealth loads the provenance events and flags the ones pointing
// outside the log. It mutates h and reports only the error.
func finishHealth(root string, h *Health, ids map[string]bool) error {
	pins, err := PinnedIDs(root)
	if err != nil {
		return err
	}
	for id := range pins {
		if !ids[id] {
			h.OrphanPins = append(h.OrphanPins, id)
		}
	}
	sort.Strings(h.OrphanPins)
	rels, err := Relations(root)
	if err != nil {
		return err
	}
	for _, r := range rels {
		if !ids[r.From] || !ids[r.To] {
			h.OrphanRelations = append(h.OrphanRelations, r)
		}
	}
	return nil
}
