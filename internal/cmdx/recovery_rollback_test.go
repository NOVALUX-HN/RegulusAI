// Recovery rollback cover (CRAP paydown sin cambiar comportamiento):
// 4 casos args/load-fail/ok/partial, sin git real (fakes). El store se
// siembra con la API durable (NewRun/RecordCheckpoint/SaveRun); el
// comando solo lee flags, carga el run y registra el undo.
package cmdx

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/recovery"
)

// seedRollbackRun siembra un run con un checkpoint en el estado pedido.
// failed lleva las compensaciones (el undo a aplicar); running queda
// sin compensar (el rollback lo rehúsa en parcial).
func seedRollbackRun(t *testing.T, dir, run, task, agent, state string, comp []recovery.Compensation) {
	t.Helper()
	now := time.Now().UTC()
	rec, err := recovery.NewRun(run, now)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if _, err := rec.RecordCheckpoint(recovery.CheckpointInput{
		TaskID: task, AgentID: agent, State: recovery.StateRunning,
	}, now); err != nil {
		t.Fatalf("running checkpoint: %v", err)
	}
	if state == recovery.StateFailed {
		if _, err := rec.RecordCheckpoint(recovery.CheckpointInput{
			TaskID: task, AgentID: agent, State: recovery.StateFailed,
			Compensate: comp,
		}, now); err != nil {
			t.Fatalf("failed checkpoint: %v", err)
		}
	}
	if _, err := recovery.SaveRun(dir, rec, now); err != nil {
		t.Fatalf("save run: %v", err)
	}
}

func TestParseRollbackArgsTable(t *testing.T) {
	var discard bytes.Buffer
	req, code, ok := parseRollbackArgs([]string{"--run", "r", "--task", "t", "--agent", "a"}, &discard)
	if !ok || code != 0 || req.run != "r" || req.task != "t" || req.agent != "a" {
		t.Fatalf("complete args must parse: %+v %d %v", req, code, ok)
	}
	cases := []struct {
		name string
		args []string
	}{
		{name: "empty", args: nil},
		{name: "missing agent", args: []string{"--run", "r", "--task", "t"}},
		{name: "unknown flag", args: []string{"--run", "r", "--task", "t", "--agent", "a", "--bogus"}},
		{name: "positional", args: []string{"--run", "r", "--task", "t", "--agent", "a", "extra"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			io, _, errb := missionIO(t.TempDir())
			if code := io.RecoveryRollbackCmd(c.args); code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if !strings.Contains(errb.String(), "recovery rollback") && !strings.Contains(errb.String(), "usage:") {
				t.Fatalf("parse refusal must quote the usage, got %q", errb.String())
			}
		})
	}
}

func TestRollbackState(t *testing.T) {
	if got := rollbackState(nil); got != recovery.StateRolledBack {
		t.Fatalf("nil error = %q, want rolled_back", got)
	}
	if got := rollbackState(errors.New("stuck")); got != recovery.StateRolledBackPartial {
		t.Fatalf("error = %q, want rolled_back_partial", got)
	}
}

func TestRecoveryRollbackCmdTable(t *testing.T) {
	t.Run("load failure", func(t *testing.T) {
		io, _, errb := missionIO(t.TempDir())
		args := []string{"--run", "ghost", "--task", "t", "--agent", "a"}
		if code := io.RecoveryRollbackCmd(args); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "recovery:") {
			t.Fatalf("missing run must name recovery, got %q", errb.String())
		}
	})
	t.Run("ok applies compensations", func(t *testing.T) {
		dir := t.TempDir()
		seedRollbackRun(t, dir, "r1", "t1", "a1", recovery.StateFailed, []recovery.Compensation{
			{Key: "c1", Desc: "undo write"},
			{Key: "c2", Desc: "restore snapshot"},
		})
		io, out, _ := missionIO(dir)
		args := []string{"--run", "r1", "--task", "t1", "--agent", "a1"}
		if code := io.RecoveryRollbackCmd(args); code != 0 {
			t.Fatalf("exit = %d", code)
		}
		got := out.String()
		if !strings.Contains(got, "recovery: compensate c2: restore snapshot") {
			t.Fatalf("undo runs in reverse order, got %q", got)
		}
		if !strings.Contains(got, "recovery: rollback t1/a1 2 compensation(s) applied") {
			t.Fatalf("must summarize the undo, got %q", got)
		}
		_, rec, err := recovery.LoadRun(dir, "r1")
		if err != nil {
			t.Fatalf("rolled-back run must persist: %v", err)
		}
		cp, err := rec.Lookup("t1", "a1")
		if err != nil {
			t.Fatal(err)
		}
		if cp.State != recovery.StateRolledBack {
			t.Fatalf("state = %q, want rolled_back", cp.State)
		}
	})
	t.Run("partial on live checkpoint", func(t *testing.T) {
		dir := t.TempDir()
		seedRollbackRun(t, dir, "r2", "t1", "a1", recovery.StateRunning, nil)
		io, _, errb := missionIO(dir)
		args := []string{"--run", "r2", "--task", "t1", "--agent", "a1"}
		if code := io.RecoveryRollbackCmd(args); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "recovery rollback:") {
			t.Fatalf("partial must name the command, got %q", errb.String())
		}
	})
}
