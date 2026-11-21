// Recovery resume cover (CRAP paydown sin cambiar comportamiento):
// checkpoint->resume en dir temp (args/load-fail/ok). El store se
// siembra con los comandos (checkpoint) y la API durable
// (NewRun/RecordCheckpoint/SaveRun) para el caso terminal; el
// comando solo lee flags, carga el run y reporta el plan.
package cmdx

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/recovery"
)

// seedResumeRun siembra un run con un checkpoint en el estado pedido
// (running, checkpointed o succeeded) para los casos de resume.
func seedResumeRun(t *testing.T, dir, run, task, agent, state string) {
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
	if state != recovery.StateRunning {
		if _, err := rec.RecordCheckpoint(recovery.CheckpointInput{
			TaskID: task, AgentID: agent, State: state,
		}, now); err != nil {
			t.Fatalf("%s checkpoint: %v", state, err)
		}
	}
	if _, err := recovery.SaveRun(dir, rec, now); err != nil {
		t.Fatalf("save run: %v", err)
	}
}

func TestRecoveryResumeArgsTable(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "empty", args: nil, want: "usage:"},
		{name: "missing run", args: []string{"--task", "t1", "--agent", "a1"}, want: "usage:"},
		{name: "missing task", args: []string{"--run", "r1", "--agent", "a1"}, want: "usage:"},
		{name: "missing agent", args: []string{"--run", "r1", "--task", "t1"}, want: "usage:"},
		{name: "positional", args: []string{"--run", "r1", "--task", "t1", "--agent", "a1", "extra"}, want: "usage:"},
		{name: "unknown flag", args: []string{"--run", "r1", "--task", "t1", "--agent", "a1", "--bogus"}, want: "recovery resume"},
		{name: "bad policy", args: []string{"--run", "r1", "--task", "t1", "--agent", "a1", "--max-retries", "99"}, want: "recovery:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			io, _, errb := missionIO(t.TempDir())
			if code := io.RecoveryResumeCmd(c.args); code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if !strings.Contains(errb.String(), c.want) {
				t.Fatalf("refusal must quote %q, got %q", c.want, errb.String())
			}
		})
	}
}

func TestRecoveryResumeLoadFailure(t *testing.T) {
	io, _, errb := missionIO(t.TempDir())
	args := []string{"--run", "ghost", "--task", "t1", "--agent", "a1"}
	if code := io.RecoveryResumeCmd(args); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "recovery:") {
		t.Fatalf("missing run must name recovery, got %q", errb.String())
	}
}

func TestRecoveryResumeRefusesTerminal(t *testing.T) {
	dir := t.TempDir()
	seedResumeRun(t, dir, "r1", "t1", "a1", recovery.StateSucceeded)
	io, _, errb := missionIO(dir)
	args := []string{"--run", "r1", "--task", "t1", "--agent", "a1"}
	if code := io.RecoveryResumeCmd(args); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "recovery resume:") {
		t.Fatalf("terminal resume must name the command, got %q", errb.String())
	}
}

func TestRecoveryResumeReassignRefused(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	if code := io.RecoveryCheckpointCmd([]string{"--run", "r1", "--task", "t1", "--agent", "a1", "--state", "running", "--require-cap", "gpu"}); code != 0 {
		t.Fatalf("seed checkpoint exit = %d stderr = %q", code, errb.String())
	}
	if code := io.RecoveryCheckpointCmd([]string{"--run", "r1", "--task", "t1", "--agent", "a1", "--state", "checkpointed"}); code != 0 {
		t.Fatalf("seal checkpoint exit = %d stderr = %q", code, errb.String())
	}
	errb.Reset()
	args := []string{"--run", "r1", "--task", "t1", "--agent", "a1", "--new-agent", "b1"}
	if code := io.RecoveryResumeCmd(args); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "reassign refused") {
		t.Fatalf("off-registry target without the required cap must refuse, got %q", errb.String())
	}
}

func TestRecoveryResumeCheckpointToResume(t *testing.T) {
	t.Run("ok renders plan", func(t *testing.T) {
		dir := t.TempDir()
		io, out, errb := missionIO(dir)
		if code := io.RecoveryCheckpointCmd([]string{"--run", "r1", "--task", "t1", "--agent", "a1", "--state", "running"}); code != 0 {
			t.Fatalf("running checkpoint exit = %d stderr = %q", code, errb.String())
		}
		if code := io.RecoveryCheckpointCmd([]string{"--run", "r1", "--task", "t1", "--agent", "a1", "--state", "checkpointed"}); code != 0 {
			t.Fatalf("checkpoint exit = %d stderr = %q", code, errb.String())
		}
		out.Reset()
		if code := io.RecoveryResumeCmd([]string{"--run", "r1", "--task", "t1", "--agent", "a1"}); code != 0 {
			t.Fatalf("resume exit = %d stderr = %q", code, errb.String())
		}
		if !strings.Contains(out.String(), "resume t1/a1") {
			t.Fatalf("resume must render the plan, got %q", out.String())
		}
		_, rec, err := recovery.LoadRun(dir, "r1")
		if err != nil {
			t.Fatalf("resumed run must persist: %v", err)
		}
		cp, err := rec.Lookup("t1", "a1")
		if err != nil {
			t.Fatal(err)
		}
		if cp.State != recovery.StateRunning {
			t.Fatalf("state = %q, want running", cp.State)
		}
	})
	t.Run("ok with explicit effects", func(t *testing.T) {
		dir := t.TempDir()
		io, out, errb := missionIO(dir)
		if code := io.RecoveryCheckpointCmd([]string{"--run", "r2", "--task", "t2", "--agent", "a2", "--state", "running"}); code != 0 {
			t.Fatalf("running checkpoint exit = %d stderr = %q", code, errb.String())
		}
		if code := io.RecoveryCheckpointCmd([]string{"--run", "r2", "--task", "t2", "--agent", "a2", "--state", "checkpointed"}); code != 0 {
			t.Fatalf("checkpoint exit = %d stderr = %q", code, errb.String())
		}
		out.Reset()
		args := []string{"--run", "r2", "--task", "t2", "--agent", "a2", "--effect", "e1,e2"}
		if code := io.RecoveryResumeCmd(args); code != 0 {
			t.Fatalf("resume exit = %d stderr = %q", code, errb.String())
		}
		got := out.String()
		if !strings.Contains(got, "resume t2/a2") || !strings.Contains(got, "run: e1") || !strings.Contains(got, "run: e2") {
			t.Fatalf("explicit effects must list as pending, got %q", got)
		}
	})
}
