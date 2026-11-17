// Engine tests (F13.4/NOV-107): plan by diff+impact+contracts+risk,
// risk→gates matrix, the six verdicts (BLOCKED/SKIPPED/UNKNOWN never
// PASS), toolchain guard, partial-suite contrast, contract changes,
// F12.4 regression linkage, tamper-closed evidence and reproducible
// rerun. The validation corpus (testdata/validation-corpus.json)
// drives the contract: editing a scenario means updating the test.
package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/impact"
	"regulus-ai/internal/quality"
)

// fixtureRepo builds the same small shape impact tests use: a/b
// packages, a critical gates package. Fake lookup keeps commits
// deterministic.
func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/demo\n\ngo 1.27\n")
	write("internal/a/a.go", "package a\n\n// Hello is exported.\nfunc Hello() string { return \"hi\" }\n")
	write("internal/b/b.go", "package b\n\nimport \"example.com/demo/internal/a\"\n\nfunc Shout() string { return a.Hello() }\n")
	write("internal/b/b_test.go", "package b\n\nimport \"testing\"\n\nfunc TestShout(t *testing.T) {}\n")
	write("internal/gates/g.go", "package gates\n\nfunc Check() bool { return true }\n")
	write("internal/gates/g_test.go", "package gates\n\nimport \"testing\"\n\nfunc TestCheck(t *testing.T) {}\n")
	return dir
}

func fakeLookup(rel string) (string, string, error) {
	return "abc123def456", "2026-01-02T03:04:05Z", nil
}

func mustGraph(t *testing.T, dir string) impact.Graph {
	t.Helper()
	g, err := impact.BuildGraph(dir, nil, fakeLookup)
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	return g
}

func testProv(run string) Provenance {
	return Provenance{
		Commit: "abc123def456", Toolchain: "go1.27",
		Host: "ci-1", Agent: "atlas", Version: "test", RunID: run,
	}
}

func mustPlan(t *testing.T, g impact.Graph, changed []string, opts PlanOptions) Plan {
	t.Helper()
	p, err := BuildPlan(g, changed, opts, "abc123def456")
	if err != nil {
		t.Fatalf("BuildPlan(%v): %v", changed, err)
	}
	return p
}

// passExec executes everything PASS (recorded evidence present).
func passExec() Executor {
	return func(spec GateSpec) GateResult {
		return GateResult{Kind: spec.Kind, Verdict: VerdictPass, Evidence: "test: recorded pass"}
	}
}

// injectExec fails/skips/blocks listed gates with the given cause.
func injectExec(table map[string]GateResult) Executor {
	return func(spec GateSpec) GateResult {
		if r, ok := table[spec.Kind]; ok {
			r.Kind = spec.Kind
			return r
		}
		return GateResult{Kind: spec.Kind, Verdict: VerdictPass, Evidence: "test: recorded pass"}
	}
}

func TestPlanFromDiffMultiPackage(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/a/a.go", "internal/b/b.go"}, PlanOptions{Provenance: testProv("run-1")})
	if len(p.Impact.Direct) < 2 {
		t.Fatalf("multi-paquete: directos = %v, want >= 2", p.Impact.Direct)
	}
	if p.DiffHash == "" || p.ConfigID == "" {
		t.Fatalf("plan sin trazabilidad: diff %q config %q", p.DiffHash, p.ConfigID)
	}
	if len(p.Gates) == 0 || p.Tier == "" || p.TierBudgetS <= 0 {
		t.Fatalf("plan incompleto: gates %d tier %q budget %d", len(p.Gates), p.Tier, p.TierBudgetS)
	}
	if len(p.Criteria) != len(DefaultCriteria) {
		t.Fatalf("criterios por defecto = %d, want %d", len(p.Criteria), len(DefaultCriteria))
	}
}

func TestInjectedFailureBlocks(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/b/b.go"}, PlanOptions{Provenance: testProv("run-fail")})
	r, err := RunPlan(p, injectExec(map[string]GateResult{
		GateUnit: {Verdict: VerdictFail, Cause: "panic en TestShout", Evidence: "inyección --fail"},
	}), nil, time.Now())
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if r.Overall != VerdictFail || r.Decision != DecisionBlock || r.AllowsIntegration() {
		t.Fatalf("fallo inyectado: overall=%s decision=%s (want FAIL/block)", r.Overall, r.Decision)
	}
	if len(r.FailedGates) != 1 || r.FailedGates[0] != GateUnit {
		t.Fatalf("failed gates = %v, want [unit]", r.FailedGates)
	}
}

func TestNonPassVerdictsNeverPass(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	for _, v := range []string{VerdictBlocked, VerdictSkipped, VerdictUnknown} {
		p := mustPlan(t, g, []string{"internal/b/b.go"}, PlanOptions{Provenance: testProv("run-" + v)})
		r, err := RunPlan(p, injectExec(map[string]GateResult{
			GateUnit: {Verdict: v, Cause: "causa " + v},
		}), nil, time.Now())
		if err != nil {
			t.Fatalf("RunPlan(%s): %v", v, err)
		}
		if IsPass(r.Overall) || r.Decision != DecisionBlock || r.AllowsIntegration() {
			t.Fatalf("%s interpretado como PASS: overall=%s decision=%s", v, r.Overall, r.Decision)
		}
		if r.Overall != v {
			t.Fatalf("%s: overall = %s, want %s (el peor requerido manda)", v, r.Overall, v)
		}
	}
}

func TestNotApplicableNeverBlocks(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/b/b.go"}, PlanOptions{Provenance: testProv("run-na")})
	r, err := RunPlan(p, injectExec(map[string]GateResult{
		GateIntegration: {Verdict: VerdictNotApplicable, Cause: "sin tests de integración en el foco"},
	}), nil, time.Now())
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if r.Decision != DecisionAllow || !r.AllowsIntegration() {
		t.Fatalf("NOT_APPLICABLE con causa bloqueó: %+v", r)
	}
}

func TestHostWithoutToolchainBlocks(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/b/b.go"}, PlanOptions{Provenance: testProv("run-notc")})
	// El edge convierte la ausencia de toolchain en BLOCKED por gate.
	r, err := RunPlan(p, func(spec GateSpec) GateResult {
		return GateResult{Kind: spec.Kind, Verdict: VerdictBlocked, Cause: "host sin toolchain: go no disponible (multi-host seguro: BLOCKED, nunca PASS)"}
	}, nil, time.Now())
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if r.Overall != VerdictBlocked || r.Decision != DecisionBlock {
		t.Fatalf("host sin toolchain: overall=%s decision=%s (want BLOCKED/block)", r.Overall, r.Decision)
	}
}

func TestOmittedAndPartialSuiteContrast(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/b/b.go"}, PlanOptions{Provenance: testProv("run-partial")})
	if len(p.Suite.Full) == 0 || len(p.Suite.Focused) == 0 {
		t.Fatalf("suite vacía: foco %v full %v", p.Suite.Focused, p.Suite.Full)
	}
	// El foco de un paquete sin consumidores cubre su bundle y omite el resto.
	if len(p.Suite.Omitted) == 0 {
		t.Fatalf("suite parcial: omitidos = vacíos, want al menos test:internal/gates")
	}
	found := false
	for _, o := range p.Suite.Omitted {
		if o == "test:internal/gates" {
			found = true
		}
	}
	if !found {
		t.Fatalf("omitidos = %v, want test:internal/gates dentro", p.Suite.Omitted)
	}
	if p.Suite.Metrics.Predicted != len(p.Suite.Focused) || p.Suite.Metrics.Actual != len(p.Suite.Full) {
		t.Fatalf("contraste roto: %+v", p.Suite.Metrics)
	}
	r, err := RunPlan(p, passExec(), nil, time.Now())
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if r.Precision != p.Suite.Metrics.Precision || r.Recall != p.Suite.Metrics.Recall {
		t.Fatalf("el resultado debe citar el contraste del plan: P=%.2f/%.2f R=%.2f/%.2f",
			r.Precision, p.Suite.Metrics.Precision, r.Recall, p.Suite.Metrics.Recall)
	}
}

func TestWidenOnUnknown(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"zzz/desconocido.go"}, PlanOptions{Provenance: testProv("run-widen")})
	if !p.Widened || !p.Impact.Widened {
		t.Fatalf("entrada desconocida no amplió: widened=%v impact=%v", p.Widened, p.Impact.Widened)
	}
	if len(p.Suite.Focused) != len(p.Suite.Full) {
		t.Fatalf("ampliada debe correr la completa: foco %d full %d", len(p.Suite.Focused), len(p.Suite.Full))
	}
	if p.RiskLevel == RiskLow {
		t.Fatalf("la incertidumbre suma, nunca resta: riesgo = %s", p.RiskLevel)
	}
}

func TestContractChangeEscalates(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/b/b.go"},
		PlanOptions{Contracts: []string{"api:demo/Shout"}, Provenance: testProv("run-contracts")})
	if p.RiskLevel != RiskHigh {
		t.Fatalf("contratos tocados: riesgo = %s, want high", p.RiskLevel)
	}
	for _, want := range []string{GateE2E, GateSecurity} {
		found := false
		for _, gs := range p.Gates {
			if gs.Kind == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("contratos: gate %s no exigido (%v)", want, p.Gates)
		}
	}
	b := mustPlan(t, g, []string{"internal/b/b.go"},
		PlanOptions{Contracts: []string{"breaking:api:demo/Shout"}, Provenance: testProv("run-breaking")})
	if b.RiskLevel != RiskCritical {
		t.Fatalf("breaking: riesgo = %s, want critical", b.RiskLevel)
	}
	found := false
	for _, gs := range b.Gates {
		if gs.Kind == GateMutation {
			found = true
		}
	}
	if !found {
		t.Fatalf("breaking sin mutation/fuzz: %v", b.Gates)
	}
	if b.Tier != "L2" {
		t.Fatalf("breaking: tier = %s, want L2 (nunca L3 por foco)", b.Tier)
	}
}

func TestCriticalNeverOmitted(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/gates/g.go"}, PlanOptions{Provenance: testProv("run-crit")})
	if p.RiskLevel != RiskHigh {
		t.Fatalf("crítico: riesgo = %s, want high mínimo", p.RiskLevel)
	}
	found := false
	for _, f := range p.Suite.Focused {
		if f == "test:internal/gates" {
			found = true
		}
	}
	if !found {
		t.Fatalf("crítico omitido del foco: %v", p.Suite.Focused)
	}
}

func TestWeakOverrideRefused(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	_, err := BuildPlan(g, []string{"internal/gates/g.go"}, PlanOptions{
		TierOverride: "L0", Provenance: testProv("run-weak"),
	}, "abc123def456")
	if err == nil || !strings.Contains(err.Error(), "foco débil") {
		t.Fatalf("override débil con críticos debe refutarse, got: %v", err)
	}
	if _, err := BuildPlan(g, []string{"zzz/x.go"}, PlanOptions{}, "h"); err == nil {
		t.Fatalf("BuildPlan sin run id debe refutarse")
	}
	if _, err := BuildPlan(g, []string{"internal/b/b.go"}, PlanOptions{
		TierOverride: "L3", Provenance: testProv("r"),
	}, "h"); err == nil {
		t.Fatalf("override L3 debe refutarse (holdout sellado)")
	}
}

// obs builders: good window (baseline) vs bad window (regression).
func regObs(n int, ok bool, prefix string, at time.Time) []quality.Observation {
	out := make([]quality.Observation, 0, n)
	for i := 0; i < n; i++ {
		o := quality.Observation{
			Agent: "atlas", Model: "m1", Host: "ci-1", Version: "v3",
			Success: true, Retries: 0, LatencyMS: 100, LatencyKnown: true,
			TokensIn: 400, TokensOut: 600, TokensKnown: true,
			CoveragePct: 82, CoverageKnown: true,
			Source: quality.SourceLab, RunID: fmt.Sprintf("%s-%02d", prefix, i), At: at.Add(time.Duration(i) * time.Minute),
		}
		if !ok {
			o.Success = i%2 == 0
			o.Retries = 3
			o.LatencyMS = 400
			o.Errors = 1
		}
		out = append(out, o)
	}
	return out
}

func TestRegressionDetectedAndLinked(t *testing.T) {
	seg := quality.Segment{Agent: "atlas", Model: "m1", Host: "ci-1", Version: "v3"}
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	base, err := quality.BuildBaseline(seg, regObs(6, true, "base", at), quality.DefaultThresholds(), nil, at)
	if err != nil {
		t.Fatalf("BuildBaseline: %v", err)
	}
	rep := DetectRegression(base, regObs(6, false, "bad", at), "baseline-dir", "window-file", "deadbeef", "run-reg")
	if !rep.Detected || rep.Alerts == 0 {
		t.Fatalf("regresión inyectada no detectada: %+v", rep)
	}
	if rep.BaselineN != 6 || rep.WindowN != 6 {
		t.Fatalf("el reporte muestra baseline y muestra: %+v", rep)
	}
	if len(rep.Cases) == 0 {
		t.Fatalf("el reporte vincula casos (run ids): %+v", rep)
	}
	// Con n < MinN el detector describe y no afirma: limitaciones honestas.
	small := DetectRegression(base, regObs(3, false, "tiny", at), "baseline-dir", "window-file", "deadbeef", "run-reg")
	if small.Detected || !insufficientSample(small.Limitations) {
		t.Fatalf("n=3 debe ser inconcluyente con límite de muestra: %+v", small)
	}
	// Vinculada al cambio: bloquea la integración del diff responsable.
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/b/b.go"}, PlanOptions{Provenance: testProv("run-reg")})
	r, err := RunPlan(p, passExec(), &rep, time.Now())
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if r.Decision != DecisionBlock || r.Overall != VerdictFail {
		t.Fatalf("regresión vinculada debe bloquear: %+v", r)
	}
	if !strings.Contains(r.BlockReason, "deadbe") {
		t.Fatalf("el bloqueo cita el diff responsable: %q", r.BlockReason)
	}
}

func TestEvidenceTamperFailsClosed(t *testing.T) {
	dir := t.TempDir()
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/b/b.go"}, PlanOptions{Provenance: testProv("run-ev")})
	now := time.Now()
	planDir := filepath.Join(dir, "plan")
	if _, err := WritePlanEvidence(planDir, p, "abc123", now); err != nil {
		t.Fatalf("WritePlanEvidence: %v", err)
	}
	if _, _, err := ReadPlanEvidence(planDir); err != nil {
		t.Fatalf("ReadPlanEvidence: %v", err)
	}
	// Tamper del plan: falla cerrado.
	raw, _ := os.ReadFile(filepath.Join(planDir, PlanFile))
	tampered := strings.Replace(string(raw), p.DiffHash, "ffffffff", 1)
	if err := os.WriteFile(filepath.Join(planDir, PlanFile), []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadPlanEvidence(planDir); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("plan tampered debe fallar cerrado, got: %v", err)
	}
	// Tamper del resultado: falla cerrado.
	r, err := RunPlan(p, passExec(), nil, now)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	resDir := filepath.Join(dir, "res")
	if _, err := WriteResultEvidence(resDir, p, r, "abc123", now); err != nil {
		t.Fatalf("WriteResultEvidence: %v", err)
	}
	rraw, _ := os.ReadFile(filepath.Join(resDir, ResultFile))
	tamperedR := strings.Replace(string(rraw), `"allow"`, `"block"`, 1)
	if err := os.WriteFile(filepath.Join(resDir, ResultFile), []byte(tamperedR), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ReadResultEvidence(resDir); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("result tampered debe fallar cerrado, got: %v", err)
	}
}

func TestRerunReproducibleWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/b/b.go"}, PlanOptions{Provenance: testProv("run-a")})
	now := time.Now()
	r1, err := RunPlan(p, passExec(), nil, now)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	d1 := filepath.Join(dir, "run1")
	if _, err := WriteResultEvidence(d1, p, r1, "abc123", now); err != nil {
		t.Fatalf("WriteResultEvidence: %v", err)
	}
	// Rerun con la misma configuración: mismo veredicto, dir intacto.
	_, pBack, rBack, err := ReadResultEvidence(d1)
	if err != nil {
		t.Fatalf("ReadResultEvidence: %v", err)
	}
	if pBack.ConfigID != p.ConfigID {
		t.Fatalf("el rerun relee el mismo plan: %s vs %s", pBack.ConfigID, p.ConfigID)
	}
	// Nuevo run id, misma configuración: el resultado coincide.
	p2 := pBack
	p2.Provenance = testProv("run-b")
	p2.ConfigID = configID(p2)
	if p2.ConfigID != p.ConfigID {
		t.Fatalf("misma configuración, distinto run id: config debe coincidir (%s vs %s)", p2.ConfigID, p.ConfigID)
	}
	r2, err := RunPlan(p2, passExec(), nil, now)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	d2 := filepath.Join(dir, "run2")
	if _, err := WriteResultEvidence(d2, p2, r2, "abc123", now); err != nil {
		t.Fatalf("WriteResultEvidence: %v", err)
	}
	if _, _, _, err := ReadResultEvidence(d1); err != nil {
		t.Fatalf("el rerun no sobrescribe evidencias anteriores: %v", err)
	}
	got := Comparison(ResultManifest{}, rBack, ResultManifest{}, r2)
	if !strings.Contains(got, "mismo plan") || !strings.Contains(got, "overall: PASS -> PASS") {
		t.Fatalf("comparación del rerun:\n%s", got)
	}
	// Rerun con fallo distinto: el diff lo muestra sin tocar run1.
	r3, err := RunPlan(p2, injectExec(map[string]GateResult{
		GateUnit: {Verdict: VerdictFail, Cause: "inyectado en rerun"},
	}), nil, now)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	d3 := filepath.Join(dir, "run3")
	if _, err := WriteResultEvidence(d3, p2, r3, "abc123", now); err != nil {
		t.Fatalf("WriteResultEvidence: %v", err)
	}
	got3 := Comparison(ResultManifest{}, r2, ResultManifest{}, r3)
	if !strings.Contains(got3, "≠ unit") || !strings.Contains(got3, "allow -> block") {
		t.Fatalf("el rerun con fallo difiere:\n%s", got3)
	}
}

// TestValidationCorpus ejecuta el corpus de validación: cada escenario
// fija cambio/contratos/severidad/inyección y exige riesgo, gates y
// decisión exactos sobre el fixture grafo.
func TestValidationCorpus(t *testing.T) {
	raw, err := os.ReadFile(corpusPath(t))
	if err != nil {
		t.Fatalf("corpus: %v", err)
	}
	var corpus struct {
		Version   string `json:"version"`
		Scenarios []struct {
			Name           string            `json:"name"`
			Changed        []string          `json:"changed"`
			Contracts      []string          `json:"contracts"`
			Severity       float64           `json:"severity"`
			Inject         map[string]string `json:"inject"`
			ExpectRisk     string            `json:"expect_risk"`
			ExpectGates    []string          `json:"expect_gates"`
			ExpectWidened  bool              `json:"expect_widened"`
			ExpectDecision string            `json:"expect_decision"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("corpus malformado: %v", err)
	}
	if corpus.Version != VerifyVersion {
		t.Fatalf("corpus version = %q, want %q", corpus.Version, VerifyVersion)
	}
	g := mustGraph(t, fixtureRepo(t))
	now := time.Now()
	for _, sc := range corpus.Scenarios {
		t.Run(sc.Name, func(t *testing.T) {
			p := mustPlan(t, g, sc.Changed, PlanOptions{
				Contracts: sc.Contracts, Severity: sc.Severity, Provenance: testProv("run-" + sc.Name),
			})
			if sc.ExpectRisk != "" && p.RiskLevel != sc.ExpectRisk {
				t.Fatalf("riesgo = %s, want %s", p.RiskLevel, sc.ExpectRisk)
			}
			if sc.ExpectWidened && !p.Widened {
				t.Fatalf("escenario exige suite ampliada")
			}
			if len(sc.ExpectGates) > 0 {
				var kinds []string
				for _, gs := range p.Gates {
					kinds = append(kinds, gs.Kind)
				}
				if strings.Join(kinds, ",") != strings.Join(sc.ExpectGates, ",") {
					t.Fatalf("gates = %v, want %v", kinds, sc.ExpectGates)
				}
			}
			table := map[string]GateResult{}
			for kind, spec := range sc.Inject {
				v, cause, _ := strings.Cut(spec, ":")
				vv, err := ParseVerdict(v)
				if err != nil {
					t.Fatalf("inyección %q: %v", spec, err)
				}
				table[strings.ToLower(kind)] = GateResult{Verdict: vv, Cause: cause, Evidence: "corpus " + sc.Name}
			}
			r, err := RunPlan(p, injectExec(table), nil, now)
			if err != nil {
				t.Fatalf("RunPlan: %v", err)
			}
			if r.Decision != sc.ExpectDecision {
				t.Fatalf("decisión = %s, want %s (overall %s)", r.Decision, sc.ExpectDecision, r.Overall)
			}
		})
	}
}

// corpusPath resuelve el corpus junto al test, no junto al cwd: los
// binarios de test corren desde el sidecar (.regulus-test) y el cwd
// no es el paquete.
func corpusPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("sin caller")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "validation-corpus.json")
}

func TestMatrixCoversAllGates(t *testing.T) {
	if len(AllGates()) != len(AllGateKinds) {
		t.Fatalf("matriz incompleta: %d filas, %d kinds", len(AllGates()), len(AllGateKinds))
	}
	for _, r := range []string{RiskLow, RiskMedium, RiskHigh, RiskCritical} {
		specs, required, _ := RequiredGates(r, false, false, nil)
		if len(specs) == 0 || len(required) == 0 {
			t.Fatalf("nivel %s sin gates", r)
		}
	}
	// La matriz solo añade al subir el riesgo.
	prev := -1
	for _, r := range []string{RiskLow, RiskMedium, RiskHigh, RiskCritical} {
		specs, _, _ := RequiredGates(r, false, false, nil)
		if len(specs) <= prev {
			t.Fatalf("nivel %s no añade gates (%d <= %d)", r, len(specs), prev)
		}
		prev = len(specs)
	}
}

func TestRenderReportBindsAll(t *testing.T) {
	g := mustGraph(t, fixtureRepo(t))
	p := mustPlan(t, g, []string{"internal/b/b.go"}, PlanOptions{Provenance: testProv("run-rep")})
	r, err := RunPlan(p, passExec(), nil, time.Now())
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	out := RenderReport(p, r)
	for _, want := range []string{
		"verify plan (task", shortSHA(p.DiffHash), shortSHA(p.ConfigID),
		"riesgo:", "suite:", "contraste:", "precisión", "gates (",
		"verify result", "PASS / allow", "reproduce: regulus verify rerun",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("el reporte cita %q:\n%s", want, out)
		}
	}
	if got := MatrixDoc(); !strings.Contains(got, "BLOCKED/SKIPPED/UNKNOWN") {
		t.Fatalf("la matriz documenta el nunca-PASS:\n%s", got)
	}
}
