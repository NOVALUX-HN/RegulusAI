package evolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/trace"
)

func writeReceipt(t *testing.T, root, task, gate, verdict string, ms int64) {
	t.Helper()
	dir := filepath.Join(root, ".regulus", "evidence", strings.ReplaceAll(task, "/", "__"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"id":"rec_x","task":"` + task + `","gate":"` + gate + `","layer":1,"command":"echo ok","exit_code":0,"verdict":"` + verdict + `","started_at":"2026-10-26T08:00:00Z","duration_ms":` + itoa(ms) + `}`
	if err := os.WriteFile(filepath.Join(dir, gate+"-1.json"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func TestAssembleRefusesWithoutReceipts(t *testing.T) {
	dir := t.TempDir()
	_, err := Assemble(dir, "demo/T1", "go")
	if err == nil || !strings.Contains(err.Error(), "no evidence") {
		t.Fatalf("want evidence-gate refusal, got %v", err)
	}
}

func TestAssemblePackSections(t *testing.T) {
	dir := t.TempDir()
	writeReceipt(t, dir, "demo/T1", "typecheck", "pass", 120)
	if _, err := trace.Record(dir, trace.Entry{
		Session: "s1", Message: "m1", Call: "c1", Tool: "gate_run", Status: trace.StatusCompleted,
		Files: []string{"internal/gates/gates.go"},
	}, time.Unix(1700000000, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if err := budget.Record(dir, budget.Entry{
		Kind: budget.KindReflect, Timestamp: time.Unix(1700000001, 0).UTC(),
		Task: "demo/T1", Outcome: budget.OutcomeCandidateProposed, Detail: "2 receipts",
	}); err != nil {
		t.Fatal(err)
	}
	pack, err := Assemble(dir, "demo/T1", "go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Evidence pack — task demo/T1, domain go",
		"## Session traces",
		"### session s1",
		"1 gate_run completed",
		"## Task receipts",
		"typecheck",
		"## Domain INDEX",
		"(no library for domain go yet)",
		"## Recent evolution decisions for this task",
		"reflect/candidate-proposed",
		"## Candidate template",
		"dimension:",
		"component:",
	} {
		if !strings.Contains(pack, want) {
			t.Fatalf("pack must contain %q, got:\n%s", want, pack)
		}
	}
}

func TestAssembleRejectsEmptyTaskOrDomain(t *testing.T) {
	dir := t.TempDir()
	if _, err := Assemble(dir, "", "go"); err == nil {
		t.Fatal("want task refusal")
	}
	if _, err := Assemble(dir, "demo/T1", ""); err == nil {
		t.Fatal("want domain refusal")
	}
}
