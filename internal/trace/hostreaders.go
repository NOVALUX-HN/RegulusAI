// Host readers (NOV-73): real token counts from host session data,
// read-only, failing closed on drift. Every reader returns a HostUsage
// validated by ParseHostUsage (shape, bounds, absurdity caps); transcript
// content is untrusted data and is never logged, executed or stored —
// only the extracted counts, model and identity flow out.
//
// Supported sources (verified on this machine, versions recorded):
//   - opencode: `opencode session export <id>` JSON, info.tokens subtree
//     (session_v2 accounting: input/output/reasoning/cache read/write).
//   - claude: ~/.claude/projects/*/*.jsonl transcripts, assistant
//     message.usage sums (input/output/cache_read/cache_creation).
//   - codex: ~/.codex/sessions/*/*/*/rollout-*.jsonl token_usage_record
//     lines (input/cached/cache_write/output/reasoning).
//
// Anything else stays unknown (host exposes none) with the verified
// reason — never a guessed number.
package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// maxReaderBytes caps one host session document read: transcripts and
// rollouts are small text; anything bigger is not a session export.
// Hitting the cap fails closed (partial sums never report).
const maxReaderBytes = 64 << 20

// checked validates a reader-built report through the same strict gate
// as hand-written usage: bounds, absurdity caps, single-line ids.
func checked(u HostUsage) (HostUsage, error) {
	raw, err := json.Marshal(u)
	if err != nil {
		return HostUsage{}, err
	}
	return ParseHostUsage(raw)
}

// opencodeExport mirrors the info subtree of `opencode session export`:
// only counts, model, cost and identity — never transcript content.
type opencodeExport struct {
	Info struct {
		ID    string `json:"id"`
		Model struct {
			ID         string `json:"id"`
			ProviderID string `json:"providerID"`
		} `json:"model"`
		Cost   float64 `json:"cost"`
		Tokens struct {
			Input     int64 `json:"input"`
			Output    int64 `json:"output"`
			Reasoning int64 `json:"reasoning"`
			Cache     struct {
				Read  int64 `json:"read"`
				Write int64 `json:"write"`
			} `json:"cache"`
		} `json:"tokens"`
	} `json:"info"`
}

// ParseOpencodeExport reads one `opencode session export` document,
// attributing its session totals. Reasoning tokens fold into output
// (OpenAI accounting: completions include reasoning); the fold is
// explicit so totals stay meaningful. hostVersion comes from
// `opencode --version` (empty when the binary is absent: unknown,
// never guessed).
func ParseOpencodeExport(data []byte, hostVersion string) (HostUsage, error) {
	if len(data) > maxReaderBytes {
		return HostUsage{}, fmt.Errorf("opencode export too large: %d bytes", len(data))
	}
	var exp opencodeExport
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&exp); err != nil {
		return HostUsage{}, fmt.Errorf("malformed opencode export: %v", err)
	}
	if strings.TrimSpace(exp.Info.ID) == "" {
		return HostUsage{}, fmt.Errorf("opencode export has no session id")
	}
	return checked(HostUsage{
		InputTokens:  exp.Info.Tokens.Input,
		OutputTokens: exp.Info.Tokens.Output + exp.Info.Tokens.Reasoning,
		CacheRead:    exp.Info.Tokens.Cache.Read,
		CacheWrite:   exp.Info.Tokens.Cache.Write,
		Model:        exp.Info.Model.ID,
		Host:         "opencode",
		HostVersion:  hostVersion,
		Source:       SourceMeasuredHost,
		RunID:        exp.Info.ID,
	})
}

// claudeUsage mirrors one assistant message.usage block in a Claude
// Code transcript. input_tokens excludes cache; the cache_read and
// cache_creation breakdowns map onto cache read/write.
type claudeUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

// ParseClaudeTranscript sums assistant usage over a Claude Code
// transcript (*.jsonl, one object per line). Unknown lines are skipped;
// malformed JSON fails closed. The first non-empty message.model wins;
// the transcript-level sessionId (summary lines) becomes the run id.
func ParseClaudeTranscript(r io.Reader) (HostUsage, error) {
	dec := json.NewDecoder(io.LimitReader(r, maxReaderBytes+1))
	var u HostUsage
	u.Host = "claude"
	u.Source = SourceMeasuredHost
	lines := 0
	for dec.More() {
		var o struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionId"`
			Message   *struct {
				Role  string       `json:"role"`
				Model string       `json:"model"`
				Usage *claudeUsage `json:"usage"`
			} `json:"message"`
		}
		if err := dec.Decode(&o); err != nil {
			return HostUsage{}, fmt.Errorf("malformed claude transcript at object %d: %v", lines, err)
		}
		lines++
		if u.RunID == "" && o.SessionID != "" {
			u.RunID = o.SessionID
		}
		if o.Message == nil || o.Message.Role != "assistant" || o.Message.Usage == nil {
			continue
		}
		if u.Model == "" {
			u.Model = o.Message.Model
		}
		us := o.Message.Usage
		u.InputTokens += us.InputTokens
		u.OutputTokens += us.OutputTokens
		u.CacheRead += us.CacheReadInputTokens
		u.CacheWrite += us.CacheCreationInputTokens
	}
	if lines == 0 {
		return HostUsage{}, fmt.Errorf("empty claude transcript")
	}
	return checked(u)
}

// codexUsage mirrors one token_usage_record line in a Codex rollout.
// reasoning_output_tokens is a subset breakdown of output_tokens
// (total = input + output), so output maps straight across.
type codexUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

// ParseCodexRollout sums token_usage_record lines in a Codex session
// rollout (*.jsonl). The session id comes from the session_meta line;
// the model from the first response line that names one. Malformed
// lines fail closed; lines of other types are skipped.
func ParseCodexRollout(r io.Reader) (HostUsage, error) {
	dec := json.NewDecoder(io.LimitReader(r, maxReaderBytes+1))
	var u HostUsage
	u.Host = "codex"
	u.Source = SourceMeasuredHost
	lines := 0
	records := 0
	for dec.More() {
		var o struct {
			Type    string `json:"type"`
			Payload *struct {
				SessionID string      `json:"session_id"`
				Model     string      `json:"model"`
				Usage     *codexUsage `json:"usage"`
			} `json:"payload"`
		}
		if err := dec.Decode(&o); err != nil {
			return HostUsage{}, fmt.Errorf("malformed codex rollout at object %d: %v", lines, err)
		}
		lines++
		if o.Payload == nil {
			continue
		}
		if u.RunID == "" && o.Payload.SessionID != "" {
			u.RunID = o.Payload.SessionID
		}
		if u.Model == "" && o.Payload.Model != "" {
			u.Model = o.Payload.Model
		}
		if o.Type != "token_usage_record" || o.Payload.Usage == nil {
			continue
		}
		us := o.Payload.Usage
		u.InputTokens += us.InputTokens
		u.OutputTokens += us.OutputTokens
		u.CacheRead += us.CachedInputTokens
		u.CacheWrite += us.CacheWriteInputTokens
		records++
	}
	if lines == 0 {
		return HostUsage{}, fmt.Errorf("empty codex rollout")
	}
	if records == 0 {
		return HostUsage{}, fmt.Errorf("codex rollout has no token_usage_record lines")
	}
	return checked(u)
}
