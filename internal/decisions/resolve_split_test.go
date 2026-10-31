package decisions

import (
	"strings"
	"testing"
)

// scopedContract returns a grounded contract with the given scope.
func scopedContract(scope string) Contract {
	c := groundedContract()
	c.Scope = scope
	return c
}

// mustUnderReview proposes a decision and advances it to UNDER_REVIEW,
// persisting it, so resolve paths take the direct edge.
func mustUnderReview(t *testing.T, root string, c Contract) Decision {
	t.Helper()
	d := mustPropose(t, root, c)
	if err := d.ApplyTransition(StateUnderReview, "core", "review started", "assessed"); err != nil {
		t.Fatal(err)
	}
	if err := d.Save(root); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestResolveRequiresExactlyOneAction(t *testing.T) {
	root := resolveRoot(t)
	d := mustPropose(t, root, groundedContract())
	for name, opt := range map[string]ResolveOptions{
		"none":                {},
		"two":                 {Accept: true, Reject: true},
		"three":               {Accept: true, Reject: true, Inconclusive: true},
		"reject+inconclusive": {Reject: true, Inconclusive: true},
	} {
		if _, err := Resolve(root, d.ID, opt); err == nil {
			t.Fatalf("%s: want exactly-one-action refusal", name)
		} else if !strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("%s: refusal must name exactly one action, got %q", name, err.Error())
		}
	}
	back, _ := Load(root, d.ID)
	if back.State != StateProposed {
		t.Fatalf("refusal must not persist, got %s", back.State)
	}
}

func TestResolveLoadFailure(t *testing.T) {
	root := resolveRoot(t)
	if _, err := Resolve(root, "dec_missing", ResolveOptions{Accept: true}); err == nil {
		t.Fatal("unknown id must be refused")
	}
}

func TestResolveTerminalRefused(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	d := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, d.ID, ResolveOptions{Accept: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, d.ID, ResolveOptions{Accept: true}); err == nil {
		t.Fatal("terminal states must not resolve")
	} else if !strings.Contains(err.Error(), "terminal or already resolved") {
		t.Fatalf("refusal must name terminal state, got %q", err.Error())
	}
}

func TestResolveMalformedApproval(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	d := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, d.ID, ResolveOptions{Accept: true, ApproveBy: "noreason"}); err == nil {
		t.Fatal("approval without reason must be refused")
	} else if !strings.Contains(err.Error(), "must look like") {
		t.Fatalf("refusal must name the approval shape, got %q", err.Error())
	}
}

func TestResolveRejectFromUnderReview(t *testing.T) {
	root := resolveRoot(t)
	d := mustUnderReview(t, root, groundedContract())
	got, err := Resolve(root, d.ID, ResolveOptions{Reject: true, Note: "wrong problem"})
	if err != nil || got.State != StateRejected {
		t.Fatalf("reasoned reject from UNDER_REVIEW must land, got %v/%v", got.State, err)
	}
}

func TestResolveInconclusiveNeedsNote(t *testing.T) {
	root := resolveRoot(t)
	d := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, d.ID, ResolveOptions{Inconclusive: true}); err == nil {
		t.Fatal("inconclusive without rationale must be refused")
	} else if !strings.Contains(err.Error(), "needs --note") {
		t.Fatalf("refusal must demand a note, got %q", err.Error())
	}
}

func TestResolveInconclusiveFromUnderReview(t *testing.T) {
	root := resolveRoot(t)
	d := mustUnderReview(t, root, groundedContract())
	got, err := Resolve(root, d.ID, ResolveOptions{Inconclusive: true, Note: "evidence thin"})
	if err != nil || got.State != StateInconclusive {
		t.Fatalf("abstention from UNDER_REVIEW must land, got %v/%v", got.State, err)
	}
}

func TestResolveAcceptFromUnderReview(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	d := mustUnderReview(t, root, groundedContract())
	got, err := Resolve(root, d.ID, ResolveOptions{Accept: true})
	if err != nil || got.State != StateAccepted {
		t.Fatalf("accept from UNDER_REVIEW must land, got %v/%v", got.State, err)
	}
}

func TestResolveSupersedeNeedsApproval(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	a := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, a.ID, ResolveOptions{Accept: true}); err != nil {
		t.Fatal(err)
	}
	b := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, b.ID, ResolveOptions{Accept: true, Supersede: a.ID}); err == nil {
		t.Fatal("supersede without human approval must be refused")
	} else if !strings.Contains(err.Error(), "superseding needs human approval") {
		t.Fatalf("refusal must name human approval, got %q", err.Error())
	}
}

func TestResolveSupersedeTargetMissing(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	b := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, b.ID, ResolveOptions{Accept: true, Supersede: "dec_nope", ApproveBy: "ana: replacing"}); err == nil {
		t.Fatal("supersede of a missing decision must be refused")
	} else if !strings.Contains(err.Error(), "supersede target") {
		t.Fatalf("refusal must name the supersede target, got %q", err.Error())
	}
}

func TestResolveSupersedeNeedsAccepted(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	a := mustPropose(t, root, groundedContract())
	b := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, b.ID, ResolveOptions{Accept: true, Supersede: a.ID, ApproveBy: "ana: replacing"}); err == nil {
		t.Fatal("supersede of a non-accepted decision must be refused")
	} else if !strings.Contains(err.Error(), "can only supersede an ACCEPTED decision") {
		t.Fatalf("refusal must demand an ACCEPTED target, got %q", err.Error())
	}
}

func TestResolveSupersedeScopeMismatch(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	a := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, a.ID, ResolveOptions{Accept: true}); err != nil {
		t.Fatal(err)
	}
	b := mustPropose(t, root, scopedContract("another scope"))
	if _, err := Resolve(root, b.ID, ResolveOptions{Accept: true, Supersede: a.ID, ApproveBy: "ana: replacing"}); err == nil {
		t.Fatal("supersede across scopes must be refused")
	} else if !strings.Contains(err.Error(), "scope mismatch") {
		t.Fatalf("refusal must name the scope mismatch, got %q", err.Error())
	}
}

func TestValidateOneAction(t *testing.T) {
	for name, tc := range map[string]struct {
		opt     ResolveOptions
		wantErr bool
	}{
		"none":   {ResolveOptions{}, true},
		"accept": {ResolveOptions{Accept: true}, false},
		"reject": {ResolveOptions{Reject: true}, false},
		"two":    {ResolveOptions{Accept: true, Reject: true}, true},
		"three":  {ResolveOptions{Accept: true, Reject: true, Inconclusive: true}, true},
	} {
		if err := validateOneAction(tc.opt); (err != nil) != tc.wantErr {
			t.Fatalf("%s: wantErr=%v got %v", name, tc.wantErr, err)
		}
	}
}

func TestNormalizeApproveBy(t *testing.T) {
	by, err := normalizeApproveBy(ResolveOptions{})
	if err != nil || by != "core" {
		t.Fatalf("empty approval must default to core, got %q/%v", by, err)
	}
	if _, err := normalizeApproveBy(ResolveOptions{ApproveBy: "noreason"}); err == nil {
		t.Fatal("malformed approval must be refused")
	}
	by, err = normalizeApproveBy(ResolveOptions{ApproveBy: "  ana: reviewed  "})
	if err != nil || by != "ana: reviewed" {
		t.Fatalf("approval must be trimmed, got %q/%v", by, err)
	}
}

func TestLoadResolvable(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	if _, err := loadResolvable(root, "dec_missing"); err == nil {
		t.Fatal("unknown id must fail to load")
	}
	d := mustPropose(t, root, groundedContract())
	if _, err := loadResolvable(root, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, d.ID, ResolveOptions{Accept: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadResolvable(root, d.ID); err == nil {
		t.Fatal("terminal states must not load as resolvable")
	}
}

func TestResolveHelpersRefuseBadTransitions(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	d := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, d.ID, ResolveOptions{Accept: true}); err != nil {
		t.Fatal(err)
	}
	back, err := Load(root, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyReject(&back, "core", "late note"); err == nil {
		t.Fatal("reject of a terminal decision must fail")
	}
	if err := applyInconclusive(&back, "core", "late note"); err == nil {
		t.Fatal("inconclusive of a terminal decision must fail")
	}
	if err := commitAccept(&back, "core", "late rationale"); err == nil {
		t.Fatal("accept of a terminal decision must fail")
	}
	if err := applySupersede(root, back, ResolveOptions{}, "core"); err != nil {
		t.Fatalf("empty supersede must be a no-op, got %v", err)
	}
}

func TestResolveHelpersSurfaceAdvanceErrors(t *testing.T) {
	proposed, ok := allowed[StateProposed]
	if !ok {
		t.Fatal("allowed must list PROPOSED")
	}
	allowed[StateProposed] = []string{StateRejected}
	defer func() { allowed[StateProposed] = proposed }()

	d := Decision{State: StateProposed}
	if err := commitAccept(&d, "core", "r"); err == nil {
		t.Fatal("broken PROPOSED advance must fail commit")
	}
	d = Decision{State: StateProposed}
	if err := applyReject(&d, "core", "note"); err == nil {
		t.Fatal("broken PROPOSED advance must fail reject")
	}
	d = Decision{State: StateProposed}
	if err := applyInconclusive(&d, "core", "note"); err == nil {
		t.Fatal("broken PROPOSED advance must fail inconclusive")
	}
}

func TestGuardAccept(t *testing.T) {
	clean := groundedContract()
	critical := groundedContract()
	critical.Critical = true
	for name, tc := range map[string]struct {
		pipe    Pipeline
		dec     Decision
		opt     ResolveOptions
		wantErr string
	}{
		"clean": {Pipeline{}, Decision{Contract: clean}, ResolveOptions{}, ""},
		"obsolete without supersede": {
			Pipeline{Policy: PolicyResult{ObsoleteBy: "dec_old"}},
			Decision{Contract: clean}, ResolveOptions{},
			"already decided",
		},
		"obsolete with supersede": {
			Pipeline{Policy: PolicyResult{ObsoleteBy: "dec_old"}},
			Decision{Contract: clean}, ResolveOptions{Supersede: "other"},
			"",
		},
		"blocking unaddressed": {
			Pipeline{Policy: PolicyResult{HasBlocking: true}, Findings: []Finding{{Code: "X1", Severity: SeverityBlocking, Detail: "boom"}}},
			Decision{Contract: clean}, ResolveOptions{},
			"blocked by policy",
		},
		"blocking addressed by obsolete-only supersede": {
			Pipeline{Policy: PolicyResult{HasBlocking: true, ObsoleteBy: "dec_old"}, Findings: []Finding{{Code: FindingObsolete, Severity: SeverityBlocking}}},
			Decision{Contract: clean}, ResolveOptions{Supersede: "dec_old"},
			"",
		},
		"blocking wrong supersede": {
			Pipeline{Policy: PolicyResult{HasBlocking: true, ObsoleteBy: "dec_old"}, Findings: []Finding{{Code: "X1", Severity: SeverityBlocking}}},
			Decision{Contract: clean}, ResolveOptions{Supersede: "dec_old"},
			"blocked by policy",
		},
		"critical without approval": {
			Pipeline{}, Decision{Contract: critical}, ResolveOptions{},
			"human approval",
		},
		"critical with approval": {
			Pipeline{}, Decision{Contract: critical}, ResolveOptions{ApproveBy: "ana: ok"},
			"",
		},
	} {
		err := guardAccept(tc.pipe, tc.dec, tc.opt)
		if tc.wantErr == "" && err != nil {
			t.Fatalf("%s: want pass, got %v", name, err)
		}
		if tc.wantErr != "" {
			if err == nil {
				t.Fatalf("%s: want refusal containing %q", name, tc.wantErr)
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("%s: refusal must contain %q, got %q", name, tc.wantErr, err.Error())
			}
		}
	}
}
