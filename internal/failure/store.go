// Decision audit log (F13.5/NOV-108): one append-only JSONL file
// per mission holding every sealed DecisionRecord. This log is
// the required "cause, evidence, decision, discarded
// alternatives and result" register — it cites the F12.5 ledger
// (run/task/agent ids, idempotency keys) and the mission state,
// it never duplicates either store and it never records intent
// the recovery ledger should own.
//
// Durability follows the harness discipline: private dirs, one
// complete JSON line per decision appended under the fsx
// inter-process lock (only complete lines ever hit disk), reads
// fail closed on malformed lines (an edited log is not
// evidence), and repeated idem keys replay the stored record
// instead of appending twice.
package failure

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"regulus-ai/internal/fsx"
	"regulus-ai/internal/mission"
)

// DecisionsFile names the per-mission log inside its directory.
const DecisionsFile = "decisions.jsonl"

// decisionsDir resolves the durable directory for one mission's
// decision log. Mission ids are validated so no traversal escapes
// .regulus/failure.
func decisionsDir(root, missionID string) (string, error) {
	if err := mission.ValidateID(missionID); err != nil {
		return "", err
	}
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Join(root, ".regulus", "failure", missionID), nil
}

// AppendDecision seals one record durably. A repeated non-empty
// idem key replays the stored record without appending (the
// repeated-recovery guarantee at the mission level); the boolean
// reports whether the record was newly appended.
func AppendDecision(root string, rec DecisionRecord) (stored DecisionRecord, appended bool, err error) {
	if strings.TrimSpace(rec.MissionID) == "" {
		return DecisionRecord{}, false, fmt.Errorf("failure: decision needs a mission id")
	}
	if strings.TrimSpace(rec.ID) == "" {
		return DecisionRecord{}, false, fmt.Errorf("failure: decision needs an id (mint, never hand-make)")
	}
	dir, err := decisionsDir(root, rec.MissionID)
	if err != nil {
		return DecisionRecord{}, false, err
	}
	if rec.IdemKey != "" {
		if prev, ok := findIdem(dir, rec.IdemKey); ok {
			return prev, false, nil
		}
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return DecisionRecord{}, false, err
	}
	raw = append(raw, '\n')
	if err := fsx.AppendLine(filepath.Join(dir, DecisionsFile), raw); err != nil {
		return DecisionRecord{}, false, err
	}
	return rec, true, nil
}

// findIdem scans the log for a stored idem key. Missing files
// mean "never recorded" (nil, false); malformed lines fail closed
// like every other harness read.
func findIdem(dir, idemKey string) (DecisionRecord, bool) {
	recs, err := loadDir(dir)
	if err != nil {
		return DecisionRecord{}, false
	}
	for _, r := range recs {
		if r.IdemKey != "" && r.IdemKey == idemKey {
			return r, true
		}
	}
	return DecisionRecord{}, false
}

// LoadDecisions reads every sealed record for one mission in
// append order. A missing log reads as zero decisions (nothing
// decided yet); a malformed line fails closed naming the line.
func LoadDecisions(root, missionID string) ([]DecisionRecord, error) {
	dir, err := decisionsDir(root, missionID)
	if err != nil {
		return nil, err
	}
	return loadDir(dir)
}

func loadDir(dir string) ([]DecisionRecord, error) {
	f, err := os.Open(filepath.Join(dir, DecisionsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []DecisionRecord
	lineNo := 0
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scan.Scan() {
		lineNo++
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		var r DecisionRecord
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("failure: decision log line %d is malformed (edited log is not evidence): %w", lineNo, err)
		}
		if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Decision) == "" {
			return nil, fmt.Errorf("failure: decision log line %d is incomplete (edited log is not evidence)", lineNo)
		}
		out = append(out, r)
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// FindDecision resolves a --decision selector: a 1-based index
// into append order, a full id, or a unique id prefix. Ambiguous
// prefixes and out-of-range indexes refuse with the reason.
func FindDecision(recs []DecisionRecord, sel string) (DecisionRecord, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" {
		return DecisionRecord{}, fmt.Errorf("failure: decision selector is required (1-based index, id or unique id prefix)")
	}
	if n, err := strconv.Atoi(sel); err == nil {
		if n < 1 || n > len(recs) {
			return DecisionRecord{}, fmt.Errorf("failure: decision %d out of range (1..%d)", n, len(recs))
		}
		return recs[n-1], nil
	}
	var hit *DecisionRecord
	for i := range recs {
		if recs[i].ID == sel || strings.HasPrefix(recs[i].ID, sel) {
			if hit != nil {
				return DecisionRecord{}, fmt.Errorf("failure: decision prefix %q is ambiguous (name more of the id)", sel)
			}
			hit = &recs[i]
		}
	}
	if hit == nil {
		return DecisionRecord{}, fmt.Errorf("failure: no decision %q recorded", sel)
	}
	return *hit, nil
}
