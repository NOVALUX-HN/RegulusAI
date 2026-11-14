package gates

import (
	"context"
	"strings"
	"testing"

	"regulus-ai/internal/receipts"
)

// S6 deny-by-default: a `gate run` with no allowlist configured refuses
// every gate with a Refused receipt and executes nothing.
func TestDenyByDefaultEmptyAllowlistRefuses(t *testing.T) {
	cfg := Config{
		Gates:     []Gate{{Name: "a", Layer: 1, Command: "echo hi"}},
		Execution: &ExecutionConfig{EnforceDenyByDefault: true},
	}
	r := &fakeRunner{}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("empty allowlist must refuse the gauntlet")
	}
	if len(r.calls) != 0 {
		t.Fatalf("refused gate must execute NOTHING, calls=%v", r.calls)
	}
	if res.Status != "BLOCKED" || res.Reason != "policy_violation" {
		t.Fatalf("refused result = %q/%q, want BLOCKED/policy_violation", res.Status, res.Reason)
	}
	if len(res.Receipts) != 1 {
		t.Fatalf("refusal must still write one Refused receipt, got %v", res.Receipts)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.Status != "BLOCKED" || rec.Reason != "policy_violation" {
		t.Fatalf("refused receipt = %q/%q", rec.Status, rec.Reason)
	}
	if rec.Verdict != receipts.VerdictFail {
		t.Fatalf("legacy verdict must stay fail, got %q", rec.Verdict)
	}
	if rec.ExitCode != -1 {
		t.Fatalf("refused receipt must carry no exit code, got %d", rec.ExitCode)
	}
}

// S6: explicit --yes consent bypasses ONLY the empty-allowlist denial:
// the gate runs and the receipt records consent:true.
func TestDenyByDefaultExplicitYesProceedsWithConsent(t *testing.T) {
	cfg := Config{
		Gates:     []Gate{{Name: "a", Layer: 1, Command: "echo hi"}},
		Execution: &ExecutionConfig{EnforceDenyByDefault: true, ExplicitYes: true},
	}
	r := &fakeRunner{}
	ctx := ContextWithConsent(context.Background(), true)
	res, err := Run(ctx, cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed || res.Passed != 1 {
		t.Fatalf("--yes must let the allowlisted-by-consent gate pass: %+v", res)
	}
	if len(r.calls) != 1 || r.calls[0] != "echo hi" {
		t.Fatalf("consented gate must execute, calls=%v", r.calls)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, "consent:true") {
		t.Fatalf("consented receipt must record consent:true, stderr=%q", rec.Stderr)
	}
}

// S6: without the `gate run` enforcement flag the historical
// allowlist-only behavior holds (probe and programmatic callers): no
// Execution block — or an empty allowlist without enforcement — still
// runs.
func TestDenyByDefaultWithoutEnforcementKeepsHistory(t *testing.T) {
	for name, cfg := range map[string]Config{
		"nil-execution":   {Gates: []Gate{{Name: "a", Layer: 1, Command: "echo hi"}}},
		"empty-allowlist": {Gates: []Gate{{Name: "a", Layer: 1, Command: "echo hi"}}, Execution: &ExecutionConfig{}},
	} {
		t.Run(name, func(t *testing.T) {
			r := &fakeRunner{}
			res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
			if err != nil {
				t.Fatal(err)
			}
			if res.Failed || res.Passed != 1 {
				t.Fatalf("unenforced run must keep history and pass: %+v", res)
			}
			if len(r.calls) != 1 {
				t.Fatalf("unenforced run must execute, calls=%v", r.calls)
			}
			rec := readReceiptFile(t, res.Receipts[0])
			if strings.Contains(rec.Stderr, "consent:true") {
				t.Fatalf("unconsented receipt must not claim consent, stderr=%q", rec.Stderr)
			}
		})
	}
}

// S6: --yes never bypasses a NAMED allowlist mismatch: the gate still
// refuses with a Refused receipt and no consent note.
func TestDenyByDefaultExplicitYesKeepsNamedMismatch(t *testing.T) {
	cfg := Config{
		Gates:     []Gate{{Name: "a", Layer: 1, Command: "echo hi"}},
		Execution: &ExecutionConfig{EnforceDenyByDefault: true, ExplicitYes: true, Allowlist: []string{"go"}},
	}
	r := &fakeRunner{}
	ctx := ContextWithConsent(context.Background(), true)
	res, err := Run(ctx, cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("named allowlist mismatch must refuse even with --yes")
	}
	if len(r.calls) != 0 {
		t.Fatalf("refused gate must execute NOTHING, calls=%v", r.calls)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if res.Status != "BLOCKED" || rec.Reason != "policy_violation" {
		t.Fatalf("refused result/receipt = %q/%q", res.Status, rec.Reason)
	}
	if strings.Contains(rec.Stderr, "consent:true") {
		t.Fatalf("refused receipt must not claim consent, stderr=%q", rec.Stderr)
	}
	if !strings.Contains(rec.Stderr, "not in allowlist") {
		t.Fatalf("refusal must name the allowlist cause, stderr=%q", rec.Stderr)
	}
}

// S6 unit: refusePolicy with nil Execution keeps history (the CLI
// materializes the block before enforcing, so bare refusePolicy never
// denies on nil).
func TestRefusePolicyNilExecutionAllows(t *testing.T) {
	if refused, _, _ := refusePolicy(Gate{Name: "a", Layer: 1, Command: "echo hi"}, Config{}); refused {
		t.Fatal("nil Execution must not refuse (CLI arms enforcement per run)")
	}
}
