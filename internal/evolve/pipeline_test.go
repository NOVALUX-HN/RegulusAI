package evolve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pipeline walks every stage in order; skips are refused and terminal
// stages (ADOPTED/REJECTED) advance nowhere.
func TestPipelineWalksInOrder(t *testing.T) {
	if err := Advance(StageObserve, StageProbe); err == nil {
		t.Fatal("OBSERVE -> PROBE skip must be refused")
	}
	chain := []Stage{StageObserve, StageHypothesize, StagePropose, StageProbe, StageEvaluate}
	for i := 0; i+1 < len(chain); i++ {
		if err := Advance(chain[i], chain[i+1]); err != nil {
			t.Fatalf("Advance(%s, %s) must hold: %v", chain[i], chain[i+1], err)
		}
	}
	if err := Advance(StageEvaluate, StageAdopted); err != nil {
		t.Fatalf("EVALUATE -> ADOPTED must hold: %v", err)
	}
	if err := Advance(StageEvaluate, StageRejected); err != nil {
		t.Fatalf("EVALUATE -> REJECTED must hold: %v", err)
	}
	if err := Advance(StageAdopted, StageObserve); err == nil {
		t.Fatal("terminal ADOPTED must advance nowhere")
	}
	if err := Advance(StageRejected, StageObserve); err == nil {
		t.Fatal("terminal REJECTED must advance nowhere")
	}
	if err := Advance(Stage("NOPE"), StageObserve); err == nil {
		t.Fatal("unknown stages must be refused")
	}
}

// Every stage carries an explicit contract: what it consumes and produces.
func TestPipelineStageContractsAreExplicit(t *testing.T) {
	for _, s := range []Stage{StageObserve, StageHypothesize, StagePropose, StageProbe, StageEvaluate, StageAdopted, StageRejected} {
		c := Contract(s)
		if strings.TrimSpace(c.Input) == "" || strings.TrimSpace(c.Output) == "" {
			t.Fatalf("stage %s needs an explicit input/output contract", s)
		}
	}
}

// Failed executions surface as observations with their supporting evidence.
func TestObserveFindsFailedExecutions(t *testing.T) {
	dir := t.TempDir()
	writeReceipt(t, dir, "demo/T9", "typecheck", "fail", 50)
	if err := os.WriteFile(filepath.Join(dir, ".regulus", "evidence", "demo__T9", "typecheck-2.json"),
		[]byte(`{"id":"rec_y","task":"demo/T9","gate":"typecheck","layer":1,"command":"echo ok","exit_code":1,"verdict":"fail","started_at":"2026-10-26T08:00:00Z","duration_ms":60}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, dir, "demo/T9", "lint", "pass", 10)
	obs, err := Observe(dir, "demo/T9")
	if err != nil {
		t.Fatal(err)
	}
	var failed *Observation
	for i := range obs {
		if obs[i].Source == SourceRepeatedFailures && strings.Contains(obs[i].Summary, "typecheck") {
			failed = &obs[i]
		}
	}
	if failed == nil {
		t.Fatalf("want a repeated-failures observation for typecheck, got %+v", obs)
	}
	if failed.Count != 2 {
		t.Fatalf("want Count 2 (repeated failure), got %d", failed.Count)
	}
	if !failed.Generalizable() {
		t.Fatal("a twice-failed gate is generalizable support, not an isolated case")
	}
}

// One isolated failure is not a general rule: CheckSupport refuses it.
func TestIsolatedObservationHasNoSupport(t *testing.T) {
	dir := t.TempDir()
	writeReceipt(t, dir, "demo/T10", "typecheck", "fail", 50)
	obs, err := Observe(dir, "demo/T10")
	if err != nil {
		t.Fatal(err)
	}
	var failed *Observation
	for i := range obs {
		if obs[i].Source == SourceFailedExecutions {
			failed = &obs[i]
		}
	}
	if failed == nil {
		t.Fatalf("want a failed-executions observation, got %+v", obs)
	}
	if failed.Generalizable() {
		t.Fatal("a single failure must not count as generalizable support")
	}
	if err := CheckSupport([]Observation{*failed}); !errors.Is(err, ErrIsolated) {
		t.Fatalf("CheckSupport must refuse isolated observations with ErrIsolated, got %v", err)
	}
}
