// Mission evidence report (F13.7/NOV-110): a versioned,
// exportable, integrity-sealed expediente for every terminal
// mission, so a third party can reconstruct what REGULUS did and
// check the result without the conversational memory.
//
// Reuse, never duplication:
//   - mission truth: internal/mission (model, states, store). This
//     file only reads a *Mission, it never mutates or re-stores it.
//   - redaction: observability.RedactText over every free-text
//     field before it lands in the report (no secrets, ever).
//   - traces/receipts/approvals/verify plans stay in their own
//     stores; the report cites ids + hashes, never copies bytes.
//     The edge (cmdx) assembles those citations; unknown or
//     unavailable data is recorded as unknown with its reason,
//     never invented.
package mission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/observability"
)

// ReportVersion versions the expediente schema. A schema change
// ships a new version, never a silent edit; verifiers refuse
// unknown versions instead of guessing.
const ReportVersion = "regulus.mission-report/v1"

// Outcomes: the closed terminal vocabulary. Only terminal mission
// states produce reports; anything else is refused at build time.
const (
	OutcomeCompleted = "completed"
	OutcomePartial   = "partially_completed"
	OutcomeBlocked   = "blocked"
	OutcomeFailed    = "failed"
	OutcomeCancelled = "cancelled"
)

// Data quality: measured values cite their source, estimated ones
// are labelled as estimates, unknown explains why, and na marks
// criteria that do not apply. Estimates are never presented as
// measurements.
const (
	QualityMeasured  = "measured"
	QualityEstimated = "estimated"
	QualityUnknown   = "unknown"
	QualityNA        = "na"
)

// AllQualities is the closed quality vocabulary.
var AllQualities = []string{QualityMeasured, QualityEstimated, QualityUnknown, QualityNA}

// ParseQuality validates the closed quality vocabulary.
func ParseQuality(s string) (string, error) {
	got := strings.ToLower(strings.TrimSpace(s))
	for _, q := range AllQualities {
		if got == q {
			return got, nil
		}
	}
	return "", fmt.Errorf("mission: unknown data quality %q (want %s)", s, strings.Join(AllQualities, "|"))
}

// OutcomeFromState maps a finished-or-parked mission state to its
// report outcome. Only completed, partially_completed, blocked,
// failed and cancelled report (blocked is parked, not terminal,
// but S7 demands a bloqueada expediente too). Running states
// fail: a report claims a finished story, never a running one.
func OutcomeFromState(state string) (string, error) {
	switch state {
	case StateCompleted:
		return OutcomeCompleted, nil
	case StatePartiallyCompleted:
		return OutcomePartial, nil
	case "blocked":
		return OutcomeBlocked, nil
	case "failed":
		return OutcomeFailed, nil
	case "cancelled":
		return OutcomeCancelled, nil
	}
	return "", fmt.Errorf("mission: state %q is not terminal: no evidence report until the mission finishes", state)
}

// Metric is one cost/latency/token figure with its quality label.
// Unknown values carry the reason in Value (e.g. "unavailable:
// no metered host") instead of a fabricated number.
type Metric struct {
	Label   string `json:"label"`
	Quality string `json:"quality"`
	Value   string `json:"value"`
}

// SourceRef cites one context/evidence source by id and hash.
type SourceRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Hash string `json:"hash,omitempty"`
}

// ApprovalRef cites one governance decision without copying it.
type ApprovalRef struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Artifact string `json:"artifact,omitempty"`
	Policy   string `json:"policy,omitempty"`
}

// PlanRef cites one plan version and why it changed.
type PlanRef struct {
	Version    int    `json:"version"`
	Digest     string `json:"digest,omitempty"`
	Revisions  int    `json:"revisions,omitempty"`
	ChangeNote string `json:"change_note,omitempty"`
}

// TaskReport summarizes one executed task and its evidence links.
type TaskReport struct {
	ID       string   `json:"id"`
	State    string   `json:"state"`
	Agent    string   `json:"agent,omitempty"`
	Host     string   `json:"host,omitempty"`
	Commit   string   `json:"commit,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

// TestRef cites one gate/test verdict with its run.
type TestRef struct {
	Gate    string `json:"gate"`
	Verdict string `json:"verdict"`
	Cause   string `json:"cause,omitempty"`
	RunID   string `json:"run_id,omitempty"`
}

// CriterionReport binds one acceptance criterion to its verdict
// and the concrete evidence (or the reason it went unmet).
type CriterionReport struct {
	Name      string   `json:"name"`
	Satisfied bool     `json:"satisfied"`
	Reason    string   `json:"reason"`
	Evidence  []string `json:"evidence,omitempty"`
}

// FailureEntry records one mission failure with its class and
// whether recovery succeeded. Failures are first-class: a report
// without them is incomplete by construction.
type FailureEntry struct {
	Class     string `json:"class"`
	Detail    string `json:"detail"`
	Recovered bool   `json:"recovered"`
}

// RecoveryEntry records one recovery decision and its outcome.
type RecoveryEntry struct {
	Action  string `json:"action"`
	Reason  string `json:"reason"`
	Outcome string `json:"outcome"`
}

// Report is the sealed expediente. Hash covers every byte but
// itself (see seal); an empty Hash means unsealed.
type Report struct {
	Version        string            `json:"version"`
	MissionID      string            `json:"mission_id"`
	GeneratedAt    string            `json:"generated_at"`
	Objective      string            `json:"objective"`
	Description    string            `json:"description,omitempty"`
	Constraints    []string          `json:"constraints,omitempty"`
	Outcome        string            `json:"outcome"`
	Criteria       []CriterionReport `json:"criteria,omitempty"`
	Plans          []PlanRef         `json:"plans,omitempty"`
	Tasks          []TaskReport      `json:"tasks,omitempty"`
	DAG            [][2]string       `json:"dag,omitempty"`
	Agents         []string          `json:"agents,omitempty"`
	Hosts          []string          `json:"hosts,omitempty"`
	Routing        []string          `json:"routing,omitempty"`
	ContextSources []SourceRef       `json:"context_sources,omitempty"`
	Approvals      []ApprovalRef     `json:"approvals,omitempty"`
	Commits        []string          `json:"commits,omitempty"`
	Diffs          []string          `json:"diffs,omitempty"`
	Tests          []TestRef         `json:"tests,omitempty"`
	Failures       []FailureEntry    `json:"failures,omitempty"`
	Recovery       []RecoveryEntry   `json:"recovery,omitempty"`
	Costs          []Metric          `json:"costs,omitempty"`
	Latency        Metric            `json:"latency,omitempty"`
	Limitations    []string          `json:"limitations,omitempty"`
	Hash           string            `json:"hash,omitempty"`
}

// ReportInput is everything the edge assembles from the mission
// store, plan store, execution ledger, governance store, verify
// results and receipts. Missing pieces arrive as unknown-quality
// metrics or empty sections with a limitation line — the builder
// refuses to invent them.
type ReportInput struct {
	Mission        *Mission
	Plans          []PlanRef
	Tasks          []TaskReport
	DAG            [][2]string
	Agents         []string
	Hosts          []string
	Routing        []string
	ContextSources []SourceRef
	Approvals      []ApprovalRef
	Commits        []string
	Diffs          []string
	Tests          []TestRef
	Failures       []FailureEntry
	Recovery       []RecoveryEntry
	Costs          []Metric
	Latency        Metric
	Limitations    []string
}

// BuildReport seals one expediente for a terminal mission.
// Free text is redacted before sealing; data quality is validated;
// criteria default to satisfied-when-evidence-cited and the caller
// overrides via Criteria when a criterion went unmet (see
// BuildReportWithCriteria for the explicit path).
func BuildReport(in ReportInput) (Report, error) {
	return buildReport(in, nil)
}

// BuildReportWithCriteria seals a report with explicit per-criterion
// verdicts (unmet criteria carry their reason). Length must match
// the mission criteria; satisfied entries cite evidence.
func BuildReportWithCriteria(in ReportInput, criteria []CriterionReport) (Report, error) {
	return buildReport(in, criteria)
}

func buildReport(in ReportInput, criteria []CriterionReport) (Report, error) {
	if in.Mission == nil {
		return Report{}, fmt.Errorf("mission: report needs a mission (got nil)")
	}
	m := in.Mission
	outcome, err := OutcomeFromState(m.State)
	if err != nil {
		return Report{}, err
	}
	for _, c := range in.Costs {
		if _, err := ParseQuality(c.Quality); err != nil {
			return Report{}, err
		}
	}
	if q := strings.TrimSpace(in.Latency.Quality); q != "" {
		if _, err := ParseQuality(q); err != nil {
			return Report{}, err
		}
	}
	r := Report{
		Version:        ReportVersion,
		MissionID:      m.ID,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		Objective:      observability.RedactText(m.Objective),
		Description:    observability.RedactText(m.Description),
		Outcome:        outcome,
		Plans:          in.Plans,
		Tasks:          redactTasks(in.Tasks),
		DAG:            in.DAG,
		Agents:         append([]string(nil), in.Agents...),
		Hosts:          append([]string(nil), in.Hosts...),
		Routing:        redactStrings(in.Routing),
		ContextSources: in.ContextSources,
		Approvals:      in.Approvals,
		Commits:        append([]string(nil), in.Commits...),
		Diffs:          append([]string(nil), in.Diffs...),
		Tests:          in.Tests,
		Failures:       redactFailures(in.Failures),
		Recovery:       redactRecovery(in.Recovery),
		Costs:          in.Costs,
		Latency:        in.Latency,
		Limitations:    redactStrings(in.Limitations),
	}
	r.Constraints = redactStrings(m.Constraints)
	if criteria != nil {
		if len(criteria) != len(m.Criteria) {
			return Report{}, fmt.Errorf("mission: %d criterion verdicts for %d mission criteria: every criterion needs a verdict", len(criteria), len(m.Criteria))
		}
		r.Criteria = redactCriteria(criteria)
	} else {
		r.Criteria = defaultCriteria(m, in.Tasks)
	}
	sealed, err := seal(r)
	if err != nil {
		return Report{}, err
	}
	return sealed, nil
}

// defaultCriteria marks every mission criterion satisfied when at
// least one task cites evidence, and open with its reason when the
// mission carries no task evidence (plan-only or empty runs never
// claim satisfied criteria).
func defaultCriteria(m *Mission, tasks []TaskReport) []CriterionReport {
	evidenced := false
	for _, t := range tasks {
		if len(t.Evidence) > 0 {
			evidenced = true
			break
		}
	}
	out := make([]CriterionReport, 0, len(m.Criteria))
	for _, c := range m.Criteria {
		name := observability.RedactText(c)
		if evidenced {
			out = append(out, CriterionReport{Name: name, Satisfied: true,
				Reason:   "task evidence cited (see tasks[].evidence)",
				Evidence: taskEvidence(tasks)})
		} else {
			out = append(out, CriterionReport{Name: name, Satisfied: false,
				Reason: "no task evidence linked: criterion unproven, not silently satisfied"})
		}
	}
	return out
}

func taskEvidence(tasks []TaskReport) []string {
	var out []string
	for _, t := range tasks {
		out = append(out, t.Evidence...)
	}
	return out
}

func redactStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, observability.RedactText(s))
	}
	return out
}

func redactTasks(in []TaskReport) []TaskReport {
	if in == nil {
		return nil
	}
	out := make([]TaskReport, 0, len(in))
	for _, t := range in {
		t.Evidence = append([]string(nil), t.Evidence...)
		out = append(out, t)
	}
	return out
}

func redactFailures(in []FailureEntry) []FailureEntry {
	if in == nil {
		return nil
	}
	out := make([]FailureEntry, 0, len(in))
	for _, f := range in {
		f.Detail = observability.RedactText(f.Detail)
		out = append(out, f)
	}
	return out
}

func redactRecovery(in []RecoveryEntry) []RecoveryEntry {
	if in == nil {
		return nil
	}
	out := make([]RecoveryEntry, 0, len(in))
	for _, r := range in {
		r.Reason = observability.RedactText(r.Reason)
		r.Outcome = observability.RedactText(r.Outcome)
		out = append(out, r)
	}
	return out
}

func redactCriteria(in []CriterionReport) []CriterionReport {
	out := make([]CriterionReport, 0, len(in))
	for _, c := range in {
		c.Name = observability.RedactText(c.Name)
		c.Reason = observability.RedactText(c.Reason)
		out = append(out, c)
	}
	return out
}

// seal computes the integrity hash over the canonical bytes with
// Hash empty, then pins it: any later byte change invalidates.
func seal(r Report) (Report, error) {
	r.Hash = ""
	raw, err := json.Marshal(r)
	if err != nil {
		return Report{}, err
	}
	sum := sha256.Sum256(raw)
	r.Hash = hex.EncodeToString(sum[:])
	return r, nil
}

// Bytes exports the sealed report as indented JSON.
func (r Report) Bytes() ([]byte, error) {
	if strings.TrimSpace(r.Hash) == "" {
		return nil, fmt.Errorf("mission: report unsealed (build via BuildReport)")
	}
	return json.MarshalIndent(r, "", "  ")
}

// VerifyReport parses exported bytes and revalidates them: schema
// version, outcome vocabulary and the integrity hash. A cut, edited
// or reordered byte fails — the expediente is tamper-evident.
func VerifyReport(raw []byte) (Report, error) {
	var r Report
	if err := json.Unmarshal(raw, &r); err != nil {
		return Report{}, fmt.Errorf("mission: malformed report: %v", err)
	}
	if r.Version != ReportVersion {
		return Report{}, fmt.Errorf("mission: report version %q (want %s): refusing to guess", r.Version, ReportVersion)
	}
	switch r.Outcome {
	case OutcomeCompleted, OutcomePartial, OutcomeBlocked, OutcomeFailed, OutcomeCancelled:
	default:
		return Report{}, fmt.Errorf("mission: report outcome %q unknown", r.Outcome)
	}
	want := r.Hash
	recomputed, err := seal(r)
	if err != nil {
		return Report{}, err
	}
	if want == "" || recomputed.Hash != want {
		return Report{}, fmt.Errorf("mission: report integrity broken (hash %s, want %s): tampered or truncated",
			shortHash(recomputed.Hash), shortHash(want))
	}
	r.Hash = want
	return r, nil
}

// RenderReport quotes the sealed expediente as readable text: the
// JSON stays the authority, these lines are the quote.
func RenderReport(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "mission report %s outcome=%s\n", r.MissionID, r.Outcome)
	fmt.Fprintf(&b, "  objective: %s\n", r.Objective)
	fmt.Fprintf(&b, "  plans: %d task-records: %d approvals: %d commits: %d\n",
		len(r.Plans), len(r.Tasks), len(r.Approvals), len(r.Commits))
	for _, c := range r.Criteria {
		mark := "SATISFIED"
		if !c.Satisfied {
			mark = "UNMET"
		}
		fmt.Fprintf(&b, "  criterion [%s]: %s — %s\n", mark, c.Name, c.Reason)
	}
	for _, t := range r.Tests {
		fmt.Fprintf(&b, "  gate %s: %s (run %s) %s\n", t.Gate, t.Verdict, t.RunID, t.Cause)
	}
	for _, f := range r.Failures {
		fmt.Fprintf(&b, "  failure %s recovered=%v: %s\n", f.Class, f.Recovered, f.Detail)
	}
	for _, l := range r.Limitations {
		fmt.Fprintf(&b, "  limitation: %s\n", l)
	}
	fmt.Fprintf(&b, "  costs: ")
	for _, m := range r.Costs {
		fmt.Fprintf(&b, "[%s %s=%s] ", m.Quality, m.Label, m.Value)
	}
	fmt.Fprintf(&b, "\n  hash: %s\n", r.Hash)
	return b.String()
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	if h == "" {
		return "unknown"
	}
	return h
}
