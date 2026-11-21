// BenchTierPlanCmd exit codes (CRAP<50 sin cambiar comportamiento):
// orquestador delgado sobre parse/validate/resolve existentes.
// Cubre parse fail, tier bogus->2, L0+cap->2, jobs out->2,
// splits fail->1, resolve fail->1, save ok/fail, evidence ok,
// json vs texto. IO{Dir:tmp} y splits fakes vía env.
package cmdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

func TestBenchTierPlanUsageRefusals(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{name: "parse fail", args: []string{"--bogus"}, wantCode: 2, wantErr: "flag provided but not defined"},
		{name: "tier bogus", args: []string{"--tier", "bogus"}, wantCode: 2, wantErr: "usage: regulus bench tier-plan"},
		{name: "L0 plus capability", args: []string{"--tier", "L0", "--capability", "go"}, wantCode: 2, wantErr: "usage: regulus bench tier-plan"},
		{name: "capability bogus", args: []string{"--tier", "L1", "--capability", "bogus"}, wantCode: 2, wantErr: "usage: regulus bench tier-plan"},
		{name: "jobs over cap", args: []string{"--tier", "L0", "--jobs=99"}, wantCode: 2, wantErr: "usage: regulus bench tier-plan"},
		{name: "jobs negative", args: []string{"--tier", "L0", "--jobs=-1"}, wantCode: 2, wantErr: "usage: regulus bench tier-plan"},
		{name: "extra positional", args: []string{"--tier", "L0", "extra"}, wantCode: 2, wantErr: "usage: regulus bench tier-plan"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// Missing splits on purpose: usage errors (2) deben ganar
			// al splits ausente (1). Fija el fail-closed.
			t.Setenv("REGULUS_AGENT_SPLITS", filepath.Join(dir, "missing.json"))
			io, _, errb := cmdIO(dir)
			if code := io.BenchTierPlanCmd(tc.args); code != tc.wantCode {
				t.Fatalf("tier-plan %q exit = %d, want %d (stderr=%q)", tc.name, code, tc.wantCode, errb.String())
			}
			if tc.wantErr != "" && !strings.Contains(errb.String(), tc.wantErr) {
				t.Fatalf("tier-plan %q stderr debe contener %q, got %q", tc.name, tc.wantErr, errb.String())
			}
		})
	}
}

func TestBenchTierPlanStoreFailures(t *testing.T) {
	t.Run("splits fail", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("REGULUS_AGENT_SPLITS", filepath.Join(dir, "missing.json"))
		io, _, errb := cmdIO(dir)
		if code := io.BenchTierPlanCmd([]string{"--tier", "L0"}); code != 1 {
			t.Fatalf("splits ausente exit = %d, want 1 (stderr=%q)", code, errb.String())
		}
		if !strings.Contains(errb.String(), "bench agent-tasks:") {
			t.Fatalf("splits ausente debe citar bench agent-tasks, got %q", errb.String())
		}
	})

	t.Run("resolve fail selection", func(t *testing.T) {
		dir := t.TempDir()
		repoSplits(t)
		io, _, errb := cmdIO(dir)
		// L1 con diff solo-docs no casa con ninguna capability: la
		// selección falla (1), no es error de uso (2).
		if code := io.BenchTierPlanCmd([]string{"--tier", "L1", "--changed", "docs/only.md"}); code != 1 {
			t.Fatalf("selección vacía exit = %d, want 1 (stderr=%q)", code, errb.String())
		}
		if !strings.Contains(errb.String(), "selection failed") {
			t.Fatalf("fallo de selección debe decirlo, got %q", errb.String())
		}
	})

	t.Run("save fail", func(t *testing.T) {
		dir := t.TempDir()
		repoSplits(t)
		io, _, errb := cmdIO(dir)
		bad := filepath.Join(dir, "nope", "plan.json")
		if code := io.BenchTierPlanCmd([]string{"--tier", "L0", "--save", bad}); code != 1 {
			t.Fatalf("save imposible exit = %d, want 1 (stderr=%q)", code, errb.String())
		}
		if !strings.Contains(errb.String(), "bench tier-plan:") {
			t.Fatalf("save imposible debe citar bench tier-plan, got %q", errb.String())
		}
	})
}

func TestBenchTierPlanSaveEvidenceJSONText(t *testing.T) {
	t.Run("save ok texto", func(t *testing.T) {
		dir := t.TempDir()
		repoSplits(t)
		io, out, _ := cmdIO(dir)
		saved := filepath.Join(dir, "tier-plan.json")
		if code := io.BenchTierPlanCmd([]string{"--tier", "L0", "--save", saved}); code != 0 {
			t.Fatalf("save exit = %d", code)
		}
		raw, err := os.ReadFile(saved)
		if err != nil {
			t.Fatalf("save debe persistir: %v", err)
		}
		var sel bench.TierSelection
		if err := json.Unmarshal(raw, &sel); err != nil || sel.Tier != bench.TierL0 {
			t.Fatalf("save debe guardar selección L0: %v %q", err, string(raw))
		}
		if !strings.Contains(out.String(), "selection saved to") {
			t.Fatalf("texto debe confirmar el save, got %q", out.String())
		}
		if !strings.Contains(out.String(), "tier L0") {
			t.Fatalf("texto debe rendir el plan, got %q", out.String())
		}
	})

	t.Run("save ok json sin confirmación", func(t *testing.T) {
		dir := t.TempDir()
		repoSplits(t)
		io, out, _ := cmdIO(dir)
		saved := filepath.Join(dir, "tier-plan.json")
		if code := io.BenchTierPlanCmd([]string{"--tier", "L0", "--save", saved, "--json"}); code != 0 {
			t.Fatalf("save+json exit = %d", code)
		}
		var sel bench.TierSelection
		if err := json.Unmarshal(out.Bytes(), &sel); err != nil || sel.Tier != bench.TierL0 {
			t.Fatalf("json debe decodificar L0: %v %q", err, out.String())
		}
		if strings.Contains(out.String(), "selection saved to") {
			t.Fatalf("json no debe confirmar en texto, got %q", out.String())
		}
	})

	t.Run("evidence ok texto", func(t *testing.T) {
		dir := t.TempDir()
		repoSplits(t)
		io, out, _ := cmdIO(dir)
		ev := filepath.Join(dir, "ev")
		if code := io.BenchTierPlanCmd([]string{"--tier", "L0", "--evidence-dir", ev}); code != 0 {
			t.Fatalf("evidence exit = %d", code)
		}
		if _, err := os.Stat(filepath.Join(ev, bench.TierPlanFile)); err != nil {
			t.Fatalf("evidence debe guardar el plan: %v", err)
		}
		if _, err := os.Stat(filepath.Join(ev, bench.TierPlanManifest)); err != nil {
			t.Fatalf("evidence debe guardar el manifiesto: %v", err)
		}
		if !strings.Contains(out.String(), "evidence recorded in") {
			t.Fatalf("texto debe confirmar evidence, got %q", out.String())
		}
	})

	t.Run("evidence ok json", func(t *testing.T) {
		dir := t.TempDir()
		repoSplits(t)
		io, out, _ := cmdIO(dir)
		ev := filepath.Join(dir, "ev-json")
		if code := io.BenchTierPlanCmd([]string{"--tier", "L0", "--evidence-dir", ev, "--json"}); code != 0 {
			t.Fatalf("evidence+json exit = %d", code)
		}
		var sel bench.TierSelection
		if err := json.Unmarshal(out.Bytes(), &sel); err != nil || sel.Tier != bench.TierL0 {
			t.Fatalf("evidence+json debe decodificar L0: %v %q", err, out.String())
		}
		if _, err := os.Stat(filepath.Join(ev, bench.TierPlanManifest)); err != nil {
			t.Fatalf("evidence+json debe persistir: %v", err)
		}
	})

	t.Run("json vs texto", func(t *testing.T) {
		dir := t.TempDir()
		repoSplits(t)
		io, out, _ := cmdIO(dir)
		if code := io.BenchTierPlanCmd([]string{"--tier", "L0", "--json"}); code != 0 {
			t.Fatalf("json exit = %d", code)
		}
		var sel bench.TierSelection
		if err := json.Unmarshal(out.Bytes(), &sel); err != nil {
			t.Fatalf("json debe decodificar: %v %q", err, out.String())
		}
		out.Reset()
		if code := io.BenchTierPlanCmd([]string{"--tier", "L0"}); code != 0 {
			t.Fatalf("texto exit = %d", code)
		}
		for _, want := range []string{"tier L0", "reason:", "executed"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("texto debe contener %q, got %q", want, out.String())
			}
		}
	})
}
