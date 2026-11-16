// Redacted config snapshots for host evidence (F11.7 / NOV-93):
// record non-sensitive configuration without leaking secrets and
// without touching ownership. Reading is always safe: Inspect never
// writes, and this file adds no writer — foreign files stay foreign,
// managed files stay managed. Every content string passes through
// observability.RedactText before leaving, so artifacts carry
// "[redacted]" instead of credentials (the same choke-point policy
// as the lab emitter and the almanac store).
package hostcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"regulus-ai/internal/observability"
)

// MaxSnapshotBytes caps one snapshot: evidence records shape, not
// dumps. Longer files truncate with a marker.
const MaxSnapshotBytes = 8192

// Snapshot is the attributable, non-sensitive record of one config
// path: its ownership state plus redacted content.
type Snapshot struct {
	Path     string `json:"path"`
	State    string `json:"state"` // missing | managed | foreign | unreadable
	Redacted string `json:"redacted"`
}

// SnapshotForEvidence classifies path read-only and returns its
// redacted content. Missing files record "missing" with empty
// content; unreadable files record "unreadable" with the redacted
// error. No write, no backup, no ownership change.
func SnapshotForEvidence(path string) Snapshot {
	snap := Snapshot{Path: path}
	p, err := Inspect(path)
	if err != nil {
		snap.State = "unreadable"
		snap.Redacted = observability.RedactText(err.Error())
		return snap
	}
	switch {
	case p.Missing:
		snap.State = "missing"
		return snap
	case p.Managed:
		snap.State = "managed"
	case p.Foreign:
		snap.State = "foreign"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		snap.State = "unreadable"
		snap.Redacted = observability.RedactText(err.Error())
		return snap
	}
	body := string(data)
	if len(body) > MaxSnapshotBytes {
		body = body[:MaxSnapshotBytes] + "\n[truncated]"
	}
	snap.Redacted = observability.RedactText(body)
	return snap
}

// SnapshotConfigs snapshots every path, read-only, preserving order.
func SnapshotConfigs(paths []string) []Snapshot {
	out := make([]Snapshot, 0, len(paths))
	for _, p := range paths {
		out = append(out, SnapshotForEvidence(p))
	}
	return out
}

// ConfigRefs returns the evidence-grade config reference list for a
// host: candidate paths plus their live ownership states. Contents
// never enter the map (see SnapshotConfigs for redacted bodies);
// callers store this under evidence "config" so reviewers see which
// files were consulted without ever seeing secrets.
func ConfigRefs(host string, home string) map[string]any {
	refs := HostConfigPaths(host, home)
	states := make(map[string]string, len(refs))
	for _, p := range refs {
		plan, err := Inspect(p)
		switch {
		case err != nil:
			states[p] = "unreadable"
		case plan.Missing:
			states[p] = "missing"
		case plan.Managed:
			states[p] = "managed"
		default:
			states[p] = "foreign"
		}
	}
	return map[string]any{
		"host":        host,
		"paths":       refs,
		"states":      states,
		"contents":    "never stored (see redacted snapshots)",
		"redacted_by": "observability.RedactText/RedactMap",
	}
}

// HostConfigPaths lists the config files consulted for host evidence.
// Home-scoped entries join home; the Devin pack is repo-relative and
// passes through untouched.
func HostConfigPaths(host, home string) []string {
	join := func(elems ...string) string {
		if home == "" {
			return strings.Join(elems, "/")
		}
		return filepath.Join(append([]string{home}, elems...)...)
	}
	switch host {
	case "opencode":
		return []string{join(".config", "opencode", "opencode.json")}
	case "claude":
		return []string{join(".claude.json")}
	case "codex":
		return []string{join(".codex", "config.toml")}
	case "devin":
		return []string{".devin/"}
	default:
		return []string{fmt.Sprintf("(unknown host %s: no config refs)", host)}
	}
}
