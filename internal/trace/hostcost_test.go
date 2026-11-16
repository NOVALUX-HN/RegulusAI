// F11.5 (NOV-91): money cost only with an explicit tariff/version/date.
// RED: strict rate cards, model/version mismatch, unknown-never-cost,
// explicit zero tariffs, absurd figures, per-task money folds.
package trace

import (
	"strings"
	"testing"
)

const f115RateCard = `{"model":"beam-free","host":"opencode","host_version":"2.2.5","effective_date":"2026-11-01","currency":"USD","input_per_1k":0.1,"output_per_1k":0.4,"reasoning_per_1k":0.4,"cache_read_per_1k":0.01,"cache_write_per_1k":0.02}`

func TestParseRateCardStrict(t *testing.T) {
	r, err := ParseRateCard([]byte(f115RateCard))
	if err != nil {
		t.Fatalf("valid rate card rejected: %v", err)
	}
	if r.Model != "beam-free" || r.Currency != "USD" || r.EffectiveDate != "2026-11-01" {
		t.Fatalf("rate identity lost: %+v", r)
	}
	for name, doc := range map[string]string{
		"garbage":         `not json`,
		"no model":        `{"host_version":"2.2.5","effective_date":"2026-11-01","currency":"USD"}`,
		"no version":      `{"model":"m","effective_date":"2026-11-01","currency":"USD"}`,
		"no date":         `{"model":"m","host_version":"v","currency":"USD"}`,
		"bad date":        `{"model":"m","host_version":"v","effective_date":"yesterday","currency":"USD"}`,
		"no currency":     `{"model":"m","host_version":"v","effective_date":"2026-11-01"}`,
		"negative rate":   `{"model":"m","host_version":"v","effective_date":"2026-11-01","currency":"USD","input_per_1k":-1}`,
		"absurd rate":     `{"model":"m","host_version":"v","effective_date":"2026-11-01","currency":"USD","input_per_1k":2000000}`,
		"unknown field":   `{"model":"m","host_version":"v","effective_date":"2026-11-01","currency":"USD","bogus":1}`,
		"multiline model": "{\"model\":\"a\nb\",\"host_version\":\"v\",\"effective_date\":\"2026-11-01\",\"currency\":\"USD\"}",
	} {
		if _, err := ParseRateCard([]byte(doc)); err == nil {
			t.Fatalf("%s must fail closed, passed", name)
		}
	}
}

func TestCostOfUsageNeedsExplicitTariff(t *testing.T) {
	r, err := ParseRateCard([]byte(f115RateCard))
	if err != nil {
		t.Fatal(err)
	}
	u := HostUsage{InputTokens: 1000, OutputTokens: 500, ReasoningTokens: 100, CacheRead: 2000,
		Model: "beam-free", Host: "opencode", HostVersion: "2.2.5", Source: SourceMeasuredHost}
	m, err := CostOfUsage(u, r)
	if err != nil {
		t.Fatalf("CostOfUsage: %v", err)
	}
	if !m.Known {
		t.Fatalf("measured usage with explicit tariff must price: %+v", m)
	}
	// 1.0*0.1 + 0.4*0.4 (output excl. reasoning) + 0.1*0.4 + 2.0*0.01 = 0.32
	if m.Total < 0.319999 || m.Total > 0.320001 {
		t.Fatalf("cost = %v, want 0.32", m.Total)
	}
	if m.Currency != "USD" || m.RateDate != "2026-11-01" {
		t.Fatalf("money must cite currency and date: %+v", m)
	}
	other := u
	other.Model = "ghost-model"
	if _, err := CostOfUsage(other, r); err == nil {
		t.Fatal("model mismatch must fail closed, passed")
	} else if !strings.Contains(err.Error(), "model") {
		t.Fatalf("mismatch error must name model, got %v", err)
	}
	noVer := u
	noVer.HostVersion = ""
	if _, err := CostOfUsage(noVer, r); err == nil {
		t.Fatal("missing usage version must fail closed, passed")
	}
}

func TestCostOfUsageUnknownNeverZero(t *testing.T) {
	r, err := ParseRateCard([]byte(f115RateCard))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []HostUsage{
		{Source: SourceUnknown, Reason: "host exposes none"},
		{Source: "", Host: "devin"},
	} {
		m, err := CostOfUsage(u, r)
		if err != nil {
			t.Fatalf("unknown usage must not error, only stay unknown: %v", err)
		}
		if m.Known || m.Total != 0 {
			t.Fatalf("unknown cost must be unknown-not-zero: %+v", m)
		}
		if strings.TrimSpace(m.Reason) == "" {
			t.Fatalf("unknown money must say why: %+v", m)
		}
	}
}

func TestCostOfUsageExplicitZeroTariff(t *testing.T) {
	free := `{"model":"free-beam","host":"opencode","host_version":"2.2.5","effective_date":"2026-11-01","currency":"USD"}`
	r, err := ParseRateCard([]byte(free))
	if err != nil {
		t.Fatalf("zero tariff is explicit and valid: %v", err)
	}
	u := HostUsage{InputTokens: 1000, OutputTokens: 500, Model: "free-beam", HostVersion: "2.2.5", Source: SourceMeasuredHost}
	m, err := CostOfUsage(u, r)
	if err != nil {
		t.Fatalf("CostOfUsage: %v", err)
	}
	if !m.Known || m.Total != 0 {
		t.Fatalf("explicit free tariff prices known-zero: %+v", m)
	}
}

func TestCostMoneyByTaskFoldsPerModel(t *testing.T) {
	dir := t.TempDir()
	recordEntry(t, dir, "s1", "T1", Entry{InputTokens: 1000, OutputTokens: 100, Model: "beam-free", Host: "opencode", HostVersion: "2.2.5", UsageSource: SourceMeasuredHost})
	recordEntry(t, dir, "s1", "T1", Entry{DurationMS: 5})
	recordEntry(t, dir, "s2", "T2", Entry{InputTokens: 500, Model: "ghost", Host: "opencode", HostVersion: "9.9", UsageSource: SourceMeasuredHost})
	r, err := ParseRateCard([]byte(f115RateCard))
	if err != nil {
		t.Fatal(err)
	}
	money, err := CostMoneyByTask(dir, []RateCard{r})
	if err != nil {
		t.Fatalf("CostMoneyByTask: %v", err)
	}
	byTask := map[string]TaskMoney{}
	for _, m := range money {
		byTask[m.Task+"/"+m.Model] = m
	}
	known, ok := byTask["T1/beam-free"]
	if !ok || !known.Known || known.Total <= 0 {
		t.Fatalf("T1/beam-free must price: %+v", money)
	}
	if known.Source != SourceMeasuredHost {
		t.Fatalf("money must carry provenance, got %+v", known)
	}
	ghost, ok := byTask["T2/ghost"]
	if !ok || ghost.Known {
		t.Fatalf("T2/ghost has no explicit rate and must stay unknown: %+v", money)
	}
	if !strings.Contains(ghost.Reason, "no explicit rate") {
		t.Fatalf("money unknown must say why, got %+v", ghost)
	}
	plain, ok := byTask["T1/"]
	if !ok || plain.Known {
		t.Fatalf("tokenless lines must stay unknown, never zero: %+v", money)
	}
}
