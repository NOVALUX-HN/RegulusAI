package trace

import (
	"sort"
)

// ToolUsage is one tool's measured usage: total recorded calls plus
// the distinct sessions that called it, sorted. A tool with no row was
// never called — absence, not zero, is the prune signal, so the row
// count alone never suffices without the session count beside it.
type ToolUsage struct {
	Tool     string   `json:"tool"`
	Calls    int      `json:"calls"`
	Sessions []string `json:"sessions"`
}

// UsageSessions counts the sessions with trace logs on disk: the
// evidence floor for pruning (no pruning below a minimum of traced
// sessions — a fresh repo must not lose tools for lack of history).
func UsageSessions(root string) (int, error) {
	sessions, err := Sessions(root)
	if err != nil {
		return 0, err
	}
	return len(sessions), nil
}

// UsageByTool folds every session trace under root into per-tool
// usage, sorted by tool. A root with no traces yields no rows and no
// error; a malformed line fails like List does. Every number recomputes
// from the same JSONL lines `trace show` reads, so the report squares
// with the traces.
func UsageByTool(root string) ([]ToolUsage, error) {
	sessions, err := Sessions(root)
	if err != nil {
		return nil, err
	}
	byTool := map[string]*ToolUsage{}
	for _, s := range sessions {
		entries, err := List(root, s)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			u, ok := byTool[e.Tool]
			if !ok {
				u = &ToolUsage{Tool: e.Tool}
				byTool[e.Tool] = u
			}
			u.Calls++
			found := false
			for _, have := range u.Sessions {
				if have == s {
					found = true
					break
				}
			}
			if !found {
				u.Sessions = append(u.Sessions, s)
			}
		}
	}
	out := make([]ToolUsage, 0, len(byTool))
	for _, u := range byTool {
		sort.Strings(u.Sessions)
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tool < out[j].Tool })
	return out, nil
}
