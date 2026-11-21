// Governed-cycle router cover (CRAP paydown sin cambiar
// comportamiento): tabla de los 11 subverbos cycle-* con Dir tmp
// (len==0->2, cada dispatch rehúsa con 2 antes de IO, default->2).
// Router puro: sin stores, sin red, portable (t.TempDir).
package cmdx

import (
	"strings"
	"testing"
)

func TestEvolveValidationCycleRouterTable(t *testing.T) {
	subs := []string{
		"cycle-declare",
		"cycle-propose",
		"cycle-link",
		"cycle-compare",
		"cycle-review",
		"cycle-approve",
		"cycle-canary",
		"cycle-monitor",
		"cycle-promote",
		"cycle-rollback",
		"cycle-report",
	}
	for _, sub := range subs {
		t.Run(sub, func(t *testing.T) {
			dir := t.TempDir()
			io, _, errb := missionIO(dir)
			if code := io.EvolveValidationCycleCmd([]string{sub}); code != 2 {
				t.Fatalf("%s without flags exit = %d, want 2", sub, code)
			}
			if !strings.Contains(errb.String(), "regulus evolve validation") {
				t.Fatalf("%s must print the validation usage, got %q", sub, errb.String())
			}
		})
	}
}

func TestEvolveValidationCycleRouterEmptyAndUnknown(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	if code := io.EvolveValidationCycleCmd(nil); code != 2 {
		t.Fatalf("empty exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "regulus evolve validation") {
		t.Fatalf("empty must print the validation usage, got %q", errb.String())
	}
	if code := io.EvolveValidationCycleCmd([]string{}); code != 2 {
		t.Fatalf("empty slice exit = %d, want 2", code)
	}
	io2, _, errb2 := missionIO(dir)
	if code := io2.EvolveValidationCycleCmd([]string{"bogus"}); code != 2 {
		t.Fatalf("unknown exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), `unknown subcommand "bogus"`) {
		t.Fatalf("unknown must name the subcommand, got %q", errb2.String())
	}
	if !strings.Contains(errb2.String(), "regulus evolve validation") {
		t.Fatalf("unknown must print the validation usage, got %q", errb2.String())
	}
}
