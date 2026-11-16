// NOV-73: host readers over anonymized real shapes. Fixtures mirror the
// verified on-disk formats (opencode `session export` info subtree,
// Claude Code transcript message.usage, Codex rollout token_usage_record
// lines) with fake ids and small counts, plus malformed and hostile
// inputs. Readers never see a real transcript here.
package trace

import (
	"strings"
	"testing"
)

const opencodeFixture = `{"info":{"id":"ses_fixture01","projectID":"p1","model":{"id":"beam-free","providerID":"opencode-go","variant":"default"},"cost":0,"tokens":{"input":58516,"output":1266,"reasoning":5302,"cache":{"read":519296,"write":0}},"outcome":"succeeded"}}`

func TestParseOpencodeExport(t *testing.T) {
	u, err := ParseOpencodeExport([]byte(opencodeFixture), "2.2.5")
	if err != nil {
		t.Fatal(err)
	}
	if u.InputTokens != 58516 || u.OutputTokens != 1266+5302 {
		t.Fatalf("counts = %d/%d, want 58516/6568 (reasoning folded into output)", u.InputTokens, u.OutputTokens)
	}
	if u.CacheRead != 519296 || u.CacheWrite != 0 {
		t.Fatalf("cache = %d/%d, want 519296/0", u.CacheRead, u.CacheWrite)
	}
	if u.Model != "beam-free" || u.Host != "opencode" || u.HostVersion != "2.2.5" {
		t.Fatalf("identity = %+v, want beam-free/opencode/2.2.5", u)
	}
	if u.Source != SourceMeasuredHost || u.RunID != "ses_fixture01" {
		t.Fatalf("attribution = %q/%q, want measured-host/ses_fixture01", u.Source, u.RunID)
	}
}

func TestParseOpencodeExportRejects(t *testing.T) {
	for name, doc := range map[string]string{
		"garbage":   `not json`,
		"no id":     `{"info":{"model":{"id":"m"},"tokens":{"input":1}}}`,
		"negative":  `{"info":{"id":"s","tokens":{"input":-1}}}`,
		"absurd":    `{"info":{"id":"s","tokens":{"input":2000000000000}}}`,
		"multiline": "{\"info\":{\"id\":\"s\\n\",\"tokens\":{}}}",
	} {
		if _, err := ParseOpencodeExport([]byte(doc), ""); err == nil {
			t.Fatalf("%s must fail closed", name)
		}
	}
}

const claudeFixture = `{"type":"summary","sessionId":"abc-123"}
{"type":"x","timestamp":1}
{"parentUuid":"u1","type":"assistant","message":{"role":"assistant","model":"claude-opus","usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":1000,"cache_creation_input_tokens":50}}}
{"parentUuid":"u2","type":"assistant","message":{"role":"assistant","model":"claude-opus","usage":{"input_tokens":300,"output_tokens":40,"cache_read_input_tokens":2000,"cache_creation_input_tokens":0}}}
{"parentUuid":"u3","type":"assistant","message":{"role":"user","usage":{"input_tokens":9999}}}
`

func TestParseClaudeTranscript(t *testing.T) {
	u, err := ParseClaudeTranscript(strings.NewReader(claudeFixture))
	if err != nil {
		t.Fatal(err)
	}
	if u.InputTokens != 400 || u.OutputTokens != 60 {
		t.Fatalf("counts = %d/%d, want 400/60", u.InputTokens, u.OutputTokens)
	}
	if u.CacheRead != 3000 || u.CacheWrite != 50 {
		t.Fatalf("cache = %d/%d, want 3000/50", u.CacheRead, u.CacheWrite)
	}
	if u.Model != "claude-opus" || u.Host != "claude" || u.RunID != "abc-123" {
		t.Fatalf("identity = %+v, want claude-opus/claude/abc-123", u)
	}
	if u.Source != SourceMeasuredHost {
		t.Fatalf("source = %q, want measured-host", u.Source)
	}
}

func TestParseClaudeTranscriptRejects(t *testing.T) {
	for name, doc := range map[string]string{
		"empty":    ``,
		"garbage":  `{oops`,
		"negative": "{\"message\":{\"role\":\"assistant\",\"usage\":{\"input_tokens\":-5}}}",
	} {
		if _, err := ParseClaudeTranscript(strings.NewReader(doc)); err == nil {
			t.Fatalf("%s must fail closed", name)
		}
	}
}

const codexFixture = `{"timestamp":"2026-01-01T00:00:00Z","ordinal":0,"type":"session_meta","payload":{"session_id":"01a1","model_provider":"openai"}}
{"timestamp":"2026-01-01T00:00:01Z","ordinal":1,"type":"turn_context","payload":{"model":"gpt-6-luna"}}
{"timestamp":"2026-01-01T00:00:02Z","ordinal":2,"type":"token_usage_record","payload":{"thread_id":"t","turn_id":"1","session_id":"01a1","usage":{"input_tokens":17796,"cached_input_tokens":10752,"cache_write_input_tokens":0,"output_tokens":14,"reasoning_output_tokens":0,"total_tokens":17810}}}
{"timestamp":"2026-01-01T00:00:03Z","ordinal":3,"type":"token_usage_record","payload":{"thread_id":"t","turn_id":"2","session_id":"01a1","usage":{"input_tokens":100,"cached_input_tokens":0,"cache_write_input_tokens":5,"output_tokens":7,"reasoning_output_tokens":0,"total_tokens":107}}}
`

func TestParseCodexRollout(t *testing.T) {
	u, err := ParseCodexRollout(strings.NewReader(codexFixture))
	if err != nil {
		t.Fatal(err)
	}
	if u.InputTokens != 17896 || u.OutputTokens != 21 {
		t.Fatalf("counts = %d/%d, want 17896/21", u.InputTokens, u.OutputTokens)
	}
	if u.CacheRead != 10752 || u.CacheWrite != 5 {
		t.Fatalf("cache = %d/%d, want 10752/5", u.CacheRead, u.CacheWrite)
	}
	if u.Model != "gpt-6-luna" || u.Host != "codex" || u.RunID != "01a1" {
		t.Fatalf("identity = %+v, want gpt-6-luna/codex/01a1", u)
	}
}

func TestParseCodexRolloutRejects(t *testing.T) {
	for name, doc := range map[string]string{
		"empty":      ``,
		"garbage":    `{oops`,
		"no records": "{\"type\":\"session_meta\",\"payload\":{\"session_id\":\"x\"}}\n",
		"absurd":     "{\"type\":\"token_usage_record\",\"payload\":{\"session_id\":\"x\",\"usage\":{\"input_tokens\":2000000000000}}}",
	} {
		if _, err := ParseCodexRollout(strings.NewReader(doc)); err == nil {
			t.Fatalf("%s must fail closed", name)
		}
	}
}
