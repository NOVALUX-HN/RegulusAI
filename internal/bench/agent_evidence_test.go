package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentEvidenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	outcomes := []AgentOutcome{
		{TaskID: "AT1", Split: "exploration", Rep: 0, Status: AgentPass, TokensIn: 100, TokensOut: 10, TokenSource: "measured-host", Model: "m", Host: "opencode"},
		{TaskID: "AT1", Split: "exploration", Rep: 1, Status: AgentFail, TokensIn: 50, TokensOut: 5},
		{TaskID: "AT5", Split: "holdout", Rep: 0, Status: AgentTimeout},
	}
	manPath, err := WriteAgentEvidence(dir, outcomes, AgentRunManifest{
		SuiteVersion: AgentSuiteVersion, Model: "m", Host: "opencode",
		Variant: "candidate", Reps: 2, Split: "all", GitHead: "abc123",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(manPath) != dir {
		t.Fatalf("manifest = %s, want inside %s", manPath, dir)
	}
	m, back, err := ReadAgentEvidence(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 3 || m.SuiteVersion != AgentSuiteVersion || m.GitHead != "abc123" {
		t.Fatalf("round trip wrong: %+v (%d outcomes)", m, len(back))
	}
	rep := RenderAgentReport(m, back)
	for _, want := range []string{"AT1 [exploration] pass,fail", "AT5 [holdout] timeout", "pass-rate 0.50", "measured-host", "unknown"} {
		if !strings.Contains(rep, want) {
			t.Fatalf("report missing %q:\n%s", want, rep)
		}
	}
}

func TestAgentEvidenceTamperFailsClosed(t *testing.T) {
	dir := t.TempDir()
	outcomes := []AgentOutcome{{TaskID: "AT1", Status: AgentPass}}
	if _, err := WriteAgentEvidence(dir, outcomes, AgentRunManifest{SuiteVersion: AgentSuiteVersion}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, EvidenceOutcomesFile))
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, ' ')
	if err := os.WriteFile(filepath.Join(dir, EvidenceOutcomesFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadAgentEvidence(dir); err == nil {
		t.Fatal("tampered evidence must fail verification")
	}
}

func TestAgentEvidenceMissingDirFails(t *testing.T) {
	if _, _, err := ReadAgentEvidence(t.TempDir()); err == nil {
		t.Fatal("missing evidence must fail")
	}
}
