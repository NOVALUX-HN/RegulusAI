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
