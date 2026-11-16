// Agent Lab execution (F11.2/NOV-88): isolated single runs and
// same-task comparisons over the shared fixtures, with every result
// linked to the F11.1 execution-event store.
//
// Isolation per execution: a fresh workspace (0700, removed after the
// run), captured outputs (never shared), a unique session id, and a
// filtered child environment for the check following the gates
// allowlist policy (same shape, no code touched there): toolchain,
// locale, proxy and harness namespaces pass, FIXTURE_DIR and LAB_*
// extras are appended last, everything else — secrets included — is
// dropped. The agent host itself inherits the runner process env
// (hosts are external binaries); the evaluated check is jailed.
//
// Tracing reuses internal/observability (no parallel store): one
// run_id per execution, one agent_execution_id per attempt, the
// session_id above, and event ids collected on the result so
// `trace timeline --run <id> --rebuild` replays the Lab run,
// including the post-crash reconstruction (flow mode prints it
// inline). Root "" skips tracing (hermetic unit runs); the CLI
// always passes the project root.
package bench

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/executil"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/trace"
)

// labEvidenceCap truncates captured agent output on the report (same
// cap as runOne: evidence, never transcripts).
const labEvidenceCap = 4000

// labDetailCap truncates check output kept on the outcome.
const labDetailCap = 2000

// mintLabSession mints one session id per execution (prefix + 64 bits
// crypto/rand, mirroring the observability MintID pattern).
func mintLabSession() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("agent lab: cannot mint session id: %w", err)
	}
	return "ses_" + hex.EncodeToString(b[:]), nil
}

// labFold folds an env name for comparison: Windows treats names
// case-insensitively, other platforms are exact (same rule as gates).
func labFold(name string) string {
	if isWindowsLab() {
		return strings.ToUpper(name)
	}
	return name
}

func isWindowsLab() bool {
	return os.PathSeparator == '\\'
}

// labChildEnv builds the jailed child environment for the evaluated
// check: the gates allowlist shape (toolchain + locale + proxy + CI
// and harness namespaces), extras appended last so FIXTURE_DIR always
// lands, unset/unknown names dropped. It mirrors the policy without
// importing unexported gates code, which stays the single owner.
func labChildEnv(extra []string) []string {
	allow := []string{
		"PATH", "PATHEXT",
		"HOME", "USERPROFILE", "TEMP", "TMP", "TMPDIR",
		"LOCALAPPDATA", "APPDATA",
		"SYSTEMROOT", "WINDIR", "COMSPEC",
		"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV",
		"GOFLAGS", "GOPROXY", "GOTOOLCHAIN", "GOSUMDB",
		"CI",
		"LANG", "LC_ALL", "LC_CTYPE", "TZ",
		"NO_PROXY", "no_proxy",
		"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy",
	}
	if isWindowsLab() {
		allow = append(allow, "USERDOMAIN", "USERNAME", "COMPUTERNAME")
	} else {
		allow = append(allow, "USER", "LOGNAME", "SHELL")
	}
	prefixes := []string{"REGULUS_", "GITHUB_", "RUNNER_", "NPM_CONFIG_", "npm_config_"}
	allowed := func(name string) bool {
		f := labFold(name)
		for _, p := range prefixes {
			if strings.HasPrefix(f, labFold(p)) {
				return true
			}
		}
		for _, n := range allow {
			if f == labFold(n) {
				return true
			}
		}
		return false
	}
	var out []string
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		if allowed(kv[:i]) {
			out = append(out, kv)
		}
	}
	return append(out, extra...)
}

// LabContestant pairs a registered agent with its host runner.
type LabContestant struct {
	Agent  LabAgent
	Runner HeadlessAgent
}

// labEmitter collects F11.1 event ids for one execution. A nil store
// (root "") skips tracing so unit runs stay hermetic.
type labEmitter struct {
	root     string
	runID    string
	taskID   string
	agentID  string
	session  string
	eventIDs []string
	errs     []string
}

func (e *labEmitter) emit(typ, status, agentExec, parent, host, model, hostVersion string, durationMS int64, payload map[string]any, errDetails string) {
	if e == nil || e.root == "" {
		return
	}
	saved, _, err := observability.RecordExec(e.root, observability.ExecEvent{
		RunID: e.runID, TaskID: e.taskID, AgentExecutionID: agentExec,
		SessionID: e.session, ParentEventID: parent,
		Type: typ, Status: status,
		Host: host, Model: model, HostVersion: hostVersion,
		DurationMS: durationMS, Payload: payload, ErrorDetails: errDetails,
	}, time.Now().UTC())
	if err != nil {
		e.errs = append(e.errs, "trace emit "+typ+": "+err.Error())
		return
	}
	e.eventIDs = append(e.eventIDs, saved.EventID)
}

// labExecStatus maps a Lab verdict onto the F11.1 terminal
// vocabulary (terminal states are unequivocal; infra-error rides as
// failed with the lab verdict kept in payload).
func labExecStatus(verdict string) string {
	switch verdict {
	case LabPass:
		return observability.ExecSucceeded
	case LabTimeout:
		return observability.ExecTimedOut
	case LabNotRun:
		return observability.ExecCancelled
	default:
		return observability.ExecFailed
	}
}

// RunLab executes one agent on one fixture task in full isolation and
// classifies the verdict. Cancellation stops the run: an entry-cancel
// or a mid-run cancel yields not-run (cancelled), never a verdict on
// work the agent never did.
func RunLab(ctx context.Context, runner HeadlessAgent, agent LabAgent, task AgentTask, split string, cfg LabConfig, root string) (res LabResult) {
	start := time.Now()
	res = LabResult{
		LabVersion: LabVersion, AgentID: agent.ID, TaskID: task.ID,
		Split: split, Seed: cfg.Seed, Variant: cfg.Variant,
		TokenSource: trace.SourceUnknown,
		ToolsSource: trace.SourceUnknown,
		Metadata:    map[string]any{},
	}
	if res.Variant == "" {
		res.Variant = "lab"
	}
	runID, err := observability.MintRunID()
	if err != nil {
		res.Verdict = LabInfraError
		res.Errors = []string{"mint run id: " + err.Error()}
		return
	}
	agentExecID, err := observability.MintAgentExecutionID()
	if err != nil {
		res.Verdict = LabInfraError
		res.Errors = []string{"mint agent execution id: " + err.Error()}
		return res
	}
	session, err := mintLabSession()
	if err != nil {
		res.Verdict = LabInfraError
		res.Errors = []string{err.Error()}
		return res
	}
	res.RunID, res.AgentExecutionID, res.SessionID = runID, agentExecID, session
	res.TraceHint = "regulus trace timeline --run " + runID + " --rebuild"
	em := &labEmitter{root: root, runID: runID, taskID: task.ID, agentID: agentExecID, session: session}

	if ctx.Err() != nil {
		res.Verdict = LabNotRun
		res.Errors = []string{"context canceled before start"}
		em.emit(observability.ExecRunStart, observability.ExecStarted, agentExecID, "", "", "", "", 0,
			map[string]any{"lab_version": LabVersion, "agent": agent.ID, "task": task.ID}, "")
		em.emit(observability.ExecRunEnd, observability.ExecCancelled, agentExecID, "", "", "", "", 0,
			map[string]any{"lab_verdict": LabNotRun}, "context canceled before start")
		res.EventIDs = em.eventIDs
		res.Errors = append(res.Errors, em.errs...)
		return res
	}

	em.emit(observability.ExecRunStart, observability.ExecStarted, agentExecID, "", "", cfg.Model, "", 0,
		map[string]any{"lab_version": LabVersion, "mode": cfg.Mode, "agent": agent.ID, "task": task.ID, "seed": cfg.Seed, "budget_s": cfg.BudgetS}, "")
	em.emit(observability.ExecAgentStart, observability.ExecStarted, agentExecID, "", "", cfg.Model, "", 0,
		map[string]any{"host": agent.Host, "agent": agent.ID}, "")

	work, err := os.MkdirTemp("", "lab-work-")
	if err != nil {
		res.Verdict = LabInfraError
		res.Errors = []string{"mktemp: " + err.Error()}
		finishLabCancelled(em, &res, start, cfg, agent)
		return res
	}
	res.WorkdirCleaned = false
	defer func() {
		if err := os.RemoveAll(work); err == nil {
			res.WorkdirCleaned = true
		}
	}()
	if err := os.Chmod(work, 0o700); err != nil {
		res.Verdict = LabInfraError
		res.Errors = []string{"chmod workdir: " + err.Error()}
		finishLabCancelled(em, &res, start, cfg, agent)
		return res
	}
	for name, body := range task.Files {
		p := filepath.Join(work, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			res.Verdict = LabInfraError
			res.Errors = []string{"mkdir: " + err.Error()}
			finishLabCancelled(em, &res, start, cfg, agent)
			return res
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			res.Verdict = LabInfraError
			res.Errors = []string{"write fixture: " + err.Error()}
			finishLabCancelled(em, &res, start, cfg, agent)
			return res
		}
	}
	if task.Lang == "go" {
		if err := os.WriteFile(filepath.Join(work, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o600); err != nil {
			res.Verdict = LabInfraError
			res.Errors = []string{"write go.mod: " + err.Error()}
			finishLabCancelled(em, &res, start, cfg, agent)
			return res
		}
	}

	instructions := LabInstructions(agent, work, task.Prompt)
	res.Instructions = instructions
	host := agent.Host
	if cfg.Host != "" {
		host = cfg.Host
	}
	res.Metadata = map[string]any{
		"agent": agent.ID, "responsibility": agent.Responsibility,
		"skill": agent.Skill, "tools": agent.Tools, "host": host,
		"model": cfg.Model, "task": task.ID, "split": split,
		"budget_s": cfg.BudgetS, "seed": cfg.Seed, "session": session,
		"workdir": work, "variant": res.Variant,
		"lab_version": LabVersion, "suite_version": AgentSuiteVersion,
		"env": "allowlist-filtered (gates policy shape; FIXTURE_DIR/LAB_RUN_ID/LAB_SESSION extras)",
	}
	em.emit(observability.ExecContext, observability.ExecRunning, agentExecID, "", host, cfg.Model, "", 0,
		map[string]any{"instructions": instructions, "agent": agent.ID, "host": host, "budget_s": cfg.BudgetS, "seed": cfg.Seed}, "")

	budget := task.BudgetS
	if cfg.BudgetS > 0 && cfg.BudgetS < budget {
		budget = cfg.BudgetS
	}
	testSnap := snapshotTestFiles(work, task.Files)
	var raw string
	var usage trace.HostUsage
	var runErr error
	attempts := 0
	for {
		attempts++
		tctx, cancel := context.WithTimeout(ctx, time.Duration(budget)*time.Second)
		raw, usage, runErr = runner.Run(tctx, work, instructions, cfg.Model)
		cancel()
		if runErr == nil {
			break
		}
		if ctx.Err() == context.Canceled {
			res.Verdict = LabNotRun
			res.Errors = append(res.Errors, "canceled: "+runErr.Error())
			finishLabCancelled(em, &res, start, cfg, agent)
			return res
		}
		if tctx.Err() == context.DeadlineExceeded {
			res.Verdict = LabTimeout
			res.Errors = append(res.Errors, "budget exceeded: "+runErr.Error())
			break
		}
		if attempts-1 < cfg.Retries {
			res.Retries++
			em.emit(observability.ExecRetry, observability.ExecRunning, agentExecID, "", host, cfg.Model, "", 0,
				map[string]any{"attempt": attempts, "error": runErr.Error()}, runErr.Error())
			continue
		}
		res.Verdict = LabInfraError
		res.Errors = append(res.Errors, "host infra: "+runErr.Error())
		break
	}

	oc := AgentOutcome{TaskID: task.ID, Split: split, Rep: 0, Seed: cfg.Seed,
		Variant: res.Variant, Model: cfg.Model, Host: host,
		TokenSource: trace.SourceUnknown}
	res.TokensIn, res.TokensOut = usage.InputTokens, usage.OutputTokens
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		res.TokenSource = trace.SourceMeasuredHost
		oc.TokenSource = trace.SourceMeasuredHost
	}
	if ev := strings.TrimSpace(raw); len(ev) > labEvidenceCap {
		oc.Evidence = ev[:labEvidenceCap]
	} else {
		oc.Evidence = ev
	}
	res.DurationMS = time.Since(start).Milliseconds()

	if res.Verdict == "" {
		if tampered := changedTestFiles(work, task.Files, testSnap); len(tampered) > 0 {
			res.Verdict = LabFail
			res.Errors = append(res.Errors, "gaming: test files must not be modified: "+strings.Join(tampered, ", "))
		}
	}
	if res.Verdict == "" {
		checkCtx := ctx
		cmd := exec.CommandContext(checkCtx, task.Check[0], task.Check[1:]...)
		cmd.Dir = work
		cmd.Env = labChildEnv([]string{"FIXTURE_DIR=" + work, "LAB_RUN_ID=" + runID, "LAB_SESSION=" + session})
		executil.HideWindow(cmd)
		out, err := cmd.CombinedOutput()
		detail := strings.TrimSpace(string(out))
		if len(detail) > labDetailCap {
			detail = detail[:labDetailCap]
		}
		oc.Detail = detail
		if err != nil {
			if appControlBlocked(detail) || appControlBlocked(err.Error()) {
				res.Verdict = LabInfraError
				res.Errors = append(res.Errors, "check could not execute (execution blocked by policy)")
			} else {
				res.Verdict = LabFail
				if detail != "" {
					res.Errors = append(res.Errors, "check failed: "+shortLabDetail(detail))
				} else {
					res.Errors = append(res.Errors, "check failed: "+err.Error())
				}
			}
		} else {
			res.Verdict = LabPass
		}
	}
	if res.Verdict == LabTimeout {
		em.emit(observability.ExecTimeout, observability.ExecTimedOut, agentExecID, "", host, cfg.Model, "", res.DurationMS,
			map[string]any{"lab_verdict": LabTimeout}, strings.Join(res.Errors, "; "))
	}

	changed, deleted := labChangedFiles(work, task.Files)
	res.ChangedFiles, res.DeletedFiles = changed, deleted
	switch res.Verdict {
	case LabPass:
		oc.Status = AgentPass
	case LabFail:
		oc.Status = AgentFail
	case LabTimeout:
		oc.Status = AgentTimeout
	case LabInfraError:
		oc.Status = AgentInfra
	default:
		oc.Status = AgentNotRun
	}
	oc.TaskID, oc.Split, oc.Seed = task.ID, split, cfg.Seed
	oc.TokensIn, oc.TokensOut = res.TokensIn, res.TokensOut
	oc.LatencyMS = res.DurationMS
	oc.HostVersion = ""
	res.Outcome = oc

	if usage.Source != "" {
		res.ToolsSource = usage.Source
	}
	status := labExecStatus(res.Verdict)
	em.emit(observability.ExecAgentEnd, status, agentExecID, "", host, cfg.Model, "", res.DurationMS,
		map[string]any{
			"lab_verdict": res.Verdict, "tokens_in": res.TokensIn, "tokens_out": res.TokensOut,
			"token_source": res.TokenSource, "retries": res.Retries,
			"changed": len(changed), "deleted": len(deleted),
		}, strings.Join(res.Errors, "; "))
	em.emit(observability.ExecRunEnd, status, agentExecID, "", host, cfg.Model, "", res.DurationMS,
		map[string]any{"lab_verdict": res.Verdict, "agent": agent.ID, "task": task.ID}, strings.Join(res.Errors, "; "))
	res.EventIDs = em.eventIDs
	res.Errors = append(res.Errors, em.errs...)

	if root != "" {
		if tl, err := observability.ReconstructRun(root, runID); err != nil {
			res.Errors = append(res.Errors, "reconstruct: "+err.Error())
		} else {
			res.TimelineStatus = tl.Status
			for _, d := range tl.Delegations {
				res.Delegations = append(res.Delegations, LabDelegation{
					DelegationID: d.DelegationID, FromAgent: d.FromAgent, ToAgent: d.ToAgent, Status: d.Status,
				})
			}
		}
	}
	return res
}

// finishLabCancelled closes an infra-early run with terminal failed
// events (the workspace never ran, so there is nothing to evaluate).
func finishLabCancelled(em *labEmitter, res *LabResult, start time.Time, cfg LabConfig, agent LabAgent) {
	res.DurationMS = time.Since(start).Milliseconds()
	if res.Verdict == "" {
		res.Verdict = LabInfraError
	}
	host := agent.Host
	if cfg.Host != "" {
		host = cfg.Host
	}
	em.emit(observability.ExecAgentEnd, observability.ExecFailed, res.AgentExecutionID, "", host, cfg.Model, "", res.DurationMS,
		map[string]any{"lab_verdict": res.Verdict}, strings.Join(res.Errors, "; "))
	em.emit(observability.ExecRunEnd, observability.ExecFailed, res.AgentExecutionID, "", host, cfg.Model, "", res.DurationMS,
		map[string]any{"lab_verdict": res.Verdict, "agent": agent.ID}, strings.Join(res.Errors, "; "))
	res.EventIDs = em.eventIDs
	res.Errors = append(res.Errors, em.errs...)
}

func shortLabDetail(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len(s) > 160 {
		return s[:160] + "…"
	}
	if s == "" {
		return "-"
	}
	return s
}

// CompareLab runs every contestant on the same task/fixture with the
// same criteria and limits, sequentially in registry order. A
// canceled context marks the remaining contestants not-run instead
// of aborting the comparison, so the report stays complete.
func CompareLab(ctx context.Context, cs []LabContestant, task AgentTask, split string, cfg LabConfig, root string) (LabComparison, error) {
	if len(cs) < 2 {
		return LabComparison{}, fmt.Errorf("agent lab: compare needs at least two contestants")
	}
	cmp := LabComparison{LabVersion: LabVersion, TaskID: task.ID, Split: split}
	for _, c := range cs {
		one := cfg
		one.Mode = LabModeCompare
		cmp.Results = append(cmp.Results, RunLab(ctx, c.Runner, c.Agent, task, split, one, root))
	}
	winner, reason, evaluable := CompareLabVerdict(cmp.Results)
	cmp.Winner, cmp.Reason, cmp.Evaluable = winner, reason, evaluable
	return cmp, nil
}

// labOutcomeManifest builds the shared evidence manifest for Lab
// results: the outcomes stay the suite shape (sha-verified by the
// existing reader); the Lab report rides alongside.
func labOutcomeManifest(cfg LabConfig, results []LabResult, gitHead, startedAt string) AgentRunManifest {
	model, host := cfg.Model, cfg.Host
	if len(results) > 0 {
		if h, ok := results[0].Metadata["host"].(string); ok && host == "" {
			host = h
		}
		if model == "" {
			if m, ok := results[0].Metadata["model"].(string); ok {
				model = m
			}
		}
	}
	seeds := make([]int64, 0, len(results))
	for _, r := range results {
		seeds = append(seeds, r.Seed)
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i] < seeds[j] })
	return AgentRunManifest{
		SuiteVersion: AgentSuiteVersion, Model: model, Host: host,
		Variant: labVariant(cfg), Reps: 1, Split: results[0].Split,
		Seeds: seeds, GitHead: gitHead, StartedAt: startedAt,
	}
}

func labVariant(cfg LabConfig) string {
	if cfg.Variant != "" {
		return cfg.Variant
	}
	return LabVersion
}

// Lab evidence filenames inside an evidence dir: the shared outcomes
// pair plus the Lab config and full per-execution reports.
const (
	LabConfigFile  = "config.json"
	LabReportsFile = "reports.json"
)

// WriteLabEvidence stores one Lab run: shared outcomes+manifest
// (tamper-evident, read by the existing verifier) plus the replayable
// config and the full per-execution reports. A new run uses a new
// directory.
func WriteLabEvidence(dir string, cfg LabConfig, results []LabResult, gitHead, startedAt string) (string, error) {
	if len(results) == 0 {
		return "", fmt.Errorf("agent lab: no results to store")
	}
	var outcomes []AgentOutcome
	for _, r := range results {
		outcomes = append(outcomes, r.Outcome)
	}
	stamped := cfg
	stamped.LabVersion = LabVersion
	stamped.SuiteVersion = AgentSuiteVersion
	rawCfg, err := json.MarshalIndent(stamped, "", "  ")
	if err != nil {
		return "", err
	}
	rawRep, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, LabConfigFile), append(rawCfg, '\n'), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, LabReportsFile), append(rawRep, '\n'), 0o644); err != nil {
		return "", err
	}
	return WriteAgentEvidence(dir, outcomes, labOutcomeManifest(cfg, results, gitHead, startedAt))
}

// ReadLabEvidence loads one Lab evidence dir: the shared
// outcomes+manifest first (sha-verified, tamper fails closed), then
// the config and reports strictly from those files — reports are
// never hand-written.
func ReadLabEvidence(dir string) (LabConfig, AgentRunManifest, []AgentOutcome, []LabResult, error) {
	var cfg LabConfig
	m, outcomes, err := ReadAgentEvidence(dir)
	if err != nil {
		return cfg, m, nil, nil, err
	}
	rawCfg, err := os.ReadFile(filepath.Join(dir, LabConfigFile))
	if err != nil {
		return cfg, m, nil, nil, fmt.Errorf("reading lab config: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(rawCfg)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return cfg, m, nil, nil, fmt.Errorf("malformed lab config: %v", err)
	}
	rawRep, err := os.ReadFile(filepath.Join(dir, LabReportsFile))
	if err != nil {
		return cfg, m, nil, nil, fmt.Errorf("reading lab reports: %v", err)
	}
	var results []LabResult
	rdec := json.NewDecoder(strings.NewReader(string(rawRep)))
	rdec.DisallowUnknownFields()
	if err := rdec.Decode(&results); err != nil {
		return cfg, m, nil, nil, fmt.Errorf("malformed lab reports: %v", err)
	}
	return cfg, m, outcomes, results, nil
}

// RenderLabReport builds the human report strictly from a verified
// evidence dir: config, per-execution blocks (instructions, metadata,
// events, tools, delegations, duration, tokens with provenance,
// retries, errors, changes, verdict) and the comparison verdict when
// the dir holds more than one result.
func RenderLabReport(cfg LabConfig, m AgentRunManifest, results []LabResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "agent-lab %s suite %s mode %s task %s\n", LabVersion, m.SuiteVersion, cfg.Mode, cfg.Task)
	fmt.Fprintf(&b, "evidence: %s sha256:%s git %s %s..%s\n",
		m.OutcomesFile, shortSHA(m.OutcomesSHA), m.GitHead, m.StartedAt, m.FinishedAt)
	for _, r := range results {
		fmt.Fprintf(&b, "\n== %s: %s ==\n", r.AgentID, r.Verdict)
		fmt.Fprintf(&b, "run %s agent %s session %s\n", r.RunID, r.AgentExecutionID, r.SessionID)
		fmt.Fprintf(&b, "duration %dms tokens %d in + %d out (%s) retries %d\n",
			r.DurationMS, r.TokensIn, r.TokensOut, r.TokenSource, r.Retries)
		if meta, err := json.Marshal(r.Metadata); err == nil {
			fmt.Fprintf(&b, "metadata: %s\n", meta)
		}
		fmt.Fprintf(&b, "instructions: %s\n", r.Instructions)
		fmt.Fprintf(&b, "events: %d [%s]\n", len(r.EventIDs), strings.Join(r.EventIDs, ", "))
		fmt.Fprintf(&b, "tools observed: [%s] (%s)\n", strings.Join(r.Tools, ", "), r.ToolsSource)
		if len(r.Delegations) == 0 {
			fmt.Fprintf(&b, "delegations: none (no routing in the Lab)\n")
		} else {
			for _, d := range r.Delegations {
				fmt.Fprintf(&b, "delegation %s: %s -> %s (%s)\n", d.DelegationID, d.FromAgent, d.ToAgent, d.Status)
			}
		}
		if len(r.Errors) == 0 {
			b.WriteString("errors: none\n")
		} else {
			fmt.Fprintf(&b, "errors: %s\n", strings.Join(r.Errors, " | "))
		}
		fmt.Fprintf(&b, "changed: [%s] deleted: [%s] workdir cleaned: %v\n",
			strings.Join(r.ChangedFiles, ", "), strings.Join(r.DeletedFiles, ", "), r.WorkdirCleaned)
		fmt.Fprintf(&b, "outcome: %s %s\n", r.Outcome.Status, shortLabDetail(r.Outcome.Detail))
		fmt.Fprintf(&b, "trace: %s\n", r.TraceHint)
	}
	if len(results) > 1 {
		winner, reason, evaluable := CompareLabVerdict(results)
		fmt.Fprintf(&b, "\ncomparison: winner %s (%s, %d evaluable)\n", winner, reason, evaluable)
	}
	return b.String()
}
