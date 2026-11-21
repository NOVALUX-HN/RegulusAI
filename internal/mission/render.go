// Human rendering (F13.1/NOV-104): the lifecycle summary the
// CLI prints. JSON output reuses the record shape itself; this
// file only formats the human view, stable and sorted.
package mission

import (
	"fmt"
	"sort"
	"strings"
)

// RenderShow prints one mission: identity, intent, bounds, global
// state, generation, correlation set, pause bookmark, ledger size
// and cancellation/reopen notes.
func RenderShow(m *Mission) string {
	var b strings.Builder
	fmt.Fprintf(&b, "mission %s: %s\n", m.ID, m.State)
	fmt.Fprintf(&b, "  objective: %s\n", m.Objective)
	if m.Description != "" {
		fmt.Fprintf(&b, "  description: %s\n", m.Description)
	}
	if m.Repo != "" || m.Workspace != "" {
		fmt.Fprintf(&b, "  repo=%s workspace=%s\n", m.Repo, m.Workspace)
	}
	if len(m.Constraints) > 0 {
		fmt.Fprintf(&b, "  constraints: %s\n", strings.Join(m.Constraints, ", "))
	}
	if len(m.Criteria) > 0 {
		fmt.Fprintf(&b, "  acceptance: %s\n", strings.Join(m.Criteria, ", "))
	}
	fmt.Fprintf(&b, "  risk=%s", m.Risk)
	if m.Budget.TimeMS != 0 || m.Budget.Tokens != 0 || m.Budget.Cost != 0 {
		fmt.Fprintf(&b, " budget(time=%dms tokens=%d cost=%v)", m.Budget.TimeMS, m.Budget.Tokens, m.Budget.Cost)
	}
	b.WriteString("\n")
	if m.Creator != "" {
		fmt.Fprintf(&b, "  creator: %s\n", m.Creator)
	}
	fmt.Fprintf(&b, "  generation=%d created=%s updated=%s\n",
		m.Generation,
		m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		m.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"))
	if m.PausedFrom != "" {
		fmt.Fprintf(&b, "  paused from: %s\n", m.PausedFrom)
	}
	if m.CancelReason != "" {
		fmt.Fprintf(&b, "  cancel reason: %s\n", m.CancelReason)
	}
	if m.ReopenCount > 0 {
		fmt.Fprintf(&b, "  reopened: %d time(s)\n", m.ReopenCount)
	}
	printLinks(&b, "run", m.Links.RunIDs)
	printLinks(&b, "task", m.Links.TaskIDs)
	printLinks(&b, "agent", m.Links.AgentIDs)
	printLinks(&b, "worktree", m.Links.Worktrees)
	printLinks(&b, "commit", m.Links.Commits)
	printLinks(&b, "trace", m.Links.Traces)
	printLinks(&b, "receipt", m.Links.Receipts)
	if len(m.Effects) > 0 {
		keys := make([]string, 0, len(m.Effects))
		for k := range m.Effects {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(&b, "  applied effects: %s\n", strings.Join(keys, ", "))
	}
	fmt.Fprintf(&b, "  history: %d event(s)\n", len(m.History))
	return b.String()
}

func printLinks(b *strings.Builder, name string, ids []string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(b, "  %s: %s\n", name, strings.Join(ids, ", "))
}

// RenderStatus prints the mission lifecycle: current state,
// generation, pause bookmark and the audit trail newest-last.
// Every line carries the mission id, so events stay correlatable
// in a shared log.
func RenderStatus(m *Mission) string {
	var b strings.Builder
	fmt.Fprintf(&b, "mission %s: %s generation=%d", m.ID, m.State, m.Generation)
	if m.PausedFrom != "" {
		fmt.Fprintf(&b, " (paused from %s)", m.PausedFrom)
	}
	b.WriteString("\n")
	for _, e := range m.History {
		line := fmt.Sprintf("- %s %s -> %s", m.ID, e.From, e.To)
		if e.Actor != "" {
			line += " by " + e.Actor
		}
		if e.Reason != "" {
			line += ": " + e.Reason
		}
		line += " at " + e.At.UTC().Format("2006-01-02T15:04:05Z")
		b.WriteString(line + "\n")
	}
	return b.String()
}

// RenderList prints one line per mission, corrupt entries included
// with their explicit problem instead of a silent omission.
func RenderList(entries []ListEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d mission(s)\n", len(entries))
	for _, e := range entries {
		if e.Problem != "" {
			fmt.Fprintf(&b, "- %s CORRUPT: %s\n", e.ID, e.Problem)
			continue
		}
		fmt.Fprintf(&b, "- %s %s gen=%d updated=%s\n  objective: %s\n",
			e.ID, e.State, e.Generation,
			e.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"), e.Objective)
	}
	return b.String()
}
