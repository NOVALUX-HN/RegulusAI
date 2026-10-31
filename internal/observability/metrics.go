// Metrics aggregation (Phase 8 T3, spec S3): read-only numbers over
// EXISTING stores — no producer rewrites, no vendor SDKs. Receipts give
// verification and duration facts, evidence.Verify gives completeness,
// evolution experiments give acceptance, trace entries give tool-call
// volume. Sources with no backing store today (guardian disagreement
// detail, token counts, model costs) aggregate honestly: zero values
// with explicit coverage notes, never invented numbers. Empty input
// yields zeros, never NaN (same rule as internal/evolve Summarize).
//
// Any free-text metric label MUST pass through redaction (see
// exporters.go RedactPayload); numeric fields trace to store fields as
// documented in the metric-to-store mapping below.
package observability

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/evolve"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

// Metric-to-store mapping (S3):
//
//	tasks, receipts_*            evidence.List (receipt Task/Gate/Verdict)
//	task_completion_rate         receipts: tasks whose every verdict is pass / tasks with receipts
//	verification_pass/fail_rate receipts: verdict pass|fail / total receipts
//	guardian_disagreement_rate   NO STORE: honest zero (see ProjectGuardianExecutions)
//	evidence_completeness        evidence.Verify passed/checked, quarantine disabled
//	evolution_acceptance_rate    evolve experiments adopted/total under .regulus/evolution
//	regression_rate              receipts: fail verdicts carrying a metric value / total receipts
//	total/mean_duration_ms       sum/mean of receipt duration_ms
//	total_cpu_time_ms            sum of receipt cpu_time_ms (resource consumption, lower bound)
//	tool_calls                   trace entries across all sessions (names+order only)
//	tokens_total                 NO STORE: honest zero, never estimated
//	cost_per_successful_task     NO STORE: honest zero, never estimated
const (
	unmeasuredGuardian = "guardian_disagreement_rate"
	unmeasuredTokens   = "token_usage"
	unmeasuredCosts    = "model_costs"
)

// Metrics is the S3 set aggregated read-only from existing stores.
type Metrics struct {
	Tasks                    int               `json:"tasks"`
	ReceiptsTotal            int               `json:"receipts_total"`
	ReceiptsPass             int               `json:"receipts_pass"`
	ReceiptsFail             int               `json:"receipts_fail"`
	TaskCompletionRate       float64           `json:"task_completion_rate"`
	VerificationPassRate     float64           `json:"verification_pass_rate"`
	VerificationFailureRate  float64           `json:"verification_failure_rate"`
	GuardianDisagreementRate float64           `json:"guardian_disagreement_rate"`
	EvidenceCompleteness     float64           `json:"evidence_completeness"`
	EvolutionAcceptanceRate  float64           `json:"evolution_acceptance_rate"`
	ExperimentsTotal         int               `json:"experiments_total"`
	ExperimentsAdopted       int               `json:"experiments_adopted"`
	RegressionRate           float64           `json:"regression_rate"`
	TotalDurationMS          int64             `json:"total_duration_ms"`
	MeanDurationMS           float64           `json:"mean_duration_ms"`
	TotalCPUTimeMS           int64             `json:"total_cpu_time_ms"`
	ToolCalls                int               `json:"tool_calls"`
	TokensTotal              int64             `json:"tokens_total"`
	CostPerSuccessfulTask    float64           `json:"cost_per_successful_task"`
	Coverage                 map[string]string `json:"coverage"`
	Unmeasured               []string          `json:"unmeasured"`
}

// listExperiments reads every stored evolution experiment, sorted by
// id. Shared by metrics and budget aggregation (both read-only).
func listExperiments(root string) ([]evolve.Experiment, error) {
	paths, err := filepath.Glob(filepath.Join(evolve.EvolveDir(root), "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []evolve.Experiment
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var exp evolve.Experiment
		if err := json.Unmarshal(data, &exp); err != nil {
			return nil, fmt.Errorf("observability: malformed experiment %s: %w", p, err)
		}
		out = append(out, exp)
	}
	return out, nil
}

// probeAttempts counts strategy probe attempts recorded in an
// experiment's history (Kind "probe", see evolve.RunExperiments).
func probeAttempts(exp evolve.Experiment) int {
	n := 0
	for _, h := range exp.History {
		if h.Kind == "probe" {
			n++
		}
	}
	return n
}

// Summarize aggregates the S3 metric set for one task ("" for every
// task). It only reads: evidence receipts, evidence.Verify with
// quarantine disabled, evolution experiments and trace logs. A task or
// root with no data yields zero metrics, never NaN.
func Summarize(root, task string) (Metrics, error) {
	m := Metrics{
		Coverage:   map[string]string{},
		Unmeasured: []string{unmeasuredGuardian, unmeasuredTokens, unmeasuredCosts},
	}

	entries, err := evidence.List(root, task)
	if err != nil {
		return Metrics{}, err
	}
	byTask := map[string][]receipts.Receipt{}
	for _, e := range entries {
		byTask[e.Receipt.Task] = append(byTask[e.Receipt.Task], e.Receipt)
	}
	m.Tasks = len(byTask)
	m.ReceiptsTotal = len(entries)
	complete := 0
	regressed := 0
	for _, recs := range byTask {
		done := len(recs) > 0
		for _, r := range recs {
			m.TotalDurationMS += r.DurationMS
			m.TotalCPUTimeMS += r.CPUTimeMS
			switch r.Verdict {
			case receipts.VerdictPass:
				m.ReceiptsPass++
			case receipts.VerdictFail:
				m.ReceiptsFail++
				if r.MetricValue != nil {
					regressed++
				}
			}
			if r.Verdict != receipts.VerdictPass {
				done = false
			}
		}
		if done {
			complete++
		}
	}
	if m.ReceiptsTotal > 0 {
		n := float64(m.ReceiptsTotal)
		m.VerificationPassRate = float64(m.ReceiptsPass) / n
		m.VerificationFailureRate = float64(m.ReceiptsFail) / n
		m.RegressionRate = float64(regressed) / n
		m.MeanDurationMS = float64(m.TotalDurationMS) / n
	}
	if m.Tasks > 0 {
		m.TaskCompletionRate = float64(complete) / float64(m.Tasks)
	}

	// Completeness is integrity: sound receipts over checked files,
	// quarantine disabled so aggregation never writes.
	rep, err := evidence.Verify(root, task, false)
	if err != nil {
		return Metrics{}, err
	}
	if rep.Checked > 0 {
		m.EvidenceCompleteness = float64(rep.Passed) / float64(rep.Checked)
	}

	exps, err := listExperiments(root)
	if err != nil {
		return Metrics{}, err
	}
	m.ExperimentsTotal = len(exps)
	for _, e := range exps {
		if e.State == evolve.StateAdopted {
			m.ExperimentsAdopted++
		}
	}
	if m.ExperimentsTotal > 0 {
		m.EvolutionAcceptanceRate = float64(m.ExperimentsAdopted) / float64(m.ExperimentsTotal)
	}

	sessions, err := trace.Sessions(root)
	if err != nil {
		return Metrics{}, err
	}
	for _, s := range sessions {
		got, err := trace.List(root, s)
		if err != nil {
			return Metrics{}, err
		}
		m.ToolCalls += len(got)
	}

	m.Coverage["task_completion_rate"] = "receipts: tasks with every verdict pass / tasks with any receipts"
	m.Coverage["verification_pass_rate"] = "receipts: verdict pass / total receipts"
	m.Coverage["verification_failure_rate"] = "receipts: verdict fail / total receipts"
	m.Coverage["guardian_disagreement_rate"] = "no guardian execution store; honest zero (see ProjectGuardianExecutions)"
	m.Coverage["evidence_completeness"] = "evidence.Verify passed/checked with quarantine disabled (hash pins and manifest links)"
	m.Coverage["evolution_acceptance_rate"] = "evolve experiments adopted/total under .regulus/evolution; experiments are global (no task field)"
	m.Coverage["regression_rate"] = "receipts: fail verdicts carrying a metric value / total receipts; non-metric failures excluded"
	m.Coverage["execution_duration"] = "sum/mean of receipt duration_ms"
	m.Coverage["resource_consumption"] = "sum of receipt cpu_time_ms (gates omitting the field contribute 0) plus trace tool-call count"
	m.Coverage["tool_calls"] = "trace entries across all sessions; entries carry names and order only, never values"
	m.Coverage[unmeasuredTokens] = "no token-count store; honest zero — supply reliable figures via budgets Figures for measured token usage"
	m.Coverage[unmeasuredCosts] = "no model-cost store; honest zero, never estimated — supply reliable figures via budgets Figures for measured costs"
	return m, nil
}
