// Engine tests (F13.6/NOV-109): risk matrix, deny-by-default
// policies, autonomy profiles and the approval lifecycle.
//
// Note on RED: the engine and these tests were written in one
// worker turn (no shared harness yet to run RED against), so no
// failing-first run was observed; every case below asserts the
// exact observable (decision, state, error) so regressions fail
// loudly from here on.
package govern

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/verify"
)

func mustPolicy(t *testing.T, profile string) Policy {
	t.Helper()
	p, err := ResolvePolicy("test", profile)
	if err != nil {
		t.Fatalf("ResolvePolicy(%s): %v", profile, err)
	}
	return p
}

func TestClassifyRiskMatrix(t *testing.T) {
	cases := []struct {
		name    string
		op      Operation
		want    string
		approve bool
	}{
		{"trivial read", Operation{Kind: "read", Surface: "docs", Reversible: true}, verify.RiskLow, false},
		{"routine write", Operation{Kind: "write", Surface: "code", Reversible: true}, verify.RiskMedium, false},
		{"routine execute", Operation{Kind: "execute", Surface: "code", Reversible: true}, verify.RiskMedium, false},
		{"deploy needs human", Operation{Kind: "deploy", Surface: "deploy", Reversible: true}, verify.RiskHigh, true},
		{"core write escalates", Operation{Kind: "write", Surface: "core", Reversible: true}, verify.RiskHigh, true},
		{"policy kind sensitive", Operation{Kind: "policy", Surface: "code", Reversible: true}, verify.RiskHigh, true},
		{"evaluator kind sensitive", Operation{Kind: "evaluator", Surface: "code", Reversible: true}, verify.RiskHigh, true},
		{"security surface sensitive", Operation{Kind: "read", Surface: "security", Reversible: true}, verify.RiskHigh, true},
		{"irreversible kind critical", Operation{Kind: "irreversible", Surface: "data", Reversible: false}, verify.RiskCritical, true},
		{"irreversible deploy critical", Operation{Kind: "deploy", Surface: "deploy", Reversible: false}, verify.RiskCritical, true},
		{"irreversible docs high floor", Operation{Kind: "write", Surface: "docs", Reversible: false}, verify.RiskHigh, true},
	}
	for _, c := range cases {
		level, reasons, err := ClassifyRisk(c.op)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if level != c.want {
			t.Errorf("%s: risk = %s, want %s (%s)", c.name, level, c.want, strings.Join(reasons, "; "))
		}
		if got := RequiresApproval(c.op, level); got != c.approve {
			t.Errorf("%s: RequiresApproval = %v, want %v", c.name, got, c.approve)
		}
	}
}

func TestClassifyRiskUnknownDenies(t *testing.T) {
	if _, _, err := ClassifyRisk(Operation{Kind: "teleport", Surface: "docs", Reversible: true}); err == nil {
		t.Error("unknown kind must fail classification (deny-by-default)")
	}
	if _, _, err := ClassifyRisk(Operation{Kind: "read", Surface: "shadow", Reversible: true}); err == nil {
		t.Error("unknown surface must fail classification (deny-by-default)")
	}
	d := Authorize(Operation{Kind: "teleport", Surface: "docs", Reversible: true}, []string{"read"}, mustPolicy(t, "wide"))
	if d.Allow || d.NeedsApproval {
		t.Error("unclassifiable operation must deny, never allow or route to approval")
	}
}

func TestAuthorizeScopesAndProfiles(t *testing.T) {
	standard := mustPolicy(t, "standard")
	// Trivial read inside explicit scopes: autonomous.
	d := Authorize(Operation{Kind: "read", Surface: "docs", Reversible: true, Scopes: []string{"memory:read"}}, []string{"memory:read"}, standard)
	if !d.Allow || d.NeedsApproval {
		t.Errorf("trivial read in scope should allow, got %+v", d)
	}
	// Missing granted scope: deny.
	d = Authorize(Operation{Kind: "read", Surface: "docs", Reversible: true, Scopes: []string{"memory:read"}}, []string{}, standard)
	if d.Allow || d.NeedsApproval {
		t.Errorf("missing granted scope must deny, got %+v", d)
	}
	// Scope outside the profile autonomy: deny even when granted.
	d = Authorize(Operation{Kind: "write", Surface: "code", Reversible: true, Scopes: []string{"gate:execute"}}, []string{"gate:execute"}, standard)
	if d.Allow || d.NeedsApproval {
		t.Errorf("scope outside profile must deny, got %+v", d)
	}
	// Unknown scope names deny (closed world, like F11.8).
	d = Authorize(Operation{Kind: "read", Surface: "docs", Reversible: true, Scopes: []string{"shadow:read"}}, []string{"shadow:read"}, mustPolicy(t, "wide"))
	if d.Allow || d.NeedsApproval {
		t.Errorf("unknown scope must deny, got %+v", d)
	}
	// Unknown profile: deny.
	d = Authorize(Operation{Kind: "read", Surface: "docs", Reversible: true}, []string{"read"}, Policy{})
	if d.Allow || d.NeedsApproval {
		t.Errorf("unresolved policy must deny, got %+v", d)
	}
	if _, err := ParseProfile("yolo"); err == nil {
		t.Error("unknown profile must fail (deny-by-default extends to configuration)")
	}
	// Locked: every effect denies, even with scopes.
	locked := mustPolicy(t, "locked")
	d = Authorize(Operation{Kind: "write", Surface: "docs", Reversible: true}, []string{"memory:write"}, locked)
	if d.Allow {
		t.Errorf("locked profile must not allow writes, got %+v", d)
	}
	// Wide: medium autonomous in explicit scopes.
	wide := mustPolicy(t, "wide")
	d = Authorize(Operation{Kind: "execute", Surface: "code", Reversible: true, Scopes: []string{"gate:execute"}}, []string{"gate:execute", "memory:read"}, wide)
	if !d.Allow {
		t.Errorf("wide profile should allow medium in scope, got %+v", d)
	}
	// High never autonomous, whatever the profile.
	d = Authorize(Operation{Kind: "deploy", Surface: "deploy", Reversible: true}, []string{"memory:read"}, wide)
	if d.Allow || !d.NeedsApproval {
		t.Errorf("deploy must need approval under every profile, got %+v", d)
	}
}

func TestAuthorizeIntegratesVerifyGates(t *testing.T) {
	level, _, err := ClassifyRisk(Operation{Kind: "deploy", Surface: "deploy", Reversible: true})
	if err != nil {
		t.Fatal(err)
	}
	specs, reasons := VerificationGates(level, nil)
	if len(specs) == 0 || len(reasons) == 0 {
		t.Fatal("high risk must resolve verify gates with reasons")
	}
	found := map[string]bool{}
	for _, s := range specs {
		found[s.Kind] = true
	}
	for _, k := range []string{verify.GateBuild, verify.GateStatic, verify.GateUnit, verify.GateSecurity} {
		if !found[k] {
			t.Errorf("high risk gates miss %s (got %v)", k, specs)
		}
	}
}

func baseSpec() ApprovalSpec {
	return ApprovalSpec{
		Kind: "deploy", Surface: "deploy", Reversible: true,
		Summary: "ship gateway fix to staging", DiffHash: "abc123def456",
		PlanVersion: "plan-v3", Risks: []string{"staging latency spike"},
		Tests: []string{"go test ./..."}, Rollback: "redeploy previous tag",
		Cost: "10m staging", Scope: []string{"gateway/", "config/staging.yaml"},
		PolicyName: "default", Proposer: "agent-a", Reviewer: "agent-b", Executor: "agent-c",
	}
}

func TestApprovalLifecycleBindsAndChecks(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	req, err := RequestApproval(baseSpec(), mustPolicy(t, "standard"), now)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	if req.State != ApprovalRequested || req.PolicyVersion != PolicyVersion {
		t.Fatalf("request must open REQUESTED pinned to %s, got %+v", PolicyVersion, req)
	}
	appr, err := Approve(req, "humana", "abc123def456", "staging risk accepted, rollback ready", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if appr.State != ApprovalApproved || appr.BoundArtifact != "abc123def456" {
		t.Fatalf("approval must bind the artifact, got %+v", appr)
	}
	if err := Check(appr, "abc123def456", []string{"gateway/"}, PolicyVersion, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("Check valid: %v", err)
	}
	// Narrowed scope still validates.
	if err := Check(appr, "abc123def456", []string{"gateway/"}, PolicyVersion, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("Check narrowed scope: %v", err)
	}
	// Decided approvals never re-decide (replay-safe by construction).
	if _, err := Approve(appr, "humana2", "abc123def456", "again", now.Add(3*time.Hour)); err == nil {
		t.Error("second approve must fail: decided approvals never re-decide")
	}
}

func TestApprovalMaterialChangeInvalidates(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	req, _ := RequestApproval(baseSpec(), mustPolicy(t, "standard"), now)
	appr, _ := Approve(req, "humana", "abc123def456", "ok", now.Add(time.Hour))
	if err := Check(appr, "deadbeef0000", []string{"gateway/"}, PolicyVersion, now.Add(2*time.Hour)); err == nil {
		t.Error("altered artifact hash must invalidate the approval")
	}
	if err := Check(appr, "abc123def456", []string{"gateway/", "prod/"}, PolicyVersion, now.Add(2*time.Hour)); err == nil {
		t.Error("widened scope must invalidate the approval")
	}
	if err := Check(appr, "abc123def456", []string{"gateway/"}, "regulus.govern-policy/v2", now.Add(2*time.Hour)); err == nil {
		t.Error("policy change must invalidate prior approvals")
	}
	late := now.Add(DefaultApprovalTTL + time.Hour)
	if !Expired(appr, late) {
		t.Error("approval past TTL must read expired")
	}
	if err := Check(appr, "abc123def456", []string{"gateway/"}, PolicyVersion, late); err == nil {
		t.Error("expired approval must not validate")
	}
}

func TestApprovalSelfApprovalRefused(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	req, _ := RequestApproval(baseSpec(), mustPolicy(t, "standard"), now)
	for _, who := range []string{"agent-a", "agent-b", "agent-c"} {
		if _, err := Approve(req, who, "abc123def456", "trust me", now.Add(time.Hour)); err == nil {
			t.Errorf("approver %q holds another duty: self-approval must fail", who)
		}
	}
	for _, bot := range []string{"auto", "bot", "system", "ai"} {
		if _, err := Approve(req, bot, "abc123def456", "ok", now.Add(time.Hour)); err == nil {
			t.Errorf("automated approver %q must fail", bot)
		}
	}
	// Distinct roles at request time too.
	bad := baseSpec()
	bad.Reviewer = "agent-a"
	if _, err := RequestApproval(bad, mustPolicy(t, "standard"), now); err == nil {
		t.Error("collapsed proposer/reviewer must fail the request")
	}
}

func TestApprovalPauseResumeRejectRevoke(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	req, _ := RequestApproval(baseSpec(), mustPolicy(t, "standard"), now)
	paused, err := Pause(req, "agent-b", "waiting on load test", now)
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if _, err := Approve(paused, "humana", "abc123def456", "ok", now); err == nil {
		t.Error("paused requests approve only after resume")
	}
	resumed, err := Resume(paused, "agent-b", "load test green", now)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.State != ApprovalRequested {
		t.Fatalf("resume must reopen REQUESTED, got %s", resumed.State)
	}
	if _, err := Pause(resumed, "agent-c", "executor freezes", now); err == nil {
		t.Error("executor must not pause what it will run")
	}
	rejected, err := Reject(resumed, "humana", "staging frozen this week", now)
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if rejected.State != ApprovalRejected {
		t.Fatalf("reject must seal REJECTED, got %s", rejected.State)
	}
	if _, err := Revoke(rejected, "humana", "late", now); err == nil {
		t.Error("terminal approvals have nothing to revoke")
	}
	// Revocation stops a live approval.
	req2, _ := RequestApproval(baseSpec(), mustPolicy(t, "standard"), now)
	appr, _ := Approve(req2, "humana", "abc123def456", "ok", now)
	revoked, err := Revoke(appr, "humana", "incident: hold all ships", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := Check(revoked, "abc123def456", []string{"gateway/"}, PolicyVersion, now.Add(3*time.Hour)); err == nil {
		t.Error("revoked approval must stop pending actions")
	}
}
