// Human rendering (F12.5/NOV-100): one-line states plus the
// multi-line status the CLI prints. JSON output reuses the record
// shape itself; this file only formats the human view.
package recovery

import (
	"fmt"
	"sort"
	"strings"
)

// RenderStatus prints one run: every checkpoint with its last
// confirmed state, attempt/budget position, idempotency key and
// open circuits. Verbose and stable: checkpoints sort by key.
func RenderStatus(r *RunRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "recovery %s: %d checkpoint(s)\n", r.RunID, len(r.Checks))
	keys := make([]string, 0, len(r.Checks))
	for k := range r.Checks {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c := r.Checks[k]
		fmt.Fprintf(&b, "- %s %s attempt=%d retries=%d key=%s confirmed=%s\n",
			k, c.State, c.Attempt, c.Retries, c.IdemKey,
			c.LastConfirm.UTC().Format("2006-01-02T15:04:05Z"))
		if len(c.Scopes) > 0 {
			fmt.Fprintf(&b, "  scopes: %s\n", strings.Join(c.Scopes, ", "))
		}
		if len(c.Capabilities) > 0 {
			fmt.Fprintf(&b, "  caps: %s\n", strings.Join(c.Capabilities, ", "))
		}
		if len(c.RequiredCaps) > 0 {
			fmt.Fprintf(&b, "  required: %s\n", strings.Join(c.RequiredCaps, ", "))
		}
		if c.Host != "" || c.Version != "" {
			fmt.Fprintf(&b, "  host=%s version=%s\n", c.Host, c.Version)
		}
		if c.LastClass != "" {
			fmt.Fprintf(&b, "  last error: %s\n", c.LastClass)
		}
		if len(c.Applied) > 0 {
			fmt.Fprintf(&b, "  applied effects: %s\n", strings.Join(c.PendingEffects(), ", "))
		}
		pending := 0
		for _, s := range c.Compensate {
			if !s.Compensated {
				pending++
			}
		}
		if pending > 0 {
			fmt.Fprintf(&b, "  pending compensations: %d\n", pending)
		}
	}
	hosts := make([]string, 0, len(r.Circuit.OpenedAt))
	for h := range r.Circuit.OpenedAt {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		fmt.Fprintf(&b, "! circuit open: host %s (%d consecutive failures)\n", h, r.Circuit.Failures[h])
	}
	return b.String()
}

// RenderPlan prints one resume plan: what the executor must skip
// (already applied, exactly-once) and what still runs.
func RenderPlan(p ResumePlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "resume %s/%s attempt=%d key=%s\n", p.TaskID, p.AgentID, p.Attempt, p.Key)
	if len(p.Skip) == 0 && len(p.Run) == 0 {
		b.WriteString("no effects registered: nothing to skip, nothing pending\n")
		return b.String()
	}
	for _, k := range p.Skip {
		fmt.Fprintf(&b, "  skip (applied): %s\n", k)
	}
	for _, k := range p.Run {
		fmt.Fprintf(&b, "  run: %s\n", k)
	}
	return b.String()
}
