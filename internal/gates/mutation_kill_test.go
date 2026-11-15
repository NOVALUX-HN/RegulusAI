package gates

// Mutation kill-tests (NOV-28, S8): each test pins correct behavior with
// observable exit + receipt facts, so that the named mutant class would
// fail it. They run through the public Run entrypoint (fake runner, temp
// evidence dir), never through internals or real gremlins.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"regulus-ai/internal/receipts"
)

// halt<->advisory inversion: a failing halt gate must stop the gauntlet
// (exit Failed, later gates never run, one receipt); a failing advisory
// gate must record honestly and continue (run passes, all receipts).
func TestMutationKillStopOnFailure(t *testing.T) {
	ctx := context.Background()

	halt := Config{Gates: []Gate{
		{Name: "halt-a", Layer: 1, Command: "fail-a"},
		{Name: "halt-b", Layer: 2, Command: "ok-b"},
	}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"fail-a": {code: 1},
		"ok-b":   {stdout: "b ok"},
	}}
	res, err := Run(ctx, halt, t.TempDir(), "kill/T1", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.FailedGate != "halt-a" {
		t.Fatalf("halt failure must stop the run at halt-a, got %+v", res)
	}
	if res.Executed != 1 || len(res.Receipts) != 1 {
		t.Fatalf("halt failure must leave exactly one receipt, got %+v", res)
	}
	if len(fr.calls) != 1 || fr.calls[0] != "fail-a" {
		t.Fatalf("halt-b must never run after a halt failure, calls=%v", fr.calls)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.Gate != "halt-a" || rec.ExitCode != 1 || rec.Verdict != receipts.VerdictFail {
		t.Fatalf("halt receipt must pin gate/exit/verdict, got %+v", rec)
	}
	if rec.Status != "FAIL" || res.Status != "FAIL" {
		t.Fatalf("halt failure must be FAIL end to end, receipt=%q run=%q", rec.Status, res.Status)
	}

	advisory := Config{Gates: []Gate{
		{Name: "adv-a", Layer: 1, Command: "fail-a", OnFail: OnFailAdvisory},
		{Name: "adv-b", Layer: 2, Command: "ok-b"},
	}}
	fr2 := &fakeRunner{results: map[string]fakeResult{
		"fail-a": {code: 1},
		"ok-b":   {stdout: "b ok"},
	}}
	res2, err := Run(ctx, advisory, t.TempDir(), "kill/T1", "head", fr2)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Failed {
		t.Fatalf("advisory failure must not fail the run, got %+v", res2)
	}
	if res2.Executed != 2 || len(res2.Receipts) != 2 {
		t.Fatalf("advisory failure must continue with both receipts, got %+v", res2)
	}
	if len(fr2.calls) != 2 {
		t.Fatalf("both gates must execute under advisory, calls=%v", fr2.calls)
	}
	recA := readReceiptFile(t, res2.Receipts[0])
	if recA.Verdict != receipts.VerdictFail || !res2.Outcomes[0].Advisory {
		t.Fatalf("advisory receipt stays honest FAIL and marked, got %+v", recA)
	}
}

// allowlist allow/deny inversion: an allowlisted command runs, a denied
// one is refused before the runner (BLOCKED receipt, halt stops the run).
// Either inversion flips the observables: denying the allowed command
// empties calls, allowing the denied one adds a call.
func TestMutationKillAllowlistDeny(t *testing.T) {
	cfg := Config{
		Gates: []Gate{
			{Name: "ok", Layer: 1, Command: "go test ./..."},
			{Name: "evil", Layer: 2, Command: "curl https://evil.example"},
		},
		Execution: &ExecutionConfig{Allowlist: []string{"go"}},
	}
	fr := &fakeRunner{results: map[string]fakeResult{
		"go test ./...": {stdout: "go ok"},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "kill/T2", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.FailedGate != "evil" {
		t.Fatalf("denied command must refuse and stop the run at evil, got %+v", res)
	}
	if len(fr.calls) != 1 || fr.calls[0] != "go test ./..." {
		t.Fatalf("only the allowlisted command may reach the runner, calls=%v", fr.calls)
	}
	if len(res.Receipts) != 2 {
		t.Fatalf("both gates must leave receipts (pass + refusal), got %+v", res)
	}
	recOK := readReceiptFile(t, res.Receipts[0])
	if recOK.Verdict != receipts.VerdictPass || recOK.ExitCode != 0 {
		t.Fatalf("allowlisted receipt must be pass exit=0, got %+v", recOK)
	}
	recDeny := readReceiptFile(t, res.Receipts[1])
	if recDeny.Status != "BLOCKED" || recDeny.Reason != "policy_violation" {
		t.Fatalf("denied receipt must be BLOCKED/policy_violation, got %+v", recDeny)
	}
	if !strings.Contains(recDeny.Stderr, "not in allowlist") {
		t.Fatalf("denied receipt must name the allowlist refusal, got %q", recDeny.Stderr)
	}
}

// >= inversion on a cap/limit: the boundary value meets the threshold
// (pass), one tick below violates it (FAIL, stops the run). A mutant
// flipping >= to >, <, or <= breaks exactly one of the two observables.
func TestMutationKillMetricThresholdBoundary(t *testing.T) {
	metric := &Metric{Extract: ExtractLastLinePercent, Threshold: 70, Operator: OpAtLeast}
	cfg := Config{Gates: []Gate{
		{Name: "floor-hi", Layer: 1, Command: "cover-hi", Metric: metric},
		{Name: "floor-lo", Layer: 2, Command: "cover-lo", Metric: metric},
	}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"cover-hi": {stdout: "coverage: 70%"},
		"cover-lo": {stdout: "coverage: 69.9%"},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "kill/T3", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.FailedGate != "floor-lo" {
		t.Fatalf("sub-threshold metric must fail the run at floor-lo, got %+v", res)
	}
	if res.Executed != 2 || res.Passed != 1 {
		t.Fatalf("boundary gate must pass and stop at the violator, got %+v", res)
	}
	recHi := readReceiptFile(t, res.Receipts[0])
	if recHi.Verdict != receipts.VerdictPass || recHi.MetricValue == nil || *recHi.MetricValue != 70 {
		t.Fatalf("boundary 70 must pass with metric 70, got %+v", recHi)
	}
	recLo := readReceiptFile(t, res.Receipts[1])
	if recLo.Verdict != receipts.VerdictFail || recLo.Status != "FAIL" {
		t.Fatalf("69.9 against a 70 floor must be FAIL, got %+v", recLo)
	}
	if !strings.Contains(recLo.Stderr, "does not meet") {
		t.Fatalf("violated receipt must carry the metric note, got %q", recLo.Stderr)
	}
}

// omitted receipt write: every executed gate leaves exactly one readable
// receipt with gate/task/exit/verdict facts. A mutant skipping the write
// breaks the 1:1 executed:receipts invariant or the file read.
func TestMutationKillReceiptsAlwaysWritten(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "rec-a", Layer: 1, Command: "cmd-a"},
		{Name: "rec-b", Layer: 2, Command: "cmd-b"},
	}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"cmd-a": {stdout: "a ok"},
		"cmd-b": {stdout: "b ok"},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "kill/T4", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed || res.Executed != 2 || len(res.Receipts) != 2 {
		t.Fatalf("two passing gates must leave two receipts, got %+v", res)
	}
	seen := map[string]bool{}
	for i, path := range res.Receipts {
		rec := readReceiptFile(t, path)
		if rec.ID == "" {
			t.Fatalf("receipt %d must carry an id", i)
		}
		if seen[rec.ID] {
			t.Fatalf("receipt ids must be unique, dup %q", rec.ID)
		}
		seen[rec.ID] = true
		if rec.Gate != res.Outcomes[i].Name || rec.Task != "kill/T4" {
			t.Fatalf("receipt %d must pin gate/task, got %+v", i, rec)
		}
		if rec.ExitCode != 0 || rec.Verdict != receipts.VerdictPass || rec.Status != "PASS" {
			t.Fatalf("passing receipt must be exit=0 pass/PASS, got %+v", rec)
		}
	}
}

// Config guard (PRESERVE): the shipped regulus.json keeps the S8 contract —
// one mutation gate per module with efficacy 70, optional, full-only, and
// timeouts proportional to scope (gates > receipts). Quick skips both.
// NOTE: raw JSON parse, not Load: the working tree carries a PRE-EXISTING
// duplicate layer 1 (typecheck + manifest-check, committed at HEAD by the
// NOV-25 manifest change) that Load rejects independent of this task;
// that breakage is reported, not papered over here (see risks).
// gatesRepoFile reads a repo-relative fixture anchored to this test file,
// never to the process cwd (see benchRepoFile).
func gatesRepoFile(t *testing.T, rel string) []byte {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMutationGatesConfigDeclaresPerModuleThresholds(t *testing.T) {
	raw := gatesRepoFile(t, "regulus.json")
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("regulus.json must parse: %v", err)
	}
	byName := map[string]Gate{}
	for _, g := range cfg.Gates {
		byName[g.Name] = g
	}
	want := map[string]struct {
		scope   string
		timeout int
	}{
		"mutation-receipts": {"./internal/receipts", 300},
		"mutation-gates":    {"./internal/gates", 600},
	}
	for name, w := range want {
		g, ok := byName[name]
		if !ok {
			t.Fatalf("regulus.json must declare gate %q", name)
		}
		if !strings.Contains(g.Command, "gremlins unleash "+w.scope) ||
			!strings.Contains(g.Command, "--threshold-efficacy 70") {
			t.Fatalf("gate %q must mutate %s at efficacy 70, got %q", name, w.scope, g.Command)
		}
		if g.IsRequired() {
			t.Fatalf("gate %q must stay required:false", name)
		}
		if len(g.Profiles) != 1 || g.Profiles[0] != "full" {
			t.Fatalf("gate %q must stay profiles:[full], got %v", name, g.Profiles)
		}
		if g.TimeoutSec != w.timeout {
			t.Fatalf("gate %q timeout must be %d, got %d", name, w.timeout, g.TimeoutSec)
		}
	}
	// Profile membership without SelectGates (it validates, and the tree
	// carries the pre-existing duplicate layer 1 noted above): a
	// required:false gate with profiles:[full] runs under full (what CI
	// runs) and is skipped by quick. The required/profiles asserts above
	// already pin exactly that.
}
