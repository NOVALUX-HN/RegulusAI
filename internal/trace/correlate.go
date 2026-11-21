// Usage correlation (F11.5, NOV-91): host reports become trace
// entries through one auditable join, and attributed totals must
// reconcile with the primary source. The join maps the host session
// (HostUsage.RunID) onto the trace session, carries task/agent/run
// keys from F11.1, preserves the original reasoning split, labels
// the measurement source explicitly and redacts before storing.
// Anything that would misattribute — a run/session mismatch, or
// counts with unknown provenance — fails closed.
package trace

import (
	"fmt"
	"strings"
	"time"
)

// CorrelateKeys carries the F11.1 correlation keys a host usage
// report is attributed with: Session is the trace session (the host
// session id), Task and Agent the served task and agent,
// ExecutionID the gate run id, Message/Call/Tool the new trace
// entry's identity.
type CorrelateKeys struct {
	Session     string
	Message     string
	Call        string
	Tool        string
	Task        string
	Agent       string
	ExecutionID string
}

// CorrelateUsage joins one host usage report to its trace identity,
// returning the Entry to record. The host session (u.RunID) and the
// trace session must agree when both are set; task/agent fall back
// to the report's own correlation keys; execution prefers the
// caller's run id, then the report's. Counts with unknown provenance
// are refused (unknown is not zero and not attributable); free text
// is redacted before it can reach the store.
func CorrelateUsage(u HostUsage, k CorrelateKeys) (Entry, error) {
	u = RedactHostUsage(u)
	if strings.TrimSpace(k.Session) == "" || strings.TrimSpace(k.Message) == "" ||
		strings.TrimSpace(k.Call) == "" || strings.TrimSpace(k.Tool) == "" {
		return Entry{}, fmt.Errorf("correlate: session, message, call and tool are required")
	}
	if u.RunID != "" && k.Session != u.RunID {
		return Entry{}, fmt.Errorf("correlate: session %q mismatches host run %q (refusing misattribution)", k.Session, u.RunID)
	}
	hasCounts := u.InputTokens > 0 || u.OutputTokens > 0 || u.ReasoningTokens > 0 ||
		u.CacheRead > 0 || u.CacheWrite > 0
	if hasCounts && (u.Source == "" || u.Source == SourceUnknown) {
		return Entry{}, fmt.Errorf("correlate: counts with unknown provenance are not attributable (host %q)", u.Host)
	}
	task := k.Task
	if task == "" {
		task = u.Task
	}
	agent := k.Agent
	if agent == "" {
		agent = u.Agent
	}
	exec := k.ExecutionID
	if exec == "" {
		exec = u.ExecutionID
	}
	src := u.Source
	if src == "" {
		src = SourceUnknown
	}
	e := Entry{
		At:              time.Now().UTC(),
		Session:         k.Session,
		Message:         k.Message,
		Call:            k.Call,
		Tool:            k.Tool,
		Status:          StatusCompleted,
		Task:            task,
		Agent:           agent,
		ExecutionID:     exec,
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
		ReasoningTokens: u.ReasoningTokens,
		Model:           u.Model,
		Host:            u.Host,
		HostVersion:     u.HostVersion,
		CacheRead:       u.CacheRead,
		CacheWrite:      u.CacheWrite,
		UsageSource:     src,
	}
	if err := validate(e); err != nil {
		return Entry{}, fmt.Errorf("correlate: %v", err)
	}
	return e, nil
}

// ReconcileTotals checks attributed trace entries against their
// primary host source: the summed input/output/reasoning/cache
// counts must equal the primary report exactly. A gap names the
// field, the primary figure and the attributed sum — totals
// reconcile with the primary source or the report does not ship.
// An unknown primary with no attributed entries reconciles
// vacuously (nothing measured, nothing attributed).
func ReconcileTotals(primary HostUsage, attributed []Entry) error {
	if primary.Source == SourceUnknown || primary.Source == "" {
		if len(attributed) != 0 {
			return fmt.Errorf("reconcile: unknown primary with %d attributed entries (nothing to reconcile against)", len(attributed))
		}
		return nil
	}
	var in, out, reasoning, read, write int64
	for i, e := range attributed {
		var err error
		if in, err = addCounts(in, e.InputTokens); err != nil {
			return fmt.Errorf("reconcile: %v (entry %d)", err, i)
		}
		if out, err = addCounts(out, e.OutputTokens); err != nil {
			return fmt.Errorf("reconcile: %v (entry %d)", err, i)
		}
		if reasoning, err = addCounts(reasoning, e.ReasoningTokens); err != nil {
			return fmt.Errorf("reconcile: %v (entry %d)", err, i)
		}
		if read, err = addCounts(read, e.CacheRead); err != nil {
			return fmt.Errorf("reconcile: %v (entry %d)", err, i)
		}
		if write, err = addCounts(write, e.CacheWrite); err != nil {
			return fmt.Errorf("reconcile: %v (entry %d)", err, i)
		}
	}
	for _, f := range []struct {
		name    string
		primary int64
		sum     int64
	}{
		{"input_tokens", primary.InputTokens, in},
		{"output_tokens", primary.OutputTokens, out},
		{"reasoning_tokens", primary.ReasoningTokens, reasoning},
		{"cache_read_tokens", primary.CacheRead, read},
		{"cache_write_tokens", primary.CacheWrite, write},
	} {
		if f.primary != f.sum {
			return fmt.Errorf("reconcile: %s primary %d != attributed %d (source %q run %q)",
				f.name, f.primary, f.sum, primary.Source, primary.RunID)
		}
	}
	return nil
}
