// NOV-68/71: unit coverage for the opencode headless runner and its
// defensive usage scanner (keeps the CRAP gate green: pure parsing,
// no spawn, no model).
package bench

import (
	"context"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/trace"
)

func TestScanOpenCodeUsageRealShapes(t *testing.T) {
	raw := "{\"type\":\"step_start\"}\n" +
		"{\"usage\":{\"inputTokens\":120,\"outputTokens\":30},\"model\":\"m1\"}\n" +
		"not json\n" +
		"\n" +
		"{\"tokens\":{\"input\":5,\"output\":7}}\n" +
		"{\"usage\":{\"inputTokens\":0},\"model\":\"\"}\n"
	u := scanOpenCodeUsage(raw)
	if u.Host != "opencode" {
		t.Fatalf("host = %q", u.Host)
	}
	if u.InputTokens != 5 || u.OutputTokens != 7 {
		t.Fatalf("counts = %+v, want last-wins 5/7", u)
	}
	if u.Model != "m1" {
		t.Fatalf("model = %q, want m1 (empty must not overwrite)", u.Model)
	}
	if u.Source != trace.SourceMeasuredHost {
		t.Fatalf("source = %q", u.Source)
	}
}

func TestScanOpenCodeUsageReasoningAndCache(t *testing.T) {
	raw := `{"tokens":{"input":3865,"output":4,"reasoning":22,"cache":{"read":1600,"write":0}}}
`
	u := scanOpenCodeUsage(raw)
	if u.InputTokens != 3865 || u.OutputTokens != 26 {
		t.Fatalf("counts = %+v, want 3865/26 (reasoning folded)", u)
	}
	if u.CacheRead != 1600 || u.CacheWrite != 0 {
		t.Fatalf("cache = %+v, want 1600/0", u)
	}
	if u.Source != trace.SourceMeasuredHost {
		t.Fatalf("source = %q", u.Source)
	}
}

func TestScanOpenCodeUsageUnknownStaysUnknown(t *testing.T) {
	for _, raw := range []string{"", "\n\n", "plain text", "[1,2]", "{\"usage\":{\"inputTokens\":-3}}", strings.Repeat("x", 300*1024)} {
		u := scanOpenCodeUsage(raw)
		if u.InputTokens != 0 || u.OutputTokens != 0 {
			t.Fatalf("hostile input must yield zero usage")
		}
		if u.Source != trace.SourceUnknown {
			t.Fatalf("source must stay unknown: %+v", u)
		}
	}
}

func TestNumAtShapes(t *testing.T) {
	doc := map[string]any{"usage": map[string]any{"inputTokens": 1.5, "outputTokens": float64(4)}}
	if got := numAt(doc, "usage", "outputTokens"); got != 4 {
		t.Fatalf("int float = %d", got)
	}
	if got := numAt(doc, "usage", "inputTokens"); got != 0 {
		t.Fatalf("fractional must be 0, got %d", got)
	}
	if got := numAt(doc, "missing", "deep"); got != 0 {
		t.Fatalf("missing path = %d", got)
	}
	if got := numAt(doc, "usage"); got != 0 {
		t.Fatalf("non-map leaf = %d", got)
	}
}

func TestOpenCodeRunnerMissingBinaryIsInfra(t *testing.T) {
	r := OpenCodeRunner{Bin: "/nonexistent-bin-xyz"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, err := r.Run(ctx, t.TempDir(), "hi", "")
	if err == nil || strings.Contains(err.Error(), "panic") {
		t.Fatalf("missing binary must fail cleanly, got %v", err)
	}
}

func TestScanOpenCodeUsageSumsSteps(t *testing.T) {
	raw := "{\"type\":\"step_finish\",\"part\":{\"type\":\"step-finish\",\"tokens\":{\"input\":100,\"output\":10,\"reasoning\":5,\"cache\":{\"read\":50,\"write\":1}}}}\n" +
		"{\"type\":\"step_finish\",\"part\":{\"type\":\"step-finish\",\"tokens\":{\"input\":200,\"output\":20,\"reasoning\":0,\"cache\":{\"read\":70,\"write\":2}}}}\n"
	u := scanOpenCodeUsage(raw)
	if u.InputTokens != 300 || u.OutputTokens != 35 {
		t.Fatalf("step sums = %+v, want 300 in / 35 out", u)
	}
	if u.CacheRead != 120 || u.CacheWrite != 3 {
		t.Fatalf("step cache = %+v, want 120/3", u)
	}
	if u.Source != trace.SourceMeasuredHost {
		t.Fatalf("source = %q", u.Source)
	}
}
