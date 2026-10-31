// Redaction and retention policy (Phase 8 T1, spec S6).
//
// Redaction: secret-bearing values never reach disk. Matching is
// deny-list, case-insensitive substring over key names — api_key,
// apikey, secret, token, password, credential, authorization, and
// private prompt fields (system_prompt, private_prompt, prompt,
// private_key) — with values replaced by "[redacted]". Free-text fields
// (notably error details) are additionally scrubbed for embedded
// key=value / "key": "value" pairs and for bearer/api-key credential
// patterns, so secrets inline in messages are redacted too. Every Event
// write path goes through RedactEvent; there is no unredacted writer.
//
// Retention, local-only: telemetry lives under .regulus/observability/,
// which is gitignored with the rest of .regulus and never committed.
// Logs are append-only JSONL and unbounded by default; callers cap a
// scope with Prune, keeping only the last keep lines (DefaultKeep is
// the suggested cap). Prune mirrors the trace package rotation:
// atomic temp-file rewrite, keep <= 0 keeps everything, unknown scopes
// are a no-op.
package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Redacted replaces secret-bearing values.
const Redacted = "[redacted]"

// DefaultKeep is the suggested per-scope retention cap (lines) for
// Prune. Scopes grow append-only; callers prune beyond this.
const DefaultKeep = 1000

// sensitiveKeys is the deny-list for key-name matching (case-insensitive
// substring): API keys, secrets, credentials, private prompts.
var sensitiveKeys = []string{
	"api_key",
	"apikey",
	"secret",
	"token",
	"password",
	"credential",
	"authorization",
	"system_prompt",
	"private_prompt",
	"prompt",
	"private_key",
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, k := range sensitiveKeys {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

var (
	bearerRe = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9\-._~+/=]+`)
	jsonKVRe = regexp.MustCompile(`(?i)"([^"]*(?:api_key|apikey|secret|token|password|credential|authorization|system_prompt|private_prompt|prompt|private_key)[^"]*)"\s*:\s*"[^"]*"`)
	bareKVRe = regexp.MustCompile(`(?i)\b(api_key|apikey|secret|token|password|credential|authorization|system_prompt|private_prompt|prompt|private_key)\b\s*([:=])\s*("[^"]*"|'[^']*'|[^\s,}]+)`)
)

// redactText scrubs bearer credentials and deny-listed key=value pairs
// embedded in free text.
func redactText(s string) string {
	s = bearerRe.ReplaceAllString(s, "bearer "+Redacted)
	s = jsonKVRe.ReplaceAllString(s, `"$1": "`+Redacted+`"`)
	s = bareKVRe.ReplaceAllString(s, `$1$2`+Redacted)
	return s
}

// RedactMap returns a copy of in with deny-listed keys replaced by
// Redacted. String values under benign keys are still scrubbed for
// embedded credential patterns; nested maps and slices recurse.
func RedactMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if isSensitiveKey(k) {
			out[k] = Redacted
			continue
		}
		out[k] = redactValue(v)
	}
	return out
}

func redactValue(v any) any {
	switch t := v.(type) {
	case string:
		return redactText(t)
	case map[string]any:
		return RedactMap(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = redactValue(e)
		}
		return out
	default:
		return v
	}
}

// RedactEvent returns a copy of e with every string field scrubbed for
// embedded credential patterns.
func RedactEvent(e Event) Event {
	e.Component = redactText(e.Component)
	e.ExecutionID = redactText(e.ExecutionID)
	e.TaskID = redactText(e.TaskID)
	e.AgentID = redactText(e.AgentID)
	e.CorrelationID = redactText(e.CorrelationID)
	e.EventType = redactText(e.EventType)
	e.ErrorDetails = redactText(e.ErrorDetails)
	return e
}

// Prune rewrites a task scope keeping only the last keep events
// (rotation for unbounded logs). keep <= 0 keeps everything; unknown
// scopes are a no-op. It returns the kept event count.
func Prune(root, taskID string, keep int) (int, error) {
	entries, err := List(root, taskID)
	if err != nil {
		return 0, err
	}
	if keep <= 0 || len(entries) <= keep {
		return len(entries), nil
	}
	entries = entries[len(entries)-keep:]
	var lines []string
	for _, e := range entries {
		raw, err := json.Marshal(e)
		if err != nil {
			return 0, err
		}
		lines = append(lines, string(raw))
	}
	if err := os.MkdirAll(filepath.Dir(File(root, taskID)), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(File(root, taskID)), "observability-*.jsonl")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()
	body := strings.Join(lines, "\n") + "\n"
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return 0, err
	}
	if err := os.Rename(tmpName, File(root, taskID)); err != nil {
		return 0, err
	}
	return len(entries), nil
}
