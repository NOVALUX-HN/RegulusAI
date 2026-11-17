// Handler tests for F13.5/NOV-108: the failure command family
// through the cmdx API — classify purity, decide round-trips
// (retry, verified reassign, learn feeding, idempotent replay),
// refusal on corrupt missions and status/explain evidence.
package cmdx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailureClassifyNamesClass(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	if code := io.FailureClassifyCmd([]string{"--error", "connection reset by peer"}); code != 0 {
		t.Fatalf("classify exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "class=transient") {
		t.Fatalf("classify must name transient, got %q", out.String())
	}
	out.Reset()
	if code := io.FailureClassifyCmd([]string{"--error", "policy denied scope"}); code != 0 {
		t.Fatalf("classify exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "class=permission_denied") {
		t.Fatalf("classify must name permission_denied, got %q", out.String())
	}
}

func TestFailureDecideRetryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := missionIO(dir)
	missionCreate(t, io, "--id", "m1")
	out.Reset()
	if code := io.FailureDecideCmd([]string{"--mission", "m1", "--error", "tls handshake timeout"}); code != 0 {
		t.Fatalf("decide exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "decision=retry") {
		t.Fatalf("decide must seal retry, got %q", out.String())
	}
	out.Reset()
	if code := io.FailureStatusCmd([]string{"--mission", "m1"}); code != 0 {
		t.Fatalf("status exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "class=transient") || !strings.Contains(out.String(), "decision=retry") {
		t.Fatalf("status must list the sealed retry, got %q", out.String())
	}
	out.Reset()
	if code := io.FailureExplainCmd([]string{"--mission", "m1", "--decision", "1"}); code != 0 {
		t.Fatalf("explain exit = %d stderr = %q", code, errb.String())
	}
	for _, want := range []string{"reason:", "discarded alternatives", "result:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("explain must carry %q, got:\n%s", want, out.String())
		}
	}
}

func TestFailureDecideVerifiedReassign(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := missionIO(dir)
	missionCreate(t, io, "--id", "m1")
	failureCheckpoint(t, io, errb)
	out.Reset()
	if code := io.FailureDecideCmd([]string{
		"--mission", "m1", "--run", "r1", "--task", "t1", "--agent", "a1",
		"--error", "worker lost heartbeat",
		"--to-agent", "b1", "--to-caps", "go", "--verified-caps", "go",
		"--scope", "memory:read", "--worktree-state", "clean", "--reason", "worker gone",
	}); code != 0 {
		t.Fatalf("decide exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "decision=reassign") {
		t.Fatalf("verified bid must seal reassign, got %q", out.String())
	}
}

func TestFailureDecideUnverifiedBidIsolates(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := missionIO(dir)
	missionCreate(t, io, "--id", "m1")
	failureCheckpoint(t, io, errb)
	out.Reset()
	if code := io.FailureDecideCmd([]string{
		"--mission", "m1", "--run", "r1", "--task", "t1", "--agent", "a1",
		"--error", "host down, node offline",
		"--to-agent", "b1", "--to-caps", "go", "--verified-caps", "go",
		"--scope", "memory:read,trace:read,memory:write", "--worktree-state", "clean",
	}); code != 0 {
		t.Fatalf("decide exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "decision=isolate") {
		t.Fatalf("widened scopes must degrade to isolate, got %q", out.String())
	}
}

// failureCheckpoint records the failed source checkpoint a reassignment
// bid verifies against: running first, then failed (the F12.5 legal
// edge — pending never jumps straight to failed), with caller-held
// scopes and one required capability.
func failureCheckpoint(t *testing.T, io IO, errb *bytes.Buffer) {
	t.Helper()
	if code := io.RecoveryCheckpointCmd([]string{
		"--run", "r1", "--task", "t1", "--agent", "a1",
		"--state", "running", "--scope", "memory:read,trace:read", "--require-cap", "go",
	}); code != 0 {
		t.Fatalf("checkpoint running exit = %d stderr = %q", code, errb.String())
	}
	if code := io.RecoveryCheckpointCmd([]string{
		"--run", "r1", "--task", "t1", "--agent", "a1", "--state", "failed",
	}); code != 0 {
		t.Fatalf("checkpoint failed exit = %d stderr = %q", code, errb.String())
	}
}

func TestFailureDecideFeedsHypothesisOnly(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := missionIO(dir)
	missionCreate(t, io, "--id", "m1")
	out.Reset()
	if code := io.FailureDecideCmd([]string{
		"--mission", "m1", "--gate", "red", "--feed-learn",
	}); code != 0 {
		t.Fatalf("decide exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "hypothesis saved as") {
		t.Fatalf("feed-learn must save a hypothesis, got %q", out.String())
	}
	if !strings.Contains(out.String(), "verify before trusting") {
		t.Fatalf("feed-learn must warn hypotheses are not facts, got %q", out.String())
	}
}

func TestFailureDecideReplaysIdemKey(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := missionIO(dir)
	missionCreate(t, io, "--id", "m1")
	args := []string{"--mission", "m1", "--error", "boom", "--idem-key", "k1"}
	if code := io.FailureDecideCmd(args); code != 0 {
		t.Fatalf("first decide exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := io.FailureDecideCmd(args); code != 0 {
		t.Fatalf("replay exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "replayed") {
		t.Fatalf("replay must say so, got %q", out.String())
	}
	out.Reset()
	if code := io.FailureStatusCmd([]string{"--mission", "m1", "--json"}); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	if strings.Count(out.String(), "\"id\": \"dec_") != 1 {
		t.Fatalf("replay must not append twice, got %q", out.String())
	}
}

func TestFailureDecideRefusesCorruptMission(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	missionCreate(t, io, "--id", "m1")
	raw, err := os.ReadFile(filepath.Join(dir, ".regulus", "missions", "m1", "mission.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".regulus", "missions", "m1", "mission.json"), append(raw, 'X'), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := io.FailureDecideCmd([]string{"--mission", "m1", "--error", "boom"}); code == 0 {
		t.Fatal("decide on a tampered mission must fail closed")
	}
	if !strings.Contains(errb.String(), "failing closed") {
		t.Fatalf("refusal must say it fails closed, got %q", errb.String())
	}
}
