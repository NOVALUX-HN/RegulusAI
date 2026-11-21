// CompareGovernedCycle directo: root temp con tabla de
// precondiciones (Load/Verify/state/evaluator/PlanID/candidates)
// y los 3 bloqueos gobernados. Portable: solo t.TempDir().
package evolve

import (
	"strings"
	"testing"
)

func cgtWinningExploration(t *testing.T, root, planID string) {
	t.Helper()
	if _, err := RecordValidationTrial(root, planID, govTrial("baseline", "AT1", ManifestFail)); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if _, err := RecordValidationTrial(root, planID, govTrial("candidate", "AT1", ManifestPass)); err != nil {
		t.Fatalf("candidate: %v", err)
	}
}

func cgtDeclareLinked(t *testing.T, root string, spec GovernedSpec, planSpec ValidationPlanSpec) (string, string) {
	t.Helper()
	c, err := DeclareGovernedCycle(root, spec)
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	if _, err := ProposeGovernedCandidate(root, c.ID, "retry-cap-2", "cap retries", "sextus"); err != nil {
		t.Fatalf("propose: %v", err)
	}
	p, err := DeclareValidationPlan(root, planSpec)
	if err != nil {
		t.Fatalf("declare plan: %v", err)
	}
	if err := LinkGovernedPlan(root, c.ID, p.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	return c.ID, p.ID
}

func TestCompareGovernedPreconditionsTable(t *testing.T) {
	t.Run("load ausente", func(t *testing.T) {
		root := t.TempDir()
		if _, err := CompareGovernedCycle(root, "gov_missing", "andragon"); err == nil {
			t.Fatal("ciclo ausente debe fallar")
		}
	})
	t.Run("verify corrompido", func(t *testing.T) {
		root := t.TempDir()
		c, err := DeclareGovernedCycle(root, govSpec())
		if err != nil {
			t.Fatal(err)
		}
		bad, err := LoadGovernedCycle(root, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		bad.State = "BOGUS"
		if err := bad.saveGovernedCycle(root); err != nil {
			t.Fatal(err)
		}
		if _, err := CompareGovernedCycle(root, c.ID, "andragon"); err == nil || !strings.Contains(err.Error(), "unknown state") {
			t.Fatalf("estado corrupto debe fallar Verify, got %v", err)
		}
	})
	t.Run("state no declarado", func(t *testing.T) {
		root := t.TempDir()
		id := govWinningCycle(t, root)
		if _, err := CompareGovernedCycle(root, id, "andragon"); err == nil || !strings.Contains(err.Error(), "DECLARED/CANDIDATES") {
			t.Fatalf("recomparar COMPARED debe rehusar por estado, got %v", err)
		}
	})
	t.Run("evaluador ajeno", func(t *testing.T) {
		root := t.TempDir()
		id, _ := cgtDeclareLinked(t, root, govSpec(), govPlanSpec())
		_ = id
		c, _ := LoadGovernedCycle(root, id)
		_ = c
		if _, err := CompareGovernedCycle(root, id, "sextus"); err == nil || !strings.Contains(err.Error(), "pinned evaluator") {
			t.Fatalf("solo el evaluador compara, got %v", err)
		}
	})
	t.Run("sin plan", func(t *testing.T) {
		root := t.TempDir()
		c, err := DeclareGovernedCycle(root, govSpec())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ProposeGovernedCandidate(root, c.ID, "x", "cap retries", "sextus"); err != nil {
			t.Fatal(err)
		}
		if _, err := CompareGovernedCycle(root, c.ID, "andragon"); err == nil || !strings.Contains(err.Error(), "linked validation plan") {
			t.Fatalf("sin plan debe pedir link, got %v", err)
		}
	})
	t.Run("sin candidatos", func(t *testing.T) {
		root := t.TempDir()
		c, err := DeclareGovernedCycle(root, govSpec())
		if err != nil {
			t.Fatal(err)
		}
		p, err := DeclareValidationPlan(root, govPlanSpec())
		if err != nil {
			t.Fatal(err)
		}
		if err := LinkGovernedPlan(root, c.ID, p.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := CompareGovernedCycle(root, c.ID, "andragon"); err == nil || !strings.Contains(err.Error(), "versioned candidate") {
			t.Fatalf("sin candidatos debe fallar, got %v", err)
		}
	})
}

func TestCompareGovernedBlockedTable(t *testing.T) {
	t.Run("insuficiente", func(t *testing.T) {
		root := t.TempDir()
		id, pid := cgtDeclareLinked(t, root, govSpec(), govPlanSpec())
		if _, err := RecordValidationTrial(root, pid, govTrial("baseline", "AT1", ManifestInfra)); err != nil {
			t.Fatal(err)
		}
		if _, err := RecordValidationTrial(root, pid, govTrial("candidate", "AT1", ManifestPass)); err != nil {
			t.Fatal(err)
		}
		if _, err := CompareGovernedCycle(root, id, "andragon"); err == nil || !strings.Contains(err.Error(), "BLOCKED") {
			t.Fatalf("insuficiente debe bloquear, got %v", err)
		}
		got, _ := LoadGovernedCycle(root, id)
		if got.State != GovernedBlocked {
			t.Fatalf("estado = %s, want BLOCKED", got.State)
		}
	})
	t.Run("regresion", func(t *testing.T) {
		root := t.TempDir()
		id, pid := cgtDeclareLinked(t, root, govSpec(), govPlanSpec())
		if _, err := RecordValidationTrial(root, pid, govTrial("baseline", "AT1", ManifestPass)); err != nil {
			t.Fatal(err)
		}
		if _, err := RecordValidationTrial(root, pid, govTrial("candidate", "AT1", ManifestFail)); err != nil {
			t.Fatal(err)
		}
		if _, err := CompareGovernedCycle(root, id, "andragon"); err == nil || !strings.Contains(err.Error(), "BLOCKED") {
			t.Fatalf("regresion debe bloquear, got %v", err)
		}
		got, _ := LoadGovernedCycle(root, id)
		if got.State != GovernedBlocked {
			t.Fatalf("estado = %s, want BLOCKED", got.State)
		}
	})
	t.Run("holdout contradice", func(t *testing.T) {
		root := t.TempDir()
		spec := govPlanSpec()
		spec.Tier = "L3"
		spec.Budget = ValidationBudget{MaxRuns: 16, MaxWallTimeMS: 60000, MaxTokens: 100000}
		c, err := DeclareGovernedCycle(root, govSpec())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ProposeGovernedCandidate(root, c.ID, "retry-cap-2", "cap retries", "sextus"); err != nil {
			t.Fatal(err)
		}
		p, err := DeclareValidationPlan(root, spec)
		if err != nil {
			t.Fatal(err)
		}
		cgtWinningExploration(t, root, p.ID)
		if _, err := CompareValidation(root, p.ID); err != nil {
			t.Fatalf("freeze: %v", err)
		}
		holdBad := govTrial("candidate", "AT9", ManifestFail)
		holdBad.Split = ValidationSplitHoldout
		if _, err := RecordHoldoutTrial(root, p.ID, "audit: L3 gate", holdBad); err != nil {
			t.Fatalf("holdout: %v", err)
		}
		holdBase := govTrial("baseline", "AT9", ManifestPass)
		holdBase.Split = ValidationSplitHoldout
		if _, err := RecordHoldoutTrial(root, p.ID, "audit: L3 gate", holdBase); err != nil {
			t.Fatalf("holdout base: %v", err)
		}
		if err := LinkGovernedPlan(root, c.ID, p.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := CompareGovernedCycle(root, c.ID, "andragon"); err == nil || !strings.Contains(err.Error(), "holdout contradicts") {
			t.Fatalf("holdout contradictorio debe bloquear, got %v", err)
		}
		got, _ := LoadGovernedCycle(root, c.ID)
		if got.State != GovernedBlocked {
			t.Fatalf("estado = %s, want BLOCKED", got.State)
		}
	})
}
