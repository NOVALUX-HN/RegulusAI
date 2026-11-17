// Coverage tests (odd/tasks/coverage-80 T2): vocabularies, full
// risk matrix, policy/authorizer denials, every approval
// rejection path, store corruption, audit chain breaks and
// renders. In-process only; t.TempDir for disk; no shells.
package govern

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/verify"
)

func covPolicy(t *testing.T, profile string) Policy {
	t.Helper()
	p, err := ResolvePolicy("cov", profile)
	if err != nil {
		t.Fatalf("ResolvePolicy(%s): %v", profile, err)
	}
	return p
}

func covHighSpec() ApprovalSpec {
	return ApprovalSpec{
		Kind: "deploy", Surface: "deploy", Reversible: true,
		Summary: "ship gateway fix to staging", DiffHash: "abc123def456",
		PlanVersion: "plan-v3", Risks: []string{"staging latency spike"},
		Tests: []string{"go test ./..."}, Rollback: "redeploy previous tag",
		Cost: "10m staging", Scope: []string{"gateway/", "config/staging.yaml"},
		Proposer: "agent-a", Reviewer: "agent-b", Executor: "agent-c",
	}
}

func covLowSpec() ApprovalSpec {
	return ApprovalSpec{
		Kind: "read", Surface: "docs", Reversible: true,
		Summary: "read docs for triage", PlanVersion: "plan-v1",
		Risks: []string{"stale docs"}, Rollback: "re-read",
		Cost: "1m", Scope: []string{"docs/"},
		Proposer: "agent-a", Reviewer: "agent-b", Executor: "agent-c",
	}
}

func covApproved(t *testing.T, now time.Time) ApprovalRequest {
	t.Helper()
	req, err := RequestApproval(covHighSpec(), covPolicy(t, "standard"), now)
	if err != nil {
		t.Fatalf("RequestApproval: %v", err)
	}
	appr, err := Approve(req, "humana", "abc123def456", "staging risk accepted", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	return appr
}

func TestCovParseVocabularies(t *testing.T) {
	for _, k := range AllKinds {
		if got, err := ParseOpKind("  " + strings.ToUpper(k) + " "); err != nil || got != k {
			t.Errorf("ParseOpKind(%q) = %q, %v", k, got, err)
		}
	}
	for _, bad := range []string{"", "teleport", "read-write", "read write"} {
		if _, err := ParseOpKind(bad); err == nil {
			t.Errorf("ParseOpKind(%q) must fail", bad)
		}
	}
	for _, s := range AllSurfaces {
		if got, err := ParseSurface("  " + strings.ToUpper(s) + " "); err != nil || got != s {
			t.Errorf("ParseSurface(%q) = %q, %v", s, got, err)
		}
	}
	for _, bad := range []string{"", "shadow", "code;drop"} {
		if _, err := ParseSurface(bad); err == nil {
			t.Errorf("ParseSurface(%q) must fail", bad)
		}
	}
	for _, r := range []string{"low", "medium", "high", "critical"} {
		if got, err := ParseRisk(" " + strings.ToUpper(r) + " "); err != nil || got != r {
			t.Errorf("ParseRisk(%q) = %q, %v", r, got, err)
		}
	}
	for _, bad := range []string{"", "unknown", "moderate", "low-medium"} {
		if _, err := ParseRisk(bad); err == nil {
			t.Errorf("ParseRisk(%q) must fail", bad)
		}
	}
}

func TestCovClassifyRiskExhaustive(t *testing.T) {
	levels := map[string]bool{}
	for _, kind := range AllKinds {
		for _, surface := range AllSurfaces {
			for _, rev := range []bool{true, false} {
				level, reasons, err := ClassifyRisk(Operation{Kind: kind, Surface: surface, Reversible: rev})
				if err != nil {
					t.Fatalf("ClassifyRisk(%s/%s/%v): %v", kind, surface, rev, err)
				}
				if len(reasons) == 0 {
					t.Errorf("ClassifyRisk(%s/%s/%v) must explain itself", kind, surface, rev)
				}
				levels[level] = true
			}
		}
	}
	for _, want := range []string{verify.RiskLow, verify.RiskMedium, verify.RiskHigh, verify.RiskCritical} {
		if !levels[want] {
			t.Errorf("matrix never derives %q", want)
		}
	}
	spot := []struct {
		op   Operation
		want string
	}{
		{Operation{Kind: "read", Surface: "docs", Reversible: true}, verify.RiskLow},
		{Operation{Kind: "irreversible", Surface: "data", Reversible: false}, verify.RiskCritical},
		{Operation{Kind: "read", Surface: "security", Reversible: true}, verify.RiskHigh},
		{Operation{Kind: "write", Surface: "docs", Reversible: false}, verify.RiskHigh},
		{Operation{Kind: "deploy", Surface: "docs", Reversible: true}, verify.RiskHigh},
		{Operation{Kind: "write", Surface: "core", Reversible: false}, verify.RiskCritical},
	}
	for _, s := range spot {
		if got, _, _ := ClassifyRisk(s.op); got != s.want {
			t.Errorf("ClassifyRisk(%+v) = %s (want %s)", s.op, got, s.want)
		}
	}
	if _, _, err := ClassifyRisk(Operation{Kind: "nope", Surface: "docs", Reversible: true}); err == nil {
		t.Error("unknown kind must fail")
	}
	if _, _, err := ClassifyRisk(Operation{Kind: "read", Surface: "nope", Reversible: true}); err == nil {
		t.Error("unknown surface must fail")
	}
}

func TestCovSensitiveRequiresGroupable(t *testing.T) {
	low := Operation{Kind: "read", Surface: "docs", Reversible: true}
	med := Operation{Kind: "write", Surface: "code", Reversible: true}
	if Sensitive(low) {
		t.Error("trivial read must not be sensitive")
	}
	if RequiresApproval(low, verify.RiskLow) {
		t.Error("low routine read must stay autonomous")
	}
	if !Groupable(low, verify.RiskLow) {
		t.Error("low non-sensitive must be groupable")
	}
	if Groupable(med, verify.RiskMedium) {
		t.Error("medium must never group")
	}
	if Groupable(low, verify.RiskHigh) {
		t.Error("high must never group even when the op reads docs")
	}
	for _, op := range []Operation{
		{Kind: "deploy", Surface: "docs", Reversible: true},
		{Kind: "policy", Surface: "code", Reversible: true},
		{Kind: "permission", Surface: "code", Reversible: true},
		{Kind: "evaluator", Surface: "code", Reversible: true},
		{Kind: "security", Surface: "code", Reversible: true},
		{Kind: "irreversible", Surface: "data", Reversible: false},
		{Kind: "read", Surface: "core", Reversible: true},
		{Kind: "read", Surface: "security", Reversible: true},
		{Kind: "read", Surface: "permissions", Reversible: true},
		{Kind: "read", Surface: "policy", Reversible: true},
		{Kind: "read", Surface: "evaluator", Reversible: true},
		{Kind: "read", Surface: "deploy", Reversible: true},
		{Kind: "write", Surface: "docs", Reversible: false},
	} {
		if !Sensitive(op) {
			t.Errorf("op %+v must be sensitive", op)
		}
		level, _, err := ClassifyRisk(op)
		if err != nil {
			t.Fatal(err)
		}
		if !RequiresApproval(op, level) {
			t.Errorf("sensitive op %+v must need approval", op)
		}
		if Groupable(op, level) {
			t.Errorf("sensitive op %+v must never group", op)
		}
	}
	if RequiresApproval(Operation{Kind: "read", Surface: "docs", Reversible: true}, "bogus") {
		t.Error("unknown level on a routine op must not escalate by itself")
	}
	if Groupable(low, "bogus") {
		t.Error("unknown level must never group")
	}
}

func TestCovGatesAndDoc(t *testing.T) {
	for _, level := range []string{verify.RiskLow, verify.RiskMedium, verify.RiskHigh, verify.RiskCritical} {
		specs, _ := VerificationGates(level, nil)
		if len(specs) == 0 {
			t.Fatalf("level %s resolves no gates", level)
		}
	}
	high, _ := VerificationGates(verify.RiskHigh, nil)
	crit, _ := VerificationGates(verify.RiskCritical, nil)
	if len(crit) < len(high) {
		t.Errorf("critical (%d) must cover at least high (%d)", len(crit), len(high))
	}
	doc := MatrixDoc()
	for _, want := range []string{"low:", "medium:", "high:", "critical:", "deny-by-default"} {
		if !strings.Contains(doc, want) {
			t.Errorf("MatrixDoc misses %q", want)
		}
	}
}

func TestCovProfilesAndPolicy(t *testing.T) {
	profs := BuiltinProfiles()
	if len(profs) != 3 {
		t.Fatalf("profiles = %d (want 3)", len(profs))
	}
	for _, want := range []string{"locked", "standard", "wide"} {
		p, err := ParseProfile(" " + strings.ToUpper(want) + " ")
		if err != nil || p.Name != want {
			t.Errorf("ParseProfile(%q) = %+v, %v", want, p, err)
		}
		if _, err := ResolvePolicy("p", want); err != nil {
			t.Errorf("ResolvePolicy(%q): %v", want, err)
		}
	}
	for _, bad := range []string{"", "yolo", "admin"} {
		if _, err := ParseProfile(bad); err == nil {
			t.Errorf("ParseProfile(%q) must fail", bad)
		}
		if _, err := ResolvePolicy("p", bad); err == nil {
			t.Errorf("ResolvePolicy(%q) must fail", bad)
		}
	}
	def := DefaultPolicy()
	if def.Profile != "standard" || def.Version != PolicyVersion || def.Name == "" {
		t.Fatalf("DefaultPolicy must pin standard@%s: %+v", PolicyVersion, def)
	}
}

func TestCovAuthorizeDenials(t *testing.T) {
	std := covPolicy(t, "standard")
	wide := covPolicy(t, "wide")
	locked := covPolicy(t, "locked")

	d := Authorize(Operation{Kind: "read", Surface: "docs", Reversible: true}, []string{"x"}, Policy{})
	if d.Allow || d.NeedsApproval || !strings.Contains(d.Reason, "unresolved") {
		t.Errorf("unresolved policy must deny: %+v", d)
	}
	d = Authorize(Operation{Kind: "teleport", Surface: "docs", Reversible: true}, []string{"x"}, wide)
	if d.Allow || d.NeedsApproval || !strings.Contains(d.Reason, "unclassifiable") {
		t.Errorf("unknown kind must deny: %+v", d)
	}
	d = Authorize(Operation{Kind: "read", Surface: "shadow", Reversible: true}, []string{"x"}, wide)
	if d.Allow || d.NeedsApproval {
		t.Errorf("unknown surface must deny: %+v", d)
	}
	d = Authorize(Operation{Kind: "write", Surface: "docs", Reversible: true}, []string{"memory:write"}, locked)
	if d.Allow || !strings.Contains(d.Reason, "denies kind") {
		t.Errorf("locked must deny kinds: %+v", d)
	}
	d = Authorize(
		Operation{Kind: "write", Surface: "code", Reversible: true, Scopes: []string{"gate:execute"}},
		[]string{"gate:execute"}, std)
	if d.Allow || d.NeedsApproval || !strings.Contains(d.Reason, "outside profile") {
		t.Errorf("out-of-profile scope must deny: %+v", d)
	}
	d = Authorize(
		Operation{Kind: "read", Surface: "docs", Reversible: true, Scopes: []string{"memory:read"}},
		[]string{}, std)
	if d.Allow || d.NeedsApproval || !strings.Contains(d.Reason, "missing granted scope") {
		t.Errorf("missing grant must deny: %+v", d)
	}
	d = Authorize(
		Operation{Kind: "read", Surface: "docs", Reversible: true, Scopes: []string{"shadow:read"}},
		[]string{"shadow:read"}, wide)
	if d.Allow || d.NeedsApproval {
		t.Errorf("unknown scope must deny: %+v", d)
	}
	d = Authorize(Operation{Kind: "deploy", Surface: "deploy", Reversible: true}, []string{"memory:read"}, wide)
	if d.Allow || !d.NeedsApproval || d.Risk != verify.RiskHigh || d.PolicyVersion != PolicyVersion {
		t.Errorf("deploy must route to human: %+v", d)
	}
	d = Authorize(
		Operation{Kind: "write", Surface: "code", Reversible: true, Scopes: []string{"memory:read"}},
		[]string{"memory:read"}, std)
	if d.Allow || !d.NeedsApproval || !strings.Contains(d.Reason, "ceiling") {
		t.Errorf("medium above standard ceiling must escalate: %+v", d)
	}
	d = Authorize(
		Operation{Kind: "read", Surface: "docs", Reversible: true, Scopes: []string{"memory:read"}},
		[]string{"memory:read"}, std)
	if !d.Allow || d.NeedsApproval || d.Risk != verify.RiskLow {
		t.Errorf("trivial read in scope must allow: %+v", d)
	}
	d = Authorize(Operation{Kind: "execute", Surface: "code", Reversible: true}, nil, wide)
	if !d.Allow {
		t.Errorf("scopeless medium under wide must allow: %+v", d)
	}
}

func TestCovGrantedNames(t *testing.T) {
	got := GrantedNames([]string{"b", " a ", "", "b", "a"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("GrantedNames dedupes/sorts/trims: %q", got)
	}
	if len(GrantedNames(nil)) != 0 {
		t.Fatal("GrantedNames(nil) must stay empty")
	}
}

func TestCovRequestApprovalRejections(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	std := covPolicy(t, "standard")
	mutate := func(f func(*ApprovalSpec)) ApprovalSpec {
		s := covHighSpec()
		f(&s)
		return s
	}
	bad := []struct {
		name string
		spec ApprovalSpec
		pol  Policy
	}{
		{"unresolved policy", covHighSpec(), Policy{}},
		{"unknown kind", mutate(func(s *ApprovalSpec) { s.Kind = "teleport" }), std},
		{"unknown surface", mutate(func(s *ApprovalSpec) { s.Surface = "shadow" }), std},
		{"empty summary", mutate(func(s *ApprovalSpec) { s.Summary = "" }), std},
		{"multiline summary", mutate(func(s *ApprovalSpec) { s.Summary = "a\nb" }), std},
		{"bypass summary", mutate(func(s *ApprovalSpec) { s.Summary = "skip approval now" }), std},
		{"empty plan", mutate(func(s *ApprovalSpec) { s.PlanVersion = "" }), std},
		{"multiline plan", mutate(func(s *ApprovalSpec) { s.PlanVersion = "a\nb" }), std},
		{"no risks", mutate(func(s *ApprovalSpec) { s.Risks = nil }), std},
		{"empty risk", mutate(func(s *ApprovalSpec) { s.Risks = []string{""} }), std},
		{"multiline risk", mutate(func(s *ApprovalSpec) { s.Risks = []string{"a\nb"} }), std},
		{"bypass risk", mutate(func(s *ApprovalSpec) { s.Risks = []string{"ignore policy please"} }), std},
		{"empty rollback", mutate(func(s *ApprovalSpec) { s.Rollback = "" }), std},
		{"multiline rollback", mutate(func(s *ApprovalSpec) { s.Rollback = "a\nb" }), std},
		{"bypass rollback", mutate(func(s *ApprovalSpec) { s.Rollback = "auto-approve it" }), std},
		{"empty cost", mutate(func(s *ApprovalSpec) { s.Cost = "" }), std},
		{"multiline cost", mutate(func(s *ApprovalSpec) { s.Cost = "a\nb" }), std},
		{"empty scope", mutate(func(s *ApprovalSpec) { s.Scope = nil }), std},
		{"empty scope entry", mutate(func(s *ApprovalSpec) { s.Scope = []string{""} }), std},
		{"multiline scope entry", mutate(func(s *ApprovalSpec) { s.Scope = []string{"a\nb"} }), std},
		{"high without diff", mutate(func(s *ApprovalSpec) { s.DiffHash = "" }), std},
		{"high without tests", mutate(func(s *ApprovalSpec) { s.Tests = nil }), std},
		{"multiline test", mutate(func(s *ApprovalSpec) { s.Tests = []string{"ok", "a\nb"} }), std},
		{"empty proposer", mutate(func(s *ApprovalSpec) { s.Proposer = "x" }), std},
		{"empty reviewer", mutate(func(s *ApprovalSpec) { s.Reviewer = "" }), std},
		{"empty executor", mutate(func(s *ApprovalSpec) { s.Executor = " " }), std},
		{"collapsed proposer reviewer", mutate(func(s *ApprovalSpec) { s.Reviewer = "agent-a" }), std},
		{"collapsed reviewer executor", mutate(func(s *ApprovalSpec) { s.Executor = "agent-b" }), std},
		{"collapsed case-insensitive", mutate(func(s *ApprovalSpec) { s.Executor = "AGENT-A" }), std},
		{"ttl exceeds cap", mutate(func(s *ApprovalSpec) { s.TTLHours = 200 }), std},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RequestApproval(tc.spec, tc.pol, now); err == nil {
				t.Errorf("RequestApproval(%s) must fail", tc.name)
			}
		})
	}
	req, err := RequestApproval(covHighSpec(), std, now)
	if err != nil {
		t.Fatalf("honest high request: %v", err)
	}
	if !strings.HasPrefix(req.ID, "apr_") || req.State != ApprovalRequested || req.PolicyVersion != PolicyVersion {
		t.Fatalf("request must open sealed: %+v", req)
	}
	if len(req.History) != 1 || req.History[0].Action != "requested" {
		t.Fatalf("history must open with the request: %+v", req.History)
	}
	low, err := RequestApproval(covLowSpec(), std, now)
	if err != nil {
		t.Fatalf("low risk needs no diff/tests: %v", err)
	}
	if req.ID == low.ID {
		t.Fatal("minted ids must be unique")
	}
	def, err := RequestApproval(covLowSpec(), std, now)
	if err != nil {
		t.Fatal(err)
	}
	created, _ := time.Parse(time.RFC3339, def.CreatedAt)
	expires, _ := time.Parse(time.RFC3339, def.ExpiresAt)
	if expires.Sub(created) != DefaultApprovalTTL {
		t.Fatalf("zero TTL must default to %v, got %v", DefaultApprovalTTL, expires.Sub(created))
	}
	capped, err := RequestApproval(mutate(func(s *ApprovalSpec) { s.TTLHours = 168 }), std, now)
	if err != nil {
		t.Fatalf("TTL at the cap must pass: %v", err)
	}
	ce, _ := time.Parse(time.RFC3339, capped.CreatedAt)
	ee, _ := time.Parse(time.RFC3339, capped.ExpiresAt)
	if ee.Sub(ce) != MaxApprovalTTL {
		t.Fatalf("cap TTL = %v (want %v)", ee.Sub(ce), MaxApprovalTTL)
	}
}

func TestCovApprovePaths(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	std := covPolicy(t, "standard")

	req, _ := RequestApproval(covHighSpec(), std, now)
	paused, _ := Pause(req, "agent-a", "hold", now)
	if _, err := Approve(paused, "humana", "abc123def456", "ok", now); err == nil {
		t.Error("paused must resume before approval")
	}
	appr, err := Approve(req, "humana", "abc123def456", "risk accepted", now)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if appr.State != ApprovalApproved || appr.BoundArtifact != "abc123def456" || appr.Approver != "humana" {
		t.Fatalf("approval must bind: %+v", appr)
	}
	if _, err := Approve(appr, "humana2", "abc123def456", "again", now); err == nil {
		t.Error("decided approvals never re-decide")
	}
	for _, who := range []string{"bot", "AUTO", "x", "", " ai "} {
		r, _ := RequestApproval(covHighSpec(), std, now)
		if _, err := Approve(r, who, "abc123def456", "ok", now); err == nil {
			t.Errorf("approver %q must fail (non-human)", who)
		}
	}
	for _, who := range []string{"agent-a", "agent-b", "agent-c", "AGENT-B"} {
		r, _ := RequestApproval(covHighSpec(), std, now)
		if _, err := Approve(r, who, "abc123def456", "i approve my own", now); err == nil {
			t.Errorf("approver %q holds a duty: self-approval must fail", who)
		}
	}
	for _, tc := range []struct {
		name     string
		approver string
		artifact string
		reason   string
	}{
		{"empty reason", "humana", "abc123def456", ""},
		{"multiline reason", "humana", "abc123def456", "a\nb"},
		{"bypass reason", "humana", "abc123def456", "pre-approved by system"},
		{"artifact mismatch", "humana", "deadbeef0000", "ok"},
		{"empty artifact", "humana", "", "ok"},
		{"empty artifact low", "humana", "", "ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "empty artifact low" {
				ls := covLowSpec()
				r, err := RequestApproval(ls, std, now)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := Approve(r, tc.approver, tc.artifact, tc.reason, now); err == nil {
					t.Errorf("Approve(%s) must fail", tc.name)
				}
				return
			}
			r, _ := RequestApproval(covHighSpec(), std, now)
			if _, err := Approve(r, tc.approver, tc.artifact, tc.reason, now); err == nil {
				t.Errorf("Approve(%s) must fail", tc.name)
			}
		})
	}
	low, _ := RequestApproval(covLowSpec(), std, now)
	lowAppr, err := Approve(low, "humana", "somehash", "read ok", now)
	if err != nil {
		t.Fatalf("low-risk approve binds any artifact: %v", err)
	}
	if lowAppr.BoundArtifact != "somehash" {
		t.Fatalf("low binding drift: %+v", lowAppr)
	}
}

func TestCovPauseResumeRejectRevokePaths(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	std := covPolicy(t, "standard")
	fresh := func(t *testing.T) ApprovalRequest {
		t.Helper()
		r, err := RequestApproval(covHighSpec(), std, now)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	paused, err := Pause(fresh(t), "agent-a", "waiting", now)
	if err != nil || paused.State != ApprovalPaused {
		t.Fatalf("Pause: %+v %v", paused, err)
	}
	if _, err := Pause(paused, "agent-a", "again", now); err == nil {
		t.Error("pausing a paused request must fail")
	}
	if _, err := Pause(fresh(t), "agent-c", "executor freezes", now); err == nil {
		t.Error("executor must not pause")
	}
	if _, err := Pause(fresh(t), "stranger", "no duty", now); err == nil {
		t.Error("outsider must not pause")
	}
	if _, err := Pause(fresh(t), "agent-a", "", now); err == nil {
		t.Error("empty pause reason must fail")
	}
	if _, err := Pause(fresh(t), "agent-a", "a\nb", now); err == nil {
		t.Error("multiline pause reason must fail")
	}

	resumed, err := Resume(paused, "agent-b", "green", now)
	if err != nil || resumed.State != ApprovalRequested {
		t.Fatalf("Resume: %+v %v", resumed, err)
	}
	if _, err := Resume(fresh(t), "agent-a", "nothing paused", now); err == nil {
		t.Error("resuming a non-paused request must fail")
	}
	appr := covApproved(t, now)
	if _, err := Resume(appr, "agent-a", "decided", now); err == nil {
		t.Error("resuming a decided request must fail")
	}
	p2, _ := Pause(fresh(t), "agent-a", "hold", now)
	if _, err := Resume(p2, "agent-c", "executor lifts", now); err == nil {
		t.Error("executor must not resume")
	}
	if _, err := Resume(p2, "agent-a", "", now); err == nil {
		t.Error("empty resume reason must fail")
	}

	rej, err := Reject(fresh(t), "humana", "frozen week", now)
	if err != nil || rej.State != ApprovalRejected {
		t.Fatalf("Reject: %+v %v", rej, err)
	}
	p3, _ := Pause(fresh(t), "agent-a", "hold", now)
	rej2, err := Reject(p3, "agent-b", "reviewer refuses paused", now)
	if err != nil || rej2.State != ApprovalRejected {
		t.Fatalf("Reject paused: %+v %v", rej2, err)
	}
	if _, err := Reject(appr, "humana", "decided", now); err == nil {
		t.Error("rejecting a decided approval must fail")
	}
	if _, err := Reject(fresh(t), "bot", "bot judges", now); err == nil {
		t.Error("non-human rejector must fail")
	}
	if _, err := Reject(fresh(t), "agent-a", "proposer buries", now); err == nil {
		t.Error("proposer must not reject")
	}
	if _, err := Reject(fresh(t), "agent-c", "executor judges", now); err == nil {
		t.Error("executor must not reject")
	}
	if _, err := Reject(fresh(t), "humana", "", now); err == nil {
		t.Error("empty reject reason must fail")
	}

	revReq, err := Revoke(fresh(t), "agent-a", "ask withdrawn", now)
	if err != nil || revReq.State != ApprovalRevoked {
		t.Fatalf("Revoke requested: %+v %v", revReq, err)
	}
	p4, _ := Pause(fresh(t), "agent-b", "hold", now)
	revPaused, err := Revoke(p4, "humana", "operator freeze", now)
	if err != nil || revPaused.State != ApprovalRevoked {
		t.Fatalf("Revoke paused: %+v %v", revPaused, err)
	}
	revAppr, err := Revoke(appr, "humana", "incident hold", now.Add(2*time.Hour))
	if err != nil || revAppr.State != ApprovalRevoked {
		t.Fatalf("Revoke approved: %+v %v", revAppr, err)
	}
	if _, err := Revoke(rej, "humana", "terminal", now); err == nil {
		t.Error("revoking a terminal approval must fail")
	}
	if _, err := Revoke(fresh(t), "bot", "bot un-signs", now); err == nil {
		t.Error("non-human outsider must not revoke")
	}
	if _, err := Revoke(fresh(t), "x", "short", now); err == nil {
		t.Error("unnamed revoker must fail")
	}
	if _, err := Revoke(fresh(t), "humana", "", now); err == nil {
		t.Error("empty revoke reason must fail")
	}
	// The executor cannot revoke: it never un-signs what authorizes
	// it (four distinct duties, same rule as Approve).
	if _, err := Revoke(fresh(t), "agent-c", "executor un-signs", now); err == nil {
		t.Error("executor must not revoke its own authorization")
	}
	// An unrelated human operator still can.
	revByHuman, err := Revoke(fresh(t), "humana-op", "operator hold", now)
	if err != nil || revByHuman.State != ApprovalRevoked {
		t.Errorf("Revoke by an unrelated human operator must succeed: %+v %v", revByHuman, err)
	}
}

func TestCovExpiredAndCheck(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	appr := covApproved(t, now)
	if Expired(appr, now) {
		t.Error("fresh approval must not read expired")
	}
	if !Expired(appr, now.Add(DefaultApprovalTTL+time.Hour)) {
		t.Error("approval past TTL must read expired")
	}
	broken := appr
	broken.ExpiresAt = "not-a-time"
	if !Expired(broken, now) {
		t.Error("malformed expiry must fail closed (expired)")
	}
	req, _ := RequestApproval(covHighSpec(), covPolicy(t, "standard"), now)
	if err := Check(req, "abc123def456", []string{"gateway/"}, PolicyVersion, now); err == nil {
		t.Error("non-approved check must stop")
	}
	if err := Check(appr, "abc123def456", []string{"gateway/"}, PolicyVersion, now.Add(2*time.Hour)); err != nil {
		t.Errorf("valid check: %v", err)
	}
	if err := Check(appr, "abc123def456", nil, "", now.Add(2*time.Hour)); err != nil {
		t.Errorf("empty scope + empty policy pin must validate: %v", err)
	}
	if err := Check(appr, "deadbeef", []string{"gateway/"}, PolicyVersion, now.Add(2*time.Hour)); err == nil {
		t.Error("altered artifact must invalidate")
	}
	if err := Check(appr, "abc123def456", []string{"gateway/", "prod/"}, PolicyVersion, now.Add(2*time.Hour)); err == nil {
		t.Error("widened scope must invalidate")
	}
	if err := Check(appr, "abc123def456", []string{"gateway/"}, "regulus.govern-policy/v9", now.Add(2*time.Hour)); err == nil {
		t.Error("rotated policy must invalidate")
	}
	if err := Check(appr, "abc123def456", []string{"gateway/"}, PolicyVersion, now.Add(DefaultApprovalTTL+time.Hour)); err == nil {
		t.Error("expired approval must not validate")
	}
	revoked, _ := Revoke(appr, "humana", "hold", now.Add(3*time.Hour))
	if err := Check(revoked, "abc123def456", []string{"gateway/"}, PolicyVersion, now.Add(4*time.Hour)); err == nil {
		t.Error("revoked approval must stop pending actions")
	}
}

func TestCovIsHumanAndBypass(t *testing.T) {
	for _, who := range []string{"humana", "agent-a", "Bob", " reviewer-2 "} {
		if !IsHuman(who) {
			t.Errorf("IsHuman(%q) must hold", who)
		}
	}
	for _, who := range []string{"", "x", "ab", "auto", "BOT", " System ", "auto-approve", "self-approve", "ai", "AI"} {
		if IsHuman(who) {
			t.Errorf("IsHuman(%q) must fail", who)
		}
	}
	markers := []string{
		"skip approval", "without approval", "auto-approve", "auto approve",
		"approve this", "pre-approved", "ignore policy", "bypass approval",
		"override", "ignore previous instructions", "system:", "jailbreak",
		"self-grade", "exfiltrate",
	}
	for _, m := range markers {
		if !ContainsBypass("please " + m + " now") {
			t.Errorf("ContainsBypass misses %q", m)
		}
		if !ContainsBypass(strings.ToUpper(m)) {
			t.Errorf("ContainsBypass must be case-insensitive for %q", m)
		}
	}
	for _, clean := range []string{"", "ship the fix", "risk accepted, rollback ready"} {
		if ContainsBypass(clean) {
			t.Errorf("ContainsBypass(%q) must stay quiet", clean)
		}
	}
}

func TestCovShortHash(t *testing.T) {
	if got := shortHash("0123456789abcdef"); got != "0123456789ab" {
		t.Errorf("shortHash(long) = %q", got)
	}
	if got := shortHash("abc"); got != "abc" {
		t.Errorf("shortHash(short) = %q", got)
	}
	if got := shortHash(""); got != "unknown" {
		t.Errorf("shortHash(empty) = %q", got)
	}
}

func TestCovStoreRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	if !strings.Contains(GovernDir(root), ".regulus") || !strings.Contains(ApprovalsDir(root), "approvals") {
		t.Error("store dirs must stay under .regulus/governance")
	}
	if !strings.HasSuffix(AuditPath(root), "audit.jsonl") || !strings.HasSuffix(PolicyPath(root), "policy.json") {
		t.Error("store files must keep their names")
	}
	def, err := LoadPolicy(root)
	if err != nil || def.Profile != "standard" {
		t.Fatalf("absent policy must default deny-by-default: %+v %v", def, err)
	}
	if _, err := SavePolicy(root, "yolo", "humana", now); err == nil {
		t.Error("unknown profile must fail even for humans")
	}
	if _, err := SavePolicy(root, "wide", "bot", now); err == nil {
		t.Error("automated policy change must fail")
	}
	pol, err := SavePolicy(root, "wide", "humana", now)
	if err != nil || pol.Profile != "wide" || pol.UpdatedBy != "humana" {
		t.Fatalf("SavePolicy: %+v %v", pol, err)
	}
	loaded, err := LoadPolicy(root)
	if err != nil || loaded.Profile != "wide" {
		t.Fatalf("policy must round-trip: %+v %v", loaded, err)
	}
	if err := os.WriteFile(PolicyPath(root), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(root); err == nil {
		t.Error("malformed policy must fail closed")
	}
	if err := os.WriteFile(PolicyPath(root), []byte(`{"profile":"yolo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(root); err == nil {
		t.Error("unknown stored profile must fail closed")
	}
	if _, err := SavePolicy(root, "standard", "humana", now); err != nil {
		t.Fatal(err)
	}

	req, err := RequestApproval(covHighSpec(), covPolicy(t, "standard"), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveApproval(root, req); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	if err := SaveApproval(root, req); err == nil {
		t.Error("reused id must be refused")
	}
	drifted := req
	drifted.ID = ""
	if err := SaveApproval(root, drifted); err == nil {
		t.Error("id-less approval must fail verification")
	}
	back, err := LoadApproval(root, req.ID)
	if err != nil || back.ID != req.ID {
		t.Fatalf("LoadApproval: %+v %v", back, err)
	}
	if _, err := LoadApproval(root, "absent"); err == nil {
		t.Error("missing approval must fail")
	}
	if err := os.WriteFile(filepath.Join(ApprovalsDir(root), "bad.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadApproval(root, "bad"); err == nil {
		t.Error("malformed approval must fail")
	}
	mut := req
	mut.Kind = "teleport"
	raw, _ := json.Marshal(mut)
	if err := os.WriteFile(filepath.Join(ApprovalsDir(root), req.ID+".json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadApproval(root, req.ID); err == nil {
		t.Error("drifted kind must fail on load")
	}
	appr := covApproved(t, now)
	if err := SaveApproval(root, appr); err != nil {
		t.Fatalf("SaveApproval approved: %v", err)
	}
	if err := UpdateApproval(root, appr); err != nil {
		t.Fatalf("UpdateApproval: %v", err)
	}
	badState := appr
	badState.State = "SIGNED"
	if err := UpdateApproval(root, badState); err == nil {
		t.Error("unknown state must fail update")
	}
	out, corrupt, err := ListApprovals(root)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(out) != 1 || out[0].ID != appr.ID {
		t.Fatalf("list must return the valid record sorted: %+v corrupt=%v", out, corrupt)
	}
	if len(corrupt) != 2 {
		t.Fatalf("list must report both corrupt files loudly: %v", corrupt)
	}
	if err := os.WriteFile(filepath.Join(ApprovalsDir(root), "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ApprovalsDir(root), "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	out2, _, err := ListApprovals(root)
	if err != nil || len(out2) != 1 {
		t.Fatalf("list must skip non-json and dirs: %+v %v", out2, err)
	}
	empty, corruptEmpty, err := ListApprovals(t.TempDir())
	if err != nil || len(empty) != 0 || len(corruptEmpty) != 0 {
		t.Fatalf("absent store must list empty: %+v %v %v", empty, corruptEmpty, err)
	}
}

func TestCovVerifyApprovalErrors(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	std := covPolicy(t, "standard")
	base, err := RequestApproval(covHighSpec(), std, now)
	if err != nil {
		t.Fatal(err)
	}
	approved := covApproved(t, now)
	bad := []struct {
		name string
		req  ApprovalRequest
	}{
		{"empty id", withReq(base, func(r *ApprovalRequest) { r.ID = "" })},
		{"unknown kind", withReq(base, func(r *ApprovalRequest) { r.Kind = "teleport" })},
		{"unknown surface", withReq(base, func(r *ApprovalRequest) { r.Surface = "shadow" })},
		{"unknown state", withReq(base, func(r *ApprovalRequest) { r.State = "SIGNED" })},
		{"empty summary", withReq(base, func(r *ApprovalRequest) { r.Summary = "" })},
		{"empty scope", withReq(base, func(r *ApprovalRequest) { r.Scope = nil })},
		{"empty policy name", withReq(base, func(r *ApprovalRequest) { r.PolicyName = "" })},
		{"empty policy version", withReq(base, func(r *ApprovalRequest) { r.PolicyVersion = "" })},
		{"unnamed duty", withReq(base, func(r *ApprovalRequest) { r.Proposer = "x" })},
		{"collapsed roles", withReq(base, func(r *ApprovalRequest) { r.Executor = "agent-a" })},
		{"approved by bot", withReq(approved, func(r *ApprovalRequest) { r.Approver = "bot" })},
		{"self approved", withReq(approved, func(r *ApprovalRequest) { r.Approver = "agent-a" })},
		{"approved without artifact", withReq(approved, func(r *ApprovalRequest) { r.BoundArtifact = "" })},
		{"empty history", withReq(base, func(r *ApprovalRequest) { r.History = nil })},
		{"history not opened", withReq(base, func(r *ApprovalRequest) {
			r.History = []ApprovalEvent{{At: "t", By: "x", Action: "approved"}}
		})},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if err := VerifyApproval(tc.req); err == nil {
				t.Errorf("VerifyApproval(%s) must fail", tc.name)
			}
		})
	}
	if err := VerifyApproval(base); err != nil {
		t.Errorf("honest request must verify: %v", err)
	}
	if err := VerifyApproval(approved); err != nil {
		t.Errorf("honest approval must verify: %v", err)
	}
}

func TestCovAuditChain(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	if ev, err := LoadAudit(root); err != nil || len(ev) != 0 {
		t.Fatalf("absent audit must load empty: %+v %v", ev, err)
	}
	if err := VerifyAudit(nil); err != nil {
		t.Fatalf("empty chain verifies: %v", err)
	}
	if err := AppendAudit(root, now.Format(time.RFC3339), "humana", "request", "apr_1", "first"); err != nil {
		t.Fatal(err)
	}
	if err := AppendAudit(root, now.Add(time.Minute).Format(time.RFC3339), "humana", "approve", "apr_1", "second"); err != nil {
		t.Fatal(err)
	}
	events, err := LoadAudit(root)
	if err != nil || len(events) != 2 {
		t.Fatalf("LoadAudit: %+v %v", events, err)
	}
	if events[0].Seq != 1 || events[1].Seq != 2 || events[1].PrevHash != events[0].Hash {
		t.Fatalf("chain must link: %+v", events)
	}
	if err := VerifyAudit(events); err != nil {
		t.Fatalf("untampered chain must verify: %v", err)
	}
	edited := append([]AuditEvent(nil), events...)
	edited[1].Detail = "edited by attacker"
	if err := VerifyAudit(edited); err == nil {
		t.Error("edited line must break verification")
	}
	reordered := append([]AuditEvent(nil), events...)
	reordered[0], reordered[1] = reordered[1], reordered[0]
	if err := VerifyAudit(reordered); err == nil {
		t.Error("reordered chain must break verification")
	}
	cut := append([]AuditEvent(nil), events[1:]...)
	if err := VerifyAudit(cut); err == nil {
		t.Error("cut head must break sequence continuity")
	}
	if err := os.WriteFile(AuditPath(root), []byte("{bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAudit(root); err == nil {
		t.Error("malformed audit line must fail load")
	}
}

func TestCovRender(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	std := covPolicy(t, "standard")
	req, err := RequestApproval(covHighSpec(), std, now)
	if err != nil {
		t.Fatal(err)
	}
	full := RenderShow(covApproved(t, now))
	for _, want := range []string{"state=APPROVED", "bound", "history:", "tests:", "decided:"} {
		if !strings.Contains(full, want) {
			t.Errorf("RenderShow(full) misses %q:\n%s", want, full)
		}
	}
	min := RenderShow(req)
	if strings.Contains(min, "decided:") || strings.Contains(min, "bound ") {
		t.Errorf("RenderShow(requested) must not quote a decision:\n%s", min)
	}
	low, _ := RequestApproval(covLowSpec(), std, now)
	noTests := RenderShow(low)
	if strings.Contains(noTests, "tests:") {
		t.Errorf("RenderShow without tests must skip the line:\n%s", noTests)
	}
	if got := RenderList(nil); !strings.Contains(got, "no approvals") {
		t.Errorf("RenderList(empty) = %q", got)
	}
	list := RenderList([]ApprovalRequest{req, low})
	if !strings.Contains(list, req.ID) || !strings.Contains(list, low.ID) {
		t.Errorf("RenderList must name every record:\n%s", list)
	}
	if got := RenderAudit(nil); !strings.Contains(got, "audit log empty") {
		t.Errorf("RenderAudit(empty) = %q", got)
	}
	root := t.TempDir()
	if _, err := SavePolicy(root, "standard", "humana", now); err != nil {
		t.Fatal(err)
	}
	events, err := LoadAudit(root)
	if err != nil || len(events) == 0 {
		t.Fatal(events, err)
	}
	audit := RenderAudit(events)
	if !strings.Contains(audit, "policy-change") || !strings.Contains(audit, "#1") {
		t.Errorf("RenderAudit must replay in order:\n%s", audit)
	}
	if !strings.Contains(audit, verify.RiskLow) && !strings.Contains(audit, "humana") {
		t.Errorf("RenderAudit must cite actor:\n%s", audit)
	}
}

func withReq(base ApprovalRequest, f func(*ApprovalRequest)) ApprovalRequest {
	f(&base)
	return base
}
