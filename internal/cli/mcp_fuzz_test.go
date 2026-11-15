// NOV-65: fuzzing of the MCP JSON-RPC framing. Properties: never
// panics, every reply is valid JSON (or empty for notifications), and
// fractional floats never coerce to int.
package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func FuzzHandleMCPLine(f *testing.F) {
	f.Add(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	f.Add(``)
	f.Add(`not json`)
	f.Add(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	f.Add(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x","arguments":{"limit":1.5}}}`)
	f.Add(strings.Repeat("x", 70000))
	f.Fuzz(func(t *testing.T, line string) {
		c, _, _ := newCLI(nil)
		out := c.handleMCPLine(line)
		if len(out) == 0 {
			return
		}
		var v any
		if err := json.Unmarshal(out, &v); err != nil {
			t.Fatalf("reply is not JSON for %q: %v", line, err)
		}
	})
}

func FuzzMcpIntRejectsFractions(f *testing.F) {
	f.Add(`{"limit":1.5}`)
	f.Add(`{"limit":2}`)
	f.Add(`{"limit":"x"}`)
	f.Add(`{}`)
	f.Fuzz(func(t *testing.T, doc string) {
		var m map[string]any
		if err := json.Unmarshal([]byte(doc), &m); err != nil {
			return
		}
		v, ok := mcpInt(m, "limit")
		if !ok {
			return
		}
		if float64(v) != m["limit"] {
			t.Fatalf("fractional value coerced to int for %q", doc)
		}
	})
}
