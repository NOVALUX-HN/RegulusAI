// Handler tests for F13.2/NOV-105: the mission plan|replan|
// explain family through the cmdx API — dry-run purity, the
// versioned lifecycle, refusals and history preservation.
package cmdx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func planRepo(t *testing.T, dir string) {
	t.Helper()
	rel := filepath.Join("internal", "demo", "demo.go")
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("package demo\n\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func planMission(t *testing.T, io IO) string {
	t.Helper()
	return missionCreate(t, io,
		"--id", "mp1",
		"--objective", "Añadir comando demo con tests en internal/demo",
		"--criteria", "Given el comando demo When se ejecuta Then imprime el plan",
		"--risk", "low")
}

func TestMissionPlanDryRunPersistsNothing(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	planRepo(t, dir)
	planMission(t, io)
	out.Reset()
	if code := io.MissionPlanCmd([]string{"--id", "mp1", "--dry-run"}); code != 0 {
		t.Fatalf("dry-run exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "dry-run: nothing persisted") {
		t.Fatalf("dry-run must say so, got %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".regulus", "missions", "mp1", "plans")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not create the plans directory")
	}
}

func TestMissionPlanReplanExplainRoundTrip(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := missionIO(dir)
	planRepo(t, dir)
	planMission(t, io)
	out.Reset()
	if code := io.MissionPlanCmd([]string{"--id", "mp1"}); code != 0 {
		t.Fatalf("plan exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "planned v1 status=ready") {
		t.Fatalf("plan must confirm v1 ready, got %q", out.String())
	}
	out.Reset()
	if code := io.MissionExplainCmd([]string{"--id", "mp1"}); code != 0 {
		t.Fatalf("explain exit = %d stderr = %q", code, errb.String())
	}
	for _, want := range []string{"why split", "why serialize", "autonomy:", "quality:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("explain must carry %q, got:\n%s", want, out.String())
		}
	}
	out.Reset()
	if code := io.MissionReplanCmd([]string{"--id", "mp1", "--change", "el usuario pide salida json",
		"--confirm", "t1=alcance confirmado"}); code != 0 {
		t.Fatalf("replan exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "replanned v2") {
		t.Fatalf("replan must confirm v2, got %q", out.String())
	}
	out.Reset()
	if code := io.MissionExplainCmd([]string{"--id", "mp1", "--version", "1", "--json"}); code != 0 {
		t.Fatalf("explain v1 exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), `"plan_version": 1`) || !strings.Contains(out.String(), "diff_vs_original") {
		t.Fatalf("explain --json must carry the plan plus diffs, got %q", out.String())
	}
}

func TestMissionPlanRefusals(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	planRepo(t, dir)
	if code := io.MissionPlanCmd([]string{"--id", "nope"}); code != 1 {
		t.Fatalf("unknown mission exit = %d, want 1", code)
	}
	planMission(t, io)
	if code := io.MissionReplanCmd([]string{"--id", "mp1"}); code != 2 {
		t.Fatalf("replan without --change exit = %d, want 2", code)
	}
	if code := io.MissionPlanCmd([]string{"--id", "mp1", "--objective", "Mejora"}); code != 1 {
		t.Fatalf("vague override must need clarification (exit 1), got %d", code)
	}
	errb.Reset()
	if code := io.MissionCmd([]string{"plan", "--id", "mp1", "--dry-run"}); code != 0 {
		t.Fatalf("dispatch plan exit = %d stderr = %q", code, errb.String())
	}
	if code := io.MissionCmd([]string{"explain", "--id", "mp1", "--version", "1"}); code != 1 {
		t.Fatalf("dispatch explain of a clarification plan exit = %d, want 1 (never a passing zero over a blocked plan)", code)
	}
	if code := io.MissionCmd([]string{"replan", "--id", "mp1"}); code != 2 {
		t.Fatalf("dispatch replan without change exit = %d, want 2", code)
	}
	if code := io.MissionCmd([]string{"frobnicate"}); code != 2 {
		t.Fatalf("unknown mission subcommand exit = %d, want 2", code)
	}
}
