package bench

// individual benchmark checks B1-B14 (NOV-67 split; behavior unchanged).

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/crap"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/sextant"
	"regulus-ai/internal/spec"
	"regulus-ai/internal/trace"
)

// checkCrapFormula pins the CRAP formula on three points (hidden: the
// worker prompt names no numbers).
func checkCrapFormula() (string, error) {
	cases := []struct {
		comp int
		cov  float64
		want float64
	}{
		{5, 1.0, 5},
		{1, 0.0, 2},
		{2, 0.5, 2.5},
	}
	for _, tc := range cases {
		if got := crap.CRAP(tc.comp, tc.cov); math.Abs(got-tc.want) > 1e-9 {
			return "", fmt.Errorf("CRAP(%d, %.1f) = %v, want %v", tc.comp, tc.cov, got, tc.want)
		}
	}
	return "formula holds on 3 points", nil
}

// splitFixture is the visible two-row table; the row count and routes
// below stay hidden from the worker prompt.
func splitFixture() string {
	return "## Tasks\n\n" +
		"| ID | Spec | Work | Route |\n" +
		"|----|------|------|-------|\n" +
		"| B-a | S1 | one unit | inline |\n" +
		"| B-b | S2 | other unit | @regulus-worker |\n"
}

// checkSplitBounded parses the fixture: exactly two units, no
// violations, second unit carrying its Spec context.
func checkSplitBounded() (string, error) {
	tasks, violations := sextant.ParseTasks(splitFixture())
	if len(violations) != 0 {
		return "", fmt.Errorf("unexpected violations: %s", strings.Join(violations, "; "))
	}
	if len(tasks) != 2 {
		return "", fmt.Errorf("parsed %d tasks, want 2", len(tasks))
	}
	if tasks[1].Route != "@regulus-worker" || tasks[1].Spec == "" {
		return "", fmt.Errorf("second unit lost its route/spec context")
	}
	return "2 bounded units, second carries spec", nil
}

// checkSpecGWT accepts a complete scenario and refuses a Then-less one,
// a one-word step (`Given a` states no criterion), and out-of-order
// phases — the P1 hardening, machine-checked.
func checkSpecGWT() (string, error) {
	good := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When the check runs\n- Then it passes\n"
	if n, errs := spec.Check(good); n != 1 || len(errs) != 0 {
		return "", fmt.Errorf("good scenario: got %d scenario(s), %d violation(s)", n, len(errs))
	}
	bad := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When the check runs\n"
	if _, errs := spec.Check(bad); len(errs) == 0 {
		return "", fmt.Errorf("bad scenario accepted: missing Then not refused")
	}
	thin := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When the check runs\n- Then it passes\n"
	if _, errs := spec.Check(thin); len(errs) == 0 {
		return "", fmt.Errorf("thin scenario accepted: one-word Given not refused")
	}
	disordered := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- Then it passes\n- When the check runs\n"
	if _, errs := spec.Check(disordered); len(errs) == 0 {
		return "", fmt.Errorf("disordered scenario accepted: Given→When→Then order not enforced")
	}
	return "good accepted, Then-less/single-word/disordered refused", nil
}

// checkReceiptRoundTrip writes one receipt to a temp dir and reads it
// back: id assigned, full-output SHA pinned, stored-text SHA present.
func checkReceiptRoundTrip() (string, error) {
	dir, err := os.MkdirTemp("", "bench-receipt-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	body := "bench veracity body"
	text, sha := receipts.Capture(body)
	path, err := receipts.Write(dir, receipts.Receipt{
		Task: "bench/B4", Gate: "veracity", Layer: 1, Command: "bench",
		ExitCode: 0, Verdict: receipts.VerdictPass,
		StartedAt: time.Now().UTC(), DurationMS: 3,
		Stdout: text, StdoutSHA: sha,
	})
	if err != nil {
		return "", err
	}
	back, err := receipts.ReadFile(path)
	if err != nil {
		return "", err
	}
	if back.ID == "" || back.Stdout != text {
		return "", fmt.Errorf("round-trip lost id or body")
	}
	if back.StdoutSHA != sha || back.StdoutStoredSHA != receipts.StoredSHA(text) {
		return "", fmt.Errorf("SHA pins missing after round-trip")
	}
	return "receipt byte-identical with SHA pins", nil
}

// boolPtr marks one gate optional: the quick profile must skip it.
func boolPtr(b bool) *bool { return &b }

// checkGateSelection pins gate selection over the real sixteen-gate
// mirror: full resolves all fifteen, the T5 lean preset resolves exactly
// the six quick names, and an unknown profile refuses. The six names
// are pinned literally, so selection drift — a renamed gate, or
// mutation leaking into --quick — fails.
func checkGateSelection() (string, error) {
	cfg := realGateConfig()
	full, err := gates.SelectGates(cfg, "")
	if err != nil || len(full) != 16 {
		return "", fmt.Errorf("full profile: got %d gates (%v)", len(full), err)
	}
	if err := assertLeanSelection(cfg); err != nil {
		return "", err
	}
	if _, err := gates.SelectGates(cfg, "nope"); err == nil {
		return "", fmt.Errorf("unknown profile accepted")
	}
	return "full=16, lean=6 (typecheck lint build unit-tests spec gofmt), unknown refused", nil
}

// checkEvidenceJail accepts a scoped task and refuses a traversal.
func checkEvidenceJail() (string, error) {
	if _, err := evidence.TaskDirSafe(".", "ok/T1"); err != nil {
		return "", fmt.Errorf("scoped task refused: %v", err)
	}
	if _, err := evidence.TaskDirSafe(".", "../evil"); err == nil {
		return "", fmt.Errorf("traversal accepted")
	}
	return "scoped accepted, traversal refused", nil
}

// checkHonestZero folds one unattributed-cost entry: calls counted,
// host tokens honestly unknown (never estimated to zero) with the
// measured-local proxy (chars/4) summed alongside.
func checkHonestZero() (string, error) {
	root, err := os.MkdirTemp("", "bench-cost-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	empty, err := trace.CostByTask(root)
	if err != nil || len(empty) != 0 {
		return "", fmt.Errorf("empty root: got %d buckets (%v)", len(empty), err)
	}
	recorded := trace.Entry{
		Session: "s", Message: "m", Call: "c", Tool: "t",
		Status: trace.StatusCompleted, Task: "bench/B7",
	}
	if _, err := trace.Record(root, recorded, time.Now().UTC()); err != nil {
		return "", err
	}
	costs, err := trace.CostByTask(root)
	if err != nil || len(costs) != 1 {
		return "", fmt.Errorf("one entry: got %d buckets (%v)", len(costs), err)
	}
	got := costs[0]
	if got.Calls != 1 || got.Task != "bench/B7" {
		return "", fmt.Errorf("bucket miscounted: %+v", got)
	}
	if got.TokensKnown || got.TotalInputTokens != 0 || got.TotalOutputTokens != 0 {
		return "", fmt.Errorf("tokens not honestly unknown: %+v", got)
	}
	wantLocal := trace.EstimateLocalTokens(recorded)
	if !got.LocalTokensKnown || got.TotalLocalTokens != wantLocal || got.TotalLocalTokens <= 0 {
		return "", fmt.Errorf("local proxy tokens not measured: %+v (want %d)", got, wantLocal)
	}
	return fmt.Sprintf("1 call counted, host tokens unknown + %d local proxy tokens (chars/4)", got.TotalLocalTokens), nil
}

// checkRetrievalGolden pins the real retrieval path on a golden corpus:
// three field notes in a fresh store, one golden query, exact hits.
// The golden query (AND over two terms) must surface exactly the
// rollback drill; the shared term must hit two notes; an unknown term
// must hit none.
func checkRetrievalGolden() (string, error) {
	root, err := os.MkdirTemp("", "bench-retrieval-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	now := time.Now().UTC()
	notes := []struct{ title, content string }{
		{"harbor rollback drill", "Drill the harbor rollback path until restoring service is routine. Rollback first, questions later."},
		{"harbor deploy checklist", "Checklist before every harbor deploy: freeze, snapshot, canary, watch."},
		{"ledger pruning routine", "Prune the evidence ledger on Fridays; retention never touches quarantine."},
	}
	for _, n := range notes {
		if _, err := almanac.Save(root, &almanac.Observation{
			Type: almanac.TypeDiscovery, Title: n.title, Content: n.content,
		}, now); err != nil {
			return "", err
		}
		now = now.Add(time.Second)
	}
	golden, err := almanac.Search(root, almanac.Query{Terms: []string{"harbor", "rollback"}})
	if err != nil {
		return "", err
	}
	if len(golden) != 1 || golden[0].Title != "harbor rollback drill" {
		return "", fmt.Errorf("golden query: got %d hit(s), want exactly the rollback drill", len(golden))
	}
	harbor, err := almanac.Search(root, almanac.Query{Terms: []string{"harbor"}})
	if err != nil {
		return "", err
	}
	if len(harbor) != 2 {
		return "", fmt.Errorf("shared term: got %d hit(s), want 2", len(harbor))
	}
	none, err := almanac.Search(root, almanac.Query{Terms: []string{"zebracorn"}})
	if err != nil {
		return "", err
	}
	if len(none) != 0 {
		return "", fmt.Errorf("unknown term: got %d hit(s), want 0", len(none))
	}
	return "golden query pins the rollback drill (1/2/0 hits)", nil
}
