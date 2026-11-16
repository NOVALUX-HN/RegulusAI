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

// labExecution carries the mutable state of one RunLab execution so
// the size budget splits into intent helpers without changing
// messages, events or exits.
type labExecution struct {
	ctx          context.Context
	runner       HeadlessAgent
	agent        LabAgent
	task         AgentTask
	split        string
	cfg          LabConfig
	root         string
	start        time.Time
	res          *LabResult
	em           *labEmitter
	runID        string
	agentExecID  string
	session      string
	host         string
	instructions string
	work         string
	budget       int
	testSnap     map[string]string
	raw          string
	usage        trace.HostUsage
	runErr       error
	oc           AgentOutcome
}

// labInitResult seeds the result shell: ids first, verdict later.
func labInitResult(agent LabAgent, task AgentTask, split string, cfg LabConfig) LabResult {
	res := LabResult{
		LabVersion: LabVersion, AgentID: agent.ID, TaskID: task.ID,
		Split: split, Seed: cfg.Seed, Variant: cfg.Variant,
		TokenSource: trace.SourceUnknown,
		ToolsSource: trace.SourceUnknown,
		Metadata:    map[string]any{},
	}
	if res.Variant == "" {
		res.Variant = "lab"
	}
	return res
}

// labMintIDs mints run, execution and session ids and links the
// trace hint. On mint failure it records the infra verdict.
func (l *labExecution) labMintIDs() bool {
	runID, err := observability.MintRunID()
	if err != nil {
		l.res.Verdict = LabInfraError
		l.res.Errors = []string{"mint run id: " + err.Error()}
		return false
	}
	agentExecID, err := observability.MintAgentExecutionID()
	if err != nil {
		l.res.Verdict = LabInfraError
		l.res.Errors = []string{"mint agent execution id: " + err.Error()}
		return false
	}
	session, err := mintLabSession()
	if err != nil {
		l.res.Verdict = LabInfraError
		l.res.Errors = []string{err.Error()}
		return false
	}
	l.runID, l.agentExecID, l.session = runID, agentExecID, session
	l.res.RunID, l.res.AgentExecutionID, l.res.SessionID = runID, agentExecID, session
	l.res.TraceHint = "regulus trace timeline --run " + runID + " --rebuild"
	l.em = &labEmitter{root: l.root, runID: runID, taskID: l.task.ID, agentID: agentExecID, session: session}
	return true
}

// labPreCanceled handles an entry-cancel: not-run, never a verdict
// on work the agent never did.
func (l *labExecution) labPreCanceled() bool {
	if l.ctx.Err() == nil {
		return false
	}
	l.res.Verdict = LabNotRun
	l.res.Errors = []string{"context canceled before start"}
	l.em.emit(observability.ExecRunStart, observability.ExecStarted, l.agentExecID, "", "", "", "", 0,
		map[string]any{"lab_version": LabVersion, "agent": l.agent.ID, "task": l.task.ID}, "")
	l.em.emit(observability.ExecRunEnd, observability.ExecCancelled, l.agentExecID, "", "", "", "", 0,
		map[string]any{"lab_verdict": LabNotRun}, "context canceled before start")
	l.res.EventIDs = l.em.eventIDs
	l.res.Errors = append(l.res.Errors, l.em.errs...)
	return true
}

// labEmitStarts records the run/agent open events.
func (l *labExecution) labEmitStarts() {
	l.em.emit(observability.ExecRunStart, observability.ExecStarted, l.agentExecID, "", "", l.cfg.Model, "", 0,
		map[string]any{"lab_version": LabVersion, "mode": l.cfg.Mode, "agent": l.agent.ID, "task": l.task.ID, "seed": l.cfg.Seed, "budget_s": l.cfg.BudgetS}, "")
	l.em.emit(observability.ExecAgentStart, observability.ExecStarted, l.agentExecID, "", "", l.cfg.Model, "", 0,
		map[string]any{"host": l.agent.Host, "agent": l.agent.ID}, "")
}

// labMakeWorkdir creates the isolated workspace (0700, removed after
// the run). The caller installs the removal defer so early fixture
// failures still clean up.
func (l *labExecution) labMakeWorkdir() bool {
	work, err := os.MkdirTemp("", "lab-work-")
	if err != nil {
		l.res.Verdict = LabInfraError
		l.res.Errors = []string{"mktemp: " + err.Error()}
		finishLabCancelled(l.em, l.res, l.start, l.cfg, l.agent)
		return false
	}
	l.work = work
	return true
}

// labFillWorkdir fixes permissions and materializes the fixture
// files (plus go.mod for Go tasks).
func (l *labExecution) labFillWorkdir() bool {
	work := l.work
	if err := os.Chmod(work, 0o700); err != nil {
		l.res.Verdict = LabInfraError
		l.res.Errors = []string{"chmod workdir: " + err.Error()}
		finishLabCancelled(l.em, l.res, l.start, l.cfg, l.agent)
		return false
	}
	for name, body := range l.task.Files {
		p := filepath.Join(work, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			l.res.Verdict = LabInfraError
			l.res.Errors = []string{"mkdir: " + err.Error()}
			finishLabCancelled(l.em, l.res, l.start, l.cfg, l.agent)
			return false
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			l.res.Verdict = LabInfraError
			l.res.Errors = []string{"write fixture: " + err.Error()}
			finishLabCancelled(l.em, l.res, l.start, l.cfg, l.agent)
			return false
		}
	}
	if l.task.Lang == "go" {
		if err := os.WriteFile(filepath.Join(work, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o600); err != nil {
			l.res.Verdict = LabInfraError
			l.res.Errors = []string{"write go.mod: " + err.Error()}
			finishLabCancelled(l.em, l.res, l.start, l.cfg, l.agent)
			return false
		}
	}
	return true
}

// labDescribeRun builds the exact prompt and metadata one execution
// sends and records the context event.
func (l *labExecution) labDescribeRun() {
	instructions := LabInstructions(l.agent, l.work, l.task.Prompt)
	l.instructions = instructions
	l.res.Instructions = instructions
	host := l.agent.Host
	if l.cfg.Host != "" {
		host = l.cfg.Host
	}
	l.host = host
	l.res.Metadata = map[string]any{
		"agent": l.agent.ID, "responsibility": l.agent.Responsibility,
		"skill": l.agent.Skill, "tools": l.agent.Tools, "host": host,
		"model": l.cfg.Model, "task": l.task.ID, "split": l.split,
		"budget_s": l.cfg.BudgetS, "seed": l.cfg.Seed, "session": l.session,
		"workdir": l.work, "variant": l.res.Variant,
		"lab_version": LabVersion, "suite_version": AgentSuiteVersion,
		"env": "allowlist-filtered (gates policy shape; FIXTURE_DIR/LAB_RUN_ID/LAB_SESSION extras)",
	}
	l.em.emit(observability.ExecContext, observability.ExecRunning, l.agentExecID, "", host, l.cfg.Model, "", 0,
		map[string]any{"instructions": instructions, "agent": l.agent.ID, "host": host, "budget_s": l.cfg.BudgetS, "seed": l.cfg.Seed}, "")
	budget := l.task.BudgetS
	if l.cfg.BudgetS > 0 && l.cfg.BudgetS < budget {
		budget = l.cfg.BudgetS
	}
	l.budget = budget
	l.testSnap = snapshotTestFiles(l.work, l.task.Files)
}

// labRunHost runs the agent with budget and retries. A mid-run
// cancel yields not-run; timeout and infra verdicts continue to the
// evaluation stage.
func (l *labExecution) labRunHost() bool {
	attempts := 0
	for {
		attempts++
		tctx, cancel := context.WithTimeout(l.ctx, time.Duration(l.budget)*time.Second)
		l.raw, l.usage, l.runErr = l.runner.Run(tctx, l.work, l.instructions, l.cfg.Model)
		cancel()
		if l.runErr == nil {
			break
		}
		if l.ctx.Err() == context.Canceled {
			l.res.Verdict = LabNotRun
			l.res.Errors = append(l.res.Errors, "canceled: "+l.runErr.Error())
			finishLabCancelled(l.em, l.res, l.start, l.cfg, l.agent)
			return false
		}
		if tctx.Err() == context.DeadlineExceeded {
			l.res.Verdict = LabTimeout
			l.res.Errors = append(l.res.Errors, "budget exceeded: "+l.runErr.Error())
			break
		}
		if attempts-1 < l.cfg.Retries {
			l.res.Retries++
			l.em.emit(observability.ExecRetry, observability.ExecRunning, l.agentExecID, "", l.host, l.cfg.Model, "", 0,
				map[string]any{"attempt": attempts, "error": l.runErr.Error()}, l.runErr.Error())
			continue
		}
		l.res.Verdict = LabInfraError
		l.res.Errors = append(l.res.Errors, "host infra: "+l.runErr.Error())
		break
	}
	return true
}

// labEvaluateCheck folds tokens and evidence, refuses gaming, runs
// the jailed check and records the timeout event.
func (l *labExecution) labEvaluateCheck() {
	oc := AgentOutcome{TaskID: l.task.ID, Split: l.split, Rep: 0, Seed: l.cfg.Seed,
		Variant: l.res.Variant, Model: l.cfg.Model, Host: l.host,
		TokenSource: trace.SourceUnknown}
	l.res.TokensIn, l.res.TokensOut = l.usage.InputTokens, l.usage.OutputTokens
	if l.usage.InputTokens > 0 || l.usage.OutputTokens > 0 {
		l.res.TokenSource = trace.SourceMeasuredHost
		oc.TokenSource = trace.SourceMeasuredHost
	}
	if ev := strings.TrimSpace(l.raw); len(ev) > labEvidenceCap {
		oc.Evidence = ev[:labEvidenceCap]
	} else {
		oc.Evidence = ev
	}
	l.res.DurationMS = time.Since(l.start).Milliseconds()

	if l.res.Verdict == "" {
		if tampered := changedTestFiles(l.work, l.task.Files, l.testSnap); len(tampered) > 0 {
			l.res.Verdict = LabFail
			l.res.Errors = append(l.res.Errors, "gaming: test files must not be modified: "+strings.Join(tampered, ", "))
		}
	}
	if l.res.Verdict == "" {
		checkCtx := l.ctx
		cmd := exec.CommandContext(checkCtx, l.task.Check[0], l.task.Check[1:]...)
		cmd.Dir = l.work
		cmd.Env = labChildEnv([]string{"FIXTURE_DIR=" + l.work, "LAB_RUN_ID=" + l.runID, "LAB_SESSION=" + l.session})
		executil.HideWindow(cmd)
		out, err := cmd.CombinedOutput()
		detail := strings.TrimSpace(string(out))
		if len(detail) > labDetailCap {
			detail = detail[:labDetailCap]
		}
		oc.Detail = detail
		if err != nil {
			if appControlBlocked(detail) || appControlBlocked(err.Error()) {
				l.res.Verdict = LabInfraError
				l.res.Errors = append(l.res.Errors, "check could not execute (execution blocked by policy)")
			} else {
				l.res.Verdict = LabFail
				if detail != "" {
					l.res.Errors = append(l.res.Errors, "check failed: "+shortLabDetail(detail))
				} else {
					l.res.Errors = append(l.res.Errors, "check failed: "+err.Error())
				}
			}
		} else {
			l.res.Verdict = LabPass
		}
	}
	if l.res.Verdict == LabTimeout {
		l.em.emit(observability.ExecTimeout, observability.ExecTimedOut, l.agentExecID, "", l.host, l.cfg.Model, "", l.res.DurationMS,
			map[string]any{"lab_verdict": LabTimeout}, strings.Join(l.res.Errors, "; "))
	}
	l.oc = oc
}

// labFinishRun attributes changes, closes the outcome, emits the
// terminal events and replays the timeline.
func (l *labExecution) labFinishRun() {
	changed, deleted := labChangedFiles(l.work, l.task.Files)
	l.res.ChangedFiles, l.res.DeletedFiles = changed, deleted
	oc := l.oc
	switch l.res.Verdict {
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
	oc.TaskID, oc.Split, oc.Seed = l.task.ID, l.split, l.cfg.Seed
	oc.TokensIn, oc.TokensOut = l.res.TokensIn, l.res.TokensOut
	oc.LatencyMS = l.res.DurationMS
	oc.HostVersion = ""
	l.res.Outcome = oc

	if l.usage.Source != "" {
		l.res.ToolsSource = l.usage.Source
	}
	status := labExecStatus(l.res.Verdict)
	l.em.emit(observability.ExecAgentEnd, status, l.agentExecID, "", l.host, l.cfg.Model, "", l.res.DurationMS,
		map[string]any{
			"lab_verdict": l.res.Verdict, "tokens_in": l.res.TokensIn, "tokens_out": l.res.TokensOut,
			"token_source": l.res.TokenSource, "retries": l.res.Retries,
			"changed": len(changed), "deleted": len(deleted),
		}, strings.Join(l.res.Errors, "; "))
	l.em.emit(observability.ExecRunEnd, status, l.agentExecID, "", l.host, l.cfg.Model, "", l.res.DurationMS,
		map[string]any{"lab_verdict": l.res.Verdict, "agent": l.agent.ID, "task": l.task.ID}, strings.Join(l.res.Errors, "; "))
	l.res.EventIDs = l.em.eventIDs
	l.res.Errors = append(l.res.Errors, l.em.errs...)

	if l.root != "" {
		if tl, err := observability.ReconstructRun(l.root, l.runID); err != nil {
			l.res.Errors = append(l.res.Errors, "reconstruct: "+err.Error())
		} else {
			l.res.TimelineStatus = tl.Status
			for _, d := range tl.Delegations {
				l.res.Delegations = append(l.res.Delegations, LabDelegation{
					DelegationID: d.DelegationID, FromAgent: d.FromAgent, ToAgent: d.ToAgent, Status: d.Status,
				})
			}
		}
	}
}

// RunLab executes one agent on one fixture task in full isolation and
// classifies the verdict. Cancellation stops the run: an entry-cancel
// or a mid-run cancel yields not-run (cancelled), never a verdict on
// work the agent never did.
func RunLab(ctx context.Context, runner HeadlessAgent, agent LabAgent, task AgentTask, split string, cfg LabConfig, root string) (res LabResult) {
	start := time.Now()
	res = labInitResult(agent, task, split, cfg)
	l := &labExecution{ctx: ctx, runner: runner, agent: agent, task: task, split: split, cfg: cfg, root: root, start: start, res: &res}
	if !l.labMintIDs() {
		return res
	}
	if l.labPreCanceled() {
		return res
	}
	l.labEmitStarts()
	if !l.labMakeWorkdir() {
		return res
	}
	res.WorkdirCleaned = false
	defer func() {
		if err := os.RemoveAll(l.work); err == nil {
			res.WorkdirCleaned = true
		}
	}()
	if !l.labFillWorkdir() {
		return res
	}
	l.labDescribeRun()
	if !l.labRunHost() {
		return res
	}
	l.labEvaluateCheck()
	l.labFinishRun()
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
