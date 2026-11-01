package envfacts

import (
	"sort"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

// Stats reports what the scan saw and skipped. Receipts and trace
// entries without a resolvable chart domain cannot form domain facts
// and are counted, never silently dropped.
type Stats struct {
	Receipts             int
	ReceiptsUnattributed int
	TraceSessions        int
	TraceUnattributed    int
}

// ReceiptSeries folds every gauntlet receipt into per-(domain, gate)
// chronological series. Executions carry both passes and failures so
// the contradiction window sees success after failure. The session
// proxy is the run id (schema v1 receipts); legacy receipts without
// one fall back to their file path — each receipt file is one gate
// execution, so distinct paths are distinct executions.
func ReceiptSeries(root string) ([]Series, Stats, error) {
	entries, err := evidence.List(root, "")
	if err != nil {
		return nil, Stats{}, err
	}
	cache := map[string]string{}
	byKey := map[Key][]Execution{}
	var st Stats
	for _, e := range entries {
		st.Receipts++
		domain := DomainOfTask(root, e.Receipt.Task, cache)
		if domain == "" {
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
	return foldSeries(byKey), st, nil
}

// TraceSeries folds session traces into per-(domain, tool) chronological
// series. A session contributes one execution per tool it used; the
// execution fails when any attributed entry errored. Entries without a
// task predate per-task attribution and cannot join a domain series.
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
	when := map[string]int64{}
	for _, s := range sessions {
		entries, err := trace.List(root, s)
		if err != nil {
			return nil, st, err
		}
		var latest int64
		for _, e := range entries {
			if e.At.UnixNano() > latest {
				latest = e.At.UnixNano()
			}
		}
		when[s] = latest
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
			}
			if _, ok := used[k][s]; !ok {
				used[k][s] = false
				evFor[k][s] = trace.File(root, s)
			}
			if e.Status == trace.StatusError {
				used[k][s] = true
			}
		}
	}
	folded := map[Key][]Execution{}
	for k, perSession := range used {
		for s, failed := range perSession {
			folded[k] = append(folded[k], Execution{
				Session:  s,
				Failed:   failed,
				Evidence: evFor[k][s],
			})
		}
	}
	return foldTraceSeries(folded, when), st, nil
}

// foldSeries orders each series chronologically (ties by evidence path)
// and sorts the series by key for deterministic plans.
func foldSeries(byKey map[Key][]Execution) []Series {
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
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out
}

// foldTraceSeries orders each trace series by session time (ties by
// session name) and stamps that time on the executions.
func foldTraceSeries(byKey map[Key][]Execution, when map[string]int64) []Series {
	out := make([]Series, 0, len(byKey))
	for k, execs := range byKey {
		sort.Slice(execs, func(i, j int) bool {
			if when[execs[i].Session] != when[execs[j].Session] {
				return when[execs[i].Session] < when[execs[j].Session]
			}
			return execs[i].Session < execs[j].Session
		})
		out = append(out, Series{Key: k, Execs: execs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out
}
