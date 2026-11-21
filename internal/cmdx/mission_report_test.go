// Report handler tests (F13.7/NOV-110): the mission evidence
// expediente end to end — assemble from the real stores, persist,
// verify, tamper refusal and non-terminal refusal.
package cmdx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func advance(t *testing.T, io IO, id, to string) {
	t.Helper()
	if code := io.MissionAdvanceCmd([]string{"--id", id, "--to", to, "--actor", "op"}); code != 0 {
		t.Fatalf("advance to %s exit = %d", to, code)
	}
}

func TestMissionReportVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	id := missionCreate(t, io, "--id", "mrep", "--creator", "tester",
		"--criteria", "engine persists state")
	for _, st := range []string{"analyzing", "planning", "awaiting_approval", "executing", "integrating", "validating", "completed"} {
		advance(t, io, id, st)
	}
	out.Reset()
	if code := io.MissionReportCmd([]string{"--id", id}); code != 0 {
		t.Fatalf("report exit = %d", code)
	}
	quoted := out.String()
	for _, want := range []string{"mrep", "outcome=completed", "engine persists state", "limitation:", "unknown, not defaulted"} {
		if !strings.Contains(quoted, want) {
			t.Fatalf("report must quote %q, got:\n%s", want, quoted)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".regulus", "missions", id, ReportFileName))
	if err != nil || len(raw) == 0 {
		t.Fatalf("report file must persist: %v", err)
	}
	out.Reset()
	if code := io.MissionVerifyReportCmd([]string{"--id", id}); code != 0 {
		t.Fatalf("verify-report exit = %d", code)
	}
	if !strings.Contains(out.String(), "VALID mrep outcome=completed") {
		t.Fatalf("verify must declare VALID, got %q", out.String())
	}
}

func TestMissionReportRefusesNonTerminal(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	id := missionCreate(t, io, "--id", "mrun")
	advance(t, io, id, "analyzing")
	if code := io.MissionReportCmd([]string{"--id", id}); code == 0 {
		t.Fatal("report on a running mission must fail: no expediente before the finish")
	}
}

func TestMissionVerifyReportDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	id := missionCreate(t, io, "--id", "mtamper")
	for _, st := range []string{"analyzing", "planning", "awaiting_approval", "executing", "integrating", "validating", "failed"} {
		advance(t, io, id, st)
	}
	if code := io.MissionReportCmd([]string{"--id", id}); code != 0 {
		t.Fatalf("report exit = %d", code)
	}
	path := filepath.Join(dir, ".regulus", "missions", id, ReportFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-20] ^= 0xFF
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := io.MissionVerifyReportCmd([]string{"--id", id}); code == 0 {
		t.Fatal("verify-report must fail on a tampered expediente")
	}
}
