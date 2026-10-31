package decisions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

func resolveRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "charts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "charts", "c.md"), []byte("# c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func resolveReceipt(t *testing.T, root, task string) {
	t.Helper()
	dir := evidence.TaskDir(root, task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: task, Gate: "build", Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: time.Unix(1700000000, 0).UTC(),
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("ok")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	if _, err := receipts.Write(dir, rec); err != nil {
		t.Fatal(err)
	}
}

func groundedContract() Contract {
	c, err := ParseContract([]byte(`{
		"problem": "gate runs repeat unchanged work on every invocation",
		"scope": "gate execution",
		"context": "full profile runs all gates even when nothing relevant changed",
		"proposal": "add when-conditions so optional gates skip when inputs are unchanged",
		"alternatives": [{"name": "keep all-always", "tradeoff": "simpler but slower"}],
		"tradeoffs": ["one git status per run, cheap"],
		"constraints": ["mandatory: required gates always run"],
		"risks": ["stale skip output -> fail open with receipt note"],
		"consequences": "faster no-op runs",
		"evidence": ["chart:c"],
		"confidence": "medium",
		"reversibility": "easy",
		"rollback": "",
		"artifacts": ["internal/x.go"],
		"commits": [],
		"task": "feat/T1",
		"related": [],
		"critical": false
	}`))
	if err != nil {
		panic(err)
	}
	return c
}

func mustPropose(t *testing.T, root string, c Contract) Decision {
	t.Helper()
	d, err := Propose(root, c)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestResolveAcceptClean(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	d := mustPropose(t, root, groundedContract())
	got, err := Resolve(root, d.ID, ResolveOptions{Accept: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateAccepted {
		t.Fatalf("want ACCEPTED, got %s", got.State)
	}
	if len(got.History) != 3 || got.History[1].To != StateUnderReview || got.History[2].To != StateAccepted {
		t.Fatalf("accept must auto-advance through UNDER_REVIEW, got %+v", got.History)
	}
	back, err := Load(root, d.ID)
	if err != nil || back.State != StateAccepted {
		t.Fatalf("accept must persist, got %v/%v", back.State, err)
	}
}

func TestResolveCriticalNeedsHuman(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	c := groundedContract()
	c.Critical = true
	d := mustPropose(t, root, c)
	if _, err := Resolve(root, d.ID, ResolveOptions{Accept: true}); err == nil {
		t.Fatal("critical accept without human approval must be refused")
	} else if !strings.Contains(strings.ToLower(err.Error()), "human") {
		t.Fatalf("refusal must name human approval, got %q", err.Error())
	}
	back, _ := Load(root, d.ID)
	if back.State != StateProposed {
		t.Fatalf("refusal must not persist, got %s", back.State)
	}
	got, err := Resolve(root, d.ID, ResolveOptions{Accept: true, ApproveBy: "ana: reviewed evidence, risk accepted"})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateAccepted || got.History[len(got.History)-1].By != "ana: reviewed evidence, risk accepted" {
		t.Fatalf("human approval must be recorded, got %+v", got.History)
	}
	if _, err := Resolve(root, d.ID, ResolveOptions{Accept: true, ApproveBy: "noreason"}); err == nil {
		t.Fatal("approval without reason must be refused")
	}
}

func TestResolveBlockingRefuses(t *testing.T) {
	root := resolveRoot(t)
	c := groundedContract()
	c.Evidence = nil
	c.Task = ""
	d := mustPropose(t, root, c)
	if _, err := Resolve(root, d.ID, ResolveOptions{Accept: true}); err == nil {
		t.Fatal("blocking findings must refuse accept")
	} else if !strings.Contains(strings.ToLower(err.Error()), "inconclusive") {
		t.Fatalf("refusal must point at abstention, got %q", err.Error())
	}
	got, err := Resolve(root, d.ID, ResolveOptions{Inconclusive: true, Note: "no evidence available"})
	if err != nil || got.State != StateInconclusive {
		t.Fatalf("abstention must be allowed, got %v/%v", got.State, err)
	}
}

func TestResolveRejectNeedsNote(t *testing.T) {
	root := resolveRoot(t)
	d := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, d.ID, ResolveOptions{Reject: true}); err == nil {
		t.Fatal("reject without rationale must be refused")
	}
	got, err := Resolve(root, d.ID, ResolveOptions{Reject: true, Note: "wrong problem"})
	if err != nil || got.State != StateRejected {
		t.Fatalf("reasoned reject must land, got %v/%v", got.State, err)
	}
	if _, err := Resolve(root, d.ID, ResolveOptions{Accept: true}); err == nil {
		t.Fatal("terminal states must not resolve")
	}
}

func TestResolveObsoleteAndSupersede(t *testing.T) {
	root := resolveRoot(t)
	resolveReceipt(t, root, "feat/T1")
	a := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, a.ID, ResolveOptions{Accept: true}); err != nil {
		t.Fatal(err)
	}
	b := mustPropose(t, root, groundedContract())
	if _, err := Resolve(root, b.ID, ResolveOptions{Accept: true}); err == nil {
		t.Fatal("accepted same-scope must block a second accept")
	} else if !strings.Contains(err.Error(), a.ID) {
		t.Fatalf("refusal must name the accepted decision, got %q", err.Error())
	}
	got, err := Resolve(root, b.ID, ResolveOptions{Accept: true, Supersede: a.ID, ApproveBy: "ana: replacing with narrower scope"})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateAccepted {
		t.Fatalf("atomic supersede-accept must land, got %s", got.State)
	}
	back, _ := Load(root, a.ID)
	if back.State != StateSuperseded {
		t.Fatalf("old decision must supersede, got %s", back.State)
	}
	if _, err := Resolve(root, b.ID, ResolveOptions{Accept: true, Supersede: "dec_nope", ApproveBy: "ana: x"}); err == nil {
		t.Fatal("supersede of a non-accepted decision must be refused")
	}
}
