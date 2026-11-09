package almanac

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// missFile is the retrieval-miss log beside the observations log:
// local runtime state (gitignored like traces), never committed.
func missFile(root string) string {
	return filepath.Join(root, ".regulus", "almanac", "misses.jsonl")
}

// missEntry is one zero-hit search: the terms as asked, when asked.
type missEntry struct {
	Terms []string  `json:"terms"`
	At    time.Time `json:"at"`
}

// RecordMiss appends one zero-hit search. Best-effort by contract:
// callers ignore the error — a miss log must never break a search.
func RecordMiss(root string, terms []string, now time.Time) error {
	if len(terms) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(missFile(root)), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(missFile(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(missEntry{Terms: terms, At: now.UTC()})
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// MissStats folds the miss log: total misses plus the most-missed
// term sets (joined with spaces), by frequency then alphabetically,
// top 3. A missing log reads as zero — a fresh repo has no misses
// yet. Malformed lines are skipped, never fatal.
func MissStats(root string) (int, []string, error) {
	f, err := os.Open(missFile(root))
	if os.IsNotExist(err) {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()
	counts := map[string]int{}
	count := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e missEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if len(e.Terms) == 0 {
			continue
		}
		count++
		counts[strings.Join(e.Terms, " ")]++
	}
	if err := sc.Err(); err != nil {
		return 0, nil, err
	}
	type kv struct {
		terms string
		n     int
	}
	var rows []kv
	for terms, n := range counts {
		rows = append(rows, kv{terms, n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].terms < rows[j].terms
	})
	var top []string
	for i, r := range rows {
		if i >= 3 {
			break
		}
		top = append(top, r.terms)
	}
	return count, top, nil
}
