// Extended pure-helper tests (coverage-80/T3 stretch): token
// proxy, kind vocabulary, per-kind scopes and the ACL split. All
// deterministic, no stores or shells.
package context

import (
	"strings"
	"testing"
)

func TestCountTokensTable(t *testing.T) {
	for in, want := range map[string]int64{
		"": 0, "a": 1, "ab": 1, "abcd": 1, "abcde": 2, "abcdefgh": 2, "abcdefghi": 3,
	} {
		if got := CountTokens(in); got != want {
			t.Errorf("CountTokens(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestValidKindTable(t *testing.T) {
	for _, k := range []string{KindCode, KindSymbol, KindDependency, KindDoc, KindADR, KindIssue, KindMemory} {
		if !ValidKind(k) {
			t.Errorf("ValidKind(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"", "Code", "CODE", "memo", "memory ", " snippet"} {
		if ValidKind(k) {
			t.Errorf("ValidKind(%q) = true, want false (closed vocabulary)", k)
		}
	}
}

func TestRequiredScopeForTable(t *testing.T) {
	if got := RequiredScopeFor(KindMemory); got == "" {
		t.Fatal("memory fragments require a scope")
	} else if !strings.Contains(got, "memory") {
		t.Fatalf("memory scope must name memory, got %q", got)
	}
	for _, k := range []string{KindCode, KindSymbol, KindDependency, KindDoc, KindADR, KindIssue, "", "bogus"} {
		if got := RequiredScopeFor(k); got != "" {
			t.Errorf("RequiredScopeFor(%q) = %q, want public (empty)", k, got)
		}
	}
}

func TestFilterByACLSplit(t *testing.T) {
	frags := []Fragment{
		{ID: "public", Kind: KindCode},
		{ID: "mem", Kind: KindMemory, RequiredScope: "memory:read"},
		{ID: "other", Kind: KindDoc, RequiredScope: "custom:scope"},
	}
	visible, denied := FilterByACL(frags, Profile{AgentID: "a"})
	if len(visible) != 1 || visible[0].ID != "public" {
		t.Fatalf("without scopes only public stays visible: %v / %v", visible, denied)
	}
	if len(denied) != 2 {
		t.Fatalf("two fragments must be denied separately: %v", denied)
	}
	for _, d := range denied {
		if !hasFlag(d, FlagDenied) {
			t.Fatalf("denied fragment must carry the flag: %+v", d)
		}
	}
	visible, denied = FilterByACL(frags, Profile{AgentID: "a", Scopes: []string{" memory:read ", "custom:scope"}})
	if len(visible) != 3 || len(denied) != 0 {
		t.Fatalf("held scopes (trimmed) open every fragment: %v / %v", visible, denied)
	}
	if vis, den := FilterByACL(nil, Profile{}); len(vis) != 0 || len(den) != 0 {
		t.Fatalf("empty corpus splits empty: %v / %v", vis, den)
	}
}
