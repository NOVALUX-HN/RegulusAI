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
	"strings"
)

// Usage sources. measured-host counts come from the host itself;
// measured-local is the chars/4 proxy; proxy is an explicit fallback;
// unknown means the host exposed nothing.
const (
	SourceMeasuredHost  = "measured-host"
	SourceMeasuredLocal = "measured-local"
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
// and version, run/execution correlation). Source names which
// measurement produced the counts.
type HostUsage struct {
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read_tokens,omitempty"`
	CacheWrite   int64  `json:"cache_write_tokens,omitempty"`
	Model        string `json:"model,omitempty"`
	Host         string `json:"host,omitempty"`
	HostVersion  string `json:"host_version,omitempty"`
	Source       string `json:"source,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	ExecutionID  string `json:"execution_id,omitempty"`
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
		case SourceMeasuredHost, SourceMeasuredLocal, SourceProxy, SourceUnknown:
		default:
			return HostUsage{}, fmt.Errorf("source must be measured-host, measured-local, proxy or unknown, got %q", u.Source)
		}
	}
	return u, nil
}
