// Assessment pipeline (Phase 4, S2-S4): PERCEPTION → EVIDENCE → POLICY →
// ASSESSMENT → DECISION. Every check is a documented mechanical proxy, not
// simulated judgment: what the core cannot verify structurally becomes a
// finding or an abstention, never a confident verdict.
package decisions

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/guardians"
	"regulus-ai/internal/receipts"
)

// Finding codes.
const (
	FindingUnsupportedAssumption = "UNSUPPORTED_ASSUMPTION"
	FindingContradiction         = "CONTRADICTION"
	FindingUnjustifiedComplexity = "UNJUSTIFIED_COMPLEXITY"
	FindingHiddenDependency      = "HIDDEN_DEPENDENCY"
	FindingMissingAlternatives   = "MISSING_ALTERNATIVES"
	FindingUnmitigatedRisk       = "UNMITIGATED_RISK"
	FindingObsolete              = "OBSOLETE"
	FindingInsufficientEvidence  = "INSUFFICIENT_EVIDENCE"
)

// Severities: blocking findings refuse ACCEPT; warnings travel with it.
const (
	SeverityBlocking = "blocking"
	SeverityWarning  = "warning"
)

// EvidenceRef is one resolved contract reference.
type EvidenceRef struct {
	Raw      string
	Kind     string
	Resolved bool
	Detail   string
}

// Criterion is one assessed S3 dimension.
type Criterion struct {
	Name    string
	Verdict string
	Note    string
}

// Finding is one detection with its consequence.
type Finding struct {
	Code     string
	Severity string
	Detail   string
}

// Pipeline is the full assessment record.
type Pipeline struct {
	DecisionID     string
	Perception     string
	Evidence       []EvidenceRef
	Policy         PolicyResult
	Criteria       []Criterion
	Findings       []Finding
	Recommendation string
	Rationale      string
	Abstained      bool
	// Score/Grade carry the punctual quantitative signal (T4):
	// advisory only, never a transition input.
	Score int
	Grade string
}

// PolicyResult is the hard-gate layer resolve() enforces.
type PolicyResult struct {
	EvidenceOK  bool
	HasBlocking bool
	Critical    bool
	ObsoleteBy  string
	NeedsHuman  bool
	TaskVerdict string
	TaskLinked  bool
	TaskMissing bool
}

var (
	zeroDepRe  = regexp.MustCompile(`(?i)\bgo\s+get\b|\brequire\s+[\w./-]+\s+v\d`)
	absoluteRe = regexp.MustCompile(`(?i)\b(always|never|guarantees?|impossible|certain|obviously|clearly|trivially)\b`)
	mitigateRe = regexp.MustCompile(`(?i)(->|mitigat|handled by|fallback|rollback|fail open)`)
	costWordRe = regexp.MustCompile(`(?i)(cost|complex|slow|risk|debt|trade|expensive|cheap|simple|perf|overhead|price)`)
	wordRe     = regexp.MustCompile(`[a-z]{5,}`)
	chartRefRe = regexp.MustCompile(`^(?:charts/)?(.+?)(\.md)?$`)
	decRefRe   = regexp.MustCompile(`^dec:[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// ResolveEvidence resolves contract refs. Kinds: rec:<id>, receipt:<path>,
// chart:<name>, dec:<id>, path:<p>, task:<t>, or bare (auto-detected in
// that order: file, chart, receipt id or filename, decision).
func ResolveEvidence(root string, refs []string) []EvidenceRef {
	receiptIDs := map[string]bool{}
	receiptFiles := map[string]bool{}
	entries, _ := filepath.Glob(filepath.Join(evidence.Dir(root), "*", "*.json"))
	for _, p := range entries {
		if strings.HasPrefix(filepath.Base(p), "manifest-") {
			continue
		}
		rec, err := receipts.ReadFile(p)
		if err != nil || rec.ID == "" {
			continue
		}
		receiptIDs[rec.ID] = true
		receiptFiles[filepath.Base(p)] = true
	}
	decisions := map[string]bool{}
	if dir, err := os.ReadDir(Dir(root)); err == nil {
		for _, e := range dir {
			if strings.HasSuffix(e.Name(), ".json") {
				decisions["dec:"+strings.TrimSuffix(e.Name(), ".json")] = true
			}
		}
	}
	out := make([]EvidenceRef, 0, len(refs))
	for _, raw := range refs {
		r := strings.TrimSpace(raw)
		if r == "" {
			continue
		}
		ref := EvidenceRef{Raw: r}
		switch {
		case strings.HasPrefix(r, "rec:"):
			ref.Kind = "receipt"
			id := strings.TrimPrefix(r, "rec:")
			ref.Resolved = receiptIDs[id]
		case strings.HasPrefix(r, "receipt:"):
			ref.Kind = "receipt"
			p := filepath.Join(evidence.Dir(root), filepath.Base(strings.TrimPrefix(r, "receipt:")))
			ref.Resolved = fileExists(p)
			ref.Detail = p
		case strings.HasPrefix(r, "chart:"):
			ref.Kind = "chart"
			name := strings.TrimSuffix(strings.TrimPrefix(r, "chart:"), ".md")
			p := filepath.Join(root, "charts", name+".md")
			ref.Resolved = fileExists(p)
			ref.Detail = p
		case decRefRe.MatchString(r):
			ref.Kind = "decision"
			ref.Resolved = decisions[r]
		case strings.HasPrefix(r, "path:"):
			ref.Kind = "path"
			p := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r, "path:")))
			ref.Resolved = fileExists(p)
			ref.Detail = p
		case strings.HasPrefix(r, "task:"):
			ref.Kind = "task"
			name := strings.TrimPrefix(r, "task:")
			entries, err := evidence.List(root, name)
			ref.Resolved = err == nil && len(entries) > 0
		default:
			ref.Kind, ref.Resolved, ref.Detail = resolveBare(root, r, receiptIDs, receiptFiles, decisions)
		}
		out = append(out, ref)
	}
	return out
}

func resolveBare(root, r string, ids, files, decisions map[string]bool) (string, bool, string) {
	if fileExists(filepath.Join(root, filepath.FromSlash(r))) {
		return "path", true, r
	}
	if m := chartRefRe.FindStringSubmatch(r); m != nil {
		if p := filepath.Join(root, "charts", m[1]+".md"); fileExists(p) {
			return "chart", true, p
		}
	}
	if ids[r] || files[r] || files[filepath.Base(r)] {
		return "receipt", true, r
	}
	if decisions["dec:"+r] || decisions[r] {
		return "decision", true, r
	}
	return "unresolved", false, ""
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// Assess runs the pipeline over a stored-shaped decision. accepted holds
// other ACCEPTED decisions for the obsolete check.
//
// Assess is a thin orchestrator: perception, evidence, obsolete policy,
// the ten criteria and the recommendation each live in a small helper
// below so every unit stays under the CRAP budget. Findings, criteria
// order and recommendation strings are unchanged.
func Assess(root string, d Decision, accepted []Decision) Pipeline {
	p := Pipeline{DecisionID: d.ID}
	c := d.Contract
	grounded, perception := perceiveGrounding(c)
	p.Perception = perception
	resolved, policy, resolvedCount := collectEvidence(root, c)
	p.Evidence = resolved
	p.Policy = policy
	applyObsolete(&p, c, d.ID, accepted)
	applyTaskMissing(&p, c)
	criteria, findings := evalCriteria(root, c, grounded, resolvedCount)
	p.Criteria = criteria
	p.Findings = append(p.Findings, findings...)
	p.Findings = append(p.Findings, absoluteFinding(c, resolvedCount)...)
	recommendDecision(&p, c)
	priced := pricedAlternatives(c)
	total, mitigated := mitigatedRisks(c)
	p.Score, p.Grade = Score(ScoreInput{
		EvidenceOK: p.Policy.EvidenceOK, Blocking: countBlocking(p.Findings),
		AlternativesPriced: priced, RisksTotal: total, RisksMitigated: mitigated,
	})
	return p
}

// perceiveGrounding states the problem and reports whether it carries
// grounding hooks (evidence, task, or related decisions).
func perceiveGrounding(c Contract) (bool, string) {
	grounded := len(c.Evidence) > 0 || strings.TrimSpace(c.Task) != "" || len(c.Related) > 0
	if grounded {
		return true, "problem stated with grounding hooks (evidence, task, or related decisions)"
	}
	return false, "problem stated but ungrounded: no evidence, task, or related decisions"
}

// collectEvidence resolves every ref; task links resolve to receipt sets.
func collectEvidence(root string, c Contract) ([]EvidenceRef, PolicyResult, int) {
	var policy PolicyResult
	resolved := ResolveEvidence(root, c.Evidence)
	if strings.TrimSpace(c.Task) != "" {
		entries, err := evidence.List(root, c.Task)
		if err == nil && len(entries) > 0 {
			resolved = append(resolved, EvidenceRef{Raw: "task:" + c.Task, Kind: "task", Resolved: true})
			policy.TaskLinked = true
			policy.TaskVerdict = taskVerdict(root, c.Task)
		} else {
			resolved = append(resolved, EvidenceRef{Raw: "task:" + c.Task, Kind: "task", Resolved: false})
			policy.TaskMissing = true
		}
	}
	resolvedCount := countResolved(resolved)
	policy.EvidenceOK = resolvedCount > 0
	policy.Critical = c.Critical
	return resolved, policy, resolvedCount
}

// countResolved counts the refs that resolved.
func countResolved(refs []EvidenceRef) int {
	n := 0
	for _, r := range refs {
		if r.Resolved {
			n++
		}
	}
	return n
}

// applyObsolete runs the obsolete scope check against accepted decisions
// and records the blocking finding when the scope is already decided.
func applyObsolete(p *Pipeline, c Contract, id string, accepted []Decision) {
	for _, o := range accepted {
		if o.ID == id || o.State != StateAccepted {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(o.Contract.Scope), strings.TrimSpace(c.Scope)) && c.Scope != "" {
			p.Policy.ObsoleteBy = o.ID
			break
		}
	}
	if p.Policy.ObsoleteBy != "" {
		p.Findings = append(p.Findings, Finding{Code: FindingObsolete, Severity: SeverityBlocking,
			Detail: "scope already decided by " + p.Policy.ObsoleteBy + ": supersede it or narrow scope"})
	}
}

// applyTaskMissing records the blocking finding when the linked task has
// no receipts.
func applyTaskMissing(p *Pipeline, c Contract) {
	if p.Policy.TaskMissing {
		p.Findings = append(p.Findings, Finding{Code: FindingInsufficientEvidence, Severity: SeverityBlocking,
			Detail: "linked task " + c.Task + " has no receipts"})
	}
}

// evalCriteria runs the ten criteria in order, preserving the original
// criteria and findings order. The absolute-claims check travels with the
// recommendation helpers via absoluteFinding.
func evalCriteria(root string, c Contract, grounded bool, resolvedCount int) ([]Criterion, []Finding) {
	var criteria []Criterion
	var findings []Finding
	cr, f := criterionProblemExists(grounded, resolvedCount)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	cr, f = criterionProblemFit(c)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	cr, f = criterionAlternatives(c)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	cr, f = criterionConstraints(c)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	cr, f = criterionRisks(c)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	cr, f = criterionEvidence(c, resolvedCount)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	cr, f = criterionArchConsistency(c)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	cr, f = criterionComplexity(c)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	cr, f = criterionImpact(root, c)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	cr, f = criterionReversibility(c)
	criteria = append(criteria, cr)
	findings = append(findings, f...)
	return criteria, findings
}

// criterionProblemExists checks criterion 1: the problem is grounded.
func criterionProblemExists(grounded bool, resolvedCount int) (Criterion, []Finding) {
	if grounded && resolvedCount > 0 {
		return Criterion{Name: "problem exists", Verdict: "pass", Note: "stated with resolving evidence"}, nil
	}
	return Criterion{Name: "problem exists", Verdict: "fail", Note: "no grounding: add evidence, task, or related decisions"},
		[]Finding{{Code: FindingInsufficientEvidence, Severity: SeverityBlocking, Detail: "problem has no grounding"}}
}

// criterionProblemFit checks criterion 2: problem-solution correspondence
// (shared substance, not restatement).
func criterionProblemFit(c Contract) (Criterion, []Finding) {
	if c.Proposal != c.Problem && sharesSubstance(c.Problem+" "+c.Context, c.Proposal, c.Artifacts, c.Evidence) {
		return Criterion{Name: "problem-solution fit", Verdict: "pass", Note: "proposal shares substance with the problem or its artifacts"}, nil
	}
	return Criterion{Name: "problem-solution fit", Verdict: "fail", Note: "proposal restates or disconnects from the problem"},
		[]Finding{{Code: FindingContradiction, Severity: SeverityBlocking, Detail: "proposal does not engage the stated problem"}}
}

// criterionAlternatives checks criterion 3: alternatives with priced trade-offs.
func criterionAlternatives(c Contract) (Criterion, []Finding) {
	if len(c.Alternatives) == 0 {
		return Criterion{Name: "alternatives considered", Verdict: "fail", Note: "no alternatives stated"},
			[]Finding{{Code: FindingMissingAlternatives, Severity: SeverityBlocking, Detail: "no alternatives considered"}}
	}
	for _, a := range c.Alternatives {
		if strings.TrimSpace(a.Tradeoff) == "" {
			return Criterion{Name: "alternatives considered", Verdict: "fail", Note: "an alternative has no trade-off"},
				[]Finding{{Code: FindingMissingAlternatives, Severity: SeverityBlocking, Detail: "alternative without a priced trade-off"}}
		}
	}
	return Criterion{Name: "alternatives considered", Verdict: "pass", Note: "each alternative names its trade-off"}, nil
}

// criterionConstraints checks criterion 4: each constraint declares handling.
func criterionConstraints(c Contract) (Criterion, []Finding) {
	for _, k := range c.Constraints {
		parts := strings.SplitN(k, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return Criterion{Name: "constraints respected", Verdict: "fail", Note: "a constraint declares no handling"},
				[]Finding{{Code: FindingContradiction, Severity: SeverityBlocking, Detail: "constraint without declared handling"}}
		}
	}
	return Criterion{Name: "constraints respected", Verdict: "pass", Note: "every constraint declares its handling"}, nil
}

// criterionRisks checks criterion 5: risks identified with mitigations.
func criterionRisks(c Contract) (Criterion, []Finding) {
	if len(c.Risks) == 0 {
		return Criterion{Name: "risks identified", Verdict: "abstain", Note: "no risks stated; cannot judge exposure"},
			[]Finding{{Code: FindingUnmitigatedRisk, Severity: SeverityWarning, Detail: "no risks stated"}}
	}
	for _, r := range c.Risks {
		if !mitigateRe.MatchString(r) {
			return Criterion{Name: "risks identified", Verdict: "fail", Note: "a risk names no mitigation"},
				[]Finding{{Code: FindingUnmitigatedRisk, Severity: SeverityBlocking, Detail: "risk without mitigation"}}
		}
	}
	return Criterion{Name: "risks identified", Verdict: "pass", Note: "every risk names a mitigation"}, nil
}

// criterionEvidence checks criterion 6: evidence available.
func criterionEvidence(c Contract, resolvedCount int) (Criterion, []Finding) {
	if resolvedCount > 0 {
		return Criterion{Name: "evidence available", Verdict: "pass", Note: "at least one reference resolves"}, nil
	}
	if len(c.Evidence) > 0 || strings.TrimSpace(c.Task) != "" {
		return Criterion{Name: "evidence available", Verdict: "fail", Note: "references given but none resolve"},
			[]Finding{{Code: FindingInsufficientEvidence, Severity: SeverityBlocking, Detail: "cited evidence does not resolve"}}
	}
	return Criterion{Name: "evidence available", Verdict: "fail", Note: "no evidence cited at all"},
		[]Finding{{Code: FindingInsufficientEvidence, Severity: SeverityBlocking, Detail: "decision cites no evidence"}}
}

// criterionArchConsistency checks criterion 7: the zero-dep invariant is
// machine-checkable.
func criterionArchConsistency(c Contract) (Criterion, []Finding) {
	if zeroDepRe.MatchString(c.Proposal + "\n" + c.Context) {
		return Criterion{Name: "architectural consistency", Verdict: "fail", Note: "new Go module dependency contradicts the zero-dependency invariant"},
			[]Finding{{Code: FindingContradiction, Severity: SeverityBlocking, Detail: "new Go dependency vs zero-dep invariant"}}
	}
	return Criterion{Name: "architectural consistency", Verdict: "pass", Note: "no invariant contradiction detected"}, nil
}

// criterionComplexity checks criterion 8: complexity named in trade-offs
// when code is touched.
func criterionComplexity(c Contract) (Criterion, []Finding) {
	if len(c.Artifacts) == 0 {
		return Criterion{Name: "complexity justified", Verdict: "pass", Note: "no code touched"}, nil
	}
	joined := strings.Join(c.Tradeoffs, "\n")
	for _, a := range c.Alternatives {
		joined += "\n" + a.Tradeoff
	}
	if costWordRe.MatchString(joined) {
		return Criterion{Name: "complexity justified", Verdict: "pass", Note: "trade-offs name costs"}, nil
	}
	return Criterion{Name: "complexity justified", Verdict: "fail", Note: "code touched but no cost named in trade-offs"},
		[]Finding{{Code: FindingUnjustifiedComplexity, Severity: SeverityBlocking, Detail: "complexity introduced without a named cost"}}
}

// criterionImpact checks criterion 9: artifacts exist or the gap is named.
func criterionImpact(root string, c Contract) (Criterion, []Finding) {
	if len(c.Artifacts) == 0 {
		return Criterion{Name: "impact stated", Verdict: "abstain", Note: "no artifacts listed; blast radius unknown"},
			[]Finding{{Code: FindingHiddenDependency, Severity: SeverityWarning, Detail: "no artifacts listed"}}
	}
	for _, a := range c.Artifacts {
		if !fileExists(filepath.Join(root, filepath.FromSlash(a))) {
			return Criterion{Name: "impact stated", Verdict: "fail", Note: "a listed artifact does not exist"},
				[]Finding{{Code: FindingHiddenDependency, Severity: SeverityBlocking, Detail: "artifact path resolves to nothing"}}
		}
	}
	return Criterion{Name: "impact stated", Verdict: "pass", Note: "all artifacts resolve"}, nil
}

// criterionReversibility checks criterion 10: a way back when hard.
func criterionReversibility(c Contract) (Criterion, []Finding) {
	if c.Reversibility == "hard" && strings.TrimSpace(c.Rollback) == "" {
		return Criterion{Name: "reversibility", Verdict: "fail", Note: "hard reversal needs a rollback note"},
			[]Finding{{Code: FindingUnmitigatedRisk, Severity: SeverityBlocking, Detail: "hard to reverse with no way back"}}
	}
	return Criterion{Name: "reversibility", Verdict: "pass", Note: c.Reversibility + " reversibility with rollback note where required"}, nil
}

// absoluteFinding warns on unsupported assumptions: absolute language with
// nothing behind it.
func absoluteFinding(c Contract, resolvedCount int) []Finding {
	if absoluteRe.MatchString(c.Consequences+"\n"+strings.Join(c.Risks, "\n")+"\n"+c.Proposal) && resolvedCount == 0 {
		return []Finding{{Code: FindingUnsupportedAssumption, Severity: SeverityWarning, Detail: "absolute claims with no resolving evidence"}}
	}
	return nil
}

// recommendDecision recommends; abstention wins on insufficient evidence.
func recommendDecision(p *Pipeline, c Contract) {
	for _, f := range p.Findings {
		if f.Severity == SeverityBlocking {
			p.Policy.HasBlocking = true
		}
	}
	p.Policy.NeedsHuman = c.Critical
	switch {
	case hasCode(p.Findings, FindingInsufficientEvidence):
		p.Recommendation, p.Rationale, p.Abstained = "INCONCLUSIVE", "the system abstains: evidence does not resolve", true
	case p.Policy.HasBlocking:
		p.Recommendation, p.Rationale = "REJECT", "blocking findings must be cleared first"
	default:
		p.Recommendation, p.Rationale = "ACCEPT", "justified, constrained, and evidenced"
		if len(p.Findings) > 0 {
			p.Rationale += " (with warnings noted)"
		}
	}
}

func sharesSubstance(problem, proposal string, artifacts, evidence []string) bool {
	pt := contentTokens(problem)
	for w := range contentTokens(proposal) {
		if pt[w] {
			return true
		}
	}
	text := strings.ToLower(proposal)
	for _, a := range append(append([]string{}, artifacts...), evidence...) {
		for _, part := range strings.FieldsFunc(strings.ToLower(a), func(r rune) bool {
			return r == '/' || r == '.' || r == '-' || r == '_' || r == ':'
		}) {
			if len(part) >= 5 && strings.Contains(text, part) {
				return true
			}
		}
	}
	return false
}

func contentTokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(s), -1) {
		out[w] = true
	}
	return out
}

// taskVerdict consolidates the linked task's receipts through the Phase 3
// verdict engine. Unknown tasks were already filtered by the caller.
func taskVerdict(root, task string) string {
	con, err := guardians.Consolidate(root, task)
	if err != nil {
		return "INCONCLUSIVE"
	}
	return con.Verdict
}

func hasCode(findings []Finding, code string) bool {
	for _, f := range findings {
		if f.Code == code {
			return true
		}
	}
	return false
}
