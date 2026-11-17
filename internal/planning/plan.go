// Package planning implements F13.2/NOV-105: the Autonomous Mission
// Planner. It turns a Mission (F13.1) into a versioned, executable
// plan with hypotheses, assumptions, risks, open questions,
// deliverables and Given/When/Then criteria.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - mission: the durable entity (objective, constraints, criteria,
//     risk, budget) this planner reads, never redefines;
//   - context: Inventory (F12.3) owns the corpus walk; the planner
//     only scores its fragments for scope inference;
//   - impact: Census/BuildGraph/MapDiff (F12.8) own the dependency
//     evidence; the planner only cites nodes, tests and consumers;
//   - orchestrate: TaskDescriptor validation, Decide routing over
//     F11.7-verified capabilities, PlanParallelLevels ordering with
//     collision serialisation (F12.1/F12.2); the planner maps its
//     tasks onto ParallelTask instead of re-implementing the DAG;
//   - bench: DefaultHostManifests/EvidenceStamp grade capabilities;
//     only estado probada with dated evidence is ever assigned;
//   - mcp: the closed F11.8 scope vocabulary; minimal scopes are
//     always a subset of the assigned agent's scopes;
//   - fsx: LockDir serialises versioned plan writes, the same
//     primitive the mission store uses.
//
// Estimates are always labelled as estimates, never measurements:
// every Estimate carries Note "estimación, no medición". Unknown
// money stays unknown (CostKnown=false), never zero.
package planning

import (
	"sort"
	"strings"
	"time"
)

// PlanVersion pins the plan document shape: revisions compare only
// within one version.
const PlanVersion = "plan-v1"

// PlanFormatVersion is the integer the store orders by.
const PlanFormatVersion = 1

// Request kinds: the closed classification vocabulary.
const (
	KindSimple      = "simple"
	KindComplex     = "complex"
	KindAmbiguous   = "ambiguous"
	KindAdversarial = "adversarial"
)

// Plan statuses: the closed readiness vocabulary.
const (
	StatusReady              = "ready"
	StatusNeedsClarification = "needs-clarification"
	StatusNeedsApproval      = "needs-approval"
	StatusRefused            = "refused"
)

// Task autonomy: who may run this task and how.
const (
	AutonomyAutonomous    = "autonomous"
	AutonomyReversible    = "reversible"
	AutonomyNeedsApproval = "needs-approval"
	AutonomyNeedsUser     = "needs-user"
)

// MaxTasks bounds one plan: the DAG, the report and the review all
// stay readable.
const MaxTasks = 12

// MaxScopeFiles bounds the file scope of one task.
const MaxScopeFiles = 16

// EstimateNote is stamped on every estimate so a budget can never
// be mistaken for a measurement.
const EstimateNote = "estimación, no medición: orden de magnitud para planificar, nunca un compromiso"

// Estimate is one labelled task budget: complexity and risk words
// plus wall-clock, token and money ceilings. CostKnown=false means
// the money is unknown (never zero, never a gate).
type Estimate struct {
	Complexity string  `json:"complexity"`
	Risk       string  `json:"risk"`
	TimeMS     int64   `json:"time_ms"`
	Tokens     int64   `json:"tokens"`
	Cost       float64 `json:"cost"`
	CostKnown  bool    `json:"cost_known"`
	Currency   string  `json:"currency,omitempty"`
	Note       string  `json:"note"`
}

// Scope is what one task may touch: repo-relative files plus the
// package/API components they belong to.
type Scope struct {
	Files      []string `json:"files"`
	Components []string `json:"components"`
}

// Verification is how one task proves itself: the strategy plus
// the Given/When/Then criteria it must satisfy.
type Verification struct {
	Strategy string   `json:"strategy"`
	Criteria []string `json:"criteria"`
}

// PlanTask is one executable unit: objective, deliverable,
// dependencies, scope, verified agent/capability, minimal scopes,
// context, tests, labelled budget and autonomy.
type PlanTask struct {
	ID                   string       `json:"id"`
	Title                string       `json:"title"`
	Objective            string       `json:"objective"`
	Deliverable          string       `json:"deliverable"`
	DependsOn            []string     `json:"depends_on,omitempty"`
	Excludes             []string     `json:"excludes,omitempty"`
	Scope                Scope        `json:"scope"`
	Agent                string       `json:"agent,omitempty"`
	Reviewer             string       `json:"reviewer,omitempty"`
	RequiredCapabilities []string     `json:"required_capabilities"`
	CapabilityEvidence   []string     `json:"capability_evidence,omitempty"`
	MinScopes            []string     `json:"min_scopes"`
	ContextRefs          []string     `json:"context_refs,omitempty"`
	Tests                []string     `json:"tests,omitempty"`
	Verification         Verification `json:"verification"`
	Estimate             Estimate     `json:"estimate"`
	Autonomy             string       `json:"autonomy"`
	Blockers             []string     `json:"blockers,omitempty"`
	ImpactKnown          bool         `json:"impact_known"`
	Rationale            string       `json:"rationale,omitempty"`
}

// Question is one open question: what is missing, why it matters
// and what it blocks.
type Question struct {
	Question string `json:"question"`
	Why      string `json:"why"`
	Blocks   string `json:"blocks,omitempty"`
}

// Criterion is one acceptance criterion with its verifiability
// verdict: Given/When/Then-shaped and free of unmeasurable glue.
type Criterion struct {
	Text           string `json:"text"`
	GivenWhenThen  bool   `json:"given_when_then"`
	Verifiable     bool   `json:"verifiable"`
	CoveringTask   string `json:"covering_task,omitempty"`
	UnverifiableWs string `json:"unverifiable_why,omitempty"`
}

// ConfirmedResult is one already-confirmed task outcome carried
// across replans: replan never drops them, only appends.
type ConfirmedResult struct {
	TaskID string    `json:"task_id"`
	Result string    `json:"result"`
	At     time.Time `json:"at"`
}

// Check is one quality gate verdict.
type Check struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
}

// Quality is the plan quality report: named checks plus warnings.
type Quality struct {
	Checks   []Check  `json:"checks"`
	Warnings []string `json:"warnings,omitempty"`
}

// DAG is the executable order: deterministic levels, the merge
// order, the serialised collisions and any detected cycles.
type DAG struct {
	Levels     [][]string  `json:"levels"`
	Order      []string    `json:"order"`
	Collisions [][2]string `json:"collisions,omitempty"`
	Cycles     [][]string  `json:"cycles,omitempty"`
}

// Plan is one versioned mission plan: intent, analysis, the task
// DAG, the quality report and the reason this version exists.
type Plan struct {
	Version        string            `json:"version"`
	PlanVersion    int               `json:"plan_version"`
	MissionID      string            `json:"mission_id"`
	Objective      string            `json:"objective"`
	Kind           string            `json:"kind"`
	Status         string            `json:"status"`
	CreatedAt      time.Time         `json:"created_at"`
	Author         string            `json:"author,omitempty"`
	ChangeReason   string            `json:"change_reason,omitempty"`
	Hypotheses     []string          `json:"hypotheses,omitempty"`
	Assumptions    []string          `json:"assumptions,omitempty"`
	Risks          []string          `json:"risks,omitempty"`
	OpenQuestions  []Question        `json:"open_questions,omitempty"`
	Deliverables   []string          `json:"deliverables,omitempty"`
	Criteria       []Criterion       `json:"criteria,omitempty"`
	Tasks          []PlanTask        `json:"tasks"`
	DAG            DAG               `json:"dag"`
	Exclusions     []string          `json:"exclusions,omitempty"`
	Confirmed      []ConfirmedResult `json:"confirmed,omitempty"`
	Quality        Quality           `json:"quality"`
	RepoHead       string            `json:"repo_head,omitempty"`
	WorktreeStatus string            `json:"worktree_status,omitempty"`
}

// validKind reports whether kind belongs to the closed request
// vocabulary.
func validKind(k string) bool {
	switch k {
	case KindSimple, KindComplex, KindAmbiguous, KindAdversarial:
		return true
	}
	return false
}

// validStatus reports whether status belongs to the closed
// readiness vocabulary.
func validStatus(s string) bool {
	switch s {
	case StatusReady, StatusNeedsClarification, StatusNeedsApproval, StatusRefused:
		return true
	}
	return false
}

// validAutonomy reports whether autonomy belongs to the closed
// task vocabulary.
func validAutonomy(a string) bool {
	switch a {
	case AutonomyAutonomous, AutonomyReversible, AutonomyNeedsApproval, AutonomyNeedsUser:
		return true
	}
	return false
}

// hasGWTShape reports whether text carries a Given/When/Then
// structure (English or Spanish markers), the only criterion
// shape the planner calls verifiable-by-construction.
func hasGWTShape(text string) bool {
	lower := strings.ToLower(text)
	hasGiven := strings.Contains(lower, "given") || strings.Contains(lower, "dado ")
	hasWhen := strings.Contains(lower, "when") || strings.Contains(lower, "cuando")
	hasThen := strings.Contains(lower, "then") || strings.Contains(lower, "entonces")
	return hasGiven && hasWhen && hasThen
}

// vagueMarkers are unmeasurable glue words: a criterion leaning on
// them without a number, a named test or a GWT shape never counts
// as verifiable — the planner asks instead of inventing the metric.
var vagueMarkers = []string{
	"bien", "mejor", "rápido", "rapido", "óptimo", "optimo",
	"limpio", "robusto", "eficiente", "bonito", "fácil", "facil",
	"well", "better", "fast", "clean", "robust", "efficient", "nice",
}

// hasMeasure reports whether text names a number, a percentage, a
// test command/id or an explicit artefact — something a checker
// can observe without guessing.
func hasMeasure(text string) bool {
	lower := strings.ToLower(text)
	for _, r := range text {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	for _, m := range []string{"test:", "go test", "regulus ", "must ", "debe ", "exactly", "exactamente"} {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// AssessCriterion grades one acceptance criterion: GWT-shaped and
// (measured or concrete) reads as verifiable; anything else names
// why and forces a clarifying question instead of a silent pass.
func AssessCriterion(text string) Criterion {
	c := Criterion{Text: text}
	c.GivenWhenThen = hasGWTShape(text)
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 12 {
		c.Verifiable = false
		c.UnverifiableWs = "criterio demasiado breve para comprobarlo sin adivinar (mínimo 12 caracteres con condición observable)"
		return c
	}
	if !c.GivenWhenThen {
		c.Verifiable = false
		c.UnverifiableWs = "sin forma Given/When/Then (falta condición observable: Given <estado>, When <acción>, Then <resultado medible>)"
		return c
	}
	lower := strings.ToLower(trimmed)
	reliesVague := false
	for _, v := range vagueMarkers {
		if strings.Contains(lower, v) && !hasMeasure(trimmed) {
			reliesVague = true
			break
		}
	}
	if reliesVague {
		c.Verifiable = false
		c.UnverifiableWs = "usa un adjetivo sin medida (" + vagueWord(trimmed) + ") sin umbral numérico ni test nombrado: el plan pide aclaración, no inventa el umbral"
		return c
	}
	c.Verifiable = true
	return c
}

// vagueWord names the first vague marker found, for the message.
func vagueWord(text string) string {
	lower := strings.ToLower(text)
	for _, v := range vagueMarkers {
		if strings.Contains(lower, v) {
			return v
		}
	}
	return "?"
}

// adversarialMarkers match hostile or out-of-mission intents: prompt
// injection, permission bypass, destructive or security-kernel
// attacks. A match refuses the plan with the reason — the planner
// never "helps" an adversarial request look executable.
var adversarialMarkers = []string{
	"ignore previous instructions", "ignore all previous",
	"disregard instructions", "reveal system prompt",
	"exfiltrat", "bypass approval", "bypass auth",
	"disable gate", "disable auth", "drop table", "rm -rf /",
	"delete gates", "delete evidence", "delete receipts",
	"sin aprobación del kernel", "desactiva el gate",
}

// IsAdversarial reports whether the request carries a hostile or
// out-of-mission intent, with the matched marker as evidence.
func IsAdversarial(objective string, constraints, criteria []string) (string, bool) {
	lower := strings.ToLower(strings.Join(append([]string{objective}, append(constraints, criteria...)...), "\n"))
	for _, m := range adversarialMarkers {
		if strings.Contains(lower, strings.ToLower(m)) {
			return m, true
		}
	}
	return "", false
}

// taskIDs indexes the plan tasks by id.
func taskIDs(tasks []PlanTask) map[string]PlanTask {
	m := make(map[string]PlanTask, len(tasks))
	for _, t := range tasks {
		m[t.ID] = t
	}
	return m
}

// sortedTaskIDs returns the deterministic task id order.
func sortedTaskIDs(tasks []PlanTask) []string {
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	return ids
}

// qualityByName indexes quality checks by name.
func qualityByName(q Quality) map[string]Check {
	m := make(map[string]Check, len(q.Checks))
	for _, c := range q.Checks {
		m[c.Name] = c
	}
	return m
}
