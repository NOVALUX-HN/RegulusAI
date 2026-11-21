// ReadTierEvidence tests: direct round-trip plus every failure shape over
// temp JSON (missing file, malformed manifest, mismatched sha, malformed
// plan, missing plan, empty PlanFile default). Portable, no shell.
package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tierEvidenceSelection() TierSelection {
	return TierSelection{
		Tier:     TierL2,
		Included: []string{"AT1"},
		Omitted:  []TierOmission{{ID: "AT2", Split: "exploration", Reason: "sampled out"}},
		Reason:   "test selection",
	}
}

func writeTierManifestFile(t *testing.T, dir string, m TierEvidenceManifest) {
	t.Helper()
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, TierPlanManifest), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestReadTierEvidenceRoundTrip(t *testing.T) {
	evDir := filepath.Join(t.TempDir(), "ev")
	sel := tierEvidenceSelection()
	if _, err := WriteTierEvidence(evDir, sel, "abc123", "2026-11-01T00:00:00Z"); err != nil {
		t.Fatalf("WriteTierEvidence: %v", err)
	}
	m, got, err := ReadTierEvidence(evDir)
	if err != nil {
		t.Fatalf("ReadTierEvidence: %v", err)
	}
	if m.Tier != TierL2 || m.TierVersion != TierVersion || m.GitHead != "abc123" {
		t.Fatalf("manifest = %+v, want tier L2 version %s head abc123", m, TierVersion)
	}
	if m.PlanFile != TierPlanFile || m.PlanSHA == "" {
		t.Fatalf("manifest plan ref = %+v, want file %s with sha", m, TierPlanFile)
	}
	if got.TierVersion != TierVersion || got.Tier != TierL2 {
		t.Fatalf("selection = %+v, want version %s tier L2", got, TierVersion)
	}
	if len(got.Included) != 1 || got.Included[0] != "AT1" {
		t.Fatalf("included = %v, want [AT1]", got.Included)
	}
	if len(got.Omitted) != 1 || got.Omitted[0].ID != "AT2" {
		t.Fatalf("omitted = %+v, want one AT2 omission", got.Omitted)
	}
}

func TestReadTierEvidenceMissingManifest(t *testing.T) {
	_, _, err := ReadTierEvidence(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "reading tier manifest") {
		t.Fatalf("err = %v, want reading-tier-manifest", err)
	}
}

func TestReadTierEvidenceMalformedManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, TierPlanManifest), []byte("{not valid json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := ReadTierEvidence(dir)
	if err == nil || !strings.Contains(err.Error(), "malformed tier manifest") {
		t.Fatalf("err = %v, want malformed-tier-manifest", err)
	}
}

func TestReadTierEvidenceManifestMismatch(t *testing.T) {
	evDir := filepath.Join(t.TempDir(), "ev")
	if _, err := WriteTierEvidence(evDir, tierEvidenceSelection(), "abc123", "2026-11-01T00:00:00Z"); err != nil {
		t.Fatalf("WriteTierEvidence: %v", err)
	}
	planPath := filepath.Join(evDir, TierPlanFile)
	raw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, append(raw, ' ', '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = ReadTierEvidence(evDir)
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("err = %v, want sha256-mismatch after editing the plan", err)
	}
}

func TestReadTierEvidenceMalformedPlan(t *testing.T) {
	dir := t.TempDir()
	bad := []byte("{not valid plan json\n")
	trimmed := bad[:len(bad)-1]
	sum := sha256.Sum256(trimmed)
	if err := os.WriteFile(filepath.Join(dir, TierPlanFile), bad, 0o644); err != nil {
		t.Fatal(err)
	}
	writeTierManifestFile(t, dir, TierEvidenceManifest{
		TierVersion: TierVersion, Tier: TierL2,
		PlanFile: TierPlanFile, PlanSHA: hex.EncodeToString(sum[:]),
	})
	_, _, err := ReadTierEvidence(dir)
	if err == nil || !strings.Contains(err.Error(), "malformed tier plan") {
		t.Fatalf("err = %v, want malformed-tier-plan", err)
	}
}

func TestReadTierEvidenceMissingPlan(t *testing.T) {
	dir := t.TempDir()
	sum := sha256.Sum256([]byte("absent"))
	writeTierManifestFile(t, dir, TierEvidenceManifest{
		TierVersion: TierVersion, Tier: TierL2,
		PlanFile: TierPlanFile, PlanSHA: hex.EncodeToString(sum[:]),
	})
	_, _, err := ReadTierEvidence(dir)
	if err == nil || !strings.Contains(err.Error(), "reading tier plan") {
		t.Fatalf("err = %v, want reading-tier-plan", err)
	}
}

func TestReadTierEvidenceEmptyPlanFileDefaults(t *testing.T) {
	dir := t.TempDir()
	plan := []byte(`{"tier_version":"eval-tiers-v1","tier":"L2","included":["AT1"]}` + "\n")
	trimmed := plan[:len(plan)-1]
	sum := sha256.Sum256(trimmed)
	if err := os.WriteFile(filepath.Join(dir, TierPlanFile), plan, 0o644); err != nil {
		t.Fatal(err)
	}
	// No plan_file key: the reader must default to tier-plan.json.
	raw := `{"tier_version":"eval-tiers-v1","tier":"L2","plan_sha256":"` + hex.EncodeToString(sum[:]) + "\"}\n"
	if err := os.WriteFile(filepath.Join(dir, TierPlanManifest), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	m, sel, err := ReadTierEvidence(dir)
	if err != nil {
		t.Fatalf("ReadTierEvidence with empty plan_file: %v", err)
	}
	if m.PlanFile != TierPlanFile {
		t.Fatalf("PlanFile = %q, want default %q", m.PlanFile, TierPlanFile)
	}
	if sel.Tier != TierL2 || len(sel.Included) != 1 || sel.Included[0] != "AT1" {
		t.Fatalf("selection = %+v, want L2 with [AT1]", sel)
	}
}
