// F11.5 (NOV-91): readers preserve reasoning, fail closed on overflow,
// redact secrets, tolerate verified drift (nullable opencode model,
// extra claude usage keys, all-zero usage lines) and parse the same
// document to the same totals twice (duplicate-tolerant ingest).
package trace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpencodeReaderPreservesReasoning(t *testing.T) {
	u, err := ParseOpencodeExport([]byte(opencodeFixture), "2.2.5")
	if err != nil {
		t.Fatal(err)
	}
	if u.ReasoningTokens != 5302 {
		t.Fatalf("reasoning = %d, want 5302 preserved from the export", u.ReasoningTokens)
	}
	if u.OutputTokens != 1266+5302 {
		t.Fatalf("output = %d, want 6568 (fold documented, originals preserved)", u.OutputTokens)
	}
	// 2026-11-16 drift: some live exports carry a null model subtree.
	nullable := `{"info":{"id":"ses_drift01","model":null,"tokens":{"input":557,"output":593,"reasoning":0,"cache":{"read":42537,"write":11696}}}}`
	d, err := ParseOpencodeExport([]byte(nullable), "2.2.5")
	if err != nil {
		t.Fatalf("null model subtree must parse (unknown model, not error): %v", err)
	}
	if d.Model != "" || d.InputTokens != 557 || d.CacheRead != 42537 {
		t.Fatalf("drift export misparsed: %+v", d)
	}
}

func TestOpencodeReaderOverflowFailsClosed(t *testing.T) {
	big := `{"info":{"id":"s","tokens":{"input":900000000000,"output":900000000000,"reasoning":900000000000}}}`
	if _, err := ParseOpencodeExport([]byte(big), "2.2.5"); err == nil {
		t.Fatal("absurd export must fail closed, passed")
	}
}

func TestOpencodeReaderOversizeFailsClosed(t *testing.T) {
	huge := append([]byte(`{"info":{"id":"s","tokens":{"input":1`), []byte(strings.Repeat(" ", maxReaderBytes+8))...)
	if _, err := ParseOpencodeExport(huge, ""); err == nil {
		t.Fatal("oversize export must fail closed, passed")
	}
}

func TestClaudeReaderToleratesDriftAndZeros(t *testing.T) {
	// 2026-11-16 drift: extra usage keys are ignored; all-zero usage
	// lines (service bookkeeping) contribute nothing but still count
	// as evidence of a measured host.
	doc := `{"type":"summary","sessionId":"drift-1"}
{"type":"queue-operation","operation":"enqueue","sessionId":"drift-1"}
{"parentUuid":"u1","type":"assistant","message":{"role":"assistant","model":"claude-opus","usage":{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"server_tool_use":{"web_search_requests":0},"service_tier":"standard"}}}
{"parentUuid":"u2","type":"assistant","message":{"role":"assistant","model":"claude-opus","usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":5,"cache_creation_input_tokens":1,"output_tokens_details":null}}}
`
	u, err := ParseClaudeTranscript(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("drift transcript rejected: %v", err)
	}
	if u.InputTokens != 10 || u.OutputTokens != 2 || u.CacheRead != 5 || u.CacheWrite != 1 {
		t.Fatalf("drift transcript misparsed: %+v", u)
	}
	if u.Source != SourceMeasuredHost || u.RunID != "drift-1" {
		t.Fatalf("drift attribution lost: %+v", u)
	}
}

func TestClaudeReaderRedactsAndSkipsSecrets(t *testing.T) {
	// Transcript content is untrusted: a pasted key in a user message
	// must never flow into the usage report.
	doc := `{"type":"summary","sessionId":"sec-1"}
{"type":"user","message":{"role":"user","content":"my key is sk-ant-oat1-fakevalue1234567890 do not share"}}
{"type":"assistant","message":{"role":"assistant","model":"m","usage":{"input_tokens":3,"output_tokens":1}}}
`
	u, err := ParseClaudeTranscript(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if u.InputTokens != 3 {
		t.Fatalf("counts = %d, want 3", u.InputTokens)
	}
	raw, _ := json.Marshal(u)
	if strings.Contains(string(raw), "sk-ant-oat1") {
		t.Fatal("secret from transcript content leaked into the usage report")
	}
}

func TestReadersAreDuplicateTolerant(t *testing.T) {
	first, err := ParseOpencodeExport([]byte(opencodeFixture), "2.2.5")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseOpencodeExport([]byte(opencodeFixture), "2.2.5")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("same export parsed twice differs:\n%+v\n%+v", first, second)
	}
	c1, err := ParseCodexRollout(strings.NewReader(codexFixture))
	if err != nil {
		t.Fatal(err)
	}
	c2, err := ParseCodexRollout(strings.NewReader(codexFixture))
	if err != nil {
		t.Fatal(err)
	}
	if c1 != c2 {
		t.Fatalf("same rollout parsed twice differs:\n%+v\n%+v", c1, c2)
	}
}

func TestCodexReaderPreservesReasoning(t *testing.T) {
	doc := `{"type":"session_meta","payload":{"session_id":"rx"}}
{"type":"token_usage_record","payload":{"session_id":"rx","model":"gpt-6-luna","usage":{"input_tokens":100,"cached_input_tokens":10,"cache_write_input_tokens":2,"output_tokens":20,"reasoning_output_tokens":7,"total_tokens":120}}}
`
	u, err := ParseCodexRollout(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if u.ReasoningTokens != 7 || u.OutputTokens != 20 {
		t.Fatalf("codex reasoning must be preserved alongside output: %+v", u)
	}
}

func TestCodexReaderOverflowFailsClosed(t *testing.T) {
	doc := "{\"type\":\"session_meta\",\"payload\":{\"session_id\":\"rx\"}}\n" +
		"{\"type\":\"token_usage_record\",\"payload\":{\"session_id\":\"rx\",\"usage\":{\"input_tokens\":900000000000}}}\n" +
		"{\"type\":\"token_usage_record\",\"payload\":{\"session_id\":\"rx\",\"usage\":{\"input_tokens\":900000000000}}}\n"
	if _, err := ParseCodexRollout(strings.NewReader(doc)); err == nil {
		t.Fatal("overflowing rollout must fail closed, passed")
	}
}
