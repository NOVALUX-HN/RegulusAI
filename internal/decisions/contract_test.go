package decisions

import (
	"strings"
	"testing"
)

func validContractJSON() string {
	return `{
		"problem": "gate runs repeat unchanged work on every invocation",
		"scope": "gate execution",
		"context": "full profile runs all 12 gates even when nothing relevant changed",
		"proposal": "add when-conditions so optional gates skip on unmatched trees",
		"alternatives": [{"name": "keep all-always", "tradeoff": "simple but slow"}],
		"tradeoffs": ["skip logic costs a git status per run"],
		"constraints": ["mandatory: required gates always run"],
		"risks": ["stale skip -> fail open with receipt note"],
		"consequences": "faster no-op runs; skip decisions recorded in the manifest",
		"evidence": ["chart:regulus-ai-v1"],
		"confidence": "medium",
		"reversibility": "easy",
		"rollback": "",
		"artifacts": ["internal/gates/gates.go"],
		"commits": [],
		"task": "",
		"related": [],
		"critical": false
	}`
}

func TestParseValidContract(t *testing.T) {
	c, err := ParseContract([]byte(validContractJSON()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Problem == "" || c.Proposal == "" || c.Scope == "" {
		t.Fatalf("fields must parse, got %+v", c)
	}
	if len(c.Alternatives) != 1 || c.Alternatives[0].Tradeoff == "" {
		t.Fatalf("alternatives must parse, got %+v", c.Alternatives)
	}
}

func TestParseRejects(t *testing.T) {
	base := validContractJSON()
	cases := []struct {
		name   string
		mutate func(string) string
		badSub string
	}{
		{"missing problem", func(s string) string {
			return strings.Replace(s, `"problem": "gate runs repeat unchanged work on every invocation",`, "", 1)
		}, "problem"},
		{"missing proposal", func(s string) string {
			return strings.Replace(s, `"proposal": "add when-conditions so optional gates skip on unmatched trees"`, `"proposal": ""`, 1)
		}, "proposal"},
		{"missing scope", func(s string) string { return strings.Replace(s, `"scope": "gate execution",`, "", 1) }, "scope"},
		{"bad confidence", func(s string) string { return strings.Replace(s, `"medium"`, `"certain"`, 1) }, "confidence"},
		{"bad reversibility", func(s string) string { return strings.Replace(s, `"easy"`, `"trivial"`, 1) }, "reversibility"},
		{"bad commit", func(s string) string { return strings.Replace(s, `"commits": []`, `"commits": ["zzz"]`, 1) }, "commit"},
		{"bad related", func(s string) string { return strings.Replace(s, `"related": []`, `"related": ["other"]`, 1) }, "related"},
		{"malformed json", func(s string) string { return s[:len(s)-5] }, "json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseContract([]byte(c.mutate(base))); err == nil {
				t.Fatalf("must reject %s", c.name)
			} else if !strings.Contains(strings.ToLower(err.Error()), c.badSub) {
				t.Fatalf("error must name %q, got %q", c.badSub, err.Error())
			}
		})
	}
}

func TestMintID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id, err := MintID()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(id, "dec_") || len(id) != 12 {
			t.Fatalf("bad id shape %q", id)
		}
		if seen[id] {
			t.Fatal("duplicate id")
		}
		seen[id] = true
	}
}

func TestProposeStoresExclusively(t *testing.T) {
	root := t.TempDir()
	c, err := ParseContract([]byte(validContractJSON()))
	if err != nil {
		t.Fatal(err)
	}
	d, err := Propose(root, c)
	if err != nil {
		t.Fatal(err)
	}
	if d.State != StateProposed || d.ID == "" {
		t.Fatalf("proposed decision must be PROPOSED with an id, got %v/%v", d.State, d.ID)
	}
	if len(d.History) != 1 || d.History[0].Kind != "proposed" {
		t.Fatalf("history must open with proposed, got %+v", d.History)
	}
	back, err := Load(root, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.Contract.Proposal != c.Proposal || back.State != StateProposed {
		t.Fatalf("round trip must preserve, got %v", back)
	}
	d2, err := Propose(root, c)
	if err != nil {
		t.Fatalf("content without id mints a fresh id, got %v", err)
	}
	if d2.ID == d.ID {
		t.Fatal("minted ids must never collide")
	}
}

func TestProposeExplicitIDAndCollision(t *testing.T) {
	root := t.TempDir()
	raw := strings.Replace(validContractJSON(), `"problem":`, `"id": "dec_custom1", "problem":`, 1)
	c, err := ParseContract([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	d, err := Propose(root, c)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "dec_custom1" {
		t.Fatalf("explicit id must be honored, got %q", d.ID)
	}
	c2, _ := ParseContract([]byte(raw))
	if _, err := Propose(root, c2); err == nil {
		t.Fatal("reused id must be refused")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("collision must be named, got %q", err.Error())
	}
	if _, err := ParseContract([]byte(strings.Replace(validContractJSON(), `"problem":`, `"id": "bad id!", "problem":`, 1))); err == nil {
		t.Fatal("bad id shape must be rejected")
	}
}

func TestTransitions(t *testing.T) {
	root := t.TempDir()
	c, _ := ParseContract([]byte(validContractJSON()))
	d, err := Propose(root, c)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ApplyTransition(StateUnderReview, "assess", "auto", "assessed"); err != nil {
		t.Fatal(err)
	}
	if err := d.ApplyTransition(StateAccepted, "human", "ok", "resolved"); err != nil {
		t.Fatal(err)
	}
	if err := d.ApplyTransition(StateProposed, "x", "y", "resolved"); err == nil {
		t.Fatal("backward transition must be rejected")
	}
	if err := d.ApplyTransition(StateSuperseded, "x", "y", "resolved"); err != nil {
		t.Fatal(err)
	}
	if err := d.ApplyTransition(StateAccepted, "x", "y", "resolved"); err == nil {
		t.Fatal("terminal states must not transition")
	}
	if len(d.History) != 4 {
		t.Fatalf("every transition must append, got %d", len(d.History))
	}
}

func TestIntegrity(t *testing.T) {
	root := t.TempDir()
	c, _ := ParseContract([]byte(validContractJSON()))
	d, err := Propose(root, c)
	if err != nil {
		t.Fatal(err)
	}
	if warns := CheckIntegrity(d); len(warns) != 0 {
		t.Fatalf("fresh decision must be clean, got %v", warns)
	}
	// Hand-edited state without a transition: mismatch.
	d.State = StateAccepted
	if warns := CheckIntegrity(d); len(warns) == 0 {
		t.Fatal("state/history mismatch must be flagged")
	} else if !strings.Contains(strings.Join(warns, "\n"), "mismatch") {
		t.Fatalf("must name mismatch, got %v", warns)
	}
	// Broken chain.
	d.State = StateProposed
	d.History[0].To = StateAccepted
	if warns := CheckIntegrity(d); len(warns) == 0 {
		t.Fatal("broken chain must be flagged")
	}
}
