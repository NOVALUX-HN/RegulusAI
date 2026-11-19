// Verify commands (F13.4/NOV-107): `regulus verify ...` over the
// continuous-verification engine (internal/verify).
//
// Handlers take the minimal IO instead of cli.CLI so cli keeps one
// edge (cli->cmdx); behavior, flags, messages and exit codes are
// defined here. Outcomes are recorded evidence (flags, results
// file), never invented passes: an unrecorded required gate stays
// UNKNOWN and blocks. Every command links its F11.1 exec trace
// (best-effort, reusing the impact tracer in this package).
package cmdx

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"regulus-ai/internal/impact"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/pxexec"
	"regulus-ai/internal/quality"
	"regulus-ai/internal/verify"
)

func verifyUsage() string {
	return "usage: regulus verify plan|run|report|matrix|rerun\n" +
		"  regulus verify plan --changed <a,b> [--task <t>] [--mission <m>] [--severity N] [--contracts <c,...>] [--criteria <c,...>] [--tier L0|L1|L2] [--out <dir>] [--json]\n" +
		"  regulus verify run --changed <a,b> [--task <t>] [--mission <m>] [--severity N] [--contracts <c,...>] [--fail <gate:causa,...>] [--skip ...] [--block ...] [--na ...] [--results <file>] [--no-toolchain] [--baseline <dir> --window <file>] [--out <dir>] [--json]\n" +
		"  regulus verify report --dir <dir> [--json]\n" +
		"  regulus verify matrix [--json]\n" +
		"  regulus verify rerun --dir <dir> --out <newdir> [--fail ...] [--skip ...] [--block ...] [--na ...] [--results <file>] [--no-toolchain] [--json]"
}

// VerifyCmd routes `regulus verify ...` to one handler per subcommand.
func (io IO) VerifyCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, verifyUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, verifyUsage())
		return 2
	}
	switch args[0] {
	case "plan":
		return io.VerifyPlanCmd(args[1:])
	case "run":
		return io.VerifyRunCmd(args[1:])
	case "report":
		return io.VerifyReportCmd(args[1:])
	case "matrix":
		return io.VerifyMatrixCmd(args[1:])
	case "rerun":
		return io.VerifyRerunCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "verify: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, verifyUsage())
		return 2
	}
}

// verifyPlanFlags carries the shared plan-derivation flags.
type verifyPlanFlags struct {
	changed   string
	task      string
	mission   string
	severity  float64
	contracts string
	criteria  string
	tier      string
	out       string
	asJSON    bool
}

func addVerifyPlanFlags(fs *flag.FlagSet, f *verifyPlanFlags) {
	fs.StringVar(&f.changed, "changed", "", "changed paths, comma-separated (required)")
	fs.StringVar(&f.task, "task", "", "task id the plan verifies")
	fs.StringVar(&f.mission, "mission", "", "mission id (sets mission scope)")
	fs.Float64Var(&f.severity, "severity", 0, "observed F12.4 severity 0..1 (alert/failure rate)")
	fs.StringVar(&f.contracts, "contracts", "", "touched contracts, comma-separated (breaking: prefix escalates)")
	fs.StringVar(&f.criteria, "criteria", "", "acceptance criteria, comma-separated (default: spec six)")
	fs.StringVar(&f.tier, "tier", "", "tier override L0|L1|L2 (never below need under uncertainty, never L3)")
	fs.StringVar(&f.out, "out", "", "plan evidence dir (optional)")
	fs.BoolVar(&f.asJSON, "json", false, "print the plan as JSON")
}

// verifyProvenance mints the run provenance at the edge: HEAD,
// toolchain probe, host, agent flag, binary version, fresh run id.
func (io IO) verifyProvenance(agent string, diffHash string) verify.Provenance {
	head := gitHeadBounded(io.Runner)
	if strings.TrimSpace(head) == "" {
		head = "worktree"
	}
	runID, err := observability.MintRunID()
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify: sin run id acuñado (%v): fallback determinista\n", err)
		runID = "vrf-" + diffHash
		if len(runID) > 16 {
			runID = runID[:16]
		}
	}
	return verify.Provenance{
		Commit: head, Toolchain: probeGoToolchain(),
		Host: hostName(), Agent: strings.TrimSpace(agent),
		Version: io.Version, RunID: runID,
	}
}

// probeGoToolchain best-efforts `go version` ("go version go1.27
// linux/amd64" -> "go1.27 linux/amd64"); anything else reads as
// missing, never as ok.
func probeGoToolchain() string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res := pxexec.Run(ctx, pxexec.Spec{Name: "go", Args: []string{"version"}, Timeout: 15 * time.Second})
	if res.Outcome != pxexec.OutcomePass {
		return "missing-go"
	}
	fields := strings.Fields(strings.TrimSpace(res.Stdout))
	if len(fields) >= 4 && fields[0] == "go" && fields[1] == "version" {
		return strings.Join(fields[2:], " ")
	}
	return strings.TrimSpace(res.Stdout)
}

// hostName best-efforts the host id; unknown stays unknown, never empty-guessed.
func hostName() string {
	if h, err := os.Hostname(); err == nil && strings.TrimSpace(h) != "" {
		return strings.TrimSpace(h)
	}
	return "unknown"
}

// VerifyPlanCmd implements `regulus verify plan`.
func (io IO) VerifyPlanCmd(args []string) int {
	var f verifyPlanFlags
	fs := flag.NewFlagSet("verify plan", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	addVerifyPlanFlags(fs, &f)
	agent := fs.String("agent", "", "agent id for provenance")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(f.changed) == "" {
		fmt.Fprintln(io.Stderr, verifyUsage())
		return 2
	}
	if f.severity < 0 || f.severity > 1 {
		fmt.Fprintln(io.Stderr, "verify: --severity must be 0..1 (observed F12.4 rate)")
		return 2
	}
	scope := verify.ScopeTask
	if strings.TrimSpace(f.mission) != "" {
		scope = verify.ScopeMission
	}
	g, err := impact.BuildGraph(io.Dir, nil, nil)
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify plan: %v\n", err)
		return 1
	}
	changed := splitList(f.changed)
	prov := io.verifyProvenance(*agent, verify.DiffHash(changed))
	plan, err := verify.BuildPlan(g, changed, verify.PlanOptions{
		Scope: scope, TaskID: f.task, MissionID: f.mission,
		Severity: f.severity, Contracts: splitList(f.contracts),
		Criteria: splitList(f.criteria), TierOverride: f.tier,
		Provenance: prov,
	}, prov.Commit)
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify plan: %v\n", err)
		return 1
	}
	if strings.TrimSpace(f.out) != "" {
		if _, err := verify.WritePlanEvidence(f.out, plan, prov.Commit, time.Now()); err != nil {
			fmt.Fprintf(io.Stderr, "verify plan: %v\n", err)
			return 1
		}
	}
	if f.asJSON {
		raw, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "verify plan: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, verify.RenderPlan(plan))
	if plan.Widened {
		fmt.Fprintln(io.Stderr, "warning: suite ampliada (incertidumbre): ningún test crítico se omite por inferencia débil")
	}
	io.recordImpactTrace("verify plan", f.changed)
	return 0
}

// verifyOutcomeFlags carries the recorded-outcome flags: per-gate
// overrides plus the results file and the toolchain guard.
type verifyOutcomeFlags struct {
	fail        string
	skip        string
	block       string
	na          string
	results     string
	noToolchain bool
	agent       string
	baseline    string
	window      string
	thresholds  string
}

// parseOutcomeList parses `gate:causa,...` entries into gate results
// with a fixed verdict. Bare gate names take the flag default cause.
func parseOutcomeList(s, verdict, defCause string) (map[string]verify.GateResult, error) {
	out := map[string]verify.GateResult{}
	for _, entry := range splitList(s) {
		kind, cause, _ := strings.Cut(entry, ":")
		if k2, c2, ok := strings.Cut(entry, "="); ok && cause == "" {
			kind, cause = k2, c2
		}
		kind, err := verify.ParseGateKind(kind)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(cause) == "" {
			cause = defCause
		}
		out[strings.ToLower(kind)] = verify.GateResult{Kind: kind, Verdict: verdict, Cause: cause, Evidence: "flag --" + strings.ToLower(verdict)}
	}
	return out, nil
}

// loadOutcomeResults reads the strict-JSON results file: an array of
// {kind, verdict, cause?, evidence?}. Unknown fields fail.
func loadOutcomeResults(path string) (map[string]verify.GateResult, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var arr []verify.GateResult
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&arr); err != nil {
		return nil, fmt.Errorf("malformed results file: %v", err)
	}
	out := map[string]verify.GateResult{}
	for _, r := range arr {
		kind, err := verify.ParseGateKind(r.Kind)
		if err != nil {
			return nil, err
		}
		if _, err := verify.ParseVerdict(r.Verdict); err != nil {
			return nil, err
		}
		out[strings.ToLower(kind)] = verify.GateResult{
			Kind: kind, Verdict: strings.ToUpper(strings.TrimSpace(r.Verdict)),
			Cause: strings.TrimSpace(r.Cause), Evidence: strings.TrimSpace(r.Evidence),
		}
	}
	return out, nil
}

// verifyExecutor folds every recorded outcome into the plan executor:
// explicit flags win, then the results file, then the toolchain
// guard, then UNKNOWN (never an invented pass). A recorded PASS that
// contradicts a missing toolchain degrades to BLOCKED.
func verifyExecutor(recorded map[string]verify.GateResult, toolchainMissing bool) verify.Executor {
	return func(spec verify.GateSpec) verify.GateResult {
		if toolchainMissing {
			return verify.GateResult{Kind: spec.Kind, Verdict: verify.VerdictBlocked,
				Cause: "host sin toolchain: go no disponible (multi-host seguro: BLOCKED, nunca PASS)"}
		}
		if r, ok := recorded[strings.ToLower(spec.Kind)]; ok {
			r.Kind = spec.Kind
			return r
		}
		return verify.GateResult{Kind: spec.Kind, Verdict: verify.VerdictUnknown,
			Cause: "sin resultado registrado para el gate (UNKNOWN bloquea: aporte evidencia)"}
	}
}

// verifyRegression loads the F12.4 baseline+window pair and links the
// diagnosis to this change. Empty refs mean "no regression input".
func verifyRegression(baselineRef, windowRef, thresholdsPath, diffHash, runID string) (*verify.RegressionReport, error) {
	if strings.TrimSpace(baselineRef) == "" && strings.TrimSpace(windowRef) == "" {
		return nil, nil
	}
	if strings.TrimSpace(baselineRef) == "" || strings.TrimSpace(windowRef) == "" {
		return nil, fmt.Errorf("verify: --baseline and --window travel together")
	}
	_, b, err := quality.ReadBaselineEvidence(baselineRef)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(thresholdsPath) != "" {
		th, err := quality.LoadThresholds(thresholdsPath)
		if err != nil {
			return nil, err
		}
		b.Thresholds = th
	}
	obs, _, err := quality.LoadWindow(windowRef)
	if err != nil {
		return nil, err
	}
	rep := verify.DetectRegression(b, obs, baselineRef, windowRef, diffHash, runID)
	return &rep, nil
}

// parseVerifyRunArgs parses the shared plan flags plus the run-only
// recorded-outcome flags, refusing bad flags, missing --changed and
// out-of-range --severity before any store work.
func (io IO) parseVerifyRunArgs(args []string) (verifyPlanFlags, verifyOutcomeFlags, int, bool) {
	var f verifyPlanFlags
	var o verifyOutcomeFlags
	fs := flag.NewFlagSet("verify run", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	addVerifyPlanFlags(fs, &f)
	fs.StringVar(&o.fail, "fail", "", "inject FAIL: gate:causa,...")
	fs.StringVar(&o.skip, "skip", "", "record SKIPPED: gate:causa,...")
	fs.StringVar(&o.block, "block", "", "record BLOCKED: gate:causa,...")
	fs.StringVar(&o.na, "na", "", "record NOT_APPLICABLE: gate:causa,...")
	fs.StringVar(&o.results, "results", "", "JSON results file [{kind,verdict,cause?,evidence?}]")
	fs.BoolVar(&o.noToolchain, "no-toolchain", false, "simulate a host without toolchain (every gate BLOCKED)")
	fs.StringVar(&o.agent, "agent", "", "agent id for provenance")
	fs.StringVar(&o.baseline, "baseline", "", "F12.4 baseline dir for regression linkage (needs --window)")
	fs.StringVar(&o.window, "window", "", "F12.4 window file for regression linkage (needs --baseline)")
	fs.StringVar(&o.thresholds, "thresholds", "", "thresholds JSON overriding the stored baseline ones")
	if err := fs.Parse(args); err != nil {
		return f, o, 2, false
	}
	if fs.NArg() != 0 || strings.TrimSpace(f.changed) == "" {
		fmt.Fprintln(io.Stderr, verifyUsage())
		return f, o, 2, false
	}
	if f.severity < 0 || f.severity > 1 {
		fmt.Fprintln(io.Stderr, "verify: --severity must be 0..1 (observed F12.4 rate)")
		return f, o, 2, false
	}
	return f, o, 0, true
}

// verifyRunFlagOutcomes folds the --fail/--skip/--block/--na flags
// into recorded gate results, mirroring the rerun flag fold.
func verifyRunFlagOutcomes(o verifyOutcomeFlags) (map[string]verify.GateResult, error) {
	recorded := map[string]verify.GateResult{}
	for _, spec := range []struct {
		val, verdict, cause string
	}{
		{o.fail, verify.VerdictFail, "fallo inyectado por --fail"},
		{o.skip, verify.VerdictSkipped, "omitido con causa por --skip"},
		{o.block, verify.VerdictBlocked, "bloqueado con causa por --block"},
		{o.na, verify.VerdictNotApplicable, "no aplicable con causa por --na"},
	} {
		if strings.TrimSpace(spec.val) == "" {
			continue
		}
		m, err := parseOutcomeList(spec.val, spec.verdict, spec.cause)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			recorded[k] = v
		}
	}
	return recorded, nil
}

// mergeVerifyRunFileOutcomes overlays the results file without
// clobbering flags, mirroring the rerun file merge.
func mergeVerifyRunFileOutcomes(recorded map[string]verify.GateResult, resultsPath string) error {
	if strings.TrimSpace(resultsPath) == "" {
		return nil
	}
	m, err := loadOutcomeResults(resultsPath)
	if err != nil {
		return err
	}
	for k, v := range m {
		if _, ok := recorded[k]; !ok {
			recorded[k] = v
		}
	}
	return nil
}

// loadVerifyRunRecorded merges flag outcomes under the results file:
// explicit flags win, the file fills the gaps. Flag parse errors
// refuse with 2, file errors with 1 — like the inline code did.
func (io IO) loadVerifyRunRecorded(o verifyOutcomeFlags) (map[string]verify.GateResult, int, bool) {
	recorded, err := verifyRunFlagOutcomes(o)
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify run: %v\n", err)
		return nil, 2, false
	}
	if err := mergeVerifyRunFileOutcomes(recorded, o.results); err != nil {
		fmt.Fprintf(io.Stderr, "verify run: %v\n", err)
		return nil, 1, false
	}
	return recorded, 0, true
}

// buildVerifyRunPlan derives the scope and provenance and builds the
// plan over the impact graph, refusing graph and plan errors with 1.
func (io IO) buildVerifyRunPlan(f verifyPlanFlags, agent string) (verify.Plan, verify.Provenance, int, bool) {
	scope := verify.ScopeTask
	if strings.TrimSpace(f.mission) != "" {
		scope = verify.ScopeMission
	}
	g, err := impact.BuildGraph(io.Dir, nil, nil)
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify run: %v\n", err)
		return verify.Plan{}, verify.Provenance{}, 1, false
	}
	changed := splitList(f.changed)
	prov := io.verifyProvenance(agent, verify.DiffHash(changed))
	plan, err := verify.BuildPlan(g, changed, verify.PlanOptions{
		Scope: scope, TaskID: f.task, MissionID: f.mission,
		Severity: f.severity, Contracts: splitList(f.contracts),
		Criteria: splitList(f.criteria), TierOverride: f.tier,
		Provenance: prov,
	}, prov.Commit)
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify run: %v\n", err)
		return verify.Plan{}, verify.Provenance{}, 1, false
	}
	return plan, prov, 0, true
}

// emitVerifyRun seals the result evidence dir (when --out) and prints
// the result as JSON or human text, refusing seal/marshal errors.
func (io IO) emitVerifyRun(f verifyPlanFlags, plan verify.Plan, res verify.Result, commit string) int {
	if strings.TrimSpace(f.out) != "" {
		if _, err := verify.WriteResultEvidence(f.out, plan, res, commit, time.Now()); err != nil {
			fmt.Fprintf(io.Stderr, "verify run: %v\n", err)
			return 1
		}
	}
	if f.asJSON {
		raw, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "verify run: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, verify.RenderReport(plan, res))
	return 0
}

// VerifyRunCmd implements `regulus verify run`.
func (io IO) VerifyRunCmd(args []string) int {
	f, o, code, ok := io.parseVerifyRunArgs(args)
	if !ok {
		return code
	}
	recorded, code, ok := io.loadVerifyRunRecorded(o)
	if !ok {
		return code
	}
	plan, prov, code, ok := io.buildVerifyRunPlan(f, o.agent)
	if !ok {
		return code
	}
	toolchainMissing := o.noToolchain || strings.HasPrefix(prov.Toolchain, "missing-go")
	rep, err := verifyRegression(o.baseline, o.window, o.thresholds, plan.DiffHash, prov.RunID)
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify run: %v\n", err)
		return 1
	}
	res, err := verify.RunPlan(plan, verifyExecutor(recorded, toolchainMissing), rep, time.Now())
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify run: %v\n", err)
		return 1
	}
	if code := io.emitVerifyRun(f, plan, res, prov.Commit); code != 0 {
		return code
	}
	io.recordImpactTrace("verify run", f.changed)
	if !res.AllowsIntegration() {
		return 1
	}
	return 0
}

// VerifyReportCmd implements `regulus verify report`.
func (io IO) VerifyReportCmd(args []string) int {
	fs := flag.NewFlagSet("verify report", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	dir := fs.String("dir", "", "result evidence dir (required)")
	asJSON := fs.Bool("json", false, "print the stored report as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(*dir) == "" {
		fmt.Fprintln(io.Stderr, "usage: regulus verify report --dir <dir> [--json]")
		return 2
	}
	_, plan, res, err := verify.ReadResultEvidence(*dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify report: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(struct {
			Plan   verify.Plan   `json:"plan"`
			Result verify.Result `json:"result"`
		}{plan, res}, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "verify report: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, verify.RenderReport(plan, res))
	io.recordImpactTrace("verify report", *dir)
	return 0
}

// VerifyMatrixCmd implements `regulus verify matrix`.
func (io IO) VerifyMatrixCmd(args []string) int {
	fs := flag.NewFlagSet("verify matrix", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the matrix as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, verifyUsage())
		return 2
	}
	if *asJSON {
		raw, err := json.MarshalIndent(verify.AllGates(), "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "verify matrix: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, verify.MatrixDoc())
	io.recordImpactTrace("verify matrix", "")
	return 0
}

// rerunRequest carries the parsed `verify rerun` inputs: the prior
// evidence dir, the new evidence dir and the recorded outcomes.
type rerunRequest struct {
	dir         string
	out         string
	fail        string
	skip        string
	block       string
	na          string
	results     string
	noToolchain bool
	asJSON      bool
}

// parseRerunRequest parses flags and refuses bad dir/out pairs first.
func (io IO) parseRerunRequest(args []string) (rerunRequest, int, bool) {
	var o verifyOutcomeFlags
	fs := flag.NewFlagSet("verify rerun", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	dir := fs.String("dir", "", "prior result evidence dir (required)")
	out := fs.String("out", "", "new result evidence dir (required, never the same)")
	fs.StringVar(&o.fail, "fail", "", "inject FAIL: gate:causa,...")
	fs.StringVar(&o.skip, "skip", "", "record SKIPPED: gate:causa,...")
	fs.StringVar(&o.block, "block", "", "record BLOCKED: gate:causa,...")
	fs.StringVar(&o.na, "na", "", "record NOT_APPLICABLE: gate:causa,...")
	fs.StringVar(&o.results, "results", "", "JSON results file [{kind,verdict,cause?,evidence?}]")
	fs.BoolVar(&o.noToolchain, "no-toolchain", false, "simulate a host without toolchain (every gate BLOCKED)")
	asJSON := fs.Bool("json", false, "print the new result as JSON")
	if err := fs.Parse(args); err != nil {
		return rerunRequest{}, 2, false
	}
	if fs.NArg() != 0 || strings.TrimSpace(*dir) == "" || strings.TrimSpace(*out) == "" {
		fmt.Fprintln(io.Stderr, "usage: regulus verify rerun --dir <dir> --out <newdir> [--fail ...] [--json]")
		return rerunRequest{}, 2, false
	}
	if *out == *dir {
		fmt.Fprintln(io.Stderr, "verify rerun: --out must differ from --dir (reruns never overwrite prior evidence)")
		return rerunRequest{}, 2, false
	}
	if _, err := os.Stat(*out + string(os.PathSeparator) + "manifest.json"); err == nil {
		fmt.Fprintln(io.Stderr, "verify rerun: --out already holds evidence (reruns write to a new dir)")
		return rerunRequest{}, 2, false
	}
	return rerunRequest{
		dir: *dir, out: *out,
		fail: o.fail, skip: o.skip, block: o.block, na: o.na,
		results: o.results, noToolchain: o.noToolchain, asJSON: *asJSON,
	}, 0, true
}

// rerunFlagOutcomes folds the --fail/--skip/--block/--na flags.
func rerunFlagOutcomes(r rerunRequest) (map[string]verify.GateResult, error) {
	recorded := map[string]verify.GateResult{}
	for _, spec := range []struct {
		val, verdict, cause string
	}{
		{r.fail, verify.VerdictFail, "fallo inyectado por --fail"},
		{r.skip, verify.VerdictSkipped, "omitido con causa por --skip"},
		{r.block, verify.VerdictBlocked, "bloqueado con causa por --block"},
		{r.na, verify.VerdictNotApplicable, "no aplicable con causa por --na"},
	} {
		if strings.TrimSpace(spec.val) == "" {
			continue
		}
		m, err := parseOutcomeList(spec.val, spec.verdict, spec.cause)
		if err != nil {
			return nil, err
		}
		for k, v := range m {
			recorded[k] = v
		}
	}
	return recorded, nil
}

// mergeRerunFileOutcomes overlays the results file without clobbering flags.
func mergeRerunFileOutcomes(recorded map[string]verify.GateResult, resultsPath string) error {
	if strings.TrimSpace(resultsPath) == "" {
		return nil
	}
	m, err := loadOutcomeResults(resultsPath)
	if err != nil {
		return err
	}
	for k, v := range m {
		if _, ok := recorded[k]; !ok {
			recorded[k] = v
		}
	}
	return nil
}

// loadRerunRecorded merges flag outcomes under the results file.
func (io IO) loadRerunRecorded(r rerunRequest) (map[string]verify.GateResult, int, bool) {
	recorded, err := rerunFlagOutcomes(r)
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify rerun: %v\n", err)
		return nil, 2, false
	}
	if err := mergeRerunFileOutcomes(recorded, r.results); err != nil {
		fmt.Fprintf(io.Stderr, "verify rerun: %v\n", err)
		return nil, 1, false
	}
	return recorded, 0, true
}

// finishVerifyRerun replays the stored plan with fresh outcomes and seals it.
func (io IO) finishVerifyRerun(r rerunRequest, plan verify.Plan, oldRes verify.Result, recorded map[string]verify.GateResult) int {
	// El rerun conserva la configuración (misma config id) con un run
	// id fresco: reproducible sin sobrescribir.
	runID, err := observability.MintRunID()
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify rerun: %v\n", err)
		return 1
	}
	plan.Provenance.RunID = runID
	toolchainMissing := r.noToolchain || strings.HasPrefix(plan.Provenance.Toolchain, "missing-go")
	// La regresión viaja con el plan cuando el run original la traía.
	newRes, err := verify.RunPlan(plan, verifyExecutor(recorded, toolchainMissing), oldRes.Regression, time.Now())
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify rerun: %v\n", err)
		return 1
	}
	head := gitHeadBounded(io.Runner)
	if strings.TrimSpace(head) == "" {
		head = "worktree"
	}
	if _, err := verify.WriteResultEvidence(r.out, plan, newRes, head, time.Now()); err != nil {
		fmt.Fprintf(io.Stderr, "verify rerun: %v\n", err)
		return 1
	}
	return io.emitVerifyRerun(plan, oldRes, newRes, r)
}

// emitVerifyRerun prints the rerun comparison plus the new result.
func (io IO) emitVerifyRerun(plan verify.Plan, oldRes verify.Result, newRes verify.Result, r rerunRequest) int {
	fmt.Fprint(io.Stdout, verify.Comparison(verify.ResultManifest{}, oldRes, verify.ResultManifest{}, newRes))
	if r.asJSON {
		raw, err := json.MarshalIndent(newRes, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "verify rerun: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
	} else {
		fmt.Fprint(io.Stdout, verify.RenderResult(plan, newRes))
	}
	io.recordImpactTrace("verify rerun", r.dir)
	if !newRes.AllowsIntegration() {
		return 1
	}
	return 0
}

// VerifyRerunCmd implements `regulus verify rerun`.
func (io IO) VerifyRerunCmd(args []string) int {
	req, code, ok := io.parseRerunRequest(args)
	if !ok {
		return code
	}
	_, plan, oldRes, err := verify.ReadResultEvidence(req.dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "verify rerun: %v\n", err)
		return 1
	}
	recorded, code, ok := io.loadRerunRecorded(req)
	if !ok {
		return code
	}
	return io.finishVerifyRerun(req, plan, oldRes, recorded)
}
