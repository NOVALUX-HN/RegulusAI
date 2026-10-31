package guardians

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

func TestRegistrySixSpecializedGuardians(t *testing.T) {
	reg := Registry()
	if len(reg) != 6 {
		t.Fatalf("want 6 guardians, got %d", len(reg))
	}
	seen := map[string]bool{}
	for _, g := range reg {
		if g.ID == "" || g.Responsibility == "" || g.Scope == "" || g.Inputs == "" ||
			g.RequiredEvidence == "" || g.OutputSchema == "" || g.Acceptance == "" ||
			g.Rejection == "" || g.Abstention == "" {
			t.Fatalf("guardian must define every governance field, got %+v", g)
		}
		if !strings.Contains(strings.ToLower(g.Permissions), "read-only") {
			t.Fatalf("%s must be read-only, got %q", g.ID, g.Permissions)
		}
		if seen[g.Responsibility] {
			t.Fatalf("responsibilities must be distinct, dup %q", g.Responsibility)
		}
		seen[g.Responsibility] = true
	}
}

func TestByIDNormalization(t *testing.T) {
	for _, id := range []string{"regulus-gate-verifier", "@regulus-gate-verifier"} {
		g, ok := ByID(id)
		if !ok || g.ID != "regulus-gate-verifier" {
			t.Fatalf("ByID(%q) must resolve, got %v/%v", id, g, ok)
		}
	}
	if _, ok := ByID("regulus-ghost"); ok {
		t.Fatal("unknown id must not resolve")
	}
	if IsGuardian("regulus-worker") || !IsCrew("regulus-worker") {
		t.Fatal("crew roles are not guardians")
	}
}

func writeTaskReceipt(t *testing.T, root, evidTask, gate string) string {
	t.Helper()
	dir := evidence.TaskDir(root, evidTask)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: evidTask, Gate: gate, Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: time.Unix(1700000000, 0).UTC(),
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("ok")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	path, err := receipts.Write(dir, rec)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func dirSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		out[rel] = string(data)
		return nil
	})
	return out
}

func TestEvaluateRecordsValidAssessment(t *testing.T) {
	root := t.TempDir()
	path := writeTaskReceipt(t, root, "feat/T1", "build")
	rec, err := receipts.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := Evaluate(root, "feat/T1", "@regulus-evidence-auditor", "PASS", "receipts back every claim", []string{rec.ID})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Guardian != "regulus-evidence-auditor" || ev.Status != "PASS" {
		t.Fatalf("evaluation must be attributed, got %+v", ev)
	}
	entries, err := LoadEvaluations(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Note == "" {
		t.Fatalf("evaluation must persist, got %+v", entries)
	}
}

func TestEvaluateRejects(t *testing.T) {
	root := t.TempDir()
	writeTaskReceipt(t, root, "feat/T1", "build")
	cases := []struct {
		name     string
		guardian string
		status   string
		note     string
		evidence []string
		badSub   string
	}{
		{"unknown guardian", "regulus-ghost", "PASS", "n", []string{}, "unknown guardian"},
		{"crew cannot evaluate", "regulus-worker", "PASS", "n", []string{}, "not a guardian"},
		{"crew with at", "@regulus-scout", "PASS", "n", []string{}, "not a guardian"},
		{"bad verdict", "regulus-gate-verifier", "MAYBE", "n", []string{}, "verdict"},
		{"empty note", "regulus-gate-verifier", "PASS", "  ", []string{}, "note"},
		{"missing evidence", "regulus-gate-verifier", "PASS", "n", []string{"rec_nope"}, "missing evidence"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Evaluate(root, "feat/T1", c.guardian, c.status, c.note, c.evidence); err == nil {
				t.Fatalf("must reject %s", c.name)
			} else if !strings.Contains(strings.ToLower(err.Error()), c.badSub) {
				t.Fatalf("error must name %q, got %q", c.badSub, err.Error())
			}
		})
	}
}

func TestEvaluateIsReadOnly(t *testing.T) {
	root := t.TempDir()
	writeTaskReceipt(t, root, "feat/T1", "build")
	chart := filepath.Join(root, "chart.md")
	if err := os.WriteFile(chart, []byte("# chart\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := dirSnapshot(t, root)
	rec, _ := receipts.ReadFile(mustOneReceipt(t, root))
	if _, err := Evaluate(root, "feat/T1", "regulus-gate-verifier", "FAIL", "looks wrong", []string{rec.ID}); err != nil {
		t.Fatal(err)
	}
	after := dirSnapshot(t, root)
	for f, content := range before {
		if after[f] != content {
			t.Fatalf("evaluate must not modify %s", f)
		}
	}
	for f := range after {
		if _, ok := before[f]; !ok && f != filepath.Join(".regulus", "evidence", "feat__T1", "evaluations.jsonl") {
			t.Fatalf("evaluate must only append the evaluations log, created %s", f)
		}
	}
	if data, err := os.ReadFile(chart); err != nil || string(data) != "# chart\n" {
		t.Fatal("charts must be untouched by evaluation")
	}
}

func mustOneReceipt(t *testing.T, root string) string {
	t.Helper()
	entries, err := evidence.List(root, "feat/T1")
	if err != nil || len(entries) == 0 {
		t.Fatalf("need a receipt, got %v/%v", entries, err)
	}
	return entries[0].Path
}
