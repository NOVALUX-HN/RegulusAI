package envfacts

import (
	"sort"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

// Stats reports what the scan saw and skipped. Receipts and trace
// entries without a resolvable chart domain cannot form domain facts
// and are counted, never silently dropped. Receipts with an unknown
// verdict (neither pass nor fail) are likewise skipped and counted as
// unattributed: they never form a contradiction execution. Corrupt
// documents skipped by the tolerant H2 readers (malformed receipt
// files, malformed trace lines) are counted in Corrupt, warned about
// on stderr as path:line, and never abort the scan: healthy data keeps
// mining.
type Stats struct {
	Receipts             int
	ReceiptsUnattributed int
	TraceSessions        int
	TraceUnattributed    int
	Corrupt              int
}

// ReceiptSeries folds every gauntlet receipt into per-(domain, gate)
// chronological series. Units are executions, not sessions: one receipt
// is one gate execution. Executions carry both passes and failures so
// the contradiction window sees success after failure. Receipts whose
// verdict is neither pass nor fail (unknown, empty, error, skip,
// legacy without verdict) are skipped and counted as unattributed:
// they form no contradiction execution and never discard a live fact.
// Corrupt receipt files are skipped with a stderr path:line warning
// and counted in Stats.Corrupt (H2): they never abort the scan.
// The execution id is the run id (schema v1 receipts); legacy receipts
// without one fall back to their file path — each receipt file counts
// as one execution, so distinct paths are distinct executions. That
// over-approximates when one logical run fans out to several files,
// which is safe: it can only delay mining (needs MinSessions distinct
// executions), never invent a failure.
func ReceiptSeries(root string) ([]Series, Stats, error) {
	entries, corrupt, err := evidence.ListSkipping(root, "")
	if err != nil {
		return nil, Stats{}, err
	}
	cache := map[string]string{}
	byKey := map[Key][]Execution{}
	var st Stats
	st.Corrupt = corrupt
	for _, e := range entries {
		st.Receipts++
		domain := DomainOfTask(root, e.Receipt.Task, cache)
		if domain == "" {
			st.ReceiptsUnattributed++
			continue
		}
		if e.Receipt.Verdict != receipts.VerdictPass && e.Receipt.Verdict != receipts.VerdictFail {
			st.ReceiptsUnattributed++
			continue
		}
		session := e.Receipt.RunID
		if session == "" {
			session = e.Path
		}
		k := Key{Kind: KindGate, Name: e.Receipt.Gate, Domain: domain}
		byKey[k] = append(byKey[k], Execution{
			At:       e.Receipt.StartedAt,
			Session:  session,
			Failed:   e.Receipt.Verdict == receipts.VerdictFail,
			Evidence: e.Path,
		})
	}
	return fold(byKey), st, nil
}

// TraceSeries folds session traces into per-(domain, tool) chronological
// series. Tool granularity is one execution per (session, tool): the
// execution fails when any attributed entry of that tool in that
// session errored — a single error poisons the whole tool execution
// (coarse by design, documented here; flaky single calls read as
// session-wide failure, which can only delay discards, never hide a
// repeat). Each execution carries its own At: the last error time when
// failed, else the last use time — never the session-end max — so
// ordering reflects when the tool actually failed. Series sort by that
// At. Entries without a task predate per-task attribution, contribute
// no execution, and count in Stats.TraceUnattributed (audited, never
// silent). Corrupt trace lines are skipped with a stderr path:line
// warning and counted in Stats.Corrupt (H2): one bad line never aborts
// the session, and one bad session never aborts the scan.
func TraceSeries(root string) ([]Series, Stats, error) {
	var st Stats
	sessions, err := trace.Sessions(root)
	if err != nil {
		return nil, st, err
	}
	st.TraceSessions = len(sessions)
	cache := map[string]string{}
	used := map[Key]map[string]bool{}
	evFor := map[Key]map[string]string{}
	atFor := map[Key]map[string]int64{}
	for _, s := range sessions {
		entries, corrupt, err := trace.ListSkipping(root, s)
		if err != nil {
			return nil, st, err
		}
		st.Corrupt += corrupt
		for _, e := range entries {
			if e.Tool == "" {
				continue
			}
			domain := DomainOfTask(root, e.Task, cache)
			if domain == "" {
				st.TraceUnattributed++
				continue
			}
			k := Key{Kind: KindTool, Name: e.Tool, Domain: domain}
			if used[k] == nil {
				used[k] = map[string]bool{}
				evFor[k] = map[string]string{}
				atFor[k] = map[string]int64{}
			}
			at := e.At.UnixNano()
			if _, ok := used[k][s]; !ok {
				used[k][s] = e.Status == trace.StatusError
				evFor[k][s] = trace.File(root, s)
				atFor[k][s] = at
				continue
			}
			if e.Status == trace.StatusError {
				if !used[k][s] || at > atFor[k][s] {
					atFor[k][s] = at
				}
				used[k][s] = true
			} else if !used[k][s] && at > atFor[k][s] {
				atFor[k][s] = at
			}
		}
	}
	folded := map[Key][]Execution{}
	for k, perSession := range used {
		for s, failed := range perSession {
			folded[k] = append(folded[k], Execution{
				At:       time.Unix(0, atFor[k][s]).UTC(),
				Session:  s,
				Failed:   failed,
				Evidence: evFor[k][s],
			})
		}
	}
	return fold(folded), st, nil
}

// fold orders each series chronologically (ties by session, then
// evidence path) and sorts the series by topic for deterministic plans.
func fold(byKey map[Key][]Execution) []Series {
	out := make([]Series, 0, len(byKey))
	for k, execs := range byKey {
		sort.Slice(execs, func(i, j int) bool {
			if !execs[i].At.Equal(execs[j].At) {
				return execs[i].At.Before(execs[j].At)
			}
			if execs[i].Session != execs[j].Session {
				return execs[i].Session < execs[j].Session
			}
			return execs[i].Evidence < execs[j].Evidence
		})
		out = append(out, Series{Key: k, Execs: execs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.Topic() < out[j].Key.Topic() })
	return out
}
