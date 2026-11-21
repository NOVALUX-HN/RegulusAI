// Per-task cost accounting (F1): auditable folds over the session
// trace logs on disk. CostByTask groups every stored entry by its task
// (entries recorded before the task field existed land in the
// UnattributedTask bucket) and sums the proxy cost — call counts,
// wall time, output bytes — plus host-exposed token counts when any
// entry carries them. The host (OpenCode) exposes no token counts, so
// host token totals normally stay unknown (TokensKnown false, totals 0);
// they are never estimated. Every entry also carries a measured-local
// token proxy (EstimateLocalTokens, chars/4 over the stored fields
// plus OutputBytes): TotalLocalTokens sums it with LocalTokensKnown
// marking the measurement, so the report always distinguishes
// measured-local proxy tokens from unknown host counts.
//
// Auditability: the report recomputes from the same JSONL lines
// List reads, so re-listing any session and folding by hand reproduces
// every number. Zero means unknown, not zero: a legacy line decodes
// with every cost field 0 and contributes calls only (plus its local
// proxy, which measures the stored descriptor chars).
package trace

import (
	"fmt"
	"sort"
)

// UnattributedTask buckets trace entries recorded without a task
// (every line written before the F1 cost fields existed).
const UnattributedTask = "(unattributed)"

// Unattributed dimension buckets (NOV-39): entries recorded without a
// phase, agent or hook land here — the same "(unattributed)" style as
// tasks, so unknown stays distinguishable from a measured bucket.
const (
	UnattributedPhase = "(unattributed)"
	UnattributedAgent = "(unattributed)"
	UnattributedHook  = "(unattributed)"
)

// TaskCost is the folded cost of one task's trace entries.
// TokenSource names the measurement behind the host token totals:
// measured-host when any entry carried host-exposed counts, unknown
// otherwise. Sources are never mixed: a bucket with no host counts
// reports unknown, and unknown is never summed as zero.
type TaskCost struct {
	Task              string   `json:"task"`
	Calls             int      `json:"calls"`
	Errors            int      `json:"errors"`
	TotalDurationMS   int64    `json:"total_duration_ms"`
	TotalOutputBytes  int64    `json:"total_output_bytes"`
	TotalInputTokens  int64    `json:"total_input_tokens"`
	TotalOutputTokens int64    `json:"total_output_tokens"`
	TotalReasoning    int64    `json:"total_reasoning_tokens"`
	TotalCacheRead    int64    `json:"total_cache_read_tokens"`
	TotalCacheWrite   int64    `json:"total_cache_write_tokens"`
	TokenSource       string   `json:"token_source"`
	DurationKnown     bool     `json:"duration_known"`
	BytesKnown        bool     `json:"bytes_known"`
	TokensKnown       bool     `json:"tokens_known"`
	TotalLocalTokens  int64    `json:"total_local_tokens"`
	LocalTokensKnown  bool     `json:"local_tokens_known"`
	RateKeys          []string `json:"rate_keys,omitempty"`
	Sessions          []string `json:"sessions"`
	Entries           int      `json:"entries"`
}

// EstimateLocalTokens measures the local token proxy for one entry:
// chars/4 (rounded up) over the stored descriptor fields (Tool,
// Message, Call, Task, Files) plus OutputBytes, which counts as chars
// (bytes≈chars for proxy purposes). It never touches the host token
// fields: host-exposed InputTokens/OutputTokens stay the honest
// unknown, the proxy is the measured-local figure. Every entry Record
// accepts carries non-empty Tool/Message/Call, so a stored entry
// always measures >= 1; a zero value only comes from an entry with no
// measurable chars at all (never written by Record). The estimate
// recomputes from the same JSONL lines List reads, so any hand fold
// reproduces it exactly.
func EstimateLocalTokens(e Entry) int64 {
	n := int64(len(e.Tool) + len(e.Message) + len(e.Call) + len(e.Task))
	for _, f := range e.Files {
		n += int64(len(f))
	}
	n += e.OutputBytes
	if n <= 0 {
		return 0
	}
	return (n + 3) / 4
}

// taskKey maps an entry to its cost bucket: its task, or the
// unattributed bucket for legacy lines recorded without one.
func taskKey(e Entry) string {
	if e.Task == "" {
		return UnattributedTask
	}
	return e.Task
}

// Dimension keys (NOV-39) map an entry to its phase, agent or hook
// bucket: the recorded value, or the unattributed bucket for lines
// written without one (empty decodes from every legacy line).
func phaseKey(e Entry) string {
	if e.Phase == "" {
		return UnattributedPhase
	}
	return e.Phase
}

func agentKey(e Entry) string {
	if e.Agent == "" {
		return UnattributedAgent
	}
	return e.Agent
}

func hookKey(e Entry) string {
	if e.Hook == "" {
		return UnattributedHook
	}
	return e.Hook
}

// fold adds one entry to its bucket, tracking which costs were ever
// measured (any field > 0) so unknown stays distinguishable from a
// measured total of 0. Host tokens stay honest: TokensKnown only when
// an entry carries host-exposed counts. The local proxy is always
// measured per folded entry (LocalTokensKnown), summed from
// EstimateLocalTokens — even a legacy line with every host field 0
// contributes its descriptor-chars proxy. Reasoning sums alongside
// output (the original breakdown, never folded away). RateKeys
// collects the distinct model@version pairs behind the token totals
// so money math can demand one explicit tariff. Sums are
// overflow-safe: a hostile total aborts the fold instead of
// wrapping, so the report never prints a corrupted figure.
func (c *TaskCost) fold(session string, e Entry) error {
	c.Calls++
	c.Entries++
	if e.Status == StatusError {
		c.Errors++
	}
	if err := c.foldSums(e); err != nil {
		return fmt.Errorf("fold %q: %v", c.Task, err)
	}
	c.foldKnown(e)
	c.foldHostTokens(e)
	c.trackFoldSession(session)
	return nil
}

// foldSums accumulates every overflow-safe total of one entry,
// including the measured-local proxy. It returns the raw addCounts
// error; the caller wraps it with the bucket name.
func (c *TaskCost) foldSums(e Entry) error {
	if err := c.foldProxySums(e); err != nil {
		return err
	}
	return c.foldTokenSums(e)
}

// foldProxySums accumulates the proxy totals (wall time, bytes).
func (c *TaskCost) foldProxySums(e Entry) error {
	var err error
	if c.TotalDurationMS, err = addCounts(c.TotalDurationMS, e.DurationMS); err != nil {
		return err
	}
	if c.TotalOutputBytes, err = addCounts(c.TotalOutputBytes, e.OutputBytes); err != nil {
		return err
	}
	return nil
}

// foldTokenSums accumulates host token totals plus the local proxy.
func (c *TaskCost) foldTokenSums(e Entry) error {
	var err error
	if c.TotalInputTokens, err = addCounts(c.TotalInputTokens, e.InputTokens); err != nil {
		return err
	}
	if c.TotalOutputTokens, err = addCounts(c.TotalOutputTokens, e.OutputTokens); err != nil {
		return err
	}
	if c.TotalReasoning, err = addCounts(c.TotalReasoning, e.ReasoningTokens); err != nil {
		return err
	}
	if c.TotalCacheRead, err = addCounts(c.TotalCacheRead, e.CacheRead); err != nil {
		return err
	}
	if c.TotalCacheWrite, err = addCounts(c.TotalCacheWrite, e.CacheWrite); err != nil {
		return err
	}
	if c.TotalLocalTokens, err = addCounts(c.TotalLocalTokens, EstimateLocalTokens(e)); err != nil {
		return err
	}
	return nil
}

// foldKnown marks which proxy dimensions were ever measured (> 0)
// so unknown stays distinguishable from a measured total. The
// local proxy measures on every folded entry.
func (c *TaskCost) foldKnown(e Entry) {
	c.LocalTokensKnown = true
	if e.DurationMS > 0 {
		c.DurationKnown = true
	}
	if e.OutputBytes > 0 {
		c.BytesKnown = true
	}
}

// foldHasHostTokens reports whether one entry carries any
// host-exposed token count.
func foldHasHostTokens(e Entry) bool {
	return e.InputTokens > 0 || e.OutputTokens > 0 || e.ReasoningTokens > 0 || e.CacheRead > 0 || e.CacheWrite > 0
}

// foldRateKey records one distinct model@version pair behind the
// host token totals.
func (c *TaskCost) foldRateKey(e Entry) {
	if e.Model == "" {
		return
	}
	key := e.Model + "@" + e.HostVersion
	for _, k := range c.RateKeys {
		if k == key {
			return
		}
	}
	c.RateKeys = append(c.RateKeys, key)
}

// foldHostTokens marks host-token measurement, records the rate key
// behind it, and keeps an explicit usage source when no host count
// named one yet.
func (c *TaskCost) foldHostTokens(e Entry) {
	if foldHasHostTokens(e) {
		c.TokensKnown = true
		if c.TokenSource == "" {
			c.TokenSource = SourceMeasuredHost
		}
		c.foldRateKey(e)
	}
	if e.UsageSource != "" && c.TokenSource == "" {
		c.TokenSource = e.UsageSource
	}
}

// trackFoldSession records one distinct session behind the bucket.
func (c *TaskCost) trackFoldSession(session string) {
	for _, s := range c.Sessions {
		if s == session {
			return
		}
	}
	c.Sessions = append(c.Sessions, session)
}

// CostByTask folds every session trace under root into per-task
// costs, sorted by task. A root with no traces yields no costs and no
// error; a malformed line fails like List does.
func CostByTask(root string) ([]TaskCost, error) {
	return costByDimension(root, taskKey)
}

// costByDimension folds every session trace under root into
// per-bucket costs using one dimension key (task, phase, agent,
// host or hook), sorted by bucket. It reuses fold, so every
// dimension carries the same measured-vs-unknown accounting; a root
// with no traces yields no costs and no error, and a malformed line
// fails like List does. A hostile total aborts the fold instead of
// wrapping, so the report never prints a corrupted figure.
func costByDimension(root string, key func(Entry) string) ([]TaskCost, error) {
	return costByDimensionFiltered(root, key, nil)
}

// costByDimensionFiltered folds like costByDimension but only over
// entries the keep predicate accepts (nil keeps everything). The CLI
// scopes cost reports by task, agent or run through it, so filters
// and grouping share one auditable fold.
func costByDimensionFiltered(root string, key func(Entry) string, keep func(Entry) bool) ([]TaskCost, error) {
	sessions, err := Sessions(root)
	if err != nil {
		return nil, err
	}
	byBucket := map[string]*TaskCost{}
	for _, s := range sessions {
		entries, err := List(root, s)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if keep != nil && !keep(e) {
				continue
			}
			k := key(e)
			c, ok := byBucket[k]
			if !ok {
				c = &TaskCost{Task: k}
				byBucket[k] = c
			}
			if err := c.fold(s, e); err != nil {
				return nil, err
			}
		}
	}
	out := make([]TaskCost, 0, len(byBucket))
	for _, c := range byBucket {
		sort.Strings(c.Sessions)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Task < out[j].Task })
	return out, nil
}

// CostByPhase folds every session trace under root into per-phase
// costs, sorted by phase. Entries recorded without a phase land in
// the UnattributedPhase bucket (every line written before the NOV-39
// fields existed).
func CostByPhase(root string) ([]TaskCost, error) {
	return costByDimension(root, phaseKey)
}

// CostByAgent folds every session trace under root into per-agent
// costs, sorted by agent. Entries recorded without an agent land in
// the UnattributedAgent bucket.
func CostByAgent(root string) ([]TaskCost, error) {
	return costByDimension(root, agentKey)
}

// CostByHook folds every session trace under root into per-hook
// costs, sorted by hook. Entries recorded without a hook land in
// the UnattributedHook bucket.
func CostByHook(root string) ([]TaskCost, error) {
	return costByDimension(root, hookKey)
}

// CostScope folds costs grouped by dim (task, agent or host; empty
// means task) over only the entries matching the non-empty
// task/agent/run filters (task and agent match their dimension
// buckets, run matches the F11.1 execution id). Filters and
// grouping share the auditable fold, so a scoped report squares
// with the same hand fold over List. An unknown dim fails with a
// naming error; no match yields no buckets and no error.
func CostScope(root, dim, task, agent, run string) ([]TaskCost, error) {
	var key func(Entry) string
	switch dim {
	case "", "task":
		key = taskKey
	case "agent":
		key = agentKey
	case "host":
		key = hostKey
	default:
		return nil, fmt.Errorf("cost grouping must be task, agent or host, got %q", dim)
	}
	return costByDimensionFiltered(root, key, func(e Entry) bool {
		if task != "" && taskKey(e) != task {
			return false
		}
		if agent != "" && agentKey(e) != agent {
			return false
		}
		if run != "" && e.ExecutionID != run {
			return false
		}
		return true
	})
}

// hostKey maps an entry to its host bucket: the recorded host, or
// the unattributed bucket for lines written without one (legacy
// lines and local-only entries). An empty host never reads as a
// measured zero — it reads as unattributed.
func hostKey(e Entry) string {
	if e.Host == "" {
		return UnattributedAgent
	}
	return e.Host
}

// CostByHost folds every session trace under root into per-host
// costs, sorted by host. The host dimension keeps measured-host
// totals beside unattributed buckets so one report shows which
// host measured and which stayed unknown.
func CostByHost(root string) ([]TaskCost, error) {
	return costByDimension(root, hostKey)
}

// DedupEntries drops exact duplicate entries (same session, message
// and call id), keeping the first occurrence. Usage attribution
// runs over deduplicated entries so a re-ingested host report or a
// retried record cannot double-count one call; the input slice is
// never mutated.
func DedupEntries(entries []Entry) []Entry {
	seen := map[string]bool{}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		k := e.Session + "\x00" + e.Message + "\x00" + e.Call
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}

// ToolCost is the folded measured-local proxy cost of one tool: call
// counts plus the summed local token proxy (EstimateLocalTokens,
// chars/4 — explicitly labeled measured-local, never host counts).
// A tool with no row was never called; host token totals stay unknown
// and live on TaskCost, never here.
type ToolCost struct {
	Tool              string   `json:"tool"`
	Calls             int      `json:"calls"`
	Errors            int      `json:"errors"`
	TotalLocalTokens  int64    `json:"total_local_tokens"`
	LocalTokensKnown  bool     `json:"local_tokens_known"`
	LocalTokensSource string   `json:"local_tokens_source"`
	Sessions          []string `json:"sessions"`
	Entries           int      `json:"entries"`
}

// fold adds one entry to its tool bucket. Every folded entry carries
// a measured-local proxy (EstimateLocalTokens), so LocalTokensKnown
// marks the measurement and TotalLocalTokens sums it.
func (c *ToolCost) fold(session string, e Entry) {
	c.Calls++
	c.Entries++
	if e.Status == StatusError {
		c.Errors++
	}
	c.TotalLocalTokens += EstimateLocalTokens(e)
	c.LocalTokensKnown = true
	c.LocalTokensSource = "measured-local"
	for _, s := range c.Sessions {
		if s == session {
			return
		}
	}
	c.Sessions = append(c.Sessions, session)
}

// CostByTool folds every session trace under root into per-tool
// measured-local proxy costs, sorted by tool. A root with no traces
// yields no costs and no error; a malformed line fails like List
// does. Every number recomputes from the same JSONL lines List reads,
// so the report squares with the traces.
func CostByTool(root string) ([]ToolCost, error) {
	sessions, err := Sessions(root)
	if err != nil {
		return nil, err
	}
	byTool := map[string]*ToolCost{}
	for _, s := range sessions {
		entries, err := List(root, s)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			c, ok := byTool[e.Tool]
			if !ok {
				c = &ToolCost{Tool: e.Tool}
				byTool[e.Tool] = c
			}
			c.fold(s, e)
		}
	}
	out := make([]ToolCost, 0, len(byTool))
	for _, c := range byTool {
		sort.Strings(c.Sessions)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tool < out[j].Tool })
	return out, nil
}

// CostForTask folds the cost of one task ("" lists the unattributed
// bucket). An unknown task yields a zero cost and no error.
func CostForTask(root, task string) (TaskCost, error) {
	if task == "" {
		task = UnattributedTask
	}
	costs, err := CostByTask(root)
	if err != nil {
		return TaskCost{}, err
	}
	for _, c := range costs {
		if c.Task == task {
			return c, nil
		}
	}
	return TaskCost{Task: task}, nil
}
