// Handler tests for F13.1/NOV-104: the mission command family
// through the cmdx API — lifecycle round-trips, refusals,
// idempotent replays, corrupt stores, cancel/resume durability
// and a true multi-process generation-conflict proof.
package cmdx

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/mission"
	"regulus-ai/internal/observability"
)

func missionIO(dir string) (IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return New(&out, &errb, dir, "test", nil), &out, &errb
}

func missionCreate(t *testing.T, io IO, args ...string) string {
	t.Helper()
	base := []string{"--objective", "ship durable missions", "--risk", "medium"}
	if code := io.MissionCreateCmd(append(base, args...)); code != 0 {
		t.Fatalf("create exit = %d", code)
	}
	entries, err := mission.ListMissions(io.Dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("one mission must exist: %+v %v", entries, err)
	}
	return entries[0].ID
}

func TestMissionCreateAdvanceShowStatusRoundTrip(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	id := missionCreate(t, io, "--id", "m1", "--creator", "tester",
		"--constraint", "no network", "--criteria", "green CI")
	if code := io.MissionAdvanceCmd([]string{"--id", "m1", "--to", "analyzing", "--actor", "op"}); code != 0 {
		t.Fatalf("advance exit = %d", code)
	}
	if !strings.Contains(out.String(), "m1 advanced to analyzing generation=2") {
		t.Fatalf("advance must print the move, got %q", out.String())
	}
	out.Reset()
	if code := io.MissionShowCmd([]string{"--id", "m1"}); code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	for _, want := range []string{"m1", "analyzing", "ship durable missions", "no network", "green CI"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("show must carry %q, got:\n%s", want, out.String())
		}
	}
	out.Reset()
	if code := io.MissionStatusCmd([]string{"--id", "m1"}); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	if !strings.Contains(out.String(), "mission m1: analyzing generation=2") {
		t.Fatalf("status must print the lifecycle line, got %q", out.String())
	}
	_ = id
}

func TestMissionIllegalTransitionRefused(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	missionCreate(t, io, "--id", "m2")
	if code := io.MissionAdvanceCmd([]string{"--id", "m2", "--to", "executing"}); code != 1 {
		t.Fatalf("created -> executing exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "illegal transition") {
		t.Fatalf("must name the illegal edge, got %q", errb.String())
	}
	_, m, err := mission.LoadMission(dir, "m2")
	if err != nil {
		t.Fatal(err)
	}
	if m.State != mission.StateCreated || m.Generation != 1 {
		t.Fatalf("refused advance must not touch the store: %+v", m)
	}
}

func TestMissionPauseResumeCancelReopen(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	missionCreate(t, io, "--id", "m3")
	for _, to := range []string{"analyzing", "planning", "awaiting_approval", "executing"} {
		if code := io.MissionAdvanceCmd([]string{"--id", "m3", "--to", to}); code != 0 {
			t.Fatalf("advance to %s exit = %d", to, code)
		}
	}
	if code := io.MissionPauseCmd([]string{"--id", "m3", "--reason", "hold"}); code != 0 {
		t.Fatalf("pause exit = %d", code)
	}
	if !strings.Contains(out.String(), "paused (was executing)") {
		t.Fatalf("pause must name the parked state, got %q", out.String())
	}
	out.Reset()
	if code := io.MissionResumeCmd([]string{"--id", "m3"}); code != 0 {
		t.Fatalf("resume exit = %d", code)
	}
	if !strings.Contains(out.String(), "resumed to executing") {
		t.Fatalf("resume must return to the parked state, got %q", out.String())
	}
	out.Reset()
	if code := io.MissionResumeCmd([]string{"--id", "m3"}); code != 1 {
		t.Fatalf("second resume exit = %d, want 1", code)
	}
	if code := io.MissionCancelCmd([]string{"--id", "m3"}); code != 2 {
		t.Fatalf("cancel without reason exit = %d, want 2", code)
	}
	if code := io.MissionCancelCmd([]string{"--id", "m3", "--reason", "no longer needed"}); code != 0 {
		t.Fatalf("cancel exit = %d", code)
	}
	out.Reset()
	if code := io.MissionAdvanceCmd([]string{"--id", "m3", "--to", "recovering"}); code != 1 {
		t.Fatalf("advance out of cancelled exit = %d, want 1", code)
	}
	if code := io.MissionReopenCmd([]string{"--id", "m3", "--reason", "scope restored"}); code != 0 {
		t.Fatalf("reopen exit = %d", code)
	}
	if !strings.Contains(out.String(), "reopened to recovering") {
		t.Fatalf("reopen must land on recovering, got %q", out.String())
	}
	_, m, err := mission.LoadMission(dir, "m3")
	if err != nil {
		t.Fatal(err)
	}
	if m.State != mission.StateRecovering || m.ReopenCount != 1 {
		t.Fatalf("reopened = %+v", m)
	}
}

func TestMissionLinkCorrelates(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	missionCreate(t, io, "--id", "m4")
	args := []string{"--id", "m4", "--run", "run_1,run_1", "--task", "t1",
		"--agent", "a1", "--worktree", "wt/m4", "--commit", "abc123",
		"--trace", "run_1", "--receipt", "rc1"}
	if code := io.MissionLinkCmd(args); code != 0 {
		t.Fatalf("link exit = %d", code)
	}
	if !strings.Contains(out.String(), "m4 linked") {
		t.Fatalf("link must confirm, got %q", out.String())
	}
	out.Reset()
	if code := io.MissionShowCmd([]string{"--id", "m4"}); code != 0 {
		t.Fatalf("show exit = %d", code)
	}
	for _, want := range []string{"run_1", "t1", "a1", "wt/m4", "abc123", "rc1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("show must correlate %q, got:\n%s", want, out.String())
		}
	}
	// Trace correlation: every mission command emits under the
	// linked run with the mission id as task.
	events, _, _, err := observability.QueryExec(dir, observability.ExecFilter{TaskID: "m4"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("mission commands must leave F11.1 trace events under the mission id")
	}
	for _, e := range events {
		if payload, ok := e.Payload["mission_id"]; !ok || payload != "m4" {
			t.Fatalf("mission trace events must carry mission_id: %+v", e)
		}
	}
}

func TestMissionIdempotentReplayLeavesStoreUntouched(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	missionCreate(t, io, "--id", "m5")
	if code := io.MissionAdvanceCmd([]string{"--id", "m5", "--to", "analyzing", "--idem-key", "k1"}); code != 0 {
		t.Fatalf("first advance exit = %d", code)
	}
	_, before, err := mission.LoadMission(dir, "m5")
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := io.MissionAdvanceCmd([]string{"--id", "m5", "--to", "analyzing", "--idem-key", "k1"}); code != 0 {
		t.Fatalf("replay exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "idempotent replay") {
		t.Fatalf("replay must say so, got %q", out.String())
	}
	_, after, err := mission.LoadMission(dir, "m5")
	if err != nil {
		t.Fatal(err)
	}
	if after.Generation != before.Generation || len(after.History) != len(before.History) {
		t.Fatal("idempotent replay must not bump the generation or grow the history")
	}
}

func TestMissionMutationOfMissingMissionRefuses(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	if code := io.MissionAdvanceCmd([]string{"--id", "ghost", "--to", "analyzing"}); code != 1 {
		t.Fatalf("advance of a missing mission exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "cannot read mission") {
		t.Fatalf("must name the missing mission, got %q", errb.String())
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, ".regulus", "missions")); len(entries) != 0 {
		t.Fatalf("a refused read must not litter the store, found %v", entries)
	}
}

func TestMissionCorruptStoreExplicit(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	missionCreate(t, io, "--id", "m6")
	path := filepath.Join(dir, ".regulus", "missions", "m6", mission.MissionFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, 'X'), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := io.MissionShowCmd([]string{"--id", "m6"}); code != 1 {
		t.Fatalf("show of edited store exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "sha mismatch") {
		t.Fatalf("must name the mismatch, got %q", errb.String())
	}
	errb.Reset()
	if code := io.MissionListCmd(nil); code != 1 {
		t.Fatalf("list with a corrupt entry exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "corrupt") {
		t.Fatalf("list must report the corrupt entry explicitly, got %q", errb.String())
	}
}

func TestMissionJSONCompat(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	missionCreate(t, io, "--id", "m7", "--objective", "json compat")
	out.Reset()
	if code := io.MissionShowCmd([]string{"--id", "m7", "--json"}); code != 0 {
		t.Fatalf("show --json exit = %d", code)
	}
	var decoded mission.Mission
	if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
		t.Fatalf("show --json must decode as a mission: %v\n%s", err, out.String())
	}
	if decoded.ID != "m7" || decoded.Objective != "json compat" || decoded.Version != mission.MissionVersion {
		t.Fatalf("decoded = %+v", decoded)
	}
}

// --- True multi-process proof: a stale coordinator in another OS
// process races a live one; its write must refuse with a
// generation conflict and the store must stay valid. ---

func TestMissionHelper(t *testing.T) {
	dir := os.Getenv("MISSION_HELPER_DIR")
	if dir == "" {
		t.Skip("helper subprocess only")
	}
	id := os.Getenv("MISSION_HELPER_ID")
	mode := os.Getenv("MISSION_HELPER_MODE")
	switch mode {
	case "advance":
		to := os.Getenv("MISSION_HELPER_TO")
		_, err := mission.UpdateMission(dir, id, time.Now().UTC(), func(m *mission.Mission) error {
			_, err := m.Transition(to, "helper", "race", "", time.Now().UTC())
			return err
		})
		if err != nil {
			t.Fatalf("helper advance: %v", err)
		}
	case "stale":
		ready := os.Getenv("MISSION_HELPER_READY")
		goFile := os.Getenv("MISSION_HELPER_GO")
		_, m, err := mission.LoadMission(dir, id)
		if err != nil {
			t.Fatalf("helper load: %v", err)
		}
		if err := os.WriteFile(ready, []byte("loaded"), 0o644); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(30 * time.Second)
		for {
			if _, err := os.Stat(goFile); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("helper stale: timed out waiting for the go signal")
			}
			time.Sleep(20 * time.Millisecond)
		}
		staleGen := m.Generation
		if _, err := m.Transition(os.Getenv("MISSION_HELPER_TO"), "helper", "stale", "", time.Now().UTC()); err != nil {
			t.Fatalf("helper stale transition: %v", err)
		}
		// The sibling process advanced the store meanwhile: this
		// write carries a stale generation and must refuse.
		if _, err := mission.SaveMission(dir, m, staleGen, time.Now().UTC()); err == nil {
			t.Fatal("helper stale: stale save must refuse with a generation conflict")
		} else if !strings.Contains(err.Error(), "generation conflict") {
			t.Fatalf("helper stale: must name the conflict, got %v", err)
		}
	}
}

func runMissionHelper(t *testing.T, dir, id, mode string, extra map[string]string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("cannot locate the test binary: %v", err)
	}
	env := append(os.Environ(),
		"MISSION_HELPER_DIR="+dir, "MISSION_HELPER_ID="+id, "MISSION_HELPER_MODE="+mode)
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	cmd := exec.Command(exe, "-test.run", "TestMissionHelper$", "-test.v")
	cmd.Env = env
	return cmd
}

func TestMissionMultiProcessConflict(t *testing.T) {
	if os.Getenv("MISSION_HELPER_DIR") != "" {
		t.Skip("outer test only (no recursion into helpers)")
	}
	dir := t.TempDir()
	io, _, _ := missionIO(dir)
	missionCreate(t, io, "--id", "m-mp")
	ready := filepath.Join(dir, "ready")
	goFile := filepath.Join(dir, "go")
	stale := runMissionHelper(t, dir, "m-mp", "stale", map[string]string{
		"MISSION_HELPER_READY": ready, "MISSION_HELPER_GO": goFile,
		"MISSION_HELPER_TO": mission.StateAnalyzing,
	})
	var staleErr bytes.Buffer
	stale.Stderr = &staleErr
	if err := stale.Start(); err != nil {
		t.Fatalf("cannot spawn the stale coordinator (multi-process proof needs process spawn): %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = stale.Process.Kill()
			t.Fatal("stale helper never loaded the mission")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The live coordinator advances in a second OS process while
	// the stale one holds its outdated copy.
	advance := runMissionHelper(t, dir, "m-mp", "advance", map[string]string{
		"MISSION_HELPER_TO": mission.StateAnalyzing,
	})
	if out, err := advance.CombinedOutput(); err != nil {
		t.Fatalf("live helper must advance cleanly: %v\n%s", err, out)
	}
	if err := os.WriteFile(goFile, []byte("go"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stale.Wait(); err != nil {
		t.Fatalf("stale helper must exit 0 after observing the conflict refusal: %v\n%s", err, staleErr.String())
	}
	_, loaded, err := mission.LoadMission(dir, "m-mp")
	if err != nil {
		t.Fatalf("store must stay valid after the race: %v", err)
	}
	if loaded.State != mission.StateAnalyzing || loaded.Generation != 2 {
		t.Fatalf("store must hold exactly the live coordinator: %+v", loaded)
	}
}
