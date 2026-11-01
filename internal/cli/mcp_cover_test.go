package cli

import "testing"

// COVER (gap-sweep/G3): mcpStrList carried CRAP 25.85 at 27.3% coverage
// (array/default branches never exercised). Pure unit coverage, no
// behavior change.
func TestMcpStrListCoercionCover(t *testing.T) {
	if got, ok := mcpStrList(map[string]any{"k": "a, b"}, "k"); !ok || got != "a, b" {
		t.Fatalf("string passes through: (%q,%v)", got, ok)
	}
	if got, ok := mcpStrList(map[string]any{"k": []any{"a", "b"}}, "k"); !ok || got != "a,b" {
		t.Fatalf("array joins with comma: (%q,%v)", got, ok)
	}
	if _, ok := mcpStrList(map[string]any{"k": []any{"a", 1}}, "k"); ok {
		t.Fatal("array with non-string must refuse")
	}
	if _, ok := mcpStrList(map[string]any{}, "missing"); ok {
		t.Fatal("missing key must report absent")
	}
	if got, ok := mcpStrList(map[string]any{"k": nil}, "k"); ok || got != "" {
		t.Fatalf("nil must report absent: (%q,%v)", got, ok)
	}
	if _, ok := mcpStrList(map[string]any{"k": 42}, "k"); ok {
		t.Fatal("unsupported type must report absent")
	}
	if got, ok := mcpStrList(map[string]any{"k": []any{}}, "k"); !ok || got != "" {
		t.Fatalf("empty array joins to empty: (%q,%v)", got, ok)
	}
}
