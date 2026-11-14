// NOV-45: StructuredRunner never uses a shell.
package gates

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStructuredRunnerLiteralNoShellExpansion(t *testing.T) {
	r := StructuredRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// `echo` with shell metachars must travel literally, not expand.
	out, _, code, err := r.RunStructured(ctx, StructuredCmd{Executable: "echo", Args: []string{"a;b", "x|y", "$(id)", "`id`"}})
	if err != nil {
		t.Fatalf("RunStructured echo = %v", err)
	}
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// echo prints args joined by spaces; metachars must survive literally.
	for _, want := range []string{"a;b", "x|y", "$(id)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout %q missing literal %q (shell expanded?)", out, want)
		}
	}
}

func TestStructuredRunnerEmptyExecutable(t *testing.T) {
	r := StructuredRunner{}
	_, _, code, err := r.RunStructured(context.Background(), StructuredCmd{})
	if err == nil {
		t.Fatal("want error for empty executable")
	}
	if code != -1 {
		t.Fatalf("code = %d, want -1 no-verdict", code)
	}
}

func TestStructuredRunnerExitCodeAndEnv(t *testing.T) {
	r := StructuredRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, code, err := r.RunStructured(ctx, StructuredCmd{Executable: "sh", Args: []string{"-c", "exit 3"}})
	if err != nil {
		t.Fatalf("sh exit 3 err = %v", err)
	}
	if code != 3 {
		t.Fatalf("code = %d, want 3", code)
	}
	out, _, _, err := r.RunStructured(ctx, StructuredCmd{Executable: "sh", Args: []string{"-c", "echo $REGULUS_T45"}, Env: []string{"REGULUS_T45=ok45"}})
	if err != nil {
		t.Fatalf("env echo = %v", err)
	}
	if !strings.Contains(out, "ok45") {
		t.Fatalf("stdout %q missing ok45", out)
	}
}

func TestStructuredRunnerTimeout(t *testing.T) {
	r := StructuredRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, _, _, err := r.RunStructured(ctx, StructuredCmd{Executable: "sh", Args: []string{"-c", "sleep 5"}})
	if err == nil {
		t.Fatal("want timeout error")
	}
}
