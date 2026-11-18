// Router + flag-validation tests: every cmdx command family answers
// --help/empty/unknown the same way, and every leaf refuses missing
// required flags with exit 2 before touching any store. These pin the
// CLI contract (S2 portable: no shell, no sleeps, no env reads).
package cmdx

import (
	"strings"
	"testing"
)

func TestRoutersHelpEmptyUnknown(t *testing.T) {
	routers := []struct {
		name string
		fn   func(IO, []string) int
		help int
		want string
	}{
		{"mission", IO.MissionCmd, 0, "regulus mission"},
		{"govern", IO.GovernCmd, 0, "regulus govern"},
		{"execute", IO.ExecuteCmd, 0, "regulus execute"},
		{"verify", IO.VerifyCmd, 0, "regulus verify"},
		{"context", IO.ContextCmd, 0, "regulus context"},
		{"failure", IO.FailureCmd, 0, "regulus failure"},
		{"recovery", IO.RecoveryCmd, 0, "regulus recovery"},
		{"impact", IO.ImpactCmd, 0, "regulus impact"},
		{"learn", IO.LearnCmd, 0, "regulus learn"},
		{"quality", IO.QualityCmd, 0, "regulus quality"},
		{"probe", IO.ProbeCmd, 0, "regulus probe"},
		{"orchestrate", IO.OrchestrateCmd, 0, "regulus orchestrate"},
		{"bench-lab", IO.BenchLabCmd, 0, "regulus bench lab"},
		// EvolveValidationCmd has no --help branch: anything unknown
		// (including --help) falls to the default refusal.
		{"evolve-validation", IO.EvolveValidationCmd, 2, "regulus evolve validation"},
	}
	for _, r := range routers {
		t.Run(r.name, func(t *testing.T) {
			dir := t.TempDir()
			io, out, _ := missionIO(dir)
			if code := r.fn(io, []string{"--help"}); code != r.help {
				t.Fatalf("%s --help exit = %d, want %d", r.name, code, r.help)
			}
			if r.help == 0 && !strings.Contains(out.String(), r.want) {
				t.Fatalf("%s --help must quote %q, got %q", r.name, r.want, out.String())
			}
			if code := r.fn(io, nil); code != 2 {
				t.Fatalf("%s with no args exit = %d, want 2", r.name, code)
			}
			if code := r.fn(io, []string{"frobnicate"}); code != 2 {
				t.Fatalf("%s unknown subcommand exit = %d, want 2", r.name, code)
			}
		})
	}
}

func TestRouterDispatches(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	planRepo(t, dir)
	if code := io.MissionCmd([]string{"create", "--id", "rd1", "--objective", "route dispatch"}); code != 0 {
		t.Fatalf("mission dispatch create exit = %d", code)
	}
	out.Reset()
	if code := io.MissionCmd([]string{"list"}); code != 0 {
		t.Fatalf("mission dispatch list exit = %d", code)
	}
	if code := io.GovernCmd([]string{"matrix"}); code != 0 {
		t.Fatalf("govern dispatch matrix exit = %d", code)
	}
	if code := io.VerifyCmd([]string{"matrix"}); code != 0 {
		t.Fatalf("verify dispatch matrix exit = %d", code)
	}
	if code := io.ContextCmd([]string{"inventory", "--no-memory"}); code != 0 {
		t.Fatalf("context dispatch inventory exit = %d", code)
	}
	if code := io.FailureCmd([]string{"classify", "--error", "timeout"}); code != 0 {
		t.Fatalf("failure dispatch classify exit = %d", code)
	}
	out.Reset()
	if code := io.LearnCmd([]string{"search", "nothing"}); code != 0 {
		t.Fatalf("learn dispatch search exit = %d", code)
	}
	if code := io.OrchestrateCmd([]string{"agents"}); code != 0 {
		t.Fatalf("orchestrate dispatch agents exit = %d", code)
	}
	if code := io.ImpactCmd([]string{"inventory"}); code != 0 {
		t.Fatalf("impact dispatch inventory exit = %d", code)
	}
	if code := io.BenchLabCmd([]string{"agents"}); code != 0 {
		t.Fatalf("bench lab dispatch agents exit = %d", code)
	}
	if code := io.EvolveValidationCmd([]string{"cycle-declare"}); code != 2 {
		t.Fatalf("evolve validation dispatch without --spec exit = %d, want 2", code)
	}
	if code := io.EvolveValidationCycleCmd([]string{}); code != 2 {
		t.Fatalf("evolve cycle router empty exit = %d, want 2", code)
	}
	if code := io.EvolveValidationCycleCmd([]string{"bogus"}); code != 2 {
		t.Fatalf("evolve cycle router unknown exit = %d, want 2", code)
	}
}

// TestLeafValidationRefusesGuaranteed documents that every leaf
// command refuses a call that cannot name its required inputs with
// exit 2, before any store IO. Each entry is a regression pin: a
// handler that started acting on missing flags would exit 0/1.
func TestLeafValidationRefuses(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	two := []struct {
		name string
		fn   func(IO, []string) int
		args []string
	}{
		{"mission show", IO.MissionShowCmd, nil},
		{"mission list extra", IO.MissionListCmd, []string{"x"}},
		{"mission status", IO.MissionStatusCmd, nil},
		{"mission advance", IO.MissionAdvanceCmd, nil},
		{"mission pause", IO.MissionPauseCmd, nil},
		{"mission resume", IO.MissionResumeCmd, nil},
		{"mission cancel", IO.MissionCancelCmd, nil},
		{"mission link", IO.MissionLinkCmd, nil},
		{"mission reopen", IO.MissionReopenCmd, nil},
		{"mission plan", IO.MissionPlanCmd, nil},
		{"mission replan", IO.MissionReplanCmd, nil},
		{"mission explain", IO.MissionExplainCmd, nil},
		{"mission report", IO.MissionReportCmd, nil},
		{"mission verify-report", IO.MissionVerifyReportCmd, nil},
		{"govern matrix extra", IO.GovernMatrixCmd, []string{"x"}},
		{"govern authorize", IO.GovernAuthorizeCmd, nil},
		{"govern request", IO.GovernRequestCmd, nil},
		{"govern approve", IO.GovernApproveCmd, nil},
		{"govern check", IO.GovernCheckCmd, nil},
		{"govern pause", IO.GovernPauseCmd, nil},
		{"govern resume", IO.GovernResumeCmd, nil},
		{"govern reject", IO.GovernRejectCmd, nil},
		{"govern revoke", IO.GovernRevokeCmd, nil},
		{"govern show", IO.GovernShowCmd, nil},
		{"govern list extra", IO.GovernListCmd, []string{"x"}},
		{"govern audit extra", IO.GovernAuditCmd, []string{"x"}},
		{"govern policy", IO.GovernPolicyCmd, nil},
		{"execute run", IO.ExecuteRunCmd, nil},
		{"execute status", IO.ExecuteStatusCmd, nil},
		{"execute approve", IO.ExecuteApproveCmd, nil},
		{"execute cancel", IO.ExecuteCancelCmd, nil},
		{"execute bench", IO.ExecuteBenchCmd, nil},
		{"verify plan", IO.VerifyPlanCmd, nil},
		{"verify run", IO.VerifyRunCmd, nil},
		{"verify report", IO.VerifyReportCmd, nil},
		{"verify matrix extra", IO.VerifyMatrixCmd, []string{"x"}},
		{"verify rerun", IO.VerifyRerunCmd, nil},
		{"context inventory extra", IO.ContextInventoryCmd, []string{"x"}},
		{"context select", IO.ContextSelectCmd, nil},
		{"failure classify extra", IO.FailureClassifyCmd, []string{"x"}},
		{"failure decide", IO.FailureDecideCmd, nil},
		{"failure status", IO.FailureStatusCmd, nil},
		{"failure explain", IO.FailureExplainCmd, nil},
		{"recovery checkpoint", IO.RecoveryCheckpointCmd, nil},
		{"recovery resume", IO.RecoveryResumeCmd, nil},
		{"recovery retry", IO.RecoveryRetryCmd, nil},
		{"recovery status", IO.RecoveryStatusCmd, nil},
		{"recovery rollback", IO.RecoveryRollbackCmd, nil},
		{"recovery cancel", IO.RecoveryCancelCmd, nil},
		{"impact inventory extra", IO.ImpactInventoryCmd, []string{"x"}},
		{"impact graph extra", IO.ImpactGraphCmd, []string{"x"}},
		{"impact affect", IO.ImpactAffectCmd, nil},
		{"impact suite", IO.ImpactSuiteCmd, nil},
		{"impact cycles extra", IO.ImpactCyclesCmd, []string{"x"}},
		{"impact breaking", IO.ImpactBreakingCmd, nil},
		{"impact benchmark extra", IO.ImpactBenchmarkCmd, []string{"x"}},
		{"learn save", IO.LearnSaveCmd, nil},
		{"learn verify", IO.LearnVerifyCmd, nil},
		{"learn invalidate", IO.LearnInvalidateCmd, nil},
		{"learn resolve", IO.LearnResolveCmd, nil},
		{"learn metrics extra", IO.LearnMetricsCmd, []string{"x"}},
		{"quality baseline", IO.QualityBaselineCmd, nil},
		{"quality check", IO.QualityCheckCmd, nil},
		{"quality alerts", IO.QualityAlertsCmd, nil},
		{"orchestrate agents extra", IO.OrchestrateAgentsCmd, []string{"x"}},
		{"orchestrate decide", IO.OrchestrateDecideCmd, nil},
		{"orchestrate evaluate", IO.OrchestrateEvaluateCmd, nil},
		{"orchestrate orphans extra", IO.OrchestrateOrphansCmd, []string{"x"}},
		{"probe contrast", IO.ProbeContrastCmd, nil},
		{"probe compose", IO.ProbeComposeCmd, nil},
		{"trace emit", IO.TraceEmitCmd, nil},
		{"trace timeline", IO.TraceTimelineCmd, nil},
		{"trace prune-exec extra", IO.TracePruneExecCmd, []string{"x"}},
		{"bench tiers extra", IO.BenchTiersCmd, []string{"x"}},
		{"bench tier-report", IO.BenchTierReportCmd, nil},
		{"bench diag", IO.BenchDiagCmd, nil},
		{"bench hostcap extra", IO.BenchHostCapabilitiesCmd, []string{"x"}},
		{"bench lab report", IO.BenchLabReportCmd, nil},
		{"evolve cycle-declare", IO.EvolveCycleDeclareCmd, nil},
		{"evolve cycle-propose", IO.EvolveCycleProposeCmd, nil},
		{"evolve cycle-link", IO.EvolveCycleLinkCmd, nil},
		{"evolve cycle-compare", IO.EvolveCycleCompareCmd, nil},
		{"evolve cycle-review", IO.EvolveCycleReviewCmd, nil},
		{"evolve cycle-approve", IO.EvolveCycleApproveCmd, nil},
		{"evolve cycle-canary", IO.EvolveCycleCanaryCmd, nil},
		{"evolve cycle-monitor", IO.EvolveCycleMonitorCmd, nil},
		{"evolve cycle-promote", IO.EvolveCyclePromoteCmd, nil},
		{"evolve cycle-rollback", IO.EvolveCycleRollbackCmd, nil},
		{"evolve cycle-report", IO.EvolveCycleReportCmd, nil},
		{"evolve validation declare", IO.EvolveValidationDeclareCmd, nil},
		{"evolve validation run", func(io IO, a []string) int { return io.EvolveValidationRunCmd(a, false) }, nil},
		{"evolve validation control", IO.EvolveValidationControlCmd, nil},
		{"evolve validation discard", IO.EvolveValidationDiscardCmd, nil},
		{"evolve validation link", IO.EvolveValidationLinkCmd, nil},
		{"evolve validation compare", IO.EvolveValidationCompareCmd, nil},
		{"evolve validation finalize", IO.EvolveValidationFinalizeCmd, nil},
		{"evolve validation report", IO.EvolveValidationReportCmd, nil},
	}
	for _, c := range two {
		t.Run(c.name, func(t *testing.T) {
			if code := c.fn(io, c.args); code != 2 {
				t.Fatalf("%s exit = %d, want 2 (refuse before acting)", c.name, code)
			}
		})
	}
}

// TestLeafStoreFailuresRefuseDeterministic pins the exit-1 paths
// that need no valid fixture: unknown stores refuse without
// panicking and without littering.
func TestLeafStoreFailuresRefuse(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	// Missing --tasks is a usage error (flag contract), not a store
	// refusal: parseOrchestrateParallelFlags exits 2 before any IO.
	if code := io.OrchestrateParallelCmd(nil); code != 2 {
		t.Fatalf("parallel without tasks file exit = %d, want 2", code)
	}
	if code := io.BenchTierPlanCmd(nil); code != 1 {
		t.Fatalf("tier-plan without splits exit = %d, want 1", code)
	}
	if code := io.BenchLabRunCmd("run", nil); code != 1 {
		t.Fatalf("lab run without splits exit = %d, want 1", code)
	}
	if code := io.BenchLabCompareCmd(nil); code != 1 {
		t.Fatalf("lab compare without splits exit = %d, want 1", code)
	}
	if code := io.OrchestrateEvaluateCmd([]string{"--dynamic", "/nonexistent/d.json", "--fixed", "/nonexistent/f.json"}); code != 1 {
		t.Fatalf("evaluate with missing arms exit = %d, want 1", code)
	}
	if code := io.BenchTierReportCmd([]string{"--dir", "/nonexistent/tier"}); code != 1 {
		t.Fatalf("tier-report on a missing dir exit = %d, want 1", code)
	}
	if code := io.BenchLabReportCmd([]string{"--evidence-dir", "/nonexistent/lab"}); code != 1 {
		t.Fatalf("lab report on a missing dir exit = %d, want 1", code)
	}
	if code := io.QualityBaselineCmd([]string{"--window", "/nonexistent/w.json", "--out", dir}); code != 1 {
		t.Fatalf("quality baseline on a missing window exit = %d, want 1", code)
	}
	if code := io.QualityCheckCmd([]string{"--baseline", "/nonexistent/b", "--window", "/nonexistent/w"}); code != 1 {
		t.Fatalf("quality check on missing evidence exit = %d, want 1", code)
	}
	if code := io.QualityAlertsCmd([]string{"--dir", "/nonexistent/q"}); code != 1 {
		t.Fatalf("quality alerts on a missing dir exit = %d, want 1", code)
	}
	if code := io.EvolveValidationDeclareCmd([]string{"--plan", "/nonexistent/p.json"}); code != 1 {
		t.Fatalf("validation declare on a missing spec exit = %d, want 1", code)
	}
	if code := io.EvolveCycleDeclareCmd([]string{"--spec", "/nonexistent/c.json"}); code != 1 {
		t.Fatalf("cycle declare on a missing spec exit = %d, want 1", code)
	}
	if errb.Len() == 0 {
		t.Fatal("refusals must explain themselves on stderr")
	}
}
