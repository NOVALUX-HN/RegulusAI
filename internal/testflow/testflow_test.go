// Runner self-tests (F15.4/NOV-125): the verdict core, zero-test
// refusal, false-PASS guards and report validation. All pure: no
// subprocess is spawned, so these run under any policy.
package testflow

import (
	"testing"
)

func TestClassifyPass(t *testing.T) {
	o, _ := classifyGoTest("PASS", "ok  \tregulus-ai/internal/pxexec\t1.2s")
	if o != OutcomePass {
		t.Fatalf("real pass must stay PASS, got %q", o)
	}
}

func TestClassifyZeroTestsRefused(t *testing.T) {
	for _, out := range []string{"?   \tregulus-ai/internal/foo\t[no test files]", "testing: warning: no tests to run\nFAIL"} {
		o, _ := classifyGoTest("PASS", out)
		if o != OutcomeNotRun {
			t.Fatalf("zero-test output must be NOT_RUN, got %q (%q)", o, out)
		}
	}
}

func TestClassifyBlockedNeverPass(t *testing.T) {
	o, d := classifyGoTest("BLOCKED", "anything")
	if o != OutcomeBlocked || d == "" {
		t.Fatalf("blocked must stay BLOCKED with a reason, got %+v", o)
	}
}

func TestClassifyFailKeepsSignal(t *testing.T) {
	o, d := classifyGoTest("FAIL", "--- FAIL: TestX (0.1s)")
	if o != OutcomeFail || d == "" {
		t.Fatalf("failure must stay FAIL with detail, got %+v", o)
	}
}

func TestVerdictNeedsAllGreen(t *testing.T) {
	rep := Report{Mode: ModeFocused, Discovered: []string{"a", "b"},
		Results: []PackageResult{{Package: "a", Outcome: OutcomePass}, {Package: "b", Outcome: OutcomeBlocked}},
		Checks:  []PackageResult{{Package: "(check) build", Outcome: OutcomePass}}}
	if Verdict(rep, map[string]bool{}) {
		t.Fatal("blocked package must fail the verdict")
	}
	rep.Results[1].Outcome = OutcomePass
	if !Verdict(rep, map[string]bool{}) {
		t.Fatal("all green must pass")
	}
}

func TestVerdictRefusesIncomplete(t *testing.T) {
	rep := Report{Mode: ModeFull, Discovered: []string{"a", "b"},
		Results: []PackageResult{{Package: "a", Outcome: OutcomePass}},
		Checks:  []PackageResult{{Package: "(check) build", Outcome: OutcomePass}}}
	if Verdict(rep, map[string]bool{}) {
		t.Fatal("unexecuted discovered package must fail full verdict")
	}
	if !Verdict(rep, map[string]bool{"b": true}) {
		t.Fatal("explicitly allowed skip must pass")
	}
}

func TestVerdictEmptySuiteFails(t *testing.T) {
	if Verdict(Report{}, map[string]bool{}) {
		t.Fatal("empty suite must not pass")
	}
}

func TestReportRoundTrip(t *testing.T) {
	rep := Report{Version: ReportVersion, Mode: ModeFocused, Discovered: []string{"a"},
		Results: []PackageResult{{Package: "a", Outcome: OutcomePass, Detail: "ok"}},
		Checks:  []PackageResult{{Package: "(check) build", Outcome: OutcomePass}},
		Skipped: []string{"b"}}
	if !Verdict(rep, map[string]bool{"b": true}) {
		t.Fatal("round-trip fixture must pass")
	}
}

func TestVerdictChecksGate(t *testing.T) {
	rep := Report{Mode: ModeSmoke, Discovered: SmokePackages,
		Results: []PackageResult{{Package: SmokePackages[0], Outcome: OutcomePass}},
		Checks:  []PackageResult{{Package: "(check) build", Outcome: OutcomeFail}}}
	if Verdict(rep, map[string]bool{}) {
		t.Fatal("failed check must fail the verdict")
	}
}
