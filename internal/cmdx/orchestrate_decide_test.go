// Decide split cover (CRAP paydown sin cambiar comportamiento):
// 8 casos parse/rates/history/unavailable/Decide/dryRun/json/Primary,
// con IO{Dir:tmp}, portables.
package cmdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/orchestrate"
)

func decideBaseArgs() []string {
	return []string{"--domain", "harness", "--lang", "go", "--complexity", "low", "--risk", "low", "--dry-run"}
}

func TestApplyUnavailableTable(t *testing.T) {
	mk := func() []orchestrate.RoutedAgent {
		return []orchestrate.RoutedAgent{
			{ID: "a", Available: true},
			{ID: "b", Available: true},
		}
	}
	t.Run("vacio", func(t *testing.T) {
		reg := applyUnavailable(mk(), "")
		if !reg[0].Available || !reg[1].Available {
			t.Fatalf("empty input must keep availability, got %+v", reg)
		}
	})
	t.Run("marca", func(t *testing.T) {
		reg := applyUnavailable(mk(), "a")
		if reg[0].Available || !reg[1].Available {
			t.Fatalf("must mark only a unavailable, got %+v", reg)
		}
	})
	t.Run("desconocido", func(t *testing.T) {
		reg := applyUnavailable(mk(), "ghost")
		if !reg[0].Available || !reg[1].Available {
			t.Fatalf("unknown ids must be ignored, got %+v", reg)
		}
	})
}

func TestBuildTaskDescriptorTable(t *testing.T) {
	opts := orchestrateDecideOptions{
		domain: "harness", lang: "go", complexity: "low", risk: "low",
		caps: "c1, c2", constraints: "k", outputs: "o", criteria: "cr",
	}
	desc := buildTaskDescriptor(opts)
	if desc.Domain != "harness" || desc.Language != "go" {
		t.Fatalf("must carry domain/lang, got %+v", desc)
	}
	if len(desc.RequiredCapabilities) != 2 || desc.RequiredCapabilities[0] != "c1" {
		t.Fatalf("must split caps, got %+v", desc.RequiredCapabilities)
	}
	if len(desc.Constraints) != 1 || len(desc.Outputs) != 1 || len(desc.Criteria) != 1 {
		t.Fatalf("must split lists, got %+v", desc)
	}
}

func TestRunDecisionTable(t *testing.T) {
	manifests := bench.DefaultHostManifests(bench.EvidenceStamp())
	reg := orchestrate.RegistryFromManifests(manifests)
	desc := orchestrate.TaskDescriptor{Domain: "harness", Language: "go", Complexity: "low", Risk: "low"}
	t.Run("ok", func(t *testing.T) {
		var errb strings.Builder
		opts := orchestrateDecideOptions{maxDelegations: 1, fixed: "baseline"}
		dec, code, ok := runDecision(reg, manifests, nil, desc, opts, &errb)
		if !ok || code != 0 || dec.Primary == "" {
			t.Fatalf("valid verdict must pick primary, got %+v %d %v stderr=%q", dec, code, ok, errb.String())
		}
	})
	t.Run("Decide-err", func(t *testing.T) {
		var errb strings.Builder
		bad := orchestrate.TaskDescriptor{Domain: "h", Language: "go", Complexity: "cosmic", Risk: "low"}
		opts := orchestrateDecideOptions{maxDelegations: 1, fixed: "baseline"}
		_, code, ok := runDecision(reg, manifests, nil, bad, opts, &errb)
		if ok || code != 2 {
			t.Fatalf("exit = %d ok = %v, want 2 false", code, ok)
		}
		if !strings.Contains(errb.String(), "orchestrate decide:") {
			t.Fatalf("must name the command, got %q", errb.String())
		}
	})
}

func TestEmitDecisionTable(t *testing.T) {
	manifests := bench.DefaultHostManifests(bench.EvidenceStamp())
	reg := orchestrate.RegistryFromManifests(manifests)
	desc := orchestrate.TaskDescriptor{Domain: "harness", Language: "go", Complexity: "low", Risk: "low"}
	opts := orchestrateDecideOptions{maxDelegations: 1, fixed: "baseline"}
	dec, _, _ := runDecision(reg, manifests, nil, desc, opts, &strings.Builder{})
	t.Run("dryRun texto", func(t *testing.T) {
		io, out, _ := cmdIO(t.TempDir())
		dopts := opts
		dopts.dryRun = true
		if code := io.emitDecision(dec, dopts); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "routing ") {
			t.Fatalf("must render routing, got %q", out.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		io, out, _ := cmdIO(t.TempDir())
		dopts := opts
		dopts.dryRun = true
		dopts.asJSON = true
		if code := io.emitDecision(dec, dopts); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		var raw struct {
			Primary string `json:"primary"`
		}
		if err := json.Unmarshal(out.Bytes(), &raw); err != nil || raw.Primary == "" {
			t.Fatalf("json must name primary: %v %q", err, out.String())
		}
	})
	t.Run("out registra", func(t *testing.T) {
		dir := t.TempDir()
		io, out, _ := cmdIO(dir)
		dopts := opts
		outDir := filepath.Join(dir, "out")
		dopts.out = outDir
		if code := io.emitDecision(dec, dopts); code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if _, err := os.Stat(filepath.Join(outDir, "decision.json")); err != nil {
			t.Fatalf("must persist decision.json: %v", err)
		}
		if !strings.Contains(out.String(), "decision recorded in") {
			t.Fatalf("must confirm the record, got %q", out.String())
		}
	})
	t.Run("Primary vacio", func(t *testing.T) {
		io, _, errb := cmdIO(t.TempDir())
		empty := dec
		empty.Primary = ""
		dopts := opts
		dopts.dryRun = true
		if code := io.emitDecision(empty, dopts); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "sin agente elegible") {
			t.Fatalf("must name the refusal, got %q", errb.String())
		}
	})
}

func TestOrchestrateDecideCmdTable(t *testing.T) {
	writeJSON := func(t *testing.T, name, body string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	t.Run("parse", func(t *testing.T) {
		io, _, _ := cmdIO(t.TempDir())
		if code := io.OrchestrateDecideCmd(nil); code != 2 {
			t.Fatalf("missing flags exit = %d, want 2", code)
		}
	})
	t.Run("rates", func(t *testing.T) {
		io, _, errb := cmdIO(t.TempDir())
		args := append(decideBaseArgs(), "--rates", filepath.Join(t.TempDir(), "missing.json"))
		if code := io.OrchestrateDecideCmd(args); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "orchestrate:") {
			t.Fatalf("must name orchestrate, got %q", errb.String())
		}
	})
	t.Run("history", func(t *testing.T) {
		io, _, errb := cmdIO(t.TempDir())
		args := append(decideBaseArgs(), "--history", filepath.Join(t.TempDir(), "missing.json"))
		if code := io.OrchestrateDecideCmd(args); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "orchestrate:") {
			t.Fatalf("must name orchestrate, got %q", errb.String())
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		io, out, _ := cmdIO(t.TempDir())
		args := append(decideBaseArgs(), "--unavailable", "ghost-agent")
		if code := io.OrchestrateDecideCmd(args); code != 0 {
			t.Fatalf("exit = %d, want 0", code)
		}
		if !strings.Contains(out.String(), "routing ") {
			t.Fatalf("must still route, got %q", out.String())
		}
	})
	t.Run("Decide", func(t *testing.T) {
		io, _, errb := cmdIO(t.TempDir())
		args := []string{"--domain", "h", "--lang", "go", "--complexity", "cosmic", "--risk", "low"}
		if code := io.OrchestrateDecideCmd(args); code != 2 {
			t.Fatalf("bad complexity exit = %d, want 2", code)
		}
		if !strings.Contains(errb.String(), "orchestrate decide:") {
			t.Fatalf("must name the command, got %q", errb.String())
		}
	})
	t.Run("dryRun", func(t *testing.T) {
		dir := t.TempDir()
		io, out, _ := cmdIO(dir)
		if code := io.OrchestrateDecideCmd(decideBaseArgs()); code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(out.String(), "routing ") {
			t.Fatalf("must render routing, got %q", out.String())
		}
		if _, err := os.Stat(filepath.Join(dir, ".regulus")); err == nil {
			t.Fatalf("dry-run must not record a trace")
		}
	})
	t.Run("json", func(t *testing.T) {
		io, out, _ := cmdIO(t.TempDir())
		rates := writeJSON(t, "rates.json", "[]")
		hist := writeJSON(t, "hist.json", "[]")
		args := append(decideBaseArgs(), "--json", "--rates", rates, "--history", hist)
		if code := io.OrchestrateDecideCmd(args); code != 0 {
			t.Fatalf("exit = %d", code)
		}
		var dec struct {
			Primary string `json:"primary"`
		}
		if err := json.Unmarshal(out.Bytes(), &dec); err != nil || dec.Primary == "" {
			t.Fatalf("json must name primary: %v %q", err, out.String())
		}
	})
	t.Run("Primary", func(t *testing.T) {
		io, _, errb := cmdIO(t.TempDir())
		manifests := bench.DefaultHostManifests(bench.EvidenceStamp())
		reg := orchestrate.RegistryFromManifests(manifests)
		ids := make([]string, 0, len(reg))
		for _, a := range reg {
			ids = append(ids, a.ID)
		}
		args := []string{"--domain", "harness", "--lang", "go", "--complexity", "low", "--risk", "low", "--dry-run", "--unavailable", strings.Join(ids, ",")}
		if code := io.OrchestrateDecideCmd(args); code != 1 {
			t.Fatalf("all unavailable exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "sin agente elegible") {
			t.Fatalf("must name the refusal, got %q", errb.String())
		}
	})
}
