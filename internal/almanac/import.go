package almanac

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Import reads observations from a JSONL file (the `memory export`
// format: one observation per line) and appends the ones whose id is
// not already in the log — idempotent, so re-importing is safe.
// Every line is validated like a save; the first invalid line aborts
// with its line number and imports nothing. It returns imported and
// skipped (already present) counts.
func Import(root, path string) (imported, skipped int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	all, err := All(root)
	if err != nil {
		return 0, 0, err
	}
	have := map[string]bool{}
	for _, o := range all {
		have[o.ID] = true
	}
	var fresh []Observation
	ln := 0
	for _, raw := range strings.Split(string(data), "\n") {
		ln++
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var o Observation
		if err := json.Unmarshal([]byte(line), &o); err != nil {
			return 0, 0, fmt.Errorf("memory import: line %d: %v", ln, err)
		}
		if err := validate(&o); err != nil {
			return 0, 0, fmt.Errorf("memory import: line %d: %v", ln, err)
		}
		if o.ID == "" {
			return 0, 0, fmt.Errorf("memory import: line %d: missing id", ln)
		}
		if have[o.ID] {
			skipped++
			continue
		}
		have[o.ID] = true
		// Imported knowledge is never laundered: it enters marked as
		// import-sourced, keeping its origin when the bundle carried one.
		o.Source = SourceImport
		if strings.TrimSpace(o.Origin) == "" {
			o.Origin = "unknown-origin"
		}
		fresh = append(fresh, o)
	}
	if len(fresh) == 0 {
		return 0, skipped, nil
	}
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		return 0, 0, err
	}
	f, err := os.OpenFile(File(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	var ids []string
	for _, o := range fresh {
		line, err := json.Marshal(o)
		if err != nil {
			return 0, 0, err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return 0, 0, err
		}
		ids = append(ids, o.ID)
		imported++
	}
	if err := MarkImported(root, ids, time.Now().UTC()); err != nil {
		return 0, 0, err
	}
	return imported, skipped, nil
}
