// Task decomposition, routing, ordering and quality (F13.2): the
// second half of plan construction. Every task maps onto an
// orchestrate.ParallelTask so ordering, collisions and cycles reuse
// F12.1/F12.2 instead of a parallel DAG.
package planning

import (
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/impact"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
)

// taskKinds name the closed task vocabulary the planner emits.
const (
	taskSurvey  = "survey"
	taskDocs    = "docs"
	taskImpl    = "implement"
	taskVerify  = "verify"
	taskDelimit = "delimit"
)

// capsFor names the minimal F11.7 capabilities per task kind: the
// closed 12-dimension vocabulary, never invented ids.
func capsFor(kind string) []string {
	switch kind {
	case taskVerify:
		return []string{bench.CapTools, bench.CapGate}
	case taskSurvey:
		return []string{bench.CapTools, bench.CapSessions}
	default:
		return []string{bench.CapTools, bench.CapFilesystem}
	}
}

// scopesFor names the minimal F11.8 scopes per task kind. Verify
// needs gate:execute (only verify-first holds it); the assigner
// picks the first ranked agent holding the whole set, or escalates
// to needs-approval instead of exceeding anyone's grant.
func scopesFor(kind string) []string {
	switch kind {
	case taskSurvey:
		return []string{"spec:read", "trace:read"}
	case taskVerify:
		return []string{"spec:read", "trace:read", "gate:execute"}
	default:
		return []string{"spec:read", "memory:read"}
	}
}

// strategyFor names the verification strategy per task kind.
func strategyFor(kind string) string {
	switch kind {
	case taskSurvey:
		return "relevo documentado: inventario citado + alcance confirmado antes de implementar"
	case taskVerify:
		return "tests + criterios Given/When/Then en verde (go test del alcance, suite enfocada F12.8)"
	case taskDelimit:
		return "preguntas respondidas por el usuario (sin respuestas no hay ejecución)"
	case taskDocs:
		return "documentación revisada contra el cambio (docs-drift)"
	default:
		return "implementación mínima + tests del alcance en verde antes de integrar"
	}
}

// fillAmbiguous answers a vague request with one delimitation task
// and the questions blocking everything: no autonomous work is
// ever planned on a guess.
func fillAmbiguous(p *Plan, hit scopeHit, assessed []Criterion, now time.Time) {
	_ = now
	q := []Question{}
	if len(strings.Fields(p.Objective)) < 4 || len(p.Objective) < 20 {
		q = append(q, Question{
			Question: "¿Cuál es el objetivo concreto en una frase verificable?",
			Why:      "el objetivo es demasiado breve para descomponerlo sin adivinar",
			Blocks:   "todo el plan",
		})
	}
	for _, t := range hit.unknown {
		q = append(q, Question{
			Question: fmt.Sprintf("¿A qué componente corresponde %q? No aparece en el corpus F12.3.", t),
			Why:      "término sin componentes identificables: inventar ficheros está prohibido",
			Blocks:   "alcance",
		})
		if len(q) >= 6 {
			break
		}
	}
	if len(hit.files) == 0 {
		q = append(q, Question{
			Question: "¿Qué ficheros o paquetes toca el cambio?",
			Why:      "sin componentes identificables en el corpus (impacto desconocido)",
			Blocks:   "alcance",
		})
	}
	for _, c := range assessed {
		if !c.Verifiable {
			q = append(q, Question{
				Question: fmt.Sprintf("¿Cómo se comprueba %q? Falta: %s", trimWords(c.Text, 10), c.UnverifiableWs),
				Why:      "criterio no verificable: el plan no inventa el umbral",
				Blocks:   "verificación",
			})
		}
		if len(q) >= 6 {
			break
		}
	}
	if len(q) == 0 {
		q = append(q, Question{
			Question: "¿Qué entregable concreto cierra esta misión y cómo se verifica?",
			Why:      "solicitud ambigua sin criterio medible",
			Blocks:   "todo el plan",
		})
	}
	p.OpenQuestions = q
	p.Tasks = []PlanTask{{
		ID: "t1", Title: "Delimitar alcance con el usuario",
		Objective:            "Precisar objetivo, alcance y criterios verificables antes de planificar trabajo ejecutable.",
		Deliverable:          "alcance delimitado + criterios Given/When/Then acordados",
		Scope:                Scope{Files: nil, Components: nil},
		RequiredCapabilities: []string{bench.CapTools},
		MinScopes:            []string{"spec:read"},
		Verification: Verification{
			Strategy: strategyFor(taskDelimit),
			Criteria: []string{"Given preguntas abiertas When el usuario responde Then objetivo, alcance y criterios quedan verificables"},
		},
		Estimate: Estimate{
			Complexity: orchestrate.ComplexityLow, Risk: orchestrate.RiskLow,
			TimeMS: 60000, Tokens: 800, CostKnown: false, Note: EstimateNote,
		},
		Autonomy:    AutonomyNeedsUser,
		Blockers:    []string{"aclaración: preguntas abiertas sin responder"},
		ImpactKnown: false,
		Rationale:   "solicitud ambigua: la única tarea honesta es delimitar con el usuario; nada autónomo se planifica sobre un supuesto",
	}}
	p.DAG = DAG{Levels: [][]string{{"t1"}}, Order: []string{"t1"}}
}

// decompose turns an actionable request into survey + per-cluster
// implement + verify tasks, routed and budgeted one by one.
func decompose(objective string, m *mission.Mission, hit scopeHit, assessed []Criterion,
	graph impact.Graph, graphOK bool, reg []orchestrate.RoutedAgent,
	manifests map[string]bench.HostManifest, now time.Time) ([]PlanTask, error) {
	_ = now
	clusters := hit.clusters
	if len(clusters) > MaxTasks-2 {
		clusters = mergeClusters(clusters, MaxTasks-2)
	}
	var tasks []PlanTask
	survey := PlanTask{
		ID: "t1", Title: "Relevar contexto y confirmar alcance",
		Objective:   fmt.Sprintf("Inspeccionar %s y confirmar el alcance antes de implementar.", strings.Join(hit.components, ", ")),
		Deliverable: "alcance confirmado + inventario citado",
		Scope: Scope{
			Files:      headFiles(hit.files, MaxScopeFiles),
			Components: hit.components,
		},
		RequiredCapabilities: capsFor(taskSurvey),
		MinScopes:            scopesFor(taskSurvey),
		ContextRefs:          headFiles(hit.fragRefs, 8),
		Verification: Verification{
			Strategy: strategyFor(taskSurvey),
			Criteria: []string{"Given el corpus F12.3 y el grafo F12.8 When se relevan los componentes Then el alcance confirmado cita ficheros y tests existentes"},
		},
		ImpactKnown: graphOK && len(hit.files) > 0,
		Rationale:   "el relevo precede a todo cambio: confirma el alcance inferido contra el repo real antes de gastar presupuesto en implementar",
	}
	tasks = append(tasks, survey)

	n := 2
	for i, cluster := range clusters {
		id := fmt.Sprintf("t%d", n)
		n++
		t := PlanTask{
			ID: id, Title: fmt.Sprintf("Implementar cambio en %s", parentLabel(cluster)),
			Objective:   fmt.Sprintf("Aplicar el cambio en %s hacia: %s.", parentLabel(cluster), trimWords(objective, 14)),
			Deliverable: fmt.Sprintf("cambio aplicado y compilable en %s", parentLabel(cluster)),
			DependsOn:   []string{"t1"},
			Scope: Scope{
				Files:      headFiles(cluster, MaxScopeFiles),
				Components: []string{parentLabel(cluster)},
			},
			RequiredCapabilities: capsFor(taskImpl),
			MinScopes:            scopesFor(taskImpl),
			Verification: Verification{
				Strategy: strategyFor(taskImpl),
				Criteria: []string{fmt.Sprintf("Given el cambio aplicado en %s When corren los tests del alcance Then todo en verde", parentLabel(cluster))},
			},
			Rationale: fmt.Sprintf("clúster de cambio %d/%d con distinta razón de cambio: se aísla para revisar, revertir y verificar por separado", i+1, len(clusters)),
		}
		imp := impact.MapDiff(graph, t.Scope.Files)
		t.Tests = headFiles(imp.Tests, 8)
		t.ContextRefs = append(headFiles(refsFor(imp, 6), 6), headFiles(matchingRefs(hit.fragRefs, 4), 4)...)
		if len(imp.Critical) > 0 {
			t.Rationale += fmt.Sprintf(" (toca paquete(s) crítico(s) %s: exige aprobación)", strings.Join(imp.Critical, ", "))
		}
		if imp.Widened || !graphOK {
			t.ImpactKnown = false
		} else {
			t.ImpactKnown = true
		}
		tasks = append(tasks, t)
	}

	verifyDeps := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if t.ID != "t1" {
			verifyDeps = append(verifyDeps, t.ID)
		}
	}
	if len(verifyDeps) == 0 {
		verifyDeps = []string{"t1"}
	}
	verify := PlanTask{
		ID: fmt.Sprintf("t%d", n), Title: "Verificar cambio extremo a extremo",
		Objective:   "Correr tests del alcance y comprobar cada criterio Given/When/Then.",
		Deliverable: "reporte de verificación (tests + criterios en verde)",
		DependsOn:   verifyDeps,
		Scope: Scope{
			Files:      headFiles(hit.files, MaxScopeFiles),
			Components: hit.components,
		},
		RequiredCapabilities: capsFor(taskVerify),
		MinScopes:            scopesFor(taskVerify),
		Verification: Verification{
			Strategy: strategyFor(taskVerify),
			Criteria: verificationCriteria(assessed),
		},
		Rationale: "la verificación cierra el DAG: depende de todo lo implementado y no deja pasar criterios sin comprobar",
	}
	imp := impact.MapDiff(graph, verify.Scope.Files)
	verify.Tests = headFiles(imp.Tests, 8)
	verify.ImpactKnown = graphOK && !imp.Widened
	tasks = append(tasks, verify)

	for i := range tasks {
		kind := taskKindOf(tasks[i])
		routeTask(&tasks[i], m, kind, graph, reg, manifests)
	}
	return tasks, nil
}

// taskKindOf recovers the task kind from its position and caps.
func taskKindOf(t PlanTask) string {
	for _, c := range t.RequiredCapabilities {
		if c == bench.CapGate {
			return taskVerify
		}
		if c == bench.CapSessions {
			return taskSurvey
		}
	}
	return taskImpl
}

// routeDecide is the F12.1 Decide seam: production routes through
// orchestrate.Decide; tests replace it with a fake decision.
var routeDecide = func(reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest, hist []orchestrate.AgentHistory, d orchestrate.TaskDescriptor, b orchestrate.Budgets, maxDeleg int, fixedAgent string, w orchestrate.ScoreWeights) (orchestrate.Decision, error) {
	return orchestrate.Decide(reg, manifests, hist, d, b, maxDeleg, fixedAgent, w)
}

// routeRisk resolves the task risk: the mission risk by default
// (medium when unset), escalated to high when the scope touches a
// critical package.
func routeRisk(missionRisk string, critical []string) string {
	risk := strings.TrimSpace(missionRisk)
	if risk == "" {
		risk = orchestrate.RiskMedium
	}
	if len(critical) > 0 && riskLevel(risk) < riskLevel(orchestrate.RiskHigh) {
		risk = orchestrate.RiskHigh
	}
	return risk
}

// routeComplexity grades the task size: wide scopes are high,
// medium scopes or implement work are medium, the rest is low.
func routeComplexity(kind string, nfiles int) string {
	if nfiles > 5 {
		return orchestrate.ComplexityHigh
	}
	if nfiles > 2 || kind == taskImpl {
		return orchestrate.ComplexityMedium
	}
	return orchestrate.ComplexityLow
}

// routeDescriptor builds the F12.1 routing input for one task: the
// descriptor plus the mission budgets.
func routeDescriptor(t PlanTask, m *mission.Mission, kind, risk, complexity string) (orchestrate.TaskDescriptor, orchestrate.Budgets) {
	desc := orchestrate.TaskDescriptor{
		Domain: "engineering", Language: "go",
		Complexity: complexity, Risk: risk,
		Constraints: m.Constraints, Outputs: []string{t.Deliverable},
		Criteria:             t.Verification.Criteria,
		RequiredCapabilities: t.RequiredCapabilities,
	}
	budgets := orchestrate.Budgets{TimeMS: m.Budget.TimeMS, Tokens: m.Budget.Tokens, Cost: m.Budget.Cost}
	return desc, budgets
}

// chooseRouteAgent picks the routing winner: the first ranked agent
// holding every minimal scope; when none holds them, the primary
// still takes the task behind an approval blocker. It returns the
// chosen agent plus the optional approval note.
func chooseRouteAgent(dec orchestrate.Decision, reg []orchestrate.RoutedAgent, minScopes []string) (string, string) {
	for _, r := range dec.Ranking {
		agent, ok := orchestrate.FindRoutedAgent(reg, r.AgentID)
		if !ok {
			continue
		}
		if scopesHold(agent.Scopes, minScopes) {
			return agent.ID, ""
		}
	}
	if dec.Primary != "" {
		if agent, ok := orchestrate.FindRoutedAgent(reg, dec.Primary); ok {
			note := fmt.Sprintf("aprobación: la tarea pide [%s] y %s solo otorga [%s] (sin permisos superiores a los autorizados)",
				strings.Join(minScopes, ", "), agent.ID, strings.Join(agent.Scopes, ", "))
			return agent.ID, note
		}
	}
	return "", ""
}

// finalizeRouteAutonomy assigns the task autonomy and its approval
// blockers: the kind/risk policy first, then the high-risk floor
// (high/critical never runs autonomous), then the default approval
// blocker when needs-approval names none yet.
func finalizeRouteAutonomy(t *PlanTask, kind, risk string, critical []string) {
	t.Autonomy = autonomyFor(kind, risk, critical, t.ImpactKnown, t.Autonomy)
	if riskLevel(risk) >= riskLevel(orchestrate.RiskHigh) && t.Autonomy == AutonomyAutonomous {
		t.Autonomy = AutonomyNeedsApproval
		t.Blockers = append(t.Blockers, fmt.Sprintf("aprobación: riesgo %s (las tareas de riesgo alto/crítico no corren autónomas)", risk))
	}
	if t.Autonomy == AutonomyNeedsApproval && !hasApprovalBlocker(t.Blockers) {
		t.Blockers = append(t.Blockers, fmt.Sprintf("aprobación: riesgo %s o paquete crítico %s", risk, strings.Join(critical, ", ")))
	}
}

// routeTask assigns the agent, evidence, budget and autonomy of one
// task through F12.1 Decide. Unverified capabilities or scopes
// beyond the agent's grant escalate to needs-approval — never to a
// silent overreach.
func routeTask(t *PlanTask, m *mission.Mission, kind string, graph impact.Graph,
	reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest) {
	critical := touchesCritical(graph, t.Scope.Files)
	risk := routeRisk(m.Risk, critical)
	complexity := routeComplexity(kind, len(t.Scope.Files))
	desc, budgets := routeDescriptor(*t, m, kind, risk, complexity)
	dec, err := routeDecide(reg, manifests, nil, desc, budgets, 1, "", orchestrate.DefaultScoreWeights())
	if err != nil {
		t.Blockers = append(t.Blockers, fmt.Sprintf("routing imposible: %v", err))
		t.Autonomy = AutonomyNeedsApproval
		t.Estimate = estimateFor(kind, complexity, risk, len(t.Scope.Files))
		return
	}
	chosen, approvalNote := chooseRouteAgent(dec, reg, t.MinScopes)
	if approvalNote != "" {
		t.Blockers = append(t.Blockers, approvalNote)
		t.Autonomy = AutonomyNeedsApproval
	}
	if chosen == "" {
		reasons := []string{}
		for _, e := range dec.Excluded {
			reasons = append(reasons, e.AgentID+": "+e.Reason)
		}
		t.Blockers = append(t.Blockers, "sin agente elegible: "+strings.Join(reasons, "; "))
		t.Autonomy = AutonomyNeedsApproval
		t.Estimate = estimateFor(kind, complexity, risk, len(t.Scope.Files))
		return
	}
	t.Agent = chosen
	if dec.Mode == orchestrate.ModePair && dec.Reviewer != "" && dec.Reviewer != chosen {
		t.Reviewer = dec.Reviewer
	}
	for _, c := range t.RequiredCapabilities {
		t.CapabilityEvidence = append(t.CapabilityEvidence, evidenceFor(manifests, chosen, c))
	}
	t.Estimate = estimateFor(kind, complexity, risk, len(t.Scope.Files))
	finalizeRouteAutonomy(t, kind, risk, critical)
}

// riskLevel orders the F12.1 risk words.
func riskLevel(r string) int {
	switch r {
	case orchestrate.RiskLow:
		return 0
	case orchestrate.RiskMedium:
		return 1
	case orchestrate.RiskHigh:
		return 2
	case orchestrate.RiskCritical:
		return 3
	}
	return -1
}

// scopesHold reports whether the agent grant covers every minimal
// scope (delegating only restricts, F11.8).
func scopesHold(granted, needed []string) bool {
	have := map[string]bool{}
	for _, s := range granted {
		have[s] = true
	}
	for _, s := range needed {
		if !have[s] {
			return false
		}
	}
	return true
}

// evidenceFor cites the dated F11.7 evidence behind one capability
// on the agent's host.
func evidenceFor(manifests map[string]bench.HostManifest, agentID, cap string) string {
	for _, m := range manifests {
		for _, c := range m.Capabilities {
			if c.ID == cap && c.Status == bench.CapVerified && strings.TrimSpace(c.Evidence) != "" {
				return fmt.Sprintf("%s@%s: %s", cap, m.Host, c.Evidence)
			}
		}
	}
	return cap + ": sin evidencia (nunca asignar)"
}

// touchesCritical lists the F12.8 critical nodes under the scope.
func touchesCritical(graph impact.Graph, files []string) []string {
	if len(files) == 0 || len(graph.Nodes) == 0 {
		return nil
	}
	imp := impact.MapDiff(graph, files)
	return imp.Critical
}

// estimateFor prices one task, always labelled as an estimate.
func estimateFor(kind, complexity, risk string, nfiles int) Estimate {
	var ms int64
	var tok int64
	switch kind {
	case taskSurvey:
		ms, tok = 60000, 800
	case taskVerify:
		ms, tok = 180000, 1200
	case taskDelimit:
		ms, tok = 60000, 800
	default:
		ms, tok = 300000+int64(nfiles)*60000, 2000+int64(nfiles)*400
	}
	return Estimate{
		Complexity: complexity, Risk: risk,
		TimeMS: ms, Tokens: tok,
		CostKnown: false, Note: EstimateNote,
	}
}

// autonomyFor names who may run the task: read-only survey and
// low/medium implement work run autonomous; docs and test-only
// verification are reversible; anything unclear, critical-adjacent
// or over-granted needs a human.
func autonomyFor(kind, risk string, critical []string, impactKnown bool, preset string) string {
	if preset == AutonomyNeedsApproval || preset == AutonomyNeedsUser {
		return preset
	}
	if !impactKnown {
		return AutonomyNeedsUser
	}
	if len(critical) > 0 || riskLevel(risk) >= riskLevel(orchestrate.RiskHigh) {
		return AutonomyNeedsApproval
	}
	if kind == taskDocs {
		return AutonomyReversible
	}
	if kind == taskVerify && riskLevel(risk) <= riskLevel(orchestrate.RiskMedium) {
		return AutonomyReversible
	}
	return AutonomyAutonomous
}

// hasApprovalBlocker reports whether an approval blocker is named.
func hasApprovalBlocker(blockers []string) bool {
	for _, b := range blockers {
		if strings.HasPrefix(b, "aprobación:") {
			return true
		}
	}
	return false
}

// mergeClusters folds many scope clusters into at most max, merging
// the smallest into their alphabetical neighbour.
func mergeClusters(clusters [][]string, max int) [][]string {
	for len(clusters) > max {
		small := smallestClusterIndex(clusters)
		neighbour := neighbourIndex(small, len(clusters))
		clusters[neighbour] = append(clusters[neighbour], clusters[small]...)
		clusters = append(clusters[:small], clusters[small+1:]...)
	}
	return clusters
}

// smallestClusterIndex names the shortest cluster.
func smallestClusterIndex(clusters [][]string) int {
	small := 0
	for i := range clusters {
		if len(clusters[i]) < len(clusters[small]) {
			small = i
		}
	}
	return small
}

// neighbourIndex names the merge neighbour of the smallest cluster:
// the next one when the smallest is first, else the previous one.
func neighbourIndex(small, n int) int {
	if small == 0 {
		return 1
	}
	return small - 1
}

// parentLabel names one cluster by its shared component.
func parentLabel(cluster []string) string {
	if len(cluster) == 0 {
		return "alcance"
	}
	return parentDir(cluster[0])
}

// headFiles bounds a list deterministically (already sorted).
func headFiles(in []string, max int) []string {
	if len(in) <= max {
		return append([]string(nil), in...)
	}
	return append([]string(nil), in[:max]...)
}

// refsFor cites impact nodes and tests behind one diff.
func refsFor(imp impact.Impact, max int) []string {
	refs := append(append([]string{}, imp.Direct...), imp.Consumers...)
	refs = append(refs, imp.Tests...)
	if len(refs) > max {
		refs = refs[:max]
	}
	return refs
}

// matchingRefs keeps corpus fragment refs (bounded).
func matchingRefs(refs []string, max int) []string {
	return headFiles(refs, max)
}

// verificationCriteria folds mission criteria into the verify task:
// originals verbatim (never rewritten), plus the build gate.
func verificationCriteria(assessed []Criterion) []string {
	var out []string
	for _, c := range assessed {
		out = append(out, c.Text)
	}
	out = append(out, "Given el cambio integrado When corre go build ./... Then compila sin errores")
	return out
}

// orderDAG maps plan tasks onto ParallelTask and reuses the F12.2
// ordering: explicit deps plus implicit collision serialisation,
// cycles fail closed naming the loop.
func orderDAG(p *Plan) error {
	pts := make([]orchestrate.ParallelTask, 0, len(p.Tasks))
	for _, t := range p.Tasks {
		pts = append(pts, orchestrate.ParallelTask{
			ID: t.ID, Title: t.Title, Files: t.Scope.Files,
			DependsOn: t.DependsOn, Agent: t.Agent,
			Scopes:         t.MinScopes,
			EstimateMS:     t.Estimate.TimeMS,
			EstimateTokens: t.Estimate.Tokens,
		})
	}
	levels, serialized, err := orchestrate.PlanParallelLevels(pts)
	if err != nil {
		return fmt.Errorf("planning: DAG inválido: %w", err)
	}
	p.DAG = DAG{
		Levels: levels, Order: orchestrate.FlattenParallelLevels(levels),
		Collisions: serialized, Cycles: impact.DetectCycles(graphOf(p, pts)),
	}
	serialSet := map[string]string{}
	for _, s := range serialized {
		serialSet[s[0]+">"+s[1]] = s[0] + " y " + s[1] + " comparten ficheros: se serializan (" + s[0] + " antes que " + s[1] + ")"
	}
	_ = serialSet
	for i := range p.Tasks {
		for _, s := range serialized {
			if p.Tasks[i].ID == s[1] {
				p.Tasks[i].Blockers = append(p.Tasks[i].Blockers,
					fmt.Sprintf("serialización: %s comparte ficheros con %s (orden %s antes que %s)", s[1], s[0], s[0], s[1]))
			}
		}
	}
	return nil
}

// graphOf rebuilds a minimal impact graph over the plan tasks so
// cycle detection reuses F12.8 on the plan's own edges.
func graphOf(p *Plan, pts []orchestrate.ParallelTask) impact.Graph {
	var g impact.Graph
	for _, t := range pts {
		g.Nodes = append(g.Nodes, impact.Node{ID: "pkg:" + t.ID, Kind: impact.KindPackage, Path: t.ID})
		for _, d := range t.DependsOn {
			g.Edges = append(g.Edges, impact.Edge{From: "pkg:" + t.ID, To: "pkg:" + d, Source: impact.SourceGoImport, File: "plan", Line: 1})
		}
	}
	return g
}
