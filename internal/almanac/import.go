package almanac

import (
	"encoding/json"
	"fmt"
	"log"
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
//
// When path is a directory it is read as an export bundle: almanac.jsonl
// plus the pins.jsonl, relations.jsonl and status.jsonl siblings written
// by `memory export`. Observations restore as in the single-file form
// (forced Source=import, PROPOSED unless the bundle status already
// governs the id); the event stores restore verbatim — only lines not
// already present are appended, so re-importing is a no-op, and events
// pointing at observations missing from the merged log are skipped with
// a warning, never a failure. Missing sibling files are not an error:
// a directory with only almanac.jsonl (like an old export) imports
// exactly like the single-file form. Validation stays atomic — every
// present file is fully parsed before anything is written, so the first
// invalid line aborts with its file and line number and the target tree
// is left untouched. The returned counts always cover observations;
// pins, relations and status merges are idempotent side effects.
func Import(root, path string) (imported, skipped int, err error) {
	if st, serr := os.Stat(path); serr == nil && st.IsDir() {
		return importBundle(root, path)
	}
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
	fresh, skipped, err := parseObservationData(data, have, "memory import: line %d")
	if err != nil {
		return 0, 0, err
	}
	var mark []string
	for _, o := range fresh {
		mark = append(mark, o.ID)
	}
	return appendImported(root, fresh, skipped, mark)
}

// parseObservationData validates every non-empty line of an
// observations JSONL payload like a save, marking the fresh ones
// import-sourced. have gains the fresh ids; bad lines abort with the
// label formatted with the 1-based line number.
func parseObservationData(data []byte, have map[string]bool, label string) (fresh []Observation, skipped int, err error) {
	ln := 0
	for _, raw := range strings.Split(string(data), "\n") {
		ln++
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		var o Observation
		if err := json.Unmarshal([]byte(line), &o); err != nil {
			return nil, 0, fmt.Errorf(label+": %v", ln, err)
		}
		if err := validate(&o); err != nil {
			return nil, 0, fmt.Errorf(label+": %v", ln, err)
		}
		if o.ID == "" {
			return nil, 0, fmt.Errorf(label+": missing id", ln)
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
	return fresh, skipped, nil
}

// appendImported appends fresh observations to the log and records the
// proposing event for the marked ids: foreign knowledge enters
// PROPOSED, never laundered into higher states. M2: the append runs
// under the store lock (one WAL writer at a time).
//
// NOV-29: the duplicate partition is re-checked INSIDE the lock from
// the live log (two byte-identical imports racing each other serialize:
// the loser sees every id already taken and appends nothing, so the
// second concurrent import reports 0 imported).
func appendImported(root string, fresh []Observation, skipped int, mark []string) (imported, skippedOut int, err error) {
	// NOV-22: scrub secrets at the import choke point — every bundle
	// line (single file or directory form) funnels through here.
	for i := range fresh {
		fresh[i] = RedactObservation(fresh[i])
	}
	if len(fresh) == 0 {
		return 0, skipped, nil
	}
	skippedOut = skipped
	markSet := map[string]bool{}
	for _, id := range mark {
		markSet[id] = true
	}
	err = withStoreLock(root, func() error {
		taken, err := logIDs(root)
		if err != nil {
			return err
		}
		var toWrite []Observation
		var toMark []string
		for _, o := range fresh {
			if taken[o.ID] {
				skippedOut++
				continue
			}
			taken[o.ID] = true
			toWrite = append(toWrite, o)
			if markSet[o.ID] {
				toMark = append(toMark, o.ID)
			}
		}
		n, err := writeObservationsLocked(root, toWrite, toMark)
		if err != nil {
			return err
		}
		imported = n
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return imported, skippedOut, nil
}

// writeObservationsLocked appends toWrite to the log and records the
// PROPOSED event for toMark. The caller holds the store lock (no
// nested acquire — safe to call from appendImported and from the
// single bundle commit). No-op when toWrite is empty, so a fully
// skipped re-import touches nothing.
func writeObservationsLocked(root string, toWrite []Observation, toMark []string) (int, error) {
	if len(toWrite) == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(File(root), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, o := range toWrite {
		line, err := json.Marshal(o)
		if err != nil {
			f.Close()
			return 0, err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			f.Close()
			return 0, err
		}
		n++
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	if err := MarkImported(root, toMark, time.Now().UTC()); err != nil {
		return 0, err
	}
	return n, nil
}

// appendStoreLinesFilteredLocked appends only the planned lines not
// already present verbatim in the store. The caller holds the store
// lock, so the read-present-then-append is atomic against concurrent
// bundle imports (the loser filters everything and writes nothing,
// leaving the stores byte-identical). No-op for an empty plan or a
// fully-present plan — a skipped re-import creates nothing.
func appendStoreLinesFilteredLocked(storePath string, planned []string) error {
	if len(planned) == 0 {
		return nil
	}
	present := map[string]bool{}
	if raw, err := os.ReadFile(storePath); err == nil {
		for _, l := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(l) != "" {
				present[l] = true
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	var todo []string
	for _, l := range planned {
		if !present[l] {
			todo = append(todo, l)
			present[l] = true
		}
	}
	return appendStoreLines(storePath, todo)
}

// importBundle restores a full export bundle directory: the observations
// in almanac.jsonl plus the pins.jsonl, relations.jsonl and status.jsonl
// siblings. Every present file is validated before anything is written.
//
// NOV-29: the commit (observations + pins/relations/status) runs under a
// single store lock. planStoreLines validates outside the lock; the
// in-lock phase re-checks live ids (observations) and verbatim presence
// (event stores), so two concurrent identical bundle imports serialize:
// the loser appends nothing and reports 0 imported with byte-identical
// stores. Helpers never acquire the lock themselves, so the single
// commit cannot deadlock.
func importBundle(root, dir string) (imported, skipped int, err error) {
	// NOV-30: integrity first — a tampered bundle (or a newer format)
	// refuses before any planning or writing, so the target tree stays
	// intact. Bundles without MANIFEST predate NOV-30 and import as before.
	if err := VerifyBundle(dir); err != nil {
		return 0, 0, err
	}
	obsData, err := os.ReadFile(filepath.Join(dir, "almanac.jsonl"))
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
	fresh, skipped, err := parseObservationData(obsData, have, "memory import: almanac.jsonl line %d")
	if err != nil {
		return 0, 0, err
	}
	// The merged id set decides orphans: bundle events may point at
	// observations the same bundle carries.
	for _, o := range fresh {
		have[o.ID] = true
	}
	covered := map[string]bool{}
	pins, err := planStoreLines(filepath.Join(dir, "pins.jsonl"), pinsFile(root), func(line string) (bool, error) {
		return validateBundlePin(line, have)
	})
	if err != nil {
		return 0, 0, err
	}
	rels, err := planStoreLines(filepath.Join(dir, "relations.jsonl"), relationsFile(root), func(line string) (bool, error) {
		return validateBundleRelation(line, have)
	})
	if err != nil {
		return 0, 0, err
	}
	status, err := planStoreLines(filepath.Join(dir, "status.jsonl"), StatusFile(root), func(line string) (bool, error) {
		return validateBundleStatus(line, have)
	})
	if err != nil {
		return 0, 0, err
	}
	// Ids with a planned status line keep their restored trail (covered
	// here from the validated plan, which only holds non-orphan lines).
	for _, l := range status {
		var ev StatusEvent
		if err := json.Unmarshal([]byte(l), &ev); err == nil {
			covered[ev.ID] = true
		}
	}
	// Observations first, as today — but ids the bundle status already
	// governs keep their restored trail instead of stacking a duplicate
	// PROPOSED on top of it.
	// NOV-22 redaction runs on the same choke point as the single-file
	// form (writeObservationsLocked does not redact — redact once here
	// before the commit).
	for i := range fresh {
		fresh[i] = RedactObservation(fresh[i])
	}
	// Single atomic commit under one lock: observations + all three
	// event stores. Re-checks inside serialize concurrent imports.
	skippedOut := skipped
	var importedOut int
	err = withStoreLock(root, func() error {
		taken, err := logIDs(root)
		if err != nil {
			return err
		}
		var toWrite []Observation
		var toMark []string
		for _, o := range fresh {
			if taken[o.ID] {
				skippedOut++
				continue
			}
			taken[o.ID] = true
			toWrite = append(toWrite, o)
			if !covered[o.ID] {
				toMark = append(toMark, o.ID)
			}
		}
		n, err := writeObservationsLocked(root, toWrite, toMark)
		if err != nil {
			return err
		}
		importedOut = n
		for _, s := range []struct {
			path  string
			lines []string
		}{
			{pinsFile(root), pins},
			{relationsFile(root), rels},
			{StatusFile(root), status},
		} {
			if err := appendStoreLinesFilteredLocked(s.path, s.lines); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return importedOut, skippedOut, nil
}

// validateBundlePin checks one pins.jsonl line: well-formed with an id,
// kept when its observation survives in the merged log (orphans skip).
func validateBundlePin(line string, have map[string]bool) (bool, error) {
	var ev struct {
		ID     string `json:"id"`
		Pinned bool   `json:"pinned"`
	}
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return false, err
	}
	if strings.TrimSpace(ev.ID) == "" {
		return false, fmt.Errorf("missing id")
	}
	return have[ev.ID], nil
}

// validateBundleRelation checks one relations.jsonl line: a known kind
// between two distinct, surviving observations (orphans skip).
func validateBundleRelation(line string, have map[string]bool) (bool, error) {
	var r Relation
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		return false, err
	}
	switch r.Relation {
	case RelationSupersedes, RelationConflictsWith, RelationRelated,
		RelationRelatedTo, RelationInvalidatedBy, RelationDerivedFrom, RelationVerifiedBy:
	default:
		return false, fmt.Errorf("relation %q is not one of supersedes, conflicts_with, related, related_to, invalidated_by, derived_from, verified_by", r.Relation)
	}
	if r.From == r.To {
		return false, fmt.Errorf("an observation cannot relate to itself")
	}
	if strings.TrimSpace(r.From) == "" || strings.TrimSpace(r.To) == "" {
		return false, fmt.Errorf("missing endpoint")
	}
	return have[r.From] && have[r.To], nil
}

// validateBundleStatus checks one status.jsonl line: a known governance
// state for a surviving observation (orphans skip). Transition rules
// are not re-checked: the bundle restores recorded history verbatim,
// and every recorded transition was valid where it was written.
func validateBundleStatus(line string, have map[string]bool) (bool, error) {
	var ev StatusEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return false, err
	}
	if strings.TrimSpace(ev.ID) == "" {
		return false, fmt.Errorf("missing id")
	}
	switch ev.To {
	case StatusObserved, StatusProposed, StatusVerified, StatusAccepted, StatusRejected, StatusSuperseded:
	default:
		return false, fmt.Errorf("status %q is not a known governance state", ev.To)
	}
	return have[ev.ID], nil
}

// planStoreLines validates one bundle store file and returns the exact
// lines to append. A missing bundle file is a no-op (old bundles carry
// no extra stores). Lines already present verbatim in the target store
// are idempotent skips; keep decides the rest — false means an orphan
// event, skipped with a warning instead of a failure. The first
// malformed line aborts with its file and line number.
func planStoreLines(bundleFile, storePath string, keep func(line string) (bool, error)) ([]string, error) {
	data, err := os.ReadFile(bundleFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	present := map[string]bool{}
	if raw, err := os.ReadFile(storePath); err == nil {
		for _, l := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(l) != "" {
				present[l] = true
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	name := filepath.Base(bundleFile)
	var out []string
	ln := 0
	for _, raw := range strings.Split(string(data), "\n") {
		ln++
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if present[raw] {
			continue
		}
		ok, err := keep(raw)
		if err != nil {
			return nil, fmt.Errorf("memory import: %s line %d: %v", name, ln, err)
		}
		if !ok {
			log.Printf("almanac: skipping import of %s line %d: unknown observation", name, ln)
			continue
		}
		out = append(out, raw)
	}
	return out, nil
}

// appendStoreLines appends planned lines to a store file, creating it
// (and its directory) on first use. Nothing is written for an empty
// plan, so a fully-skipped re-import leaves the stores byte-identical.
func appendStoreLines(storePath string, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(storePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.Write([]byte(l + "\n")); err != nil {
			return err
		}
	}
	return nil
}
