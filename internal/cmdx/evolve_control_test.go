// Evolve validation control cover (CRAP paydown sin cambiar
// comportamiento): tabla held + faltantes en dir temp. Reusa el
// fixture de evolve_validation_run_test (declareRunPlan) sin
// redefinirlo; cada caso corre sobre t.TempDir() (portable, sin
// red/shell/git real).
package cmdx

import (
	"strings"
	"testing"

	"regulus-ai/internal/evolve"
)

func validControlArgs(plan string) []string {
	return []string{
		"--plan", plan, "--kind", "positive", "--name", "c1",
		"--expectation", "holds", "--outcome", "holds", "--held", "true",
	}
}

func TestEvolveValidationControlMissingTable(t *testing.T) {
	base := validControlArgs("val_1")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "empty", args: nil, want: "usage:"},
		{name: "missing plan", args: filterFlag(base, "--plan"), want: "usage:"},
		{name: "missing kind", args: filterFlag(base, "--kind"), want: "usage:"},
		{name: "missing name", args: filterFlag(base, "--name"), want: "usage:"},
		{name: "missing expectation", args: filterFlag(base, "--expectation"), want: "usage:"},
		{name: "missing outcome", args: filterFlag(base, "--outcome"), want: "usage:"},
		{name: "missing held", args: filterFlag(base, "--held"), want: "usage:"},
		{name: "bad held yes", args: withFlag(base, "--held", "yes"), want: "usage:"},
		{name: "bad held maybe", args: withFlag(base, "--held", "maybe"), want: "usage:"},
		{name: "positional", args: append(append([]string{}, base...), "extra"), want: "usage:"},
		{name: "unknown flag", args: append(append([]string{}, base...), "--bogus", "x"), want: "evolve validation control"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			io, _, errb := missionIO(t.TempDir())
			if code := io.EvolveValidationControlCmd(c.args); code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if !strings.Contains(errb.String(), c.want) {
				t.Fatalf("refusal must quote %q, got %q", c.want, errb.String())
			}
		})
	}
}

// filterFlag quita un flag y su valor de una lista k/v.
func filterFlag(args []string, flag string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == flag {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// withFlag reemplaza el valor de un flag existente.
func withFlag(args []string, flag, value string) []string {
	out := append([]string{}, args...)
	for i := 0; i < len(out); i++ {
		if out[i] == flag && i+1 < len(out) {
			out[i+1] = value
			return out
		}
	}
	return append(out, flag, value)
}

func TestEvolveValidationControlHeldTable(t *testing.T) {
	cases := []struct {
		name string
		kind string
		held string
		want string
	}{
		{name: "positive held", kind: "positive", held: "true", want: "held=true"},
		{name: "positive not held", kind: "positive", held: "false", want: "held=false"},
		{name: "negative held", kind: "negative", held: "true", want: "held=true"},
		{name: "negative not held", kind: "negative", held: "false", want: "held=false"},
		{name: "upper true", kind: "positive", held: "TRUE", want: "held=true"},
		{name: "spaced false", kind: "negative", held: " False ", want: "held=false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			p := declareRunPlan(t, dir)
			io, out, errb := missionIO(dir)
			args := []string{
				"--plan", p.ID, "--kind", c.kind, "--name", "c1",
				"--expectation", "holds", "--outcome", "holds", "--held", c.held,
			}
			if code := io.EvolveValidationControlCmd(args); code != 0 {
				t.Fatalf("control exit = %d stderr = %q", code, errb.String())
			}
			got := out.String()
			if !strings.Contains(got, "CONTROL_RECORDED "+p.ID) || !strings.Contains(got, c.want) {
				t.Fatalf("stdout must record the control with %s, got %q", c.want, got)
			}
			loaded, err := evolve.LoadValidationPlan(dir, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.Controls) != 1 || loaded.Controls[0].Name != "c1" {
				t.Fatalf("control must persist, got %+v", loaded.Controls)
			}
		})
	}
}

func TestEvolveValidationControlFailures(t *testing.T) {
	t.Run("unknown plan", func(t *testing.T) {
		io, _, errb := missionIO(t.TempDir())
		if code := io.EvolveValidationControlCmd(validControlArgs("val_missing")); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "evolve validation control:") {
			t.Fatalf("missing plan must name the command, got %q", errb.String())
		}
	})
	t.Run("bad kind", func(t *testing.T) {
		dir := t.TempDir()
		p := declareRunPlan(t, dir)
		io, _, errb := missionIO(dir)
		args := withFlag(withFlag(validControlArgs(p.ID), "--kind", "weird"), "--name", "c-bad")
		if code := io.EvolveValidationControlCmd(args); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "evolve validation control:") {
			t.Fatalf("bad kind must name the command, got %q", errb.String())
		}
	})
}
