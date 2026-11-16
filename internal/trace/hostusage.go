// Neutral host-usage schema (NOV-62, S3): real token counts from a
// host, attributed by task/session/execution. Every unknown field is
// unknown, never 0: zero means the host exposed nothing. Transcript
// content is untrusted data (NOV-31): ParseHostUsage validates shape
// and bounds but never executes or interprets it.
package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Usage sources. measured-host counts come from the host itself;
// measured-local is the chars/4 proxy; estimated is an explicit
// fallback the caller labels as an estimate (never silent);
// proxy is an explicit fallback; unknown means the host exposed
// nothing. Unknown is never a number: it is the absence of a
// measurement, so unknown counts never sum to zero.
const (
	SourceMeasuredHost  = "measured-host"
	SourceMeasuredLocal = "measured-local"
	SourceEstimated     = "estimated"
	SourceProxy         = "proxy"
	SourceUnknown       = "unknown"
)

// maxUsageBytes caps one normalized usage document: usage is a small
// struct, never a transcript dump.
const maxUsageBytes = 64 * 1024

// maxUsageCount caps any single token count: above this the figure is
// corrupt or hostile, never a real call.
const maxUsageCount = int64(1_000_000_000_000)

// maxUsageField caps free-text usage fields (model, host, versions,
// ids): identifiers, not prose.
const maxUsageField = 512

// HostUsage is one neutral usage report: real token counts from a
// host plus the identity needed to attribute them (model, host name
// and version, run/execution/task/agent correlation). Source names
// which measurement produced the counts. ReasoningTokens preserves
// the original reasoning breakdown: some hosts fold reasoning into
// output (OpenAI completions accounting), so OutputTokens may
// already include reasoning — ReasoningTokens keeps the split so no
// original figure is lost. Task and Agent name the served task and
// agent (F11.1 correlation keys); Reason carries the verified reason
// for declared states (unknown hosts name why, never a bare label).
type HostUsage struct {
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	ReasoningTokens int64  `json:"reasoning_tokens,omitempty"`
	CacheRead       int64  `json:"cache_read_tokens,omitempty"`
	CacheWrite      int64  `json:"cache_write_tokens,omitempty"`
	Model           string `json:"model,omitempty"`
	Host            string `json:"host,omitempty"`
	HostVersion     string `json:"host_version,omitempty"`
	Source          string `json:"source,omitempty"`
	RunID           string `json:"run_id,omitempty"`
	ExecutionID     string `json:"execution_id,omitempty"`
	Task            string `json:"task,omitempty"`
	Agent           string `json:"agent,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

// ParseHostUsage parses one normalized usage document strictly:
// exact fields only, bounded size, non-negative counts under the
// absurdity cap, single-line short strings. Hostile or malformed
// input fails closed with a naming error; nothing is executed and
// nothing is coerced into a number.
func ParseHostUsage(data []byte) (HostUsage, error) {
	var u HostUsage
	if len(data) > maxUsageBytes {
		return HostUsage{}, fmt.Errorf("usage document too large: %d bytes (max %d)", len(data), maxUsageBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&u); err != nil {
		return HostUsage{}, fmt.Errorf("malformed usage: %v", err)
	}
	if dec.More() {
		return HostUsage{}, fmt.Errorf("malformed usage: trailing data")
	}
	for field, v := range map[string]int64{
		"input_tokens": u.InputTokens, "output_tokens": u.OutputTokens,
		"reasoning_tokens":  u.ReasoningTokens,
		"cache_read_tokens": u.CacheRead, "cache_write_tokens": u.CacheWrite,
	} {
		if v < 0 {
			return HostUsage{}, fmt.Errorf("%s must not be negative, got %d", field, v)
		}
		if v > maxUsageCount {
			return HostUsage{}, fmt.Errorf("%s absurd: %d (max %d)", field, v, maxUsageCount)
		}
	}
	for field, v := range map[string]string{
		"model": u.Model, "host": u.Host, "host_version": u.HostVersion,
		"source": u.Source, "run_id": u.RunID, "execution_id": u.ExecutionID,
		"task": u.Task, "agent": u.Agent, "reason": u.Reason,
	} {
		if strings.ContainsAny(v, "\n\r") {
			return HostUsage{}, fmt.Errorf("%s must be a single line", field)
		}
		if len(v) > maxUsageField {
			return HostUsage{}, fmt.Errorf("%s too long: %d chars (max %d)", field, len(v), maxUsageField)
		}
	}
	if u.Source != "" {
		switch u.Source {
		case SourceMeasuredHost, SourceMeasuredLocal, SourceEstimated, SourceProxy, SourceUnknown:
		default:
			return HostUsage{}, fmt.Errorf("source must be measured-host, measured-local, estimated, proxy or unknown, got %q", u.Source)
		}
	}
	return u, nil
}

// secretPatterns matches credential shapes that must never reach
// the store: pasted keys travel inside untrusted transcript content
// (NOV-31) and reader-adjacent free text. Every match is replaced
// with [REDACTED]; surrounding text is preserved verbatim.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{10,}`),
	regexp.MustCompile(`xox[bpas]-[A-Za-z0-9\-]+`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]+`),
	regexp.MustCompile(`gho_[A-Za-z0-9]+`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]+`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/=]+`),
	regexp.MustCompile(`(?i)(api[_-]?key\s*[:=]\s*)[^\s"']+`),
}

// RedactSecrets masks credential shapes in free text, preserving
// everything else verbatim. Counts and labels are untouched: only
// secrets are replaced, with [REDACTED].
func RedactSecrets(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// RedactHostUsage masks credential shapes in every free-text field
// of a usage report before it reaches the store. Counts and the
// source label pass through unchanged.
func RedactHostUsage(u HostUsage) HostUsage {
	u.Model = RedactSecrets(u.Model)
	u.Host = RedactSecrets(u.Host)
	u.HostVersion = RedactSecrets(u.HostVersion)
	u.RunID = RedactSecrets(u.RunID)
	u.ExecutionID = RedactSecrets(u.ExecutionID)
	u.Task = RedactSecrets(u.Task)
	u.Agent = RedactSecrets(u.Agent)
	u.Reason = RedactSecrets(u.Reason)
	return u
}

// addCounts sums two token counts, failing closed on int64
// overflow instead of wrapping into a negative (which the bounds
// check would then misread as a different error, or worse a small
// positive on double wrap). Folds use it so hostile totals abort
// the report instead of corrupting it.
func addCounts(a, b int64) (int64, error) {
	const maxInt = int64(^uint64(0) >> 1)
	const minInt = -maxInt - 1
	if (b > 0 && a > maxInt-b) || (b < 0 && a < minInt-b) {
		return 0, fmt.Errorf("token counts overflow: %d + %d", a, b)
	}
	return a + b, nil
}

// HostProvenance is the declared per-host measurement state: which
// source the host's counts carry and the verified reason behind the
// label. Unknown always names why (format verified but unattributed,
// or the host exposes nothing) — never a bare label.
type HostProvenance struct {
	Host   string `json:"host"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// KnownHosts lists every host F11.5 audits, in stable report order.
var KnownHosts = []string{"opencode", "claude", "codex", "devin"}

// DeclareHostProvenance returns the verified F11.5 measurement state
// of one host (audit 2026-11-16, this machine; versions live in
// docs/host-usage.md). opencode reports measured-host end to end;
// claude and codex carry verified readers but no attributed live
// session here, so they stay unknown with the reason; devin exposes
// no counters. Anything else is unknown with an unverified-host
// reason — never a guessed number.
func DeclareHostProvenance(host string) HostProvenance {
	switch host {
	case "opencode":
		return HostProvenance{Host: host, Source: SourceMeasuredHost,
			Reason: "session export info.tokens verified (opencode v2.2.5, 2026-11-16); end-to-end run reused from NOV-73"}
	case "claude":
		return HostProvenance{Host: host, Source: SourceUnknown,
			Reason: "transcript message.usage format verified (claude 2.1.325, 2026-11-16); no live session attributed"}
	case "codex":
		return HostProvenance{Host: host, Source: SourceUnknown,
			Reason: "rollout token_usage_record format verified (cli 0.169.0, model gpt-6-luna, 2026-11-16); no task attributed"}
	case "devin":
		return HostProvenance{Host: host, Source: SourceUnknown,
			Reason: "host exposes none (Devin MCP exposes no counters, verified F8)"}
	default:
		return HostProvenance{Host: host, Source: SourceUnknown,
			Reason: "unverified host (no verified reader); counts never guessed"}
	}
}
