package gates

import (
	"strings"
	"testing"
)

// C6 SPLIT validate: ramas sin cubrir (when vacío, id inválido). Cada test
// fija mensajes idénticos al monolito.

func TestValidateSplitWhenEmpty(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", When: []string{""}}}}
	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "when must not contain empty patterns") {
		t.Fatalf("want when-empty error, got %v", err)
	}
	cfg2 := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", When: []string{"  "}}}}
	if err := validate(cfg2); err == nil || !strings.Contains(err.Error(), "when must not contain empty patterns") {
		t.Fatalf("want when-blank error, got %v", err)
	}
}

func TestValidateSplitIDCharset(t *testing.T) {
	bad := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", ID: "bad id!"}}}
	err := validate(bad)
	if err == nil || !strings.Contains(err.Error(), "id must match") {
		t.Fatalf("want id-charset error, got %v", err)
	}
	for _, okID := range []string{"", "  ", "ok-1_A2", "ABC-xyz_123"} {
		cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", ID: okID}}}
		if err := validate(cfg); err != nil {
			t.Fatalf("id %q must pass, got %v", okID, err)
		}
	}
}

func TestValidateSplitHelpersPreserveErrors(t *testing.T) {
	cases := []struct {
		name, want string
		cfg        Config
	}{
		{"empty gates", "no gates configured", Config{}},
		{"empty name", "name must not be empty", Config{Gates: []Gate{{Name: "", Layer: 1, Command: "x"}}}},
		{"empty command", "command must not be empty", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: ""}}}},
		{"zero layer", "layer must be", Config{Gates: []Gate{{Name: "a", Layer: 0, Command: "x"}}}},
		{"dup layer", "duplicate layer", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x"}, {Name: "b", Layer: 1, Command: "y"}}}},
		{"dup name", "duplicate gate name", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x"}, {Name: "a", Layer: 2, Command: "y"}}}},
		{"bad metric op", "metric operator", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Metric: &Metric{Extract: ExtractLastLineFloat, Threshold: 1, Operator: "<"}}}}},
		{"bad metric extract", "metric extract", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Metric: &Metric{Extract: "magic", Threshold: 1, Operator: OpAtLeast}}}}},
		{"bad repro", "reproducibility must be", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Reproducibility: "bogus"}}}},
		{"bad isolate", "isolate must be docker", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Isolate: "vm"}}}},
		{"bad onfail", "on_fail must be", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", OnFail: "explode"}}}},
		{"empty profile", "profiles must not contain empty", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Profiles: []string{""}}}}},
		{"empty when", "when must not contain empty", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", When: []string{""}}}}},
		{"bad id", "id must match", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", ID: "a/b"}}}},
		{"empty allow", "allowlist entries must not be empty", Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x"}}, Execution: &ExecutionConfig{Allowlist: []string{""}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
	ok := Config{Gates: []Gate{{
		Name: "a", Layer: 1, Command: "x",
		Metric:          &Metric{Extract: ExtractLastLineFloat, Threshold: 1, Operator: OpAtLeast},
		Reproducibility: "exact", Isolate: "docker", OnFail: "advisory",
		Profiles: []string{"full"}, When: []string{"*.go"}, ID: "ok-1",
	}}, Execution: &ExecutionConfig{Allowlist: []string{"go"}}}
	if err := validate(ok); err != nil {
		t.Fatalf("valid config must pass, got %v", err)
	}
}
