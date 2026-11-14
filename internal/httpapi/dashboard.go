// Dashboard read-only JSON (T1, S1; D2 visual): metrics + almanac summary over
// EXISTING stores. No producer rewrites. Aggregates return counts only,
// never collections. Free-text passes through observability.RedactText
// before leaving the process (D1 status/version/collections, D3 error
// details); the visual HTML is served with a restrictive CSP (D3).
package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/budget"
	"regulus-ai/internal/evolve"
	"regulus-ai/internal/library"
	"regulus-ai/internal/manifest"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/reflect"
)

// aggregateCacheControl marks successful read-only aggregate GETs
// cacheable for 10s (H5, S2-S3): the dashboard polls every 30s and fans
// each aggregate out to several panels per cycle, so a short max-age
// lets caches coalesce the duplicate in-cycle fetches. Poll frequency
// and JSON shapes stay byte-identical: headers only, never bodies.
const aggregateCacheControl = "max-age=10"

func setAggregateCache(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", aggregateCacheControl)
}

// almanacFoldEntry memoizes the uncapped almanac.All scan+fold
// (total, by_type, oldest, newest) keyed by the observations file
// identity (H5, S2-S3): the fold is pure over the file bytes, so an
// unchanged mtime+size means an unchanged fold and repeat polls skip
// the rescan. Any store append changes mtime+size and invalidates.
type almanacFoldEntry struct {
	mtime  time.Time
	size   int64
	total  int
	byType map[string]int
	oldest time.Time
	newest time.Time
	// generatedAt stamps the scan time (S18): memoized with the fold
	// so repeat polls replay byte-identical bodies while the store is
	// unchanged, under the same Cache-Control window as the headers.
	generatedAt string
}

var almanacFoldMemo = struct {
	sync.Mutex
	byFile map[string]almanacFoldEntry
}{byFile: map[string]almanacFoldEntry{}}

// cachedAlmanacFold returns the almanac.All fold, serving the memoized
// entry when the store file is unchanged (same mtime+size) and
// rescanning otherwise. Errors are never memoized: the next poll
// retries the store. A missing log folds to zeros, like almanac.All.
// The generated_at stamp rides the memo with the fold (S18) so an
// unchanged store replays the identical body, headers and JSON alike.
func cachedAlmanacFold(root string) (int, map[string]int, time.Time, time.Time, string, error) {
	path := almanac.File(root)
	key := path
	if abs, err := filepath.Abs(path); err == nil {
		key = abs
	}
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, map[string]int{}, time.Time{}, time.Time{}, time.Now().UTC().Format(time.RFC3339), nil
		}
		return 0, nil, time.Time{}, time.Time{}, "", err
	}
	almanacFoldMemo.Lock()
	if e, ok := almanacFoldMemo.byFile[key]; ok && e.mtime.Equal(fi.ModTime()) && e.size == fi.Size() {
		byType := make(map[string]int, len(e.byType))
		for k, v := range e.byType {
			byType[k] = v
		}
		total, oldest, newest, generatedAt := e.total, e.oldest, e.newest, e.generatedAt
		almanacFoldMemo.Unlock()
		return total, byType, oldest, newest, generatedAt, nil
	}
	almanacFoldMemo.Unlock()
	all, err := almanac.All(root)
	if err != nil {
		return 0, nil, time.Time{}, time.Time{}, "", err
	}
	byType := map[string]int{}
	var oldest, newest time.Time
	for _, o := range all {
		byType[o.Type]++
		if oldest.IsZero() || o.SavedAt.Before(oldest) {
			oldest = o.SavedAt
		}
		if newest.IsZero() || o.SavedAt.After(newest) {
			newest = o.SavedAt
		}
	}
	stored := make(map[string]int, len(byType))
	for k, v := range byType {
		stored[k] = v
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	almanacFoldMemo.Lock()
	almanacFoldMemo.byFile[key] = almanacFoldEntry{
		mtime: fi.ModTime(), size: fi.Size(),
		total: len(all), byType: stored,
		oldest: oldest, newest: newest,
		generatedAt: generatedAt,
	}
	almanacFoldMemo.Unlock()
	return len(all), byType, oldest, newest, generatedAt, nil
}

// dashboardPartial marks a truncated aggregate window (S18): cap is
// the window the aggregate scanned, total the full store count. It is
// omitted when the aggregate scanned the whole store (no truncation:
// every S18 aggregate today is a full fold, so partial stays absent
// until a capped aggregate exists).
type dashboardPartial struct {
	Cap   int `json:"cap"`
	Total int `json:"total"`
}

// dashboardMeta is the S18 envelope carried by every dashboard
// aggregate: when the payload was generated (RFC3339 serve time),
// which allowlisted project it covers (ProjectName, redacted like
// /v1/status) and the resolved root it was read from. partial rides
// along only where an aggregate truncates (omitted otherwise).
type dashboardMeta struct {
	GeneratedAt string            `json:"generated_at"`
	Project     string            `json:"project"`
	Root        string            `json:"root"`
	Partial     *dashboardPartial `json:"partial,omitempty"`
}

// dashboardNow stamps one aggregate payload for root.
func dashboardNow(root string) dashboardMeta {
	return dashboardMeta{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Project:     observability.RedactText(almanac.ProjectName(root)),
		Root:        root,
	}
}

// metricsData is Summarize plus today's evolution against the cap.
type metricsData struct {
	dashboardMeta
	Metrics       observability.Metrics `json:"metrics"`
	EvolutionUsed int                   `json:"evolution_used"`
	EvolutionCap  int                   `json:"evolution_cap"`
}

// almanacSummaryData counts what the Almanac holds, by type.
type almanacSummaryData struct {
	dashboardMeta
	Total       int            `json:"total"`
	ByType      map[string]int `json:"by_type"`
	Lessons     int            `json:"lessons_admitted"`
	Candidates  int            `json:"candidates_pending"`
	Oldest      string         `json:"oldest,omitempty"`
	Newest      string         `json:"newest,omitempty"`
	IndexStatus string         `json:"index_status"`
}

// evolutionData is the evolution economics aggregate: admitted
// lessons, proxy tokens, rot count, pending candidates, tick
// position. Counts only, never collections (D3 aggregates policy).
type evolutionData struct {
	dashboardMeta
	Admitted    int   `json:"admitted"`
	ProxyTokens int64 `json:"proxy_tokens"`
	Rot         int   `json:"rot"`
	Pending     int   `json:"pending"`
	Sessions    int   `json:"sessions"`
	TickAt      int   `json:"tick_at"`
}

func (s Server) handleEvolution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	rep, _ := evolve.BuildReport(root, "")
	pending := 0
	if domains, err := library.Domains(root); err == nil {
		for _, d := range domains {
			if cs, err := library.Candidates(root, d); err == nil {
				pending += len(cs)
			}
		}
	}
	sessions, atTick := evolve.TickPosition(root)
	meta := dashboardNow(root)
	status, env := MapResult(evolutionData{
		dashboardMeta: meta,
		Admitted:      rep.Admitted, ProxyTokens: rep.TotalProxy, Rot: rep.Rot,
		Pending: pending, Sessions: sessions, TickAt: atTick,
	}, "", 0)
	setAggregateCache(w)
	writeEnvelope(w, status, env)
}

func (s Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	m, err := observability.Summarize(root, "")
	if err != nil {
		writeEnvelope(w, http.StatusInternalServerError, Internal("metrics: "+observability.RedactText(err.Error())))
		return
	}
	used, err := budget.TodayEvolutionCount(root, time.Now().UTC())
	if err != nil {
		writeEnvelope(w, http.StatusInternalServerError, Internal("metrics budget: "+observability.RedactText(err.Error())))
		return
	}
	status, env := MapResult(metricsData{dashboardMeta: dashboardNow(root), Metrics: m, EvolutionUsed: used, EvolutionCap: reflect.EvolutionCap(root)}, "", 0)
	setAggregateCache(w)
	writeEnvelope(w, status, env)
}

func (s Server) handleAlmanacSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	total, byType, oldest, newest, generatedAt, err := cachedAlmanacFold(root)
	if err != nil {
		writeEnvelope(w, http.StatusInternalServerError, Internal("almanac: "+observability.RedactText(err.Error())))
		return
	}
	lessons, candidates := 0, 0
	if domains, err := library.Domains(root); err == nil {
		for _, d := range domains {
			if ls, err := library.List(root, d); err == nil {
				lessons += len(ls)
			}
			if cs, err := library.Candidates(root, d); err == nil {
				candidates += len(cs)
			}
		}
	}
	idx, _ := almanac.IndexState(root)
	oldestS, newestS := "", ""
	if !oldest.IsZero() {
		oldestS = oldest.UTC().Format(time.RFC3339)
	}
	if !newest.IsZero() {
		newestS = newest.UTC().Format(time.RFC3339)
	}
	meta := dashboardNow(root)
	meta.GeneratedAt = generatedAt
	status, env := MapResult(almanacSummaryData{
		dashboardMeta: meta,
		Total:         total, ByType: byType,
		Lessons: lessons, Candidates: candidates,
		Oldest: oldestS, Newest: newestS, IndexStatus: idx,
	}, "", 0)
	setAggregateCache(w)
	writeEnvelope(w, status, env)
}

// dashboardCSP is the restrictive policy for the embedded visual (D3):
// same-origin only, no external scripts/styles/images/objects. The UI
// uses inline <style> and one inline <script> (stdlib fetch only, all
// dynamic text via textContent), so exactly 'unsafe-inline' for scripts
// and styles is allowed — nothing else.
const dashboardCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// handleDashboard serves the embedded visual (D2): header with the
// project state, cards per area, plain-language explanations, paginated
// tables and dependency-free mini charts with 30s auto-refresh. Unknown
// /v1/* still 404s through the shared envelope so versioned-contract
// tests stay green.
func (s Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/v1/" {
		s.handleNotFound(w, r)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/dashboard" {
		s.handleNotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", dashboardCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardHTML))
}

// gitMetaData is the checkout identity of /v1/git plus the S18
// envelope. The route is served here (not via executions.go) so the
// meta lives with the other dashboard aggregates; branch/HEAD
// resolution reuses the same manifest calls, no logic change.
type gitMetaData struct {
	dashboardMeta
	Branch string `json:"branch"`
	Head   string `json:"head"`
}

// handleGitMeta serves GET /v1/git with the S18 envelope: branch plus
// short HEAD (Unknown "—" outside git) stamped like every aggregate.
func (s Server) handleGitMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	data := gitMetaData{
		dashboardMeta: dashboardNow(root),
		Branch:        observability.RedactText(manifest.BranchOf(root)),
		Head:          manifest.HeadOf(root),
	}
	status, env := MapResult(data, "", 0)
	writeEnvelope(w, status, env)
}
