// Paginated read-only collections (D1): receipts, observations with
// search, lessons/candidates, trace sessions and entries, budget
// history. Every list is bounded (?limit default 50, max 200,
// ?offset, ?task) so the panel never full-scans in the client.
// Free-text fields pass through observability.RedactText before
// leaving the process; no endpoint mutates state.
package httpapi

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/budget"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/library"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

// defaultPageLimit is the page size when ?limit is absent; maxPageLimit
// is the hard ceiling so a client cannot ask for an unbounded dump.
const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// parsePagination resolves ?limit/?offset: absent limit means the
// default, absent offset means 0. A non-empty detail reports the
// usage failure for a 400 envelope.
func parsePagination(r *http.Request) (limit, offset int, detail string) {
	q := r.URL.Query()
	limit = defaultPageLimit
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxPageLimit {
			return 0, 0, "limit must be an integer 1.." + strconv.Itoa(maxPageLimit)
		}
		limit = n
	}
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return 0, 0, "offset must be a non-negative integer"
		}
		offset = n
	}
	return limit, offset, ""
}

// pageData is the shared paginated shape: the total before paging plus
// the window the client asked for.
type pageData struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Items  any `json:"items"`
}

// writePage renders one paginated success envelope.
func writePage(w http.ResponseWriter, total, limit, offset int, items any) {
	status, env := MapResult(pageData{Total: total, Limit: limit, Offset: offset, Items: items}, "", 0)
	writeEnvelope(w, status, env)
}

// paginate clips [offset:offset+limit] over total, tolerating an offset
// past the end (empty page, total preserved).
func paginate(total, limit, offset int) (lo, hi int) {
	if offset > total {
		return total, total
	}
	lo = offset
	hi = offset + limit
	if hi > total {
		hi = total
	}
	return lo, hi
}

// redactReceipt scrubs the free-text surface of one receipt (captured
// stdout/stderr plus the command line) via the real redactor.
func redactReceipt(r receipts.Receipt) receipts.Receipt {
	r.Stdout = observability.RedactText(r.Stdout)
	r.Stderr = observability.RedactText(r.Stderr)
	r.Command = observability.RedactText(r.Command)
	return r
}

// redactObservation scrubs the free-text surface of one observation
// (title, content, topic, domain, lesson block, provenance).
func redactObservation(o almanac.Observation) almanac.Observation {
	o.Title = observability.RedactText(o.Title)
	o.Content = observability.RedactText(o.Content)
	o.TopicKey = observability.RedactText(o.TopicKey)
	o.Domain = observability.RedactText(o.Domain)
	if o.Lesson != nil {
		c := *o.Lesson
		c.Cue = observability.RedactText(c.Cue)
		c.Lesson = observability.RedactText(c.Lesson)
		for i, e := range c.Evidence {
			c.Evidence[i] = observability.RedactText(e)
		}
		o.Lesson = &c
	}
	if o.Provenance != nil {
		p := *o.Provenance
		p.Task = observability.RedactText(p.Task)
		p.GitHead = observability.RedactText(p.GitHead)
		for i, v := range p.Paths {
			p.Paths[i] = observability.RedactText(v)
		}
		for i, v := range p.Receipts {
			p.Receipts[i] = observability.RedactText(v)
		}
		o.Provenance = &p
	}
	return o
}

// redactLesson scrubs the free-text surface of one library lesson or
// candidate (cue, lesson text, task, evidence refs).
func redactLesson(l library.Lesson) library.Lesson {
	l.Task = observability.RedactText(l.Task)
	l.Cue = observability.RedactText(l.Cue)
	l.Lesson = observability.RedactText(l.Lesson)
	for i, e := range l.Evidence {
		l.Evidence[i] = observability.RedactText(e)
	}
	return l
}

// redactTraceEntry scrubs the free-text surface of one trace entry
// (message, call, tool, session, task, touched paths).
func redactTraceEntry(e trace.Entry) trace.Entry {
	e.Session = observability.RedactText(e.Session)
	e.Message = observability.RedactText(e.Message)
	e.Call = observability.RedactText(e.Call)
	e.Tool = observability.RedactText(e.Tool)
	e.Task = observability.RedactText(e.Task)
	for i, f := range e.Files {
		e.Files[i] = observability.RedactText(f)
	}
	return e
}

// redactBudgetEntry scrubs the free-text surface of one ledger entry
// (task, domain, detail; kind/outcome are fixed vocabularies).
func redactBudgetEntry(e budget.Entry) budget.Entry {
	e.Task = observability.RedactText(e.Task)
	e.Domain = observability.RedactText(e.Domain)
	e.Detail = observability.RedactText(e.Detail)
	return e
}

// receiptItem pairs a redacted receipt with its project-relative path
// (relative so absolute local paths never reach the browser).
type receiptItem struct {
	Path    string           `json:"path"`
	Receipt receipts.Receipt `json:"receipt"`
}

// handleReceipts serves GET /v1/receipts?task=&limit=&offset=: gauntlet
// receipts oldest first, redacted, paginated.
func (s Server) handleReceipts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	limit, offset, bad := parsePagination(r)
	if bad != "" {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("receipts: "+bad))
		return
	}
	task := strings.TrimSpace(r.URL.Query().Get("task"))
	entries, err := evidence.List(".", task)
	if err != nil {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("receipts: "+err.Error()))
		return
	}
	lo, hi := paginate(len(entries), limit, offset)
	items := make([]receiptItem, 0, hi-lo)
	for _, e := range entries[lo:hi] {
		rel := e.Path
		if r, err := filepath.Rel(".", e.Path); err == nil {
			rel = r
		}
		items = append(items, receiptItem{
			Path:    observability.RedactText(rel),
			Receipt: redactReceipt(e.Receipt),
		})
	}
	if items == nil {
		items = []receiptItem{}
	}
	writePage(w, len(entries), limit, offset, items)
}

// handleObservations serves GET
// /v1/observations?q=&task=&type=&limit=&offset=: Almanac observations
// matching every q term (case-insensitive substring over title and
// content), optionally narrowed by provenance task and observation
// type, newest-relevant first per the store ranking, redacted.
func (s Server) handleObservations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	limit, offset, bad := parsePagination(r)
	if bad != "" {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("observations: "+bad))
		return
	}
	q := r.URL.Query()
	task := strings.TrimSpace(q.Get("task"))
	typ := strings.TrimSpace(q.Get("type"))
	if typ != "" {
		switch typ {
		case almanac.TypeSessionNote, almanac.TypeDecision, almanac.TypeDiscovery, almanac.TypeLesson, almanac.TypePreference:
		default:
			writeEnvelope(w, http.StatusBadRequest, InvalidParams("observations: unknown type "+typ))
			return
		}
	}
	hits, err := almanac.Search(".", almanac.Query{
		Terms: strings.Fields(strings.TrimSpace(q.Get("q"))),
		Type:  typ,
	})
	if err != nil {
		writeEnvelope(w, http.StatusOK, Internal("observations: "+err.Error()))
		return
	}
	if task != "" {
		kept := hits[:0:0]
		for _, o := range hits {
			if o.Provenance != nil && o.Provenance.Task == task {
				kept = append(kept, o)
			}
		}
		hits = kept
	}
	lo, hi := paginate(len(hits), limit, offset)
	items := make([]almanac.Observation, 0, hi-lo)
	for _, o := range hits[lo:hi] {
		items = append(items, redactObservation(o))
	}
	if items == nil {
		items = []almanac.Observation{}
	}
	writePage(w, len(hits), limit, offset, items)
}

// handleLessons serves GET
// /v1/lessons?domain=&status=&task=&q=&limit=&offset=: admitted lessons
// plus pending candidates across library domains, oldest first,
// redacted. ?status=admitted|candidate narrows (absent means both).
func (s Server) handleLessons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	limit, offset, bad := parsePagination(r)
	if bad != "" {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("lessons: "+bad))
		return
	}
	domain, status, task, needles, detail := parseLessonsQuery(r)
	if detail != "" {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("lessons: "+detail))
		return
	}
	all, storeErr := loadLessons(domain, status)
	if storeErr != "" {
		writeEnvelope(w, http.StatusOK, Internal("lessons: "+storeErr))
		return
	}
	all = filterLessons(all, task, needles)
	lo, hi := paginate(len(all), limit, offset)
	items := all[lo:hi]
	if items == nil {
		items = []library.Lesson{}
	}
	writePage(w, len(all), limit, offset, items)
}

// parseLessonsQuery resolves the ?domain/?status/?task/?q filters: the
// status defaults to both stores, and a non-empty detail reports the
// usage failure for a 400 envelope.
func parseLessonsQuery(r *http.Request) (domain, status, task string, needles []string, detail string) {
	q := r.URL.Query()
	domain = strings.TrimSpace(q.Get("domain"))
	if strings.Contains(domain, "/") || strings.Contains(domain, "\\") || strings.Contains(domain, "..") {
		return "", "", "", nil, "bad domain"
	}
	status = strings.TrimSpace(q.Get("status"))
	if status == "" {
		status = "all"
	}
	switch status {
	case "all", "admitted", "candidate":
	default:
		return "", "", "", nil, "status must be admitted, candidate or all"
	}
	return domain, status, strings.TrimSpace(q.Get("task")),
		strings.Fields(strings.ToLower(strings.TrimSpace(q.Get("q")))), ""
}

// loadLessons reads admitted lessons and pending candidates for the
// selected domains, oldest first. A non-empty storeErr names the store
// failure for a 200 error envelope.
func loadLessons(domain, status string) ([]library.Lesson, string) {
	domains := []string{domain}
	if domain == "" {
		var err error
		domains, err = library.Domains(".")
		if err != nil {
			return nil, err.Error()
		}
	}
	var all []library.Lesson
	for _, d := range domains {
		if status == "all" || status == "admitted" {
			ls, err := library.List(".", d)
			if err != nil {
				return nil, err.Error()
			}
			for _, l := range ls {
				l.Status = "admitted"
				all = append(all, l)
			}
		}
		if status == "all" || status == "candidate" {
			cs, err := library.Candidates(".", d)
			if err != nil {
				return nil, err.Error()
			}
			for _, l := range cs {
				if strings.TrimSpace(l.Status) == "" {
					l.Status = "candidate"
				}
				all = append(all, l)
			}
		}
	}
	return all, ""
}

// filterLessons narrows lessons by exact task and every q term
// (case-insensitive substring over cue and lesson text), redacting the
// survivors.
func filterLessons(all []library.Lesson, task string, needles []string) []library.Lesson {
	kept := all[:0:0]
	for _, l := range all {
		if task != "" && l.Task != task {
			continue
		}
		if !matchLesson(l, needles) {
			continue
		}
		kept = append(kept, redactLesson(l))
	}
	return kept
}

// matchLesson reports whether one lesson contains every q term.
func matchLesson(l library.Lesson, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	hay := strings.ToLower(l.Cue + "\n" + l.Lesson)
	for _, n := range needles {
		if !strings.Contains(hay, n) {
			return false
		}
	}
	return true
}

// traceSessionItem names one session trace plus its entry count.
type traceSessionItem struct {
	Session string `json:"session"`
	Entries int    `json:"entries"`
}

// handleTraces serves GET /v1/traces?session=&task=&limit=&offset=:
// without ?session= it lists session ids (sorted, paginated, with
// entry counts); with ?session= it lists that session's entries in
// call order, optionally narrowed by task, redacted, paginated.
func (s Server) handleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	limit, offset, bad := parsePagination(r)
	if bad != "" {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("traces: "+bad))
		return
	}
	q := r.URL.Query()
	session := strings.TrimSpace(q.Get("session"))
	task := strings.TrimSpace(q.Get("task"))
	if session == "" {
		sessions, err := trace.Sessions(".")
		if err != nil {
			writeEnvelope(w, http.StatusOK, Internal("traces: "+err.Error()))
			return
		}
		lo, hi := paginate(len(sessions), limit, offset)
		items := make([]traceSessionItem, 0, hi-lo)
		for _, name := range sessions[lo:hi] {
			entries, err := trace.List(".", name)
			if err != nil {
				writeEnvelope(w, http.StatusOK, Internal("traces: "+err.Error()))
				return
			}
			n := 0
			if task == "" {
				n = len(entries)
			} else {
				for _, e := range entries {
					if e.Task == task {
						n++
					}
				}
			}
			items = append(items, traceSessionItem{
				Session: observability.RedactText(name),
				Entries: n,
			})
		}
		if items == nil {
			items = []traceSessionItem{}
		}
		writePage(w, len(sessions), limit, offset, items)
		return
	}
	entries, err := trace.List(".", session)
	if err != nil {
		writeEnvelope(w, http.StatusOK, Internal("traces: "+err.Error()))
		return
	}
	if task != "" {
		kept := entries[:0:0]
		for _, e := range entries {
			if e.Task == task {
				kept = append(kept, e)
			}
		}
		entries = kept
	}
	lo, hi := paginate(len(entries), limit, offset)
	items := make([]trace.Entry, 0, hi-lo)
	for _, e := range entries[lo:hi] {
		items = append(items, redactTraceEntry(e))
	}
	if items == nil {
		items = []trace.Entry{}
	}
	writePage(w, len(entries), limit, offset, items)
}

// handleBudget serves GET /v1/budget?task=&limit=&offset=: the
// evolution budget ledger oldest first, optionally narrowed by task,
// redacted, paginated.
func (s Server) handleBudget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	limit, offset, bad := parsePagination(r)
	if bad != "" {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("budget: "+bad))
		return
	}
	task := strings.TrimSpace(r.URL.Query().Get("task"))
	entries, err := budget.Show(".")
	if err != nil {
		writeEnvelope(w, http.StatusOK, Internal("budget: "+err.Error()))
		return
	}
	if task != "" {
		kept := entries[:0:0]
		for _, e := range entries {
			if e.Task == task {
				kept = append(kept, e)
			}
		}
		entries = kept
	}
	lo, hi := paginate(len(entries), limit, offset)
	items := make([]budget.Entry, 0, hi-lo)
	for _, e := range entries[lo:hi] {
		items = append(items, redactBudgetEntry(e))
	}
	if items == nil {
		items = []budget.Entry{}
	}
	writePage(w, len(entries), limit, offset, items)
}
