// Plan coverage, quality gates, validation and version diff
// (F13.2/NOV-105): the grading half of plan construction. Coverage
// is structural (the verify task carries every mission criterion
// verbatim), validation fails closed before anything executes, and
// DiffPlans renders exactly what a revision changed and why.
package planning

import (
	"fmt"
	"sort"
	"strings"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
)

// coverCriteria links every criterion to its covering task: the
// verify task carries all mission criteria verbatim, so coverage
// is structural — an uncovered criterion is a no-omission bug.
func coverCriteria(p *Plan) {
	verifier := ""
	for _, t := range p.Tasks {
		if taskKindOf(t) == taskVerify {
			verifier = t.ID
		}
	}
	if verifier == "" && len(p.Tasks) > 0 {
		verifier = p.Tasks[len(p.Tasks)-1].ID
	}
	for i := range p.Criteria {
		if verifier != "" {
			p.Criteria[i].CoveringTask = verifier
		}
	}
}

// missionRiskLine names the mission risk ceiling every task inherits.
func missionRiskLine(m *mission.Mission) string {
	return fmt.Sprintf("riesgo de misión %s: las tareas heredan el techo y las de riesgo alto/crítico exigen aprobación", m.Risk)
}

// criticalTaskIDs lists the tasks touching critical packages, sorted.
func criticalTaskIDs(tasks []PlanTask) []string {
	crit := map[string]bool{}
	for _, t := range tasks {
		for _, b := range t.Blockers {
			if strings.Contains(b, "crítico") {
				crit[t.ID] = true
			}
		}
		if strings.Contains(t.Rationale, "crítico") {
			crit[t.ID] = true
		}
	}
	if len(crit) == 0 {
		return nil
	}
	ids := make([]string, 0, len(crit))
	for id := range crit {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// criticalRiskLine names the critical-package risk when any task touches one.
func criticalRiskLine(tasks []PlanTask) (string, bool) {
	ids := criticalTaskIDs(tasks)
	if len(ids) == 0 {
		return "", false
	}
	return fmt.Sprintf("paquetes críticos bajo el alcance (tareas %s): gates/evidence/receipts/manifest/verdict/contract nunca se degradan", strings.Join(ids, ", ")), true
}

// countCollisions counts the tasks serialized by file collisions.
func countCollisions(tasks []PlanTask) int {
	coll := 0
	for _, t := range tasks {
		for _, b := range t.Blockers {
			if strings.HasPrefix(b, "serialización:") {
				coll++
				break
			}
		}
	}
	return coll
}

// collisionRiskLine names the serialization risk when collisions exist.
func collisionRiskLine(tasks []PlanTask) (string, bool) {
	coll := countCollisions(tasks)
	if coll == 0 {
		return "", false
	}
	return fmt.Sprintf("%d tarea(s) serializada(s) por colisión de ficheros: el orden es obligatorio, no optimizable en paralelo", coll), true
}

// graphRiskLine names the unknown-impact risk when the graph is missing.
func graphRiskLine(graphOK bool) (string, bool) {
	if graphOK {
		return "", false
	}
	return "grafo F12.8 no disponible: impacto tratado como desconocido (se pide aprobación donde el impacto no se verifica)", true
}

// unknownRiskLine names the unmapped-terms risk when the corpus matched nothing.
func unknownRiskLine(hit scopeHit) (string, bool) {
	if len(hit.unknown) == 0 {
		return "", false
	}
	return fmt.Sprintf("términos sin correspondencia en el corpus (%s): no se inventó alcance para ellos", strings.Join(headFiles(hit.unknown, 6), ", ")), true
}

// budgetRiskLine names the budget-pressure risk when the mission sets a ceiling.
func budgetRiskLine(m *mission.Mission, tasks []PlanTask) (string, bool) {
	if m.Budget.TimeMS == 0 && m.Budget.Tokens == 0 && m.Budget.Cost == 0 {
		return "", false
	}
	var total int64
	for _, t := range tasks {
		total += t.Estimate.Tokens
	}
	return fmt.Sprintf("presupuesto estimado total %d tokens (estimación, no medición) frente al techo de misión tokens=%d", total, m.Budget.Tokens), true
}

// risksFor names the plan risks explicitly: mission risk,
// critical packages, collisions, budget pressure and unknowns.
func risksFor(m *mission.Mission, hit scopeHit, graphOK bool, tasks []PlanTask) []string {
	r := []string{missionRiskLine(m)}
	if line, ok := criticalRiskLine(tasks); ok {
		r = append(r, line)
	}
	if line, ok := collisionRiskLine(tasks); ok {
		r = append(r, line)
	}
	if line, ok := graphRiskLine(graphOK); ok {
		r = append(r, line)
	}
	if line, ok := unknownRiskLine(hit); ok {
		r = append(r, line)
	}
	if line, ok := budgetRiskLine(m, tasks); ok {
		r = append(r, line)
	}
	return r
}

// questionsFor collects the blocking questions: unverifiable
// criteria, needs-user tasks and unknown impacts.
func questionsFor(p *Plan, hit scopeHit) []Question {
	var q []Question
	for _, c := range p.Criteria {
		if !c.Verifiable {
			q = append(q, Question{
				Question: fmt.Sprintf("¿Cómo se comprueba %q?", trimWords(c.Text, 10)),
				Why:      c.UnverifiableWs,
				Blocks:   "verificación" + coverSuffix(c),
			})
		}
	}
	for _, t := range p.Tasks {
		if t.Autonomy == AutonomyNeedsUser && !t.ImpactKnown {
			q = append(q, Question{
				Question: fmt.Sprintf("¿Qué alcance exacto tiene %q (%s)? El impacto no se verifica contra el grafo.", t.ID, t.Title),
				Why:      "impacto desconocido: sin evidencia de cobertura no hay ejecución autónoma",
				Blocks:   t.ID,
			})
		}
	}
	if len(hit.unknown) > 3 {
		q = append(q, Question{
			Question: fmt.Sprintf("Los términos %s no aparecen en el corpus: ¿qué componentes designan?", strings.Join(headFiles(hit.unknown, 6), ", ")),
			Why:      "alcance no cubierto: lo no mapeado no se planifica por defecto",
			Blocks:   "alcance",
		})
	}
	return q
}

// coverSuffix names the covering task for a question, if any.
func coverSuffix(c Criterion) string {
	if c.CoveringTask != "" {
		return " (cubre " + c.CoveringTask + ")"
	}
	return ""
}

// gradeQuality runs the named quality gates over the finished plan.
func gradeQuality(p *Plan, reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest) Quality {
	q := Quality{}
	byAgent := map[string]orchestrate.RoutedAgent{}
	for _, a := range reg {
		byAgent[a.ID] = a
	}
	check := func(name string, pass bool, detail string) {
		q.Checks = append(q.Checks, Check{Name: name, Pass: pass, Detail: detail})
	}
	if p.Kind == KindAdversarial {
		check("plan-no-vacio", false, "rehusado a propósito: cero tareas sobre solicitud adversarial")
		check("capabilities-verificadas", true, "nada asignado sin evidencia")
		return q
	}
	check("plan-no-vacio", len(p.Tasks) > 0,
		fmt.Sprintf("%d tarea(s) planificada(s)", len(p.Tasks)))
	uncovered := []string{}
	for _, c := range p.Criteria {
		if c.CoveringTask == "" {
			uncovered = append(uncovered, trimWords(c.Text, 8))
		}
	}
	check("criterios-cubiertos", len(uncovered) == 0,
		func() string {
			if len(uncovered) == 0 {
				return fmt.Sprintf("los %d criterio(s) tienen tarea covering (no-omisión)", len(p.Criteria))
			}
			return "sin covering: " + strings.Join(uncovered, "; ")
		}())
	badCrit := []string{}
	for _, c := range p.Criteria {
		if !c.Verifiable {
			badCrit = append(badCrit, trimWords(c.Text, 8))
		}
	}
	check("criterios-verificables", len(badCrit) == 0,
		func() string {
			if len(badCrit) == 0 {
				return "todo criterio es Given/When/Then medible"
			}
			return "ambiguos (bloquean antes de ejecutar): " + strings.Join(badCrit, "; ")
		}())
	badCaps := []string{}
	for _, t := range p.Tasks {
		for _, c := range t.RequiredCapabilities {
			if !verifiedCap(manifests, c) {
				badCaps = append(badCaps, t.ID+":"+c)
			}
		}
	}
	check("capabilities-verificadas", len(badCaps) == 0,
		func() string {
			if len(badCaps) == 0 {
				return "toda capability exigida está probada con evidencia fechada"
			}
			return "sin verificar: " + strings.Join(badCaps, ", ")
		}())
	badScopes := []string{}
	for _, t := range p.Tasks {
		a, ok := byAgent[t.Agent]
		if t.Agent == "" || !ok {
			if t.Autonomy == AutonomyNeedsUser {
				continue
			}
			badScopes = append(badScopes, t.ID+":sin-agente")
			continue
		}
		if !scopesHold(a.Scopes, t.MinScopes) {
			badScopes = append(badScopes, t.ID+":excede-"+t.Agent)
		}
	}
	check("scopes-minimos", len(badScopes) == 0,
		func() string {
			if len(badScopes) == 0 {
				return "toda tarea pide un subconjunto de los scopes de su agente"
			}
			return "excesos: " + strings.Join(badScopes, ", ")
		}())
	check("dag-aciclico", len(p.DAG.Cycles) == 0,
		func() string {
			if len(p.DAG.Cycles) == 0 {
				return fmt.Sprintf("orden %s en %d nivel(es)", strings.Join(p.DAG.Order, ","), len(p.DAG.Levels))
			}
			return fmt.Sprintf("%d ciclo(s) detectado(s)", len(p.DAG.Cycles))
		}())
	unknown := []string{}
	for _, t := range p.Tasks {
		if !t.ImpactKnown {
			unknown = append(unknown, t.ID)
		}
	}
	check("impacto-conocido", len(unknown) == 0,
		func() string {
			if len(unknown) == 0 {
				return "todo el alcance se verifica contra corpus y grafo"
			}
			return "impacto sin verificar en: " + strings.Join(unknown, ", ") + " (pide aclaración/aprobación)"
		}())
	unlabelled := []string{}
	for _, t := range p.Tasks {
		if !strings.Contains(t.Estimate.Note, "estimación") {
			unlabelled = append(unlabelled, t.ID)
		}
	}
	check("estimaciones-etiquetadas", len(unlabelled) == 0,
		func() string {
			if len(unlabelled) == 0 {
				return "toda estimación lleva la etiqueta «estimación, no medición»"
			}
			return "sin etiqueta: " + strings.Join(unlabelled, ", ")
		}())
	for _, c := range q.Checks {
		if !c.Pass && c.Name != "impacto-conocido" && c.Name != "criterios-verificables" {
			continue
		}
		if !c.Pass {
			q.Warnings = append(q.Warnings, c.Name+": "+c.Detail)
		}
	}
	return q
}

// verifiedCap reports whether capability id is probada with dated
// evidence in any host manifest.
func verifiedCap(manifests map[string]bench.HostManifest, id string) bool {
	for _, m := range manifests {
		if c, ok := m.Capability(id); ok && c.Status == bench.CapVerified && strings.TrimSpace(c.Evidence) != "" {
			return true
		}
	}
	return false
}

// deriveStatus folds kind, criteria, autonomy and quality into the
// plan status: ready only when everything is verifiable, covered,
// verified and known.
func deriveStatus(p *Plan) string {
	if p.Kind == KindAdversarial {
		return StatusRefused
	}
	if p.Kind == KindAmbiguous {
		return StatusNeedsClarification
	}
	byName := map[string]Check{}
	for _, c := range p.Quality.Checks {
		byName[c.Name] = c
	}
	if c, ok := byName["plan-no-vacio"]; ok && !c.Pass {
		return StatusNeedsClarification
	}
	if c, ok := byName["criterios-cubiertos"]; ok && !c.Pass {
		return StatusNeedsClarification
	}
	if c, ok := byName["criterios-verificables"]; ok && !c.Pass {
		return StatusNeedsClarification
	}
	for _, t := range p.Tasks {
		if t.Autonomy == AutonomyNeedsUser {
			return StatusNeedsClarification
		}
	}
	for _, t := range p.Tasks {
		if t.Autonomy == AutonomyNeedsApproval {
			return StatusNeedsApproval
		}
	}
	if c, ok := byName["impacto-conocido"]; ok && !c.Pass {
		return StatusNeedsApproval
	}
	return StatusReady
}

// validatePlanHeader refuses a nil or mis-versioned plan before any task runs.
func validatePlanHeader(p *Plan) error {
	if p == nil {
		return fmt.Errorf("planning: plan nulo (nada que validar)")
	}
	if p.Version != PlanVersion {
		return fmt.Errorf("planning: versión %q, se exige %q", p.Version, PlanVersion)
	}
	if !validKind(p.Kind) {
		return fmt.Errorf("planning: kind %q desconocido", p.Kind)
	}
	if !validStatus(p.Status) {
		return fmt.Errorf("planning: status %q desconocido", p.Status)
	}
	return nil
}

// validateAdversarialPlan refuses any task on a refused plan.
func validateAdversarialPlan(p *Plan) error {
	if len(p.Tasks) != 0 {
		return fmt.Errorf("planning: plan adversarial con %d tareas (rehusar = cero tareas)", len(p.Tasks))
	}
	return nil
}

// validatePlanSize refuses empty and oversized plans.
func validatePlanSize(p *Plan) error {
	if len(p.Tasks) == 0 {
		return fmt.Errorf("planning: plan %s sin tareas (vacío no es ejecutable)", p.Kind)
	}
	if len(p.Tasks) > MaxTasks {
		return fmt.Errorf("planning: %d tareas, máximo %d", len(p.Tasks), MaxTasks)
	}
	return nil
}

// validateTaskContent checks objective, deliverable and autonomy words.
func validateTaskContent(t PlanTask) error {
	if strings.TrimSpace(t.Objective) == "" {
		return fmt.Errorf("planning: tarea %q sin objetivo", t.ID)
	}
	if strings.TrimSpace(t.Deliverable) == "" {
		return fmt.Errorf("planning: tarea %q sin entregable", t.ID)
	}
	if !validAutonomy(t.Autonomy) {
		return fmt.Errorf("planning: tarea %q autonomía %q desconocida", t.ID, t.Autonomy)
	}
	return nil
}

// validateTaskScope checks the declared file scope stays repo-relative.
func validateTaskScope(t PlanTask) error {
	if len(t.Scope.Files) == 0 && t.Autonomy != AutonomyNeedsUser {
		return fmt.Errorf("planning: tarea %q sin scope de ficheros (el alcance se declara, no se adivina)", t.ID)
	}
	for _, f := range t.Scope.Files {
		if strings.TrimSpace(f) == "" || strings.Contains(f, "..") {
			return fmt.Errorf("planning: tarea %q fichero %q inválido (repo-relativo, sin ..)", t.ID, f)
		}
	}
	return nil
}

// validateTaskDeps refuses self and dangling dependencies.
func validateTaskDeps(t PlanTask, ids map[string]bool, all []PlanTask) error {
	for _, d := range t.DependsOn {
		if d == t.ID {
			return fmt.Errorf("planning: tarea %q depende de sí misma", t.ID)
		}
		if !ids[d] && !depExists(all, d) {
			return fmt.Errorf("planning: tarea %q depende de %q (desconocida)", t.ID, d)
		}
	}
	return nil
}

// validateTaskCapabilities refuses capabilities without dated evidence.
func validateTaskCapabilities(t PlanTask, manifests map[string]bench.HostManifest) error {
	for _, c := range t.RequiredCapabilities {
		if !verifiedCap(manifests, c) {
			return fmt.Errorf("planning: tarea %q exige capability %q sin evidencia probada (nunca se planifica sin verificar)", t.ID, c)
		}
	}
	return nil
}

// validateTaskAgent refuses unknown agents and scopes above the grant.
func validateTaskAgent(t PlanTask, reg []orchestrate.RoutedAgent) error {
	if t.Agent == "" {
		return nil
	}
	agent, ok := orchestrate.FindRoutedAgent(reg, t.Agent)
	if !ok {
		return fmt.Errorf("planning: tarea %q agente %q fuera del registry", t.ID, t.Agent)
	}
	if !scopesHold(agent.Scopes, t.MinScopes) {
		return fmt.Errorf("planning: tarea %q pide scopes [%s] superiores a los de %s [%s] (mínimos, nunca superiores)",
			t.ID, strings.Join(t.MinScopes, ", "), t.Agent, strings.Join(agent.Scopes, ", "))
	}
	return nil
}

// validateTaskEstimate refuses figures missing the estimate label.
func validateTaskEstimate(t PlanTask) error {
	if !strings.Contains(t.Estimate.Note, "estimación") {
		return fmt.Errorf("planning: tarea %q estimación sin etiqueta (toda cifra es estimación, no medición)", t.ID)
	}
	return nil
}

// validateTaskBody runs every per-task group in declaration order.
func validateTaskBody(t PlanTask, ids map[string]bool, all []PlanTask, reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest) error {
	if err := validateTaskContent(t); err != nil {
		return err
	}
	if err := validateTaskScope(t); err != nil {
		return err
	}
	if err := validateTaskDeps(t, ids, all); err != nil {
		return err
	}
	if err := validateTaskCapabilities(t, manifests); err != nil {
		return err
	}
	if err := validateTaskAgent(t, reg); err != nil {
		return err
	}
	if err := validateTaskEstimate(t); err != nil {
		return err
	}
	return nil
}

// validatePlanTasks walks the task list keeping duplicate detection order.
func validatePlanTasks(p *Plan, reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest) error {
	ids := map[string]bool{}
	for _, t := range p.Tasks {
		if strings.TrimSpace(t.ID) == "" {
			return fmt.Errorf("planning: tarea sin id (toda tarea se nombra)")
		}
		if ids[t.ID] {
			return fmt.Errorf("planning: tarea duplicada %q", t.ID)
		}
		ids[t.ID] = true
		if err := validateTaskBody(t, ids, p.Tasks, reg, manifests); err != nil {
			return err
		}
	}
	return nil
}

// validatePlanDAG refuses a task order the F12.2 DAG cannot serialize.
func validatePlanDAG(p *Plan) error {
	pts := make([]orchestrate.ParallelTask, 0, len(p.Tasks))
	for _, t := range p.Tasks {
		pts = append(pts, orchestrate.ParallelTask{ID: t.ID, Title: t.Title, Files: t.Scope.Files, DependsOn: t.DependsOn})
	}
	if _, _, err := orchestrate.PlanParallelLevels(pts); err != nil {
		return fmt.Errorf("planning: %w", err)
	}
	return nil
}

// ValidatePlan refuses a structurally unsound plan before anything
// executes: unknown ids, dangling deps, duplicate tasks, cycles,
// unverified capabilities and scopes beyond the agent's grant.
func ValidatePlan(p *Plan, reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest) error {
	if err := validatePlanHeader(p); err != nil {
		return err
	}
	if p.Kind == KindAdversarial {
		return validateAdversarialPlan(p)
	}
	if err := validatePlanSize(p); err != nil {
		return err
	}
	if err := validatePlanTasks(p, reg, manifests); err != nil {
		return err
	}
	return validatePlanDAG(p)
}

// depExists reports whether id names any plan task.
func depExists(tasks []PlanTask, id string) bool {
	for _, t := range tasks {
		if t.ID == id {
			return true
		}
	}
	return false
}

// DiffPlans compares two versions: added, removed and changed
// tasks plus the status move, so the operator sees exactly what a
// revision changed and why.
func DiffPlans(a, b *Plan) PlanDiff {
	d := PlanDiff{From: a.PlanVersion, To: b.PlanVersion}
	am := taskIDs(a.Tasks)
	bm := taskIDs(b.Tasks)
	for id := range bm {
		if _, ok := am[id]; !ok {
			d.Added = append(d.Added, id)
		}
	}
	for id := range am {
		if _, ok := bm[id]; !ok {
			d.Removed = append(d.Removed, id)
		}
	}
	for id, bt := range bm {
		at, ok := am[id]
		if !ok {
			continue
		}
		var changes []string
		if at.Title != bt.Title {
			changes = append(changes, "título")
		}
		if at.Objective != bt.Objective {
			changes = append(changes, "objetivo")
		}
		if strings.Join(at.DependsOn, ",") != strings.Join(bt.DependsOn, ",") {
			changes = append(changes, "dependencias")
		}
		if strings.Join(at.Scope.Files, ",") != strings.Join(bt.Scope.Files, ",") {
			changes = append(changes, "scope")
		}
		if at.Agent != bt.Agent {
			changes = append(changes, "agente")
		}
		if at.Autonomy != bt.Autonomy {
			changes = append(changes, "autonomía")
		}
		if len(changes) > 0 {
			d.Changed = append(d.Changed, TaskChange{ID: id, Fields: changes})
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	sort.Slice(d.Changed, func(i, j int) bool { return d.Changed[i].ID < d.Changed[j].ID })
	if a.Status != b.Status {
		d.StatusChange = a.Status + " → " + b.Status
	}
	d.ReasonChange = b.ChangeReason
	return d
}

// TaskChange names the fields one revision touched on a task.
type TaskChange struct {
	ID     string   `json:"id"`
	Fields []string `json:"fields"`
}

// PlanDiff is the version-to-version comparison.
type PlanDiff struct {
	From         int          `json:"from"`
	To           int          `json:"to"`
	Added        []string     `json:"added,omitempty"`
	Removed      []string     `json:"removed,omitempty"`
	Changed      []TaskChange `json:"changed,omitempty"`
	StatusChange string       `json:"status_change,omitempty"`
	ReasonChange string       `json:"reason_change,omitempty"`
}
