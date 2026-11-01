// Per-task cost accounting (F1): auditable folds over the session
// trace logs on disk. CostByTask groups every stored entry by its task
// (entries recorded before the task field existed land in the
// UnattributedTask bucket) and sums the proxy cost — call counts,
// wall time, output bytes — plus host-exposed token counts when any
// entry carries them. The host (OpenCode) exposes no token counts, so
// token totals normally stay unknown (TokensKnown false, totals 0);
// they are never estimated.
//
// Auditability: the report recomputes from the same JSONL lines
// List reads, so re-listing any session and folding by hand reproduces
// every number. Zero means unknown, not zero: a legacy line decodes
// with every cost field 0 and contributes calls only.
package trace

import (
	"sort"
)

// UnattributedTask buckets trace entries recorded without a task
// (every line written before the F1 cost fields existed).
const UnattributedTask = "(unattributed)"

// TaskCost is the folded cost of one task's trace entries.
type TaskCost struct {
	Task              string   `json:"task"`
	Calls             int      `json:"calls"`
	Errors            int      `json:"errors"`
	TotalDurationMS   int64    `json:"total_duration_ms"`
	TotalOutputBytes  int64    `json:"total_output_bytes"`
	TotalInputTokens  int64    `json:"total_input_tokens"`
	TotalOutputTokens int64    `json:"total_output_tokens"`
	DurationKnown     bool     `json:"duration_known"`
	BytesKnown        bool     `json:"bytes_known"`
	TokensKnown       bool     `json:"tokens_known"`
	Sessions          []string `json:"sessions"`
	Entries           int      `json:"entries"`
}

// taskKey maps an entry to its cost bucket: its task, or the
// unattributed bucket for legacy lines recorded without one.
func taskKey(e Entry) string {
	if e.Task == "" {
		return UnattributedTask
	}
	return e.Task
}

// fold adds one entry to its bucket, tracking which costs were ever
// measured (any field > 0) so unknown stays distinguishable from a
// measured total of 0.
func (c *TaskCost) fold(session string, e Entry) {
	c.Calls++
	c.Entries++
	if e.Status == StatusError {
		c.Errors++
	}
	c.TotalDurationMS += e.DurationMS
	c.TotalOutputBytes += e.OutputBytes
	c.TotalInputTokens += e.InputTokens
	c.TotalOutputTokens += e.OutputTokens
	if e.DurationMS > 0 {
		c.DurationKnown = true
	}
	if e.OutputBytes > 0 {
		c.BytesKnown = true
	}
	if e.InputTokens > 0 || e.OutputTokens > 0 {
		c.TokensKnown = true
	}
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
	sessions, err := Sessions(root)
	if err != nil {
		return nil, err
	}
	byTask := map[string]*TaskCost{}
	for _, s := range sessions {
		entries, err := List(root, s)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			key := taskKey(e)
			c, ok := byTask[key]
			if !ok {
				c = &TaskCost{Task: key}
				byTask[key] = c
			}
			c.fold(s, e)
		}
	}
	out := make([]TaskCost, 0, len(byTask))
	for _, c := range byTask {
		sort.Strings(c.Sessions)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Task < out[j].Task })
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
