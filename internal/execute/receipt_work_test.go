// Table tests for ReceiptWork: every failure branch plus the happy
// path, through a fake CmdRunner — no real git runs, portable
// without git on PATH.
package execute

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/gates"
	"regulus-ai/internal/orchestrate"
)

// fakeReceiptRunner stands in for gates.ShRunner: no process runs,
// verdicts are scripted per command prefix.
type fakeReceiptRunner struct {
	calls      []string
	addErr     error
	commitErr  error
	commitExit int
	commitErrB string
}

func (f *fakeReceiptRunner) Run(_ context.Context, command string) (string, string, int, error) {
	f.calls = append(f.calls, command)
	if strings.HasPrefix(command, "git add ") {
		if f.addErr != nil {
			return "", "", 1, f.addErr
		}
		return "", "", 0, nil
	}
	if strings.HasPrefix(command, "git commit ") {
		return "", f.commitErrB, f.commitExit, f.commitErr
	}
	return "", "", 0, nil
}

func (f *fakeReceiptRunner) added() bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, "git add ") {
			return true
		}
	}
	return false
}

func stubReceiptRunner(t *testing.T, fake *fakeReceiptRunner) {
	t.Helper()
	old := newReceiptRunner
	newReceiptRunner = func(string) gates.CmdRunner { return fake }
	t.Cleanup(func() { newReceiptRunner = old })
}

func receiptTask() orchestrate.ParallelTask {
	return orchestrate.ParallelTask{
		ID: "t1", Agent: "codex",
		Scopes: []string{"s1"}, Files: []string{"b.go", "a.go"},
	}
}

func TestReceiptWorkTable(t *testing.T) {
	t.Run("empty scope fail-closed", func(t *testing.T) {
		fake := &fakeReceiptRunner{}
		stubReceiptRunner(t, fake)
		dir := t.TempDir()
		task := receiptTask()
		task.Files = []string{"a.go", "  "}
		err := ReceiptWork("root", "m1", 3)(context.Background(), task, dir)
		if err == nil || !strings.Contains(err.Error(), "empty scope entry") {
			t.Fatalf("err = %v, want empty scope entry", err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("runner calls = %v, want none (fail before shell)", fake.calls)
		}
		if _, stat := os.Stat(filepath.Join(dir, receiptFileName)); !os.IsNotExist(stat) {
			t.Fatalf("receipt must not be written on scope refusal")
		}
	})

	t.Run("marshal err", func(t *testing.T) {
		old := receiptMarshal
		receiptMarshal = func(any) ([]byte, error) { return nil, errors.New("boom") }
		t.Cleanup(func() { receiptMarshal = old })
		fake := &fakeReceiptRunner{}
		stubReceiptRunner(t, fake)
		err := ReceiptWork("root", "m1", 3)(context.Background(), receiptTask(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "cannot encode receipt") {
			t.Fatalf("err = %v, want cannot encode receipt", err)
		}
	})

	t.Run("write err", func(t *testing.T) {
		fake := &fakeReceiptRunner{}
		stubReceiptRunner(t, fake)
		// A regular file as dir: joining the receipt name under it
		// always fails, portably (no permissions tricks).
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := ReceiptWork("root", "m1", 3)(context.Background(), receiptTask(), blocker)
		if err == nil || !strings.Contains(err.Error(), "cannot write receipt") {
			t.Fatalf("err = %v, want cannot write receipt", err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("runner calls = %v, want none (fail before shell)", fake.calls)
		}
	})

	t.Run("git add err", func(t *testing.T) {
		fake := &fakeReceiptRunner{addErr: errors.New("no git")}
		stubReceiptRunner(t, fake)
		err := ReceiptWork("root", "m1", 3)(context.Background(), receiptTask(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "cannot stage receipt") {
			t.Fatalf("err = %v, want cannot stage receipt", err)
		}
		if !fake.added() {
			t.Fatalf("want a git add attempt, got %v", fake.calls)
		}
	})

	t.Run("git commit exit 1", func(t *testing.T) {
		fake := &fakeReceiptRunner{commitExit: 1, commitErrB: "nothing to commit"}
		stubReceiptRunner(t, fake)
		err := ReceiptWork("root", "m1", 3)(context.Background(), receiptTask(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "cannot commit receipt") {
			t.Fatalf("err = %v, want cannot commit receipt", err)
		}
		if !strings.Contains(err.Error(), "nothing to commit") {
			t.Fatalf("err = %v, want stderr detail", err)
		}
	})

	t.Run("git commit err", func(t *testing.T) {
		fake := &fakeReceiptRunner{commitErr: errors.New("spawn failed")}
		stubReceiptRunner(t, fake)
		err := ReceiptWork("root", "m1", 3)(context.Background(), receiptTask(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "cannot commit receipt") {
			t.Fatalf("err = %v, want cannot commit receipt", err)
		}
	})

	t.Run("happy JSON and commit", func(t *testing.T) {
		fake := &fakeReceiptRunner{}
		stubReceiptRunner(t, fake)
		dir := t.TempDir()
		if err := ReceiptWork("root", "m1", 3)(context.Background(), receiptTask(), dir); err != nil {
			t.Fatalf("ReceiptWork: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, receiptFileName))
		if err != nil {
			t.Fatalf("read receipt: %v", err)
		}
		if !strings.HasSuffix(string(raw), "\n") {
			t.Fatalf("receipt must end with newline: %q", raw)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("receipt is not JSON: %v", err)
		}
		if doc["mission_id"] != "m1" || doc["plan_version"] != float64(3) ||
			doc["task_id"] != "t1" || doc["agent"] != "codex" {
			t.Fatalf("receipt identity = %v", doc)
		}
		files, ok := doc["files"].([]any)
		if !ok || len(files) != 2 || files[0] != "a.go" || files[1] != "b.go" {
			t.Fatalf("receipt files = %v, want sorted [a.go b.go]", doc["files"])
		}
		scopes, ok := doc["scopes"].([]any)
		if !ok || len(scopes) != 1 || scopes[0] != "s1" {
			t.Fatalf("receipt scopes = %v", doc["scopes"])
		}
		want := []string{
			"git add " + receiptFileName,
			`git commit -m "mission m1 task t1 receipt"`,
		}
		if len(fake.calls) != len(want) {
			t.Fatalf("runner calls = %v, want %v", fake.calls, want)
		}
		for i := range want {
			if fake.calls[i] != want[i] {
				t.Fatalf("runner calls = %v, want %v", fake.calls, want)
			}
		}
	})
}

func TestValidateReceiptScope(t *testing.T) {
	if err := validateReceiptScope(receiptTask()); err != nil {
		t.Fatalf("valid scope: %v", err)
	}
	bad := receiptTask()
	bad.Files = []string{"\t "}
	if err := validateReceiptScope(bad); err == nil ||
		!strings.Contains(err.Error(), "empty scope entry") {
		t.Fatalf("blank scope err = %v", err)
	}
}

func TestSortedReceiptFiles(t *testing.T) {
	in := []string{"b.go", "a.go", "b.go"}
	got := sortedReceiptFiles(in)
	if len(got) != 3 || got[0] != "a.go" || got[1] != "b.go" || got[2] != "b.go" {
		t.Fatalf("sortedReceiptFiles = %v", got)
	}
	if len(in) != 3 || in[0] != "b.go" {
		t.Fatalf("input mutated: %v", in)
	}
	if out := sortedReceiptFiles(nil); len(out) != 0 {
		t.Fatalf("sortedReceiptFiles(nil) = %v", out)
	}
}

func TestBuildReceiptDoc(t *testing.T) {
	doc := buildReceiptDoc("m1", 3, receiptTask(), []string{"a.go"})
	if doc["mission_id"] != "m1" || doc["plan_version"] != 3 ||
		doc["task_id"] != "t1" || doc["agent"] != "codex" {
		t.Fatalf("doc = %v", doc)
	}
	if files, ok := doc["files"].([]string); !ok || len(files) != 1 || files[0] != "a.go" {
		t.Fatalf("doc files = %v", doc["files"])
	}
}

func TestReceiptCommitMessage(t *testing.T) {
	if got := receiptCommitMessage("m1", "t1"); got != "mission m1 task t1 receipt" {
		t.Fatalf("receiptCommitMessage = %q", got)
	}
}
