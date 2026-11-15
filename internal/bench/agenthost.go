// Headless opencode runner (NOV-68): executes one agent-task prompt
// with `opencode run --format json` inside the fixture repo. Output
// is parsed defensively line by line; token fields are picked up only
// when the host exposes them, otherwise usage stays unknown (NOV-62).
// Quota/auth failures are infra, never pass and never fail the task:
// the check decides pass/fail only after the agent ran.
package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"

	"regulus-ai/internal/executil"
	"regulus-ai/internal/trace"
)

// OpenCodeRunner runs prompts with the local opencode CLI.
type OpenCodeRunner struct {
	Bin string
}

// DefaultOpenCodeRunner locates the opencode binary.
func DefaultOpenCodeRunner() (OpenCodeRunner, error) {
	p, err := exec.LookPath("opencode")
	if err != nil {
		return OpenCodeRunner{}, ErrNoAgent{Reason: "opencode not on PATH"}
	}
	return OpenCodeRunner{Bin: p}, nil
}

// Run executes the prompt headless and returns raw output plus any
// host-exposed usage. A missing/unusable host is infra; only the
// hidden check decides pass/fail.
func (r OpenCodeRunner) Run(ctx context.Context, workdir, prompt, model string) (string, trace.HostUsage, error) {
	var usage trace.HostUsage
	args := []string{"run", "--format", "json"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, prompt)
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	cmd.Dir = workdir
	executil.HideWindow(cmd)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	start := time.Now()
	err := cmd.Run()
	_ = start
	raw := buf.String()
	if len(raw) > 256*1024 {
		raw = raw[:256*1024]
	}
	usage = scanOpenCodeUsage(raw)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return raw, usage, ctx.Err()
		}
		if strings.Contains(raw, "402") || strings.Contains(raw, "quota") || strings.Contains(raw, "Insufficient balance") {
			return raw, usage, fmt.Errorf("host unavailable (quota/auth): agent did not run")
		}
		return raw, usage, fmt.Errorf("host run failed: %v", err)
	}
	return raw, usage, nil
}

// scanOpenCodeUsage defensively extracts token counts from opencode
// --format json lines. Unknown shapes yield zero usage (unknown), and
// nothing is executed: JSON only. Reasoning folds into output (OpenAI
// accounting: completions include reasoning); cache read/write map to
// the matching HostUsage fields.
func scanOpenCodeUsage(raw string) trace.HostUsage {
	var u trace.HostUsage
	u.Source = trace.SourceUnknown
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			continue
		}
		u.Host = "opencode"
		if v := numAt(doc, "usage", "inputTokens"); v > 0 {
			u.InputTokens = v
		}
		if v := numAt(doc, "usage", "outputTokens"); v > 0 {
			u.OutputTokens = v
		}
		if v := numAt(doc, "tokens", "input"); v > 0 {
			u.InputTokens = v
		}
		if out, rs := numAt(doc, "tokens", "output"), numAt(doc, "tokens", "reasoning"); out+rs > 0 {
			u.OutputTokens = out + rs
		}
		if v := numAt(doc, "tokens", "cache", "read"); v > 0 {
			u.CacheRead = v
		}
		if v := numAt(doc, "tokens", "cache", "write"); v > 0 {
			u.CacheWrite = v
		}
		if m, ok := doc["model"].(string); ok && m != "" {
			u.Model = m
		}
	}
	if u.InputTokens > 0 || u.OutputTokens > 0 {
		u.Source = trace.SourceMeasuredHost
	}
	return u
}

func numAt(doc map[string]any, keys ...string) int64 {
	var cur any = doc
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0
		}
		cur = m[k]
	}
	switch v := cur.(type) {
	case float64:
		// Like mcpInt: fractional counts are refused, never truncated
		// into a quieter number.
		if v == math.Trunc(v) && v > 0 && v < 1e12 {
			return int64(v)
		}
	}
	return 0
}
