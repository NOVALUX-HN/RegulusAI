package cli

import (
	"bytes"
	"strings"
	"testing"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/doctor"
)

func newCLI(probe func() doctor.Result) (CLI, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return CLI{Version: "test", Probe: probe, Stdout: &out, Stderr: &errb}, &out, &errb
}

func TestVersionFlag(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"--version"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := out.String(); got != "regulus test\n" {
		t.Fatalf("output = %q, want %q", got, "regulus test\n")
	}
}

func TestContractCommand(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"contract"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if got := out.String(); got != contract.Version+"\n" {
		t.Fatalf("output = %q, want %q", got, contract.Version+"\n")
	}
}

func TestHelpCommand(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"help"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Usage:") || !strings.Contains(out.String(), "doctor") {
		t.Fatal("help must list usage and the doctor command")
	}
}

func TestNoArgsIsUsageError(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run(nil); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "Usage:") {
		t.Fatal("no args must print usage to stderr")
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown command "frobnicate"`) {
		t.Fatalf("stderr must name the unknown command, got %q", errb.String())
	}
}

func TestDoctorSupported(t *testing.T) {
	c, out, _ := newCLI(func() doctor.Result {
		return doctor.Result{Detected: "opencode 2.1.0", Major: 2, Supported: true}
	})
	if code := c.Run([]string{"doctor"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "supported (V2)") {
		t.Fatalf("output must report supported V2, got %q", out.String())
	}
}

func TestDoctorRefusalExitsOne(t *testing.T) {
	c, _, errb := newCLI(func() doctor.Result {
		return doctor.Result{Reason: "OpenCode V1 is not supported — Regulus requires V2. Install OpenCode V2."}
	})
	if code := c.Run([]string{"doctor"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "V1 is not supported") {
		t.Fatalf("stderr must carry the refusal reason, got %q", errb.String())
	}
}
