// Deterministic plan construction (F13.2/NOV-105): classify the
// request, inspect the repo and work state, infer scope from the
// F12.3 corpus, cite F12.8 impact, route every task through F12.1
// Decide over F11.7-verified capabilities, order with the F12.2 DAG
// and grade quality. Same mission, repo and instant always print
// the same plan.
package planning

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/bench"
	ictx "regulus-ai/internal/context"
	"regulus-ai/internal/impact"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/pxexec"
)

// Request carries one planning or replanning input: optional
// objective override (default: the mission objective), extra
// constraints/criteria the operator adds, the author, the change
// reason (required on replan, kept per version) and newly
// confirmed outcomes (appended, never dropped).
type Request struct {
	Objective    string
	Constraints  []string
	Criteria     []string
	Author       string
	ChangeReason string
	Confirmed    []ConfirmedResult
}

// scopeHit is one inferred scope: files, their components, the
// corpus fragments behind them and the terms nobody matched.
type scopeHit struct {
	files      []string
	components []string
	clusters   [][]string
	fragRefs   []string
	unknown    []string
}

// BuildPlan inspects root and returns the executable plan for m.
// It never invents: unknown scope, unverifiable criteria or
// hostile intent surface as needs-clarification/refused with the
// reason and the question, never as a confident executable plan.
func BuildPlan(root string, m *mission.Mission, req Request, now time.Time) (*Plan, error) {
	if m == nil {
		return nil, fmt.Errorf("planning: nil mission (plan without mission is not evidence)")
	}
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = strings.TrimSpace(m.Objective)
	}
	if objective == "" {
		return nil, fmt.Errorf("planning: mission %q holds an empty objective (nothing to plan)", m.ID)
	}
	constraints := append(append([]string{}, m.Constraints...), req.Constraints...)
	criteria := append(append([]string{}, m.Criteria...), req.Criteria...)

	if marker, bad := IsAdversarial(objective, constraints, criteria); bad {
		return refusedPlan(m, objective, req.Author, now,
			fmt.Sprintf("solicitud adversarial (%q): el plan se rehúsa, no se ejecuta nada", marker)), nil
	}

	corpus, _, cerr := ictx.Inventory(root, ictx.DefaultInventoryConfig())
	if cerr != nil {
		return nil, fmt.Errorf("planning: inventario: %w", cerr)
	}
	graph, gerr := impact.BuildGraph(root, nil, nil)
	graphOK := gerr == nil

	hit := inferScope(objective, constraints, criteria, corpus)
	kind := classify(objective, criteria, hit)
	assessed := assessAll(criteria)

	manifests := bench.DefaultHostManifests(bench.EvidenceStamp())
	reg := orchestrate.RegistryFromManifests(manifests)

	p := &Plan{
		Version: PlanVersion, PlanVersion: 1, MissionID: m.ID,
		Objective: objective, Kind: kind, CreatedAt: now.UTC(),
		Author: req.Author, ChangeReason: req.ChangeReason,
		Confirmed: append([]ConfirmedResult(nil), req.Confirmed...),
	}
	p.RepoHead, p.WorktreeStatus = workState(root)
	p.Hypotheses = hypothesesFor(kind, hit, graphOK)
	p.Assumptions = assumptionsFor(m, constraints)
	p.Exclusions = exclusionsFor(constraints)
	p.Deliverables = deliverablesFor(objective, hit)
	p.Criteria = assessed

	switch kind {
	case KindAmbiguous:
		fillAmbiguous(p, hit, assessed, now)
		p.Quality = gradeQuality(p, reg, manifests)
		p.Status = deriveStatus(p)
		return p, nil
	}

	tasks, verr := decompose(objective, m, hit, assessed, graph, graphOK, reg, manifests, now)
	if verr != nil {
		return nil, verr
	}
	p.Tasks = tasks
	if err := orderDAG(p); err != nil {
		return nil, err
	}
	coverCriteria(p)
	p.Risks = risksFor(m, hit, graphOK, p.Tasks)
	p.OpenQuestions = questionsFor(p, hit)
	p.Quality = gradeQuality(p, reg, manifests)
	p.Status = deriveStatus(p)
	return p, nil
}

// Replan rebuilds the plan for a new version: the change reason is
// required and kept, confirmed outcomes travel forward untouched,
// and dropping one refuses instead of losing evidence.
func Replan(root string, prev *Plan, m *mission.Mission, req Request, now time.Time) (*Plan, error) {
	if prev == nil {
		return nil, fmt.Errorf("planning: no hay plan previo que reversionar (usa plan primero)")
	}
	if strings.TrimSpace(req.ChangeReason) == "" {
		return nil, fmt.Errorf("planning: replan exige --change con el motivo (trazabilidad: cada revisión conserva por qué cambió)")
	}
	merged := Request{
		Objective: req.Objective, Constraints: req.Constraints,
		Criteria: req.Criteria, Author: req.Author,
		ChangeReason: req.ChangeReason,
	}
	// Confirmed outcomes are append-only: previous ones travel,
	// new ones join, nothing is ever dropped by a revision.
	merged.Confirmed = append(append([]ConfirmedResult{}, prev.Confirmed...), req.Confirmed...)
	next, err := BuildPlan(root, m, merged, now)
	if err != nil {
		return nil, err
	}
	next.PlanVersion = prev.PlanVersion + 1
	next.ChangeReason = req.ChangeReason
	for _, c := range prev.Confirmed {
		found := false
		for _, k := range next.Confirmed {
			if k.TaskID == c.TaskID && k.Result == c.Result {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("planning: replan perdería el resultado confirmado %s=%q (las revisiones nunca borran historial)", c.TaskID, c.Result)
		}
	}
	return next, nil
}

// refusedPlan answers a hostile request: zero tasks, the marker as
// the risk, and one question explaining the refusal.
func refusedPlan(m *mission.Mission, objective, author string, now time.Time, reason string) *Plan {
	return &Plan{
		Version: PlanVersion, PlanVersion: 1, MissionID: m.ID,
		Objective: objective, Kind: KindAdversarial, Status: StatusRefused,
		CreatedAt: now.UTC(), Author: author,
		Risks: []string{reason},
		OpenQuestions: []Question{{
			Question: "¿Cuál es el objetivo de ingeniería legítimo detrás de esta solicitud?",
			Why:      reason,
			Blocks:   "todo el plan (nada se planifica sobre una solicitud adversarial)",
		}},
		Exclusions: exclusionsFor(m.Constraints),
		Quality: Quality{Checks: []Check{
			{Name: "plan-no-vacio", Pass: false, Detail: "solicitud adversarial: cero tareas a propósito (rehusar es el comportamiento correcto)"},
			{Name: "capabilities-verificadas", Pass: true, Detail: "nada asignado: ninguna capability sin evidencia fue planificada"},
		}},
	}
}

// classify names the request kind: hostile first, then too thin or
// vague to execute, then structurally simple or complex. Zero scope
// candidates always read as ambiguous — the planner asks where the
// work lives instead of inventing files.
func classify(objective string, criteria []string, hit scopeHit) string {
	words := len(strings.Fields(objective))
	if words < 4 || len(objective) < 20 {
		return KindAmbiguous
	}
	lower := strings.ToLower(objective)
	for _, v := range []string{"mejora", "optimiza", "arregla", "revisa todo", "pule", "hazlo mejor", "refactor general", "improve everything"} {
		if strings.Contains(lower, v) && len(criteria) == 0 {
			return KindAmbiguous
		}
	}
	if len(hit.files) == 0 {
		return KindAmbiguous
	}
	if len(hit.clusters) >= 2 || len(criteria) >= 2 || len(objective) > 160 || len(hit.files) > 3 {
		return KindComplex
	}
	return KindSimple
}

// assessAll grades every acceptance criterion without dropping any.
func assessAll(criteria []string) []Criterion {
	out := make([]Criterion, 0, len(criteria))
	for _, c := range criteria {
		out = append(out, AssessCriterion(c))
	}
	return out
}

// stopSet keeps scope inference on content words (Spanish and
// English glue never matches a file).
var stopSet = map[string]bool{
	"el": true, "la": true, "los": true, "las": true, "de": true,
	"del": true, "en": true, "y": true, "o": true, "a": true,
	"con": true, "por": true, "para": true, "que": true, "se": true,
	"un": true, "una": true, "the": true, "and": true, "or": true,
	"of": true, "to": true, "in": true, "for": true, "with": true,
	"on": true, "is": true, "are": true, "as": true,
}

// contentTerms folds free text into distinct lowercase tokens of
// length 3+, minus glue.
func contentTerms(texts ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range texts {
		var cur strings.Builder
		flush := func() {
			w := strings.ToLower(cur.String())
			cur.Reset()
			if len(w) < 3 || stopSet[w] || seen[w] {
				return
			}
			seen[w] = true
			out = append(out, w)
		}
		for _, r := range t {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r >= 128 {
				cur.WriteRune(r)
			} else if cur.Len() > 0 {
				flush()
			}
		}
		if cur.Len() > 0 {
			flush()
		}
	}
	sort.Strings(out)
	return out
}

// inferScope matches request terms against the F12.3 corpus: code
// fragment sources first, then symbols and docs as context refs.
// Terms nobody matched are reported as unknown, never silently
// mapped onto unrelated files.
func inferScope(objective string, constraints, criteria []string, corpus ictx.Corpus) scopeHit {
	var hit scopeHit
	joined := append(append([]string{objective}, constraints...), criteria...)
	terms := contentTerms(joined...)
	if len(terms) == 0 {
		return hit
	}
	type scored struct {
		id, source string
		score      int
	}
	var code, other []scored
	matched := map[string]bool{}
	for _, f := range corpus.Fragments {
		hay := strings.ToLower(f.Source + "\n" + f.Title)
		n := 0
		for _, t := range terms {
			if strings.Contains(hay, t) {
				n++
				matched[t] = true
			}
		}
		if n == 0 {
			continue
		}
		s := scored{id: f.ID, source: f.Source, score: n}
		if f.Kind == ictx.KindCode {
			code = append(code, s)
		} else {
			other = append(other, s)
		}
	}
	for _, t := range terms {
		if !matched[t] && !structuralTerm(t) {
			hit.unknown = append(hit.unknown, t)
		}
	}
	orderUnknown(objective, hit.unknown)
	sort.Slice(code, func(i, j int) bool {
		if code[i].score != code[j].score {
			return code[i].score > code[j].score
		}
		return code[i].id < code[j].id
	})
	seen := map[string]bool{}
	for _, s := range code {
		if len(hit.files) >= MaxScopeFiles {
			break
		}
		if seen[s.source] {
			continue
		}
		seen[s.source] = true
		hit.files = append(hit.files, s.source)
		hit.fragRefs = append(hit.fragRefs, s.id)
	}
	sort.Strings(hit.files)
	byDir := map[string][]string{}
	for _, f := range hit.files {
		d := parentDir(f)
		byDir[d] = append(byDir[d], f)
	}
	var dirs []string
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		hit.clusters = append(hit.clusters, byDir[d])
		hit.components = append(hit.components, d)
	}
	sort.Slice(other, func(i, j int) bool {
		if other[i].score != other[j].score {
			return other[i].score > other[j].score
		}
		return other[i].id < other[j].id
	})
	for i, s := range other {
		if i >= 8 {
			break
		}
		hit.fragRefs = append(hit.fragRefs, s.id)
	}
	sort.Strings(hit.components)
	return hit
}

// structuralTerm filters tokens that are criterion scaffolding,
// not scope: GWT markers and digit-led measures ("100ms" names a
// threshold, never a component). They must not pollute the unknown
// list the clarification questions quote.
func structuralTerm(t string) bool {
	switch t {
	case "given", "when", "then", "dado", "cuando", "entonces":
		return true
	}
	if t == "" {
		return true
	}
	return t[0] >= '0' && t[0] <= '9'
}

// orderUnknown quotes objective terms first: the clarification
// questions name what the operator actually asked for before any
// boilerplate the criteria carried along.
func orderUnknown(objective string, unknown []string) {
	inObjective := map[string]bool{}
	for _, t := range contentTerms(objective) {
		inObjective[t] = true
	}
	sort.SliceStable(unknown, func(i, j int) bool {
		ai, aj := inObjective[unknown[i]], inObjective[unknown[j]]
		if ai != aj {
			return ai
		}
		return unknown[i] < unknown[j]
	})
}

// parentDir folds one repo-relative file to its scope component:
// internal/<pkg> stays whole, deeper paths fold to two segments.
func parentDir(f string) string {
	slash := strings.ReplaceAll(f, "\\", "/")
	if i := strings.LastIndex(slash, "/"); i >= 0 {
		d := slash[:i]
		parts := strings.Split(d, "/")
		if len(parts) > 2 {
			return strings.Join(parts[:2], "/")
		}
		return d
	}
	return "."
}

// workState best-efforts the repo work state: HEAD plus the dirty
// count. Git absent or failing degrades to "worktree" with the
// reason — the plan records what it saw, never a guess.
func workState(root string) (head, status string) {
	head, status = "worktree", "git no disponible (se planifica sobre el árbol visible)"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	headRes := pxexec.Run(ctx, pxexec.Spec{Name: "git", Args: []string{"rev-parse", "HEAD"}, Dir: root, Timeout: 10 * time.Second})
	if headRes.Outcome != pxexec.OutcomePass {
		return head, status
	}
	if h := strings.TrimSpace(headRes.Stdout); h != "" {
		head = h
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	statusRes := pxexec.Run(ctx2, pxexec.Spec{Name: "git", Args: []string{"status", "--porcelain"}, Dir: root, Timeout: 10 * time.Second})
	if statusRes.Outcome != pxexec.OutcomePass {
		return head, "HEAD " + shortSHA(head) + " (estado sin leer: git status falló, se planifica sobre el árbol visible)"
	}
	n := 0
	for _, line := range strings.Split(statusRes.Stdout, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	if n == 0 {
		return head, "limpio en " + shortSHA(head)
	}
	return head, fmt.Sprintf("%d fichero(s) modificado(s) en %s (el plan se verificará contra este estado)", n, shortSHA(head))
}

// shortSHA trims a sha for one-line output.
func shortSHA(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// hypothesesFor states what the plan bets on, labelled as bets:
// scope clusters, corpus/graph coverage and the autonomy rule.
func hypothesesFor(kind string, hit scopeHit, graphOK bool) []string {
	h := []string{
		"hipótesis: el objetivo se descompone en el DAG propuesto sin trabajo oculto (se valida en la primera tarea de relevo)",
	}
	if len(hit.components) > 0 {
		h = append(h, fmt.Sprintf("hipótesis: el alcance vive en %s (inferido del corpus F12.3; si el relevo lo desmiente, el plan se replanifica, no se improvisa)", strings.Join(hit.components, ", ")))
	} else {
		h = append(h, "hipótesis: alcance sin componentes identificables en el corpus (el plan pide aclaración antes de tocar ficheros)")
	}
	if graphOK {
		h = append(h, "hipótesis: el grafo F12.8 cubre consumidores y tests del alcance (verificado por tarea contra MapDiff)")
	} else {
		h = append(h, "hipótesis: grafo F12.8 no disponible (impacto tratado como desconocido: se pide aprobación, no se asume cobertura)")
	}
	if kind == KindComplex {
		h = append(h, "hipótesis: el trabajo se divide por componente porque cada clúster tiene distinta razón de cambio y distinto riesgo (ver explicación del plan)")
	}
	return h
}

// assumptionsFor records what the plan takes as given: mission
// bounds plus the non-invented defaults.
func assumptionsFor(m *mission.Mission, constraints []string) []string {
	a := []string{}
	for _, c := range constraints {
		a = append(a, "restricción de misión: "+c)
	}
	a = append(a, fmt.Sprintf("riesgo de misión %s (techo de autonomía de cada tarea)", m.Risk))
	if m.Budget.TimeMS != 0 || m.Budget.Tokens != 0 || m.Budget.Cost != 0 {
		a = append(a, fmt.Sprintf("presupuesto de misión: time=%dms tokens=%d cost=%v (0 = sin techo; las estimaciones se contrastan contra él)", m.Budget.TimeMS, m.Budget.Tokens, m.Budget.Cost))
	} else {
		a = append(a, "sin presupuesto de misión (las estimaciones siguen etiquetadas y acotadas por tarea)")
	}
	a = append(a, "solo capabilities F11.7 en estado probada con evidencia fechada; permisos mínimos (subconjunto de los del agente)")
	return a
}

// exclusionsFor names what the plan will not touch: mission
// "no/sin" constraints plus the durable store (only via CLI).
func exclusionsFor(constraints []string) []string {
	var out []string
	for _, c := range constraints {
		lower := strings.ToLower(strings.TrimSpace(c))
		if strings.HasPrefix(lower, "no ") || strings.HasPrefix(lower, "sin ") || strings.HasPrefix(lower, "never ") {
			out = append(out, c)
		}
	}
	out = append(out, ".regulus/** (estado durable: solo vía comandos regulus, nunca edición directa)")
	return out
}

// deliverablesFor names the plan outputs: one per scope cluster
// plus the verified whole.
func deliverablesFor(objective string, hit scopeHit) []string {
	var out []string
	for _, c := range hit.components {
		out = append(out, fmt.Sprintf("cambio verificado en %s hacia: %s", c, trimWords(objective, 12)))
	}
	if len(out) == 0 {
		out = append(out, "objetivo verificado: "+trimWords(objective, 12))
	}
	out = append(out, "reporte de verificación (tests + criterios Given/When/Then en verde)")
	return out
}

// trimWords shortens text to n words for one-line slots.
func trimWords(s string, n int) string {
	words := strings.Fields(s)
	if len(words) <= n {
		return s
	}
	return strings.Join(words[:n], " ") + "…"
}
