// Agent Lab (F11.2/NOV-88): isolated execution and comparison of
// agents without launching the full corpus.
//
// The Lab reuses the agent-task fixtures, splits and outcomes
// (AgentTask, AgentSplits, LoadAgentSplits, AgentOutcome) and the
// F11.1 execution-event store (internal/observability exec.jsonl via
// RunLab tracing): it adds no parallel corpus and no parallel store.
// Routing agents automatically is explicitly out (F12.1): compare
// contrasts, it never routes.
//
// Modes: single (one agent, one fixture), compare (two or more agents,
// same task/fixture/criteria/limits) and flow (single plus the F11.1
// post-crash reconstruction inline). Every execution is isolated
// (fresh workspace, captured outputs, unique session, filtered child
// environment following the gates allowlist policy without touching
// it) and classified
// pass/fail/timeout/infra-error/inconclusive/not-run.
package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LabVersion pins the Lab document shape: reports compare only within
// one version. Configs and reports carry it so replay mismatches fail
// closed instead of comparing across shapes.
const LabVersion = "agent-lab-v1"

// Lab verdicts. Only pass/fail/timeout come from an evaluated
// attempt; infra-error, inconclusive and not-run never pass.
const (
	LabPass         = "pass"
	LabFail         = "fail"
	LabTimeout      = "timeout"
	LabInfraError   = "infra-error"
	LabInconclusive = "inconclusive"
	LabNotRun       = "not-run"
)

// LabRank orders verdicts for comparison: a better rank wins, ties
// report token/latency as information only (cost never judges, same
// as bench Compare and agent-compare).
func LabRank(v string) int {
	switch v {
	case LabPass:
		return 4
	case LabFail:
		return 3
	case LabTimeout:
		return 2
	case LabInfraError:
		return 1
	default:
		return 0
	}
}

// LabAgent is one registered agent: responsibility, prompt/skill,
// declared tools, host and inputs. Every registered agent runs the
// same fixtures through the HeadlessAgent interface, so any two can
// be compared on the same task.
type LabAgent struct {
	ID             string   `json:"id"`
	Responsibility string   `json:"responsibility"`
	Prompt         string   `json:"prompt"`
	Skill          string   `json:"skill"`
	Tools          []string `json:"tools"`
	Host           string   `json:"host"`
	Inputs         []string `json:"inputs"`
}

// LabRegistry inventories the runnable agents. Baseline is the
// control (task prompt verbatim, no scaffolding); anchored adds the
// workdir/test-file discipline; verify-first reproduces the failure
// before fixing. Hosts grow here (claude/codex/devin) through the
// same interface; nothing here routes (F12.1).
func LabRegistry() []LabAgent {
	inputs := []string{"task.prompt", "fixture.files", "workdir"}
	tools := []string{"read", "edit", "bash"}
	return []LabAgent{
		{
			ID:             "baseline",
			Responsibility: "Resolve the visible task with no scaffolding; the comparison control.",
			Prompt:         "",
			Skill:          "(none — control)",
			Tools:          tools,
			Host:           "opencode",
			Inputs:         inputs,
		},
		{
			ID:             "anchored",
			Responsibility: "Resolve with workdir anchoring and test-file discipline, then self-verify.",
			Prompt:         "Work only inside the working directory. Do not modify test files. After editing, re-run the visible checks.",
			Skill:          "regulus-verify (verify subset)",
			Tools:          tools,
			Host:           "opencode",
			Inputs:         inputs,
		},
		{
			ID:             "verify-first",
			Responsibility: "Reproduce the failure with the visible checks first, then apply the minimal fix and verify.",
			Prompt:         "First reproduce the failure with the visible checks, then make the minimal fix and re-run them until they pass.",
			Skill:          "@regulus-verifier protocol",
			Tools:          tools,
			Host:           "opencode",
			Inputs:         inputs,
		},
	}
}

// FindLabAgent returns the registered agent by id.
func FindLabAgent(id string) (LabAgent, bool) {
	for _, a := range LabRegistry() {
		if a.ID == id {
			return a, true
		}
	}
	return LabAgent{}, false
}

// LabConfig is one reproducible Lab configuration: everything needed
// to repeat the execution. Export writes it; replay loads it back and
// runs it exactly (selection flags are refused alongside --config so
// a replay cannot silently differ).
type LabConfig struct {
	LabVersion   string   `json:"lab_version"`
	SuiteVersion string   `json:"suite_version"`
	Mode         string   `json:"mode"`
	Agent        string   `json:"agent,omitempty"`
	Agents       []string `json:"agents,omitempty"`
	Task         string   `json:"task"`
	Host         string   `json:"host,omitempty"`
	Model        string   `json:"model,omitempty"`
	BudgetS      int      `json:"budget_s,omitempty"`
	Seed         int64    `json:"seed,omitempty"`
	Retries      int      `json:"retries,omitempty"`
	Variant      string   `json:"variant,omitempty"`
}

// LabModes are the closed mode vocabulary.
const (
	LabModeSingle  = "single"
	LabModeCompare = "compare"
	LabModeFlow    = "flow"
)

// MaxLabRetries bounds the retry loop: retries cover transient host
// infra only, never new attempts at the task.
const MaxLabRetries = 5

// ValidateLabConfig refuses unknown modes/agents/tasks, empty tasks,
// negative budgets and unbounded retries. Corpus lookup keeps the Lab
// on the same fixtures as the suite (no parallel corpus).
func ValidateLabConfig(cfg LabConfig, corpus []AgentTask) error {
	switch cfg.Mode {
	case LabModeSingle, LabModeFlow:
		if cfg.Agent == "" {
			return fmt.Errorf("agent lab: mode %q needs one --agent", cfg.Mode)
		}
		if _, ok := FindLabAgent(cfg.Agent); !ok {
			return fmt.Errorf("agent lab: unknown agent %q", cfg.Agent)
		}
		if len(cfg.Agents) != 0 {
			return fmt.Errorf("agent lab: mode %q takes --agent, not --agents", cfg.Mode)
		}
	case LabModeCompare:
		if len(cfg.Agents) < 2 {
			return fmt.Errorf("agent lab: compare needs at least two --agents")
		}
		seen := map[string]bool{}
		for _, id := range cfg.Agents {
			if _, ok := FindLabAgent(id); !ok {
				return fmt.Errorf("agent lab: unknown agent %q", id)
			}
			if seen[id] {
				return fmt.Errorf("agent lab: duplicate agent %q (compare needs distinct agents)", id)
			}
			seen[id] = true
		}
		if cfg.Agent != "" {
			return fmt.Errorf("agent lab: compare takes --agents, not --agent")
		}
	default:
		return fmt.Errorf("agent lab: unknown mode %q (single|compare|flow)", cfg.Mode)
	}
	if cfg.Task == "" {
		return fmt.Errorf("agent lab: a --task is required (no mode runs the full corpus)")
	}
	found := false
	for _, t := range corpus {
		if t.ID == cfg.Task {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("agent lab: unknown task %q", cfg.Task)
	}
	if cfg.BudgetS < 0 {
		return fmt.Errorf("agent lab: --budget-s must not be negative")
	}
	if cfg.Retries < 0 || cfg.Retries > MaxLabRetries {
		return fmt.Errorf("agent lab: --retries must be 0..%d", MaxLabRetries)
	}
	return nil
}

// ExportLabConfig writes one replayable configuration as indented
// JSON (temp+rename: a torn config replays nothing).
func ExportLabConfig(path string, cfg LabConfig) error {
	cfg.LabVersion = LabVersion
	cfg.SuiteVersion = AgentSuiteVersion
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "lab-config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// LoadLabConfig reads one exported configuration: strict JSON
// (unknown fields fail) plus full validation, so a replay runs
// exactly what the config says or refuses.
func LoadLabConfig(path string, corpus []AgentTask) (LabConfig, error) {
	var cfg LabConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return LabConfig{}, err
	}
	if len(raw) > 64*1024 {
		return LabConfig{}, fmt.Errorf("agent lab: config too large")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return LabConfig{}, fmt.Errorf("agent lab: malformed config: %v", err)
	}
	if cfg.LabVersion != "" && cfg.LabVersion != LabVersion {
		return LabConfig{}, fmt.Errorf("agent lab: config version %q, want %q", cfg.LabVersion, LabVersion)
	}
	cfg.LabVersion = LabVersion
	if err := ValidateLabConfig(cfg, corpus); err != nil {
		return LabConfig{}, err
	}
	return cfg, nil
}

// LabDelegation attributes one delegation edge observed in the F11.1
// store for this execution. The Lab never delegates (no routing,
// F12.1): a non-empty list would name an unexpected producer.
type LabDelegation struct {
	DelegationID string `json:"delegation_id"`
	FromAgent    string `json:"from_agent"`
	ToAgent      string `json:"to_agent"`
	Status       string `json:"status"`
}

// LabResult is one isolated execution: instructions and metadata
// received, F11.1 event linkage, observed tools, delegations,
// duration, tokens with provenance, retries, errors, changes and the
// classified verdict. Outcome reuses AgentOutcome (no parallel
// outcome shape).
type LabResult struct {
	LabVersion       string          `json:"lab_version"`
	AgentID          string          `json:"agent_id"`
	TaskID           string          `json:"task_id"`
	Split            string          `json:"split"`
	Seed             int64           `json:"seed"`
	Variant          string          `json:"variant"`
	RunID            string          `json:"run_id"`
	AgentExecutionID string          `json:"agent_execution_id"`
	SessionID        string          `json:"session_id"`
	EventIDs         []string        `json:"event_ids"`
	Instructions     string          `json:"instructions"`
	Metadata         map[string]any  `json:"metadata"`
	Tools            []string        `json:"tools"`
	ToolsSource      string          `json:"tools_source"`
	Delegations      []LabDelegation `json:"delegations"`
	TimelineStatus   string          `json:"timeline_status"`
	DurationMS       int64           `json:"duration_ms"`
	TokensIn         int64           `json:"tokens_in"`
	TokensOut        int64           `json:"tokens_out"`
	TokenSource      string          `json:"token_source"`
	Retries          int             `json:"retries"`
	Errors           []string        `json:"errors"`
	ChangedFiles     []string        `json:"changed_files"`
	DeletedFiles     []string        `json:"deleted_files"`
	WorkdirCleaned   bool            `json:"workdir_cleaned"`
	Outcome          AgentOutcome    `json:"outcome"`
	Verdict          string          `json:"verdict"`
	TraceHint        string          `json:"trace_hint"`
}

// LabInstructions builds the exact prompt one execution sends: the
// baseline control sends the task prompt verbatim; strategies add
// the harness anchor plus their own discipline on top.
func LabInstructions(agent LabAgent, workdir, taskPrompt string) string {
	if agent.ID == "baseline" || strings.TrimSpace(agent.Prompt) == "" {
		return taskPrompt
	}
	return fmt.Sprintf("Working directory: %s. Read and edit only files inside it. Do not modify test files.\n\n%s\n\n%s",
		workdir, agent.Prompt, taskPrompt)
}

// labChangedFiles diffs the finished workspace against the shipped
// fixture: added/modified vs deleted, sorted. Runner-owned files
// (go.mod, check artifacts) are not agent changes and stay out.
func labChangedFiles(workdir string, files map[string]string) (changed, deleted []string) {
	seen := map[string]bool{}
	err := filepath.WalkDir(workdir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workdir, p)
		if err != nil {
			return nil
		}
		name := filepath.ToSlash(rel)
		if labRunnerOwned(name) {
			return nil
		}
		seen[name] = true
		if want, ok := files[name]; !ok || !labSameFile(p, want) {
			changed = append(changed, name)
		}
		return nil
	})
	if err != nil {
		return changed, deleted
	}
	for name := range files {
		if labRunnerOwned(name) {
			continue
		}
		if !seen[filepath.ToSlash(name)] {
			deleted = append(deleted, name)
		}
	}
	sort.Strings(changed)
	sort.Strings(deleted)
	return changed, deleted
}

// labSameFile reports whether path holds exactly want.

func labSameFile(path, want string) bool {
	raw, err := os.ReadFile(path)
	return err == nil && string(raw) == want
}

// labRunnerOwned reports files the runner (not the agent) owns:
// the injected go.mod and the hidden-check artifacts the AT28/AT29
// checks create at check time.
func labRunnerOwned(name string) bool {
	base := name
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	return base == "go.mod" || strings.HasPrefix(base, "zz_hidden")
}

// SplitOf reports the split holding id: exploration when listed,
// holdout otherwise. The CLI uses it to label single Lab executions
// with the same committed splits as the suite.
func SplitOf(sp AgentSplits, id string) string {
	for _, e := range sp.Exploration {
		if e == id {
			return "exploration"
		}
	}
	return "holdout"
}

// LabComparison contrasts two or more agents on the same
// task/fixture/criteria/limits. Winner is the best rank; ties and
// costs never judge (shown only). Nothing here routes (F12.1).
type LabComparison struct {
	LabVersion string      `json:"lab_version"`
	TaskID     string      `json:"task_id"`
	Split      string      `json:"split"`
	Results    []LabResult `json:"results"`
	Evaluable  int         `json:"evaluable"`
	Winner     string      `json:"winner"`
	Reason     string      `json:"reason"`
}

// CompareLabVerdict picks the winner over finished results: best rank
// wins; equal best ranks tie (tokens/latency shown, never judged);
// nothing evaluable means no winner.
func CompareLabVerdict(results []LabResult) (winner, reason string, evaluable int) {
	best := -1
	for _, r := range results {
		if r.Verdict == LabPass || r.Verdict == LabFail {
			evaluable++
		}
		if rank := LabRank(r.Verdict); rank > best {
			best = rank
		}
	}
	if evaluable == 0 {
		return "none", "nothing evaluable (infra/timeout/not-run only)", 0
	}
	var top []string
	for _, r := range results {
		if LabRank(r.Verdict) == best {
			top = append(top, r.AgentID)
		}
	}
	sort.Strings(top)
	if len(top) == 1 {
		second := -1
		for _, r := range results {
			if r.AgentID != top[0] {
				if rank := LabRank(r.Verdict); rank > second {
					second = rank
				}
			}
		}
		_ = second
		return top[0], fmt.Sprintf("%s wins: %s beats the field", top[0], verdictWord(results, top[0])), evaluable
	}
	return "tie", fmt.Sprintf("tie at %s (%s)", verdictWord(results, top[0]), strings.Join(top, ", ")), evaluable
}

func verdictWord(results []LabResult, agent string) string {
	for _, r := range results {
		if r.AgentID == agent {
			return r.Verdict
		}
	}
	return "unknown"
}
