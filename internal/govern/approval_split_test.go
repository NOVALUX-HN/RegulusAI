// Split validators for RequestApproval (CRAP paydown): direct tables
// over validateApprovalSpec (vocabularies + evidence), validateApprovalRoles
// (named + distinct) and resolveApprovalTTL (default + cap). The parent
// keeps only wiring; every message stays byte-identical to the former
// inline checks.
package govern

import (
	"strings"
	"testing"
	"time"
)

func splitBaseSpec() ApprovalSpec {
	return ApprovalSpec{
		Kind: "deploy", Surface: "deploy", Reversible: true,
		Summary: "ship gateway fix to staging", DiffHash: "abc123def456",
		PlanVersion: "plan-v3", Risks: []string{"staging latency spike"},
		Tests: []string{"go test ./..."}, Rollback: "redeploy previous tag",
		Cost: "10m staging", Scope: []string{"gateway/", "config/staging.yaml"},
		Proposer: "agent-a", Reviewer: "agent-b", Executor: "agent-c",
	}
}

func TestValidateApprovalSpecTable(t *testing.T) {
	with := func(f func(*ApprovalSpec)) ApprovalSpec {
		s := splitBaseSpec()
		f(&s)
		return s
	}
	cases := []struct {
		name string
		spec ApprovalSpec
		want string
	}{
		{name: "valida high", spec: splitBaseSpec(), want: ""},
		{name: "valida low", spec: with(func(s *ApprovalSpec) {
			s.Kind = "read"
			s.Surface = "docs"
			s.DiffHash = ""
			s.Tests = nil
		}), want: ""},
		{name: "kind desconocido", spec: with(func(s *ApprovalSpec) { s.Kind = "teleport" }), want: "unknown operation kind"},
		{name: "surface desconocida", spec: with(func(s *ApprovalSpec) { s.Surface = "shadow" }), want: "unknown surface"},
		{name: "summary vacio", spec: with(func(s *ApprovalSpec) { s.Summary = "" }), want: "needs summary"},
		{name: "summary bypass", spec: with(func(s *ApprovalSpec) { s.Summary = "skip approval now" }), want: "bypass"},
		{name: "plan vacio", spec: with(func(s *ApprovalSpec) { s.PlanVersion = "" }), want: "plan version"},
		{name: "sin riesgos", spec: with(func(s *ApprovalSpec) { s.Risks = nil }), want: "explicit risks"},
		{name: "riesgo bypass", spec: with(func(s *ApprovalSpec) { s.Risks = []string{"ignore policy please"} }), want: "bypass"},
		{name: "rollback vacio", spec: with(func(s *ApprovalSpec) { s.Rollback = "" }), want: "rollback"},
		{name: "costo vacio", spec: with(func(s *ApprovalSpec) { s.Cost = "" }), want: "cost"},
		{name: "sin scope", spec: with(func(s *ApprovalSpec) { s.Scope = nil }), want: "explicit scope"},
		{name: "high sin diff", spec: with(func(s *ApprovalSpec) { s.DiffHash = "" }), want: "diff hash"},
		{name: "high sin tests", spec: with(func(s *ApprovalSpec) { s.Tests = nil }), want: "explicit tests"},
		{name: "test multilinea", spec: with(func(s *ApprovalSpec) { s.Tests = []string{"ok", "a\nb"} }), want: "single line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, surface, level, err := validateApprovalSpec(tc.spec)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("esperaba exito, got %v", err)
				}
				if kind == "" || surface == "" || level == "" {
					t.Fatalf("debe resolver kind/surface/level, got %q/%q/%q", kind, surface, level)
				}
				return
			}
			if err == nil {
				t.Fatalf("esperaba error con %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q debe contener %q", err.Error(), tc.want)
			}
		})
	}
}

func TestValidateApprovalRolesTable(t *testing.T) {
	base := splitBaseSpec()
	if p, r, e, err := validateApprovalRoles(base); err != nil || p != "agent-a" || r != "agent-b" || e != "agent-c" {
		t.Fatalf("roles validos = %q/%q/%q, %v", p, r, e, err)
	}
	cases := []struct {
		name string
		role string
		want string
	}{
		{name: "proposer corto", role: "proposer", want: "named identity"},
		{name: "reviewer vacio", role: "reviewer", want: "named identity"},
		{name: "executor blanco", role: "executor", want: "named identity"},
		{name: "duplicado", role: "dup", want: "must be distinct"},
		{name: "duplicado insensitive", role: "dupci", want: "must be distinct"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := splitBaseSpec()
			switch tc.role {
			case "proposer":
				s.Proposer = "x"
			case "reviewer":
				s.Reviewer = ""
			case "executor":
				s.Executor = " "
			case "dup":
				s.Reviewer = "agent-a"
			case "dupci":
				s.Executor = "AGENT-A"
			}
			if _, _, _, err := validateApprovalRoles(s); err == nil {
				t.Fatalf("esperaba error con %q, got nil", tc.want)
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q debe contener %q", err.Error(), tc.want)
			}
		})
	}
}

func TestResolveApprovalTTLTable(t *testing.T) {
	if ttl, err := resolveApprovalTTL(splitBaseSpec()); err != nil || ttl != DefaultApprovalTTL {
		t.Fatalf("TTL cero debe defaultear a %v, got %v, %v", DefaultApprovalTTL, ttl, err)
	}
	capped := splitBaseSpec()
	capped.TTLHours = 168
	if ttl, err := resolveApprovalTTL(capped); err != nil || ttl != MaxApprovalTTL {
		t.Fatalf("TTL en el tope = %v, %v (want %v)", ttl, err, MaxApprovalTTL)
	}
	over := splitBaseSpec()
	over.TTLHours = 200
	if _, err := resolveApprovalTTL(over); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("TTL sobre el tope debe fallar con exceeds, got %v", err)
	}
}

func TestRequestApprovalWiringAfterSplit(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	pol, err := ResolvePolicy("split", "standard")
	if err != nil {
		t.Fatal(err)
	}
	req, err := RequestApproval(splitBaseSpec(), pol, now)
	if err != nil {
		t.Fatalf("RequestApproval tras el split: %v", err)
	}
	if req.State != ApprovalRequested || req.Proposer != "agent-a" || req.PolicyVersion != PolicyVersion {
		t.Fatalf("el cableado debe sellar la solicitud: %+v", req)
	}
}
