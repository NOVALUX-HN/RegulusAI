// Money cost (F11.5, NOV-91): cost computes only against an explicit
// tariff naming model, host version and effective date. No tariff
// means no cost: unknown provenance never prices, and a missing or
// mismatched tariff fails closed instead of guessing. An explicit
// zero tariff is a valid measured free call (known-zero), which is
// different from unknown (unknown, never zero).
package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// maxRateBytes caps one rate card document: tariffs are small
// structs, never dumps.
const maxRateBytes = 64 * 1024

// maxRatePer1K caps any single per-1K rate: above this the card is
// corrupt or hostile, never a real tariff.
const maxRatePer1K = 1e6

// maxMoneyTotal caps any computed money total: above this the
// arithmetic is corrupt (absurd counts slipped past the token caps),
// never a real bill.
const maxMoneyTotal = 1e9

// rateDatePattern pins effective_date to an explicit calendar day
// (YYYY-MM-DD): cost attribution needs a date, never a vague
// "current pricing".
var rateDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// RateCard is one explicit tariff: per-1K-token prices for a named
// model at a named host version, effective on a named date, in a
// named currency. Every field is explicit so a money figure always
// cites the exact tariff behind it.
type RateCard struct {
	Model           string  `json:"model"`
	Host            string  `json:"host,omitempty"`
	HostVersion     string  `json:"host_version"`
	EffectiveDate   string  `json:"effective_date"`
	Currency        string  `json:"currency"`
	InputPer1K      float64 `json:"input_per_1k"`
	OutputPer1K     float64 `json:"output_per_1k"`
	ReasoningPer1K  float64 `json:"reasoning_per_1k"`
	CacheReadPer1K  float64 `json:"cache_read_per_1k"`
	CacheWritePer1K float64 `json:"cache_write_per_1k"`
}

// ParseRateCard parses one strict rate card: exact fields only,
// bounded size, explicit model/host-version/date/currency,
// non-negative finite rates under the absurdity cap, single-line
// short strings. Hostile or vague cards fail closed with a naming
// error.
func ParseRateCard(data []byte) (RateCard, error) {
	if len(data) > maxRateBytes {
		return RateCard{}, fmt.Errorf("rate card too large: %d bytes (max %d)", len(data), maxRateBytes)
	}
	var r RateCard
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return RateCard{}, fmt.Errorf("malformed rate card: %v", err)
	}
	if dec.More() {
		return RateCard{}, fmt.Errorf("malformed rate card: trailing data")
	}
	if strings.TrimSpace(r.Model) == "" {
		return RateCard{}, fmt.Errorf("rate card must name an explicit model")
	}
	if strings.TrimSpace(r.HostVersion) == "" {
		return RateCard{}, fmt.Errorf("rate card must name an explicit host_version")
	}
	if !rateDatePattern.MatchString(r.EffectiveDate) {
		return RateCard{}, fmt.Errorf("rate card must name an explicit effective_date (YYYY-MM-DD), got %q", r.EffectiveDate)
	}
	if strings.TrimSpace(r.Currency) == "" {
		return RateCard{}, fmt.Errorf("rate card must name an explicit currency")
	}
	for field, v := range map[string]float64{
		"input_per_1k": r.InputPer1K, "output_per_1k": r.OutputPer1K,
		"reasoning_per_1k":  r.ReasoningPer1K,
		"cache_read_per_1k": r.CacheReadPer1K, "cache_write_per_1k": r.CacheWritePer1K,
	} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return RateCard{}, fmt.Errorf("%s must be a finite rate, got %v", field, v)
		}
		if v < 0 {
			return RateCard{}, fmt.Errorf("%s must not be negative, got %v", field, v)
		}
		if v > maxRatePer1K {
			return RateCard{}, fmt.Errorf("%s absurd: %v (max %v)", field, v, maxRatePer1K)
		}
	}
	for field, v := range map[string]string{
		"model": r.Model, "host": r.Host, "host_version": r.HostVersion, "currency": r.Currency,
	} {
		if strings.ContainsAny(v, "\n\r") {
			return RateCard{}, fmt.Errorf("%s must be a single line", field)
		}
		if len(v) > maxUsageField {
			return RateCard{}, fmt.Errorf("%s too long: %d chars (max %d)", field, len(v), maxUsageField)
		}
	}
	return r, nil
}

// ParseRateCards parses a JSON array of rate cards with the same
// strict gate per card. A tariff file is an array; an empty array
// parses to no tariffs (money stays unknown, never zero).
func ParseRateCards(data []byte) ([]RateCard, error) {
	if len(data) > maxRateBytes {
		return nil, fmt.Errorf("rate cards too large: %d bytes (max %d)", len(data), maxRateBytes)
	}
	var raw []json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("malformed rate cards: %v", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("malformed rate cards: trailing data")
	}
	out := make([]RateCard, 0, len(raw))
	for i, m := range raw {
		r, err := ParseRateCard(m)
		if err != nil {
			return nil, fmt.Errorf("rate card %d: %v", i, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// MoneyCost is one priced usage: the total plus the exact tariff
// cited (model, version, date, currency). Known is false when the
// cost is unknown — no explicit tariff, unknown provenance — and
// then Total is 0 and Reason names why. Unknown never reads as a
// priced zero: Known distinguishes an explicit free tariff
// (known-zero) from an unpriced unknown.
type MoneyCost struct {
	Currency  string  `json:"currency,omitempty"`
	Total     float64 `json:"total"`
	Known     bool    `json:"known"`
	RateModel string  `json:"rate_model,omitempty"`
	RateDate  string  `json:"rate_date,omitempty"`
	Reason    string  `json:"reason,omitempty"`
}

// CostOfUsage prices one usage report against one explicit tariff.
// The tariff's model and host version must match the usage exactly
// (both explicit); unknown provenance never prices. A rate/model
// mismatch or a missing version fails closed with a naming error —
// cost never guesses across tariffs.
func CostOfUsage(u HostUsage, r RateCard) (MoneyCost, error) {
	if u.Source == SourceUnknown || u.Source == "" {
		return MoneyCost{Known: false, Reason: "host exposes none (no measurement to price)"}, nil
	}
	if strings.TrimSpace(u.Model) == "" {
		return MoneyCost{}, fmt.Errorf("cost needs an explicit usage model (got none for tariff %q)", r.Model)
	}
	if u.Model != r.Model {
		return MoneyCost{}, fmt.Errorf("cost model mismatch: usage %q vs tariff %q", u.Model, r.Model)
	}
	if strings.TrimSpace(u.HostVersion) == "" {
		return MoneyCost{}, fmt.Errorf("cost needs an explicit usage host_version (tariff %q)", r.HostVersion)
	}
	if u.HostVersion != r.HostVersion {
		return MoneyCost{}, fmt.Errorf("cost version mismatch: usage %q vs tariff %q", u.HostVersion, r.HostVersion)
	}
	billableOut := u.OutputTokens - u.ReasoningTokens
	if billableOut < 0 {
		billableOut = 0
	}
	total := float64(u.InputTokens)/1000*r.InputPer1K +
		float64(billableOut)/1000*r.OutputPer1K +
		float64(u.ReasoningTokens)/1000*r.ReasoningPer1K +
		float64(u.CacheRead)/1000*r.CacheReadPer1K +
		float64(u.CacheWrite)/1000*r.CacheWritePer1K
	if math.IsNaN(total) || math.IsInf(total, 0) || total < 0 || total > maxMoneyTotal {
		return MoneyCost{}, fmt.Errorf("cost absurd: %v (counts %d+%d+%d)", total, u.InputTokens, u.OutputTokens, u.CacheRead)
	}
	return MoneyCost{Currency: r.Currency, Total: total, Known: true, RateModel: r.Model + "@" + r.HostVersion, RateDate: r.EffectiveDate}, nil
}

// TaskMoney is the folded money cost of one task/model/version
// bucket: summed token totals plus the priced total when exactly one
// explicit tariff matches, or unknown with the reason when it does
// not. Buckets never mix tariffs: one line cites one rate card.
type TaskMoney struct {
	Task            string   `json:"task"`
	Model           string   `json:"model,omitempty"`
	Host            string   `json:"host,omitempty"`
	HostVersion     string   `json:"host_version,omitempty"`
	Source          string   `json:"source,omitempty"`
	Currency        string   `json:"currency,omitempty"`
	RateDate        string   `json:"rate_date,omitempty"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	ReasoningTokens int64    `json:"reasoning_tokens"`
	CacheRead       int64    `json:"cache_read_tokens"`
	CacheWrite      int64    `json:"cache_write_tokens"`
	Total           float64  `json:"total"`
	Known           bool     `json:"known"`
	Reason          string   `json:"reason,omitempty"`
	Sessions        []string `json:"sessions"`
}

// CostMoneyByTask folds every session trace under root into
// per-task/model/version money lines priced against the explicit
// tariffs. See CostMoneyByTaskFiltered (no filters).
func CostMoneyByTask(root string, rates []RateCard) ([]TaskMoney, error) {
	return CostMoneyByTaskFiltered(root, rates, "", "", "")
}

// CostMoneyByTaskFiltered folds like CostMoneyByTask but only over
// the entries matching the non-empty task/agent/run filters, so the
// money section scopes exactly like the token buckets above it.
// Entries without host counts land in an unattributed-model
// unknown bucket (never zero); counted entries with no matching
// tariff stay unknown with the reason; exactly one matching tariff
// prices the bucket. A root with no traces yields no lines and no
// error; a malformed line or a hostile total fails like the token
// folds do.
func CostMoneyByTaskFiltered(root string, rates []RateCard, task, agent, run string) ([]TaskMoney, error) {
	sessions, err := Sessions(root)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*TaskMoney{}
	for _, s := range sessions {
		entries, err := List(root, s)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if task != "" && taskKey(e) != task {
				continue
			}
			if agent != "" && agentKey(e) != agent {
				continue
			}
			if run != "" && e.ExecutionID != run {
				continue
			}
			t := e.Task
			if t == "" {
				t = UnattributedTask
			}
			k := t + "\x00" + e.Model + "\x00" + e.HostVersion
			m, ok := byKey[k]
			if !ok {
				m = &TaskMoney{Task: t, Model: e.Model, Host: e.Host, HostVersion: e.HostVersion}
				byKey[k] = m
			}
			var aerr error
			if m.InputTokens, aerr = addCounts(m.InputTokens, e.InputTokens); aerr != nil {
				return nil, fmt.Errorf("money fold %q: %v", k, aerr)
			}
			if m.OutputTokens, aerr = addCounts(m.OutputTokens, e.OutputTokens); aerr != nil {
				return nil, fmt.Errorf("money fold %q: %v", k, aerr)
			}
			if m.ReasoningTokens, aerr = addCounts(m.ReasoningTokens, e.ReasoningTokens); aerr != nil {
				return nil, fmt.Errorf("money fold %q: %v", k, aerr)
			}
			if m.CacheRead, aerr = addCounts(m.CacheRead, e.CacheRead); aerr != nil {
				return nil, fmt.Errorf("money fold %q: %v", k, aerr)
			}
			if m.CacheWrite, aerr = addCounts(m.CacheWrite, e.CacheWrite); aerr != nil {
				return nil, fmt.Errorf("money fold %q: %v", k, aerr)
			}
			if e.UsageSource != "" && m.Source == "" {
				m.Source = e.UsageSource
			}
			found := false
			for _, have := range m.Sessions {
				if have == s {
					found = true
					break
				}
			}
			if !found {
				m.Sessions = append(m.Sessions, s)
			}
		}
	}
	out := make([]TaskMoney, 0, len(byKey))
	for _, m := range byKey {
		price(m, rates)
		out = append(out, *m)
	}
	sortTaskMoney(out)
	return out, nil
}

// price resolves one money bucket against the explicit tariffs:
// unknown when there is nothing to price or no tariff matches,
// priced when exactly one tariff names the bucket's model@version.
func price(m *TaskMoney, rates []RateCard) {
	if m.InputTokens == 0 && m.OutputTokens == 0 && m.ReasoningTokens == 0 && m.CacheRead == 0 && m.CacheWrite == 0 {
		m.Known = false
		m.Reason = "host exposes none (no measured tokens in bucket)"
		return
	}
	if m.Model == "" {
		m.Known = false
		m.Reason = "no model recorded (cost needs an explicit tariff)"
		return
	}
	var match *RateCard
	for i := range rates {
		if rates[i].Model == m.Model && rates[i].HostVersion == m.HostVersion {
			match = &rates[i]
			break
		}
	}
	if match == nil {
		m.Known = false
		m.Reason = fmt.Sprintf("no explicit rate for %s@%s", m.Model, m.HostVersion)
		return
	}
	c, err := CostOfUsage(HostUsage{
		InputTokens: m.InputTokens, OutputTokens: m.OutputTokens,
		ReasoningTokens: m.ReasoningTokens, CacheRead: m.CacheRead, CacheWrite: m.CacheWrite,
		Model: m.Model, Host: m.Host, HostVersion: m.HostVersion, Source: measuredOrUnknown(m.Source),
	}, *match)
	if err != nil {
		m.Known = false
		m.Reason = fmt.Sprintf("%v", err)
		return
	}
	*m = TaskMoney{
		Task: m.Task, Model: m.Model, Host: m.Host, HostVersion: m.HostVersion,
		Source: m.Source, Currency: c.Currency, RateDate: c.RateDate,
		InputTokens: m.InputTokens, OutputTokens: m.OutputTokens,
		ReasoningTokens: m.ReasoningTokens, CacheRead: m.CacheRead, CacheWrite: m.CacheWrite,
		Total: c.Total, Known: c.Known, Reason: c.Reason, Sessions: m.Sessions,
	}
}

// measuredOrUnknown defaults an empty bucket source to unknown so
// CostOfUsage never prices unattributed provenance.
func measuredOrUnknown(source string) string {
	if source == "" {
		return SourceUnknown
	}
	return source
}

// sortTaskMoney orders money lines by task, then model, then
// version, so reports diff cleanly.
func sortTaskMoney(out []TaskMoney) {
	sort.Slice(out, func(i, j int) bool {
		ki := out[i].Task + "\x00" + out[i].Model + "\x00" + out[i].HostVersion
		kj := out[j].Task + "\x00" + out[j].Model + "\x00" + out[j].HostVersion
		return ki < kj
	})
}
