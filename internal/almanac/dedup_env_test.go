package almanac

// M2 (S2) env-adjustable dedup: REGULUS_DEDUP_THRESHOLD overrides the
// 0.92 bar when it parses to [0,1]; anything else keeps the default.

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// dedupParaphrase is the fixture pair whose sim sits between 0.92 and
// 0.99 (typo/pooling at ~0.9350): refused by default, saved with a
// high override.
func dedupParaphraseSeed() (title, content string) {
	return "chose postgres connection pooling",
		"pooling keeps p95 latency flat under load; sized the pool to twice the worker count"
}

func dedupParaphraseCandidate() (title, content string) {
	return "chose postgres conenction pooling",
		"pooling keeps p95 latnecy flat under load; sized the pool to twice the worker count"
}

func TestDedupThresholdFromEnvDefault(t *testing.T) {
	if err := os.Unsetenv(DedupThresholdEnv); err != nil {
		t.Fatal(err)
	}
	th, fromEnv := DedupThresholdFromEnv()
	if th != DedupThreshold {
		t.Fatalf("no env must keep the default bar: want %.2f, got %.4f", DedupThreshold, th)
	}
	if fromEnv {
		t.Fatal("no env must not report an override")
	}
}

func TestDedupThresholdFromEnvOverride(t *testing.T) {
	t.Setenv(DedupThresholdEnv, "0.99")
	th, fromEnv := DedupThresholdFromEnv()
	if th != 0.99 {
		t.Fatalf("valid env must win: want 0.99, got %.4f", th)
	}
	if !fromEnv {
		t.Fatal("valid env must report an override")
	}
}

func TestDedupThresholdFromEnvInvalidFallsBack(t *testing.T) {
	for _, raw := range []string{"abc", "2", "-1", "NaN", "1.5", "+Inf", "0.92.1", ""} {
		t.Run("env="+raw, func(t *testing.T) {
			if raw == "" {
				if err := os.Unsetenv(DedupThresholdEnv); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv(DedupThresholdEnv, raw)
			}
			th, fromEnv := DedupThresholdFromEnv()
			if th != DedupThreshold {
				t.Fatalf("invalid env %q must keep the default: want %.2f, got %.4f", raw, DedupThreshold, th)
			}
			if fromEnv {
				t.Fatalf("invalid env %q must not report an override", raw)
			}
		})
	}
}

func TestCheckDuplicateDefaultRefuses(t *testing.T) {
	if err := os.Unsetenv(DedupThresholdEnv); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	st, sc := dedupParaphraseSeed()
	seed := dedupSeed(t, root, &Observation{Type: TypeDecision, Title: st, Content: sc}, base)
	ct, cc := dedupParaphraseCandidate()
	var dup *DuplicateError
	if err := CheckDuplicate(root, &Observation{Type: TypeDecision, Title: ct, Content: cc}); err == nil {
		t.Fatal("default bar must refuse the typo pair")
	} else if !errors.As(err, &dup) {
		t.Fatalf("refusal must be a *DuplicateError, got %T (%v)", err, err)
	}
	if dup.ID != seed.ID {
		t.Fatalf("refusal must name the near-duplicate id: want %s, got %s", seed.ID, dup.ID)
	}
	msg := dup.Error()
	if !strings.Contains(msg, "threshold 0.92") {
		t.Fatalf("default refusal must cite the 0.92 bar, got %q", msg)
	}
	if strings.Contains(msg, DedupThresholdEnv) {
		t.Fatalf("default refusal must not cite the env, got %q", msg)
	}
}

func TestCheckDuplicateHighThresholdSaves(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	st, sc := dedupParaphraseSeed()
	dedupSeed(t, root, &Observation{Type: TypeDecision, Title: st, Content: sc}, base)
	ct, cc := dedupParaphraseCandidate()
	// Sanity: without env the same candidate refuses (sim lands
	// between 0.92 and 0.99).
	if err := os.Unsetenv(DedupThresholdEnv); err != nil {
		t.Fatal(err)
	}
	if err := CheckDuplicate(root, &Observation{Type: TypeDecision, Title: ct, Content: cc}); err == nil {
		t.Fatal("fixture sanity: typo pair must refuse at the default bar")
	}
	// With a high override the same candidate saves.
	t.Setenv(DedupThresholdEnv, "0.99")
	if err := CheckDuplicate(root, &Observation{Type: TypeDecision, Title: ct, Content: cc}); err != nil {
		t.Fatalf("high override must save what 0.92 refuses, got: %v", err)
	}
	if _, err := Save(root, &Observation{Type: TypeDecision, Title: ct, Content: cc}, base.Add(time.Hour)); err != nil {
		t.Fatalf("save under high override must land, got: %v", err)
	}
	if all, err := All(root); err != nil || len(all) != 2 {
		t.Fatalf("overridden save must append: %+v (%v)", all, err)
	}
}

func TestCheckDuplicateHighThresholdRefusalCitesEnv(t *testing.T) {
	t.Setenv(DedupThresholdEnv, "0.99")
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	dedupSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	// Identical text (sim 1.0) still trips even the 0.99 bar.
	var dup *DuplicateError
	err := CheckDuplicate(root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	})
	if err == nil {
		t.Fatal("identical save must refuse even at 0.99")
	}
	if !errors.As(err, &dup) {
		t.Fatalf("refusal must be a *DuplicateError, got %T (%v)", err, err)
	}
	msg := dup.Error()
	if !strings.Contains(msg, "threshold 0.99") {
		t.Fatalf("env refusal must cite the effective bar, got %q", msg)
	}
	if !strings.Contains(msg, DedupThresholdEnv) {
		t.Fatalf("env refusal must cite the env source, got %q", msg)
	}
}

func TestCheckDuplicateInvalidEnvKeepsDefaultRefusal(t *testing.T) {
	for _, raw := range []string{"abc", "2", "-1"} {
		t.Run("env="+raw, func(t *testing.T) {
			t.Setenv(DedupThresholdEnv, raw)
			root := t.TempDir()
			base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
			st, sc := dedupParaphraseSeed()
			dedupSeed(t, root, &Observation{Type: TypeDecision, Title: st, Content: sc}, base)
			ct, cc := dedupParaphraseCandidate()
			var dup *DuplicateError
			err := CheckDuplicate(root, &Observation{Type: TypeDecision, Title: ct, Content: cc})
			if err == nil {
				t.Fatalf("invalid env %q must keep refusing like 0.92", raw)
			}
			if !errors.As(err, &dup) {
				t.Fatalf("refusal must be a *DuplicateError, got %T (%v)", err, err)
			}
			msg := dup.Error()
			if !strings.Contains(msg, "threshold 0.92") {
				t.Fatalf("invalid env %q must cite the default bar, got %q", raw, msg)
			}
			if strings.Contains(msg, DedupThresholdEnv) {
				t.Fatalf("invalid env %q must not cite an override, got %q", raw, msg)
			}
		})
	}
}

func TestCheckDuplicateEnvKeepsTopicKeyExemption(t *testing.T) {
	t.Setenv(DedupThresholdEnv, "0.50")
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	dedupSeed(t, root, &Observation{
		Type:     TypeDecision,
		Title:    "chose postgres connection pooling",
		Content:  "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		TopicKey: "pooling",
	}, base)
	// A stricter bar still honors declared grouping.
	if err := CheckDuplicate(root, &Observation{
		Type:     TypeDecision,
		Title:    "picked postgres connection pooling",
		Content:  "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		TopicKey: "pooling",
	}); err != nil {
		t.Fatalf("matching topic key must bypass dedup at any bar, got: %v", err)
	}
}

func TestCheckDuplicateEnvKeepsSupersededExemption(t *testing.T) {
	t.Setenv(DedupThresholdEnv, "0.50")
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	old := dedupSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	live := dedupSeed(t, root, &Observation{
		Type:     TypeDecision,
		Title:    "chose bun runtime for the plugin host",
		Content:  "cold start under fifty milliseconds keeps the picker responsive",
		TopicKey: "runtime-v2",
	}, base.Add(time.Hour))
	if err := Relate(root, live.ID, RelationSupersedes, old.ID, base.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// A paraphrase of the hidden observation saves even at a strict bar.
	if err := CheckDuplicate(root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}); err != nil {
		t.Fatalf("superseded observations must stay excluded at any bar, got: %v", err)
	}
}

func TestImportIgnoresDedupThreshold(t *testing.T) {
	// Import never calls the decider by design: near-duplicates with
	// distinct ids import cleanly at any bar.
	for _, raw := range []string{"", "0.50", "0.99", "abc"} {
		t.Run("env="+raw, func(t *testing.T) {
			if raw == "" {
				if err := os.Unsetenv(DedupThresholdEnv); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv(DedupThresholdEnv, raw)
			}
			root := t.TempDir()
			bundle := root + "/bundle.jsonl"
			lines := "{\"id\":\"obs_dup_a\",\"type\":\"decision\",\"title\":\"chose postgres connection pooling\",\"content\":\"pooling keeps p95 latency flat under load; sized the pool to twice the worker count\",\"saved_at\":\"2026-10-20T09:00:00Z\"}\n" +
				"{\"id\":\"obs_dup_b\",\"type\":\"decision\",\"title\":\"picked postgres connection pooling\",\"content\":\"pooling keeps p95 latency flat under load; sized the pool to twice the worker count\",\"saved_at\":\"2026-10-20T10:00:00Z\"}\n"
			if err := os.WriteFile(bundle, []byte(lines), 0o644); err != nil {
				t.Fatal(err)
			}
			imported, skipped, err := Import(root, bundle)
			if err != nil {
				t.Fatal(err)
			}
			if imported != 2 || skipped != 0 {
				t.Fatalf("import must take both near-duplicates (no dedup): imported=%d skipped=%d", imported, skipped)
			}
			if all, err := All(root); err != nil || len(all) != 2 {
				t.Fatalf("imported log must hold both lines: %+v (%v)", all, err)
			}
		})
	}
}
