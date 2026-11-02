// Numbered execution cycles plus the git identity (V3,
// charts/dashboard-vision.md): groups gauntlet receipts by run_id into
// sequential cycles (1 = oldest) with task, result, gate counts, summed
// duration, latest date and the manifest branch (Unknown when the run
// has no manifest or the manifest predates V3); receipts without run_id
// (legacy) group by task plus UTC hour window. Both routes are
// read-only GET, jailed to one allowlisted project root like the other
// collections, and every free-text field leaves through RedactText.
package httpapi

import (
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/manifest"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/receipts"
)

// executionItem is one numbered gauntlet cycle: its sequential number
// by date, the task it ran, whether every gate passed, how many gates
// passed out of the total, the summed duration, the latest receipt
// date, and the checkout branch pinned in the run manifest.
type executionItem struct {
	N          int    `json:"n"`
	Task       string `json:"task"`
	Result     string `json:"result"`
	GatesPass  int    `json:"gates_pass"`
	GatesTotal int    `json:"gates_total"`
	DurationMS int64  `json:"duration_ms"`
	Date       string `json:"date"`
	Branch     string `json:"branch"`
}

// execGroup is one cycle under construction: the receipts sharing a
// run id (or a legacy task+window key) plus the earliest receipt time
// that orders the sequence.
type execGroup struct {
	runID string
	task  string
	first time.Time
	recs  []receipts.Receipt
}

// groupExecutions clusters receipts into cycles ordered oldest first.
// Receipts sharing a run_id form one cycle; legacy receipts without
// run_id group by task plus UTC hour window. The task of a cycle is
// the task of its earliest receipt.
func groupExecutions(entries []evidence.Entry) []*execGroup {
	byKey := map[string]*execGroup{}
	var order []*execGroup
	for _, e := range entries {
		key := "run:" + e.Receipt.RunID
		if e.Receipt.RunID == "" {
			key = "legacy:" + e.Receipt.Task + "|" + e.Receipt.StartedAt.UTC().Truncate(time.Hour).Format(time.RFC3339)
		}
		g := byKey[key]
		if g == nil {
			g = &execGroup{runID: e.Receipt.RunID, task: e.Receipt.Task}
			byKey[key] = g
			order = append(order, g)
		}
		g.recs = append(g.recs, e.Receipt)
		if g.first.IsZero() || e.Receipt.StartedAt.Before(g.first) {
			g.first = e.Receipt.StartedAt
			g.task = e.Receipt.Task
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].first.Equal(order[j].first) {
			return order[i].runID < order[j].runID
		}
		return order[i].first.Before(order[j].first)
	})
	return order
}

// manifestBranches indexes run id to branch across every task evidence
// dir. A missing manifest (or one written before V3, with no branch)
// maps to manifest.Unknown. A corrupt manifest file is skipped: the
// index must never hide the executions themselves.
func manifestBranches(root string) map[string]string {
	out := map[string]string{}
	paths, err := filepath.Glob(filepath.Join(evidence.Dir(root), "*", "manifest-*.json"))
	if err != nil {
		return out
	}
	for _, p := range paths {
		m, err := manifest.ReadFile(p)
		if err != nil || m.RunID == "" {
			continue
		}
		if m.Branch == "" {
			out[m.RunID] = manifest.Unknown
		} else {
			out[m.RunID] = m.Branch
		}
	}
	return out
}

// buildExecutionItem folds one cycle: pass needs every receipt green,
// duration sums duration_ms, date is the latest started_at, branch is
// the run manifest branch (Unknown without a manifest, legacy included).
func buildExecutionItem(g *execGroup, n int, branches map[string]string) executionItem {
	pass := 0
	var totalDur int64
	var last time.Time
	for _, rec := range g.recs {
		if rec.Verdict == receipts.VerdictPass {
			pass++
		}
		totalDur += rec.DurationMS
		if last.IsZero() || rec.StartedAt.After(last) {
			last = rec.StartedAt
		}
	}
	result := receipts.VerdictPass
	if pass != len(g.recs) {
		result = receipts.VerdictFail
	}
	branch := manifest.Unknown
	if g.runID != "" {
		if b, ok := branches[g.runID]; ok {
			branch = b
		}
	}
	date := ""
	if !last.IsZero() {
		date = last.UTC().Format(time.RFC3339)
	}
	return executionItem{
		N: n, Task: g.task, Result: result,
		GatesPass: pass, GatesTotal: len(g.recs),
		DurationMS: totalDur, Date: date,
		Branch: observability.RedactText(branch),
	}
}

// handleExecutions serves GET /v1/executions?limit=&offset=: the
// gauntlet cycles oldest first, paginated like every collection.
func (s Server) handleExecutions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	limit, offset, bad := parsePagination(r)
	if bad != "" {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("executions: "+bad))
		return
	}
	entries, err := evidence.List(root, "")
	if err != nil {
		writeEnvelope(w, http.StatusBadRequest, InvalidParams("executions: "+observability.RedactText(err.Error())))
		return
	}
	groups := groupExecutions(entries)
	branches := manifestBranches(root)
	items := make([]executionItem, 0, len(groups))
	for i, g := range groups {
		items = append(items, buildExecutionItem(g, i+1, branches))
	}
	lo, hi := paginate(len(items), limit, offset)
	writePage(w, len(items), limit, offset, items[lo:hi])
}

// gitData is the checkout identity of the served root: the current
// branch plus the short HEAD, Unknown ("—") for either when git cannot
// resolve it.
type gitData struct {
	Branch string `json:"branch"`
	Head   string `json:"head"`
}

// handleGit serves GET /v1/git: the branch and short HEAD of the
// served project root, resolved with pure Go (no shell) under a short
// timeout, Unknown outside git. Same resolution the manifest writer
// uses for the run branch.
func (s Server) handleGit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeEnvelope(w, http.StatusMethodNotAllowed, MethodNotAllowed(r.Method))
		return
	}
	root, ok := s.projectRoot(w, r)
	if !ok {
		return
	}
	data := gitData{
		Branch: observability.RedactText(manifest.BranchOf(root)),
		Head:   manifest.HeadOf(root),
	}
	status, env := MapResult(data, "", 0)
	writeEnvelope(w, status, env)
}
