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
func Assess(root string, d Decision, accepted []Decision) Pipeline {
	p := Pipeline{DecisionID: d.ID}
	c := d.Contract

	// PERCEPTION: the problem must be stated and grounded.
	grounded := len(c.Evidence) > 0 || strings.TrimSpace(c.Task) != "" || len(c.Related) > 0
	if grounded {
		p.Perception = "problem stated with grounding hooks (evidence, task, or related decisions)"
	} else {
		p.Perception = "problem stated but ungrounded: no evidence, task, or related decisions"
	}

	// EVIDENCE: resolve every ref; task links resolve to receipt sets.
	resolved := ResolveEvidence(root, c.Evidence)
	if strings.TrimSpace(c.Task) != "" {
		entries, err := evidence.List(root, c.Task)
		if err == nil && len(entries) > 0 {
			resolved = append(resolved, EvidenceRef{Raw: "task:" + c.Task, Kind: "task", Resolved: true})
			p.Policy.TaskLinked = true
			p.Policy.TaskVerdict = taskVerdict(root, c.Task)
		} else {
			resolved = append(resolved, EvidenceRef{Raw: "task:" + c.Task, Kind: "task", Resolved: false})
			p.Policy.TaskMissing = true
		}
	}
	p.Evidence = resolved
	resolvedCount := 0
	for _, r := range resolved {
		if r.Resolved {
			resolvedCount++
		}
	}
	p.Policy.EvidenceOK = resolvedCount > 0
	p.Policy.Critical = c.Critical

	// POLICY: obsolete scope check against accepted decisions.
	for _, o := range accepted {
		if o.ID == d.ID || o.State != StateAccepted {
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
	if p.Policy.TaskMissing {
		p.Findings = append(p.Findings, Finding{Code: FindingInsufficientEvidence, Severity: SeverityBlocking,
			Detail: "linked task " + c.Task + " has no receipts"})
	}

	// ASSESSMENT: the ten criteria.
	add := func(name, v, note string) {
		p.Criteria = append(p.Criteria, Criterion{Name: name, Verdict: v, Note: note})
	}
	block := func(code, detail string) {
		p.Findings = append(p.Findings, Finding{Code: code, Severity: SeverityBlocking, Detail: detail})
	}
	warn := func(code, detail string) {
		p.Findings = append(p.Findings, Finding{Code: code, Severity: SeverityWarning, Detail: detail})
	}

	// 1. problem exists and is grounded.
	if grounded && resolvedCount > 0 {
		add("problem exists", "pass", "stated with resolving evidence")
	} else {
		add("problem exists", "fail", "no grounding: add evidence, task, or related decisions")
		block(FindingInsufficientEvidence, "problem has no grounding")
	}

	// 2. problem↔solution correspondence (shared substance, not restatement).
	if c.Proposal != c.Problem && sharesSubstance(c.Problem+" "+c.Context, c.Proposal, c.Artifacts, c.Evidence) {
		add("problem-solution fit", "pass", "proposal shares substance with the problem or its artifacts")
	} else {
		add("problem-solution fit", "fail", "proposal restates or disconnects from the problem")
		block(FindingContradiction, "proposal does not engage the stated problem")
	}

	// 3. alternatives with priced trade-offs.
	if len(c.Alternatives) == 0 {
		add("alternatives considered", "fail", "no alternatives stated")
		block(FindingMissingAlternatives, "no alternatives considered")
	} else {
		priced := true
		for _, a := range c.Alternatives {
			if strings.TrimSpace(a.Tradeoff) == "" {
				priced = false
			}
		}
		if priced {
			add("alternatives considered", "pass", "each alternative names its trade-off")
		} else {
			add("alternatives considered", "fail", "an alternative has no trade-off")
			block(FindingMissingAlternatives, "alternative without a priced trade-off")
		}
	}

	// 4. constraints respected (each declares handling).
	badConstraints := false
	for _, k := range c.Constraints {
		parts := strings.SplitN(k, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			badConstraints = true
		}
	}
	if badConstraints {
		add("constraints respected", "fail", "a constraint declares no handling")
		block(FindingContradiction, "constraint without declared handling")
	} else {
		add("constraints respected", "pass", "every constraint declares its handling")
	}

	// 5. risks identified with mitigations.
	if len(c.Risks) == 0 {
		add("risks identified", "abstain", "no risks stated; cannot judge exposure")
		warn(FindingUnmitigatedRisk, "no risks stated")
	} else {
		unmitigated := false
		for _, r := range c.Risks {
			if !mitigateRe.MatchString(r) {
				unmitigated = true
			}
		}
		if unmitigated {
			add("risks identified", "fail", "a risk names no mitigation")
			block(FindingUnmitigatedRisk, "risk without mitigation")
		} else {
			add("risks identified", "pass", "every risk names a mitigation")
		}
	}

	// 6. evidence available.
	if resolvedCount > 0 {
		add("evidence available", "pass", "at least one reference resolves")
	} else if len(c.Evidence) > 0 || strings.TrimSpace(c.Task) != "" {
		add("evidence available", "fail", "references given but none resolve")
		block(FindingInsufficientEvidence, "cited evidence does not resolve")
	} else {
		add("evidence available", "fail", "no evidence cited at all")
		block(FindingInsufficientEvidence, "decision cites no evidence")
	}

	// 7. architectural consistency (zero-dep invariant is machine-checkable).
	if zeroDepRe.MatchString(c.Proposal + "\n" + c.Context) {
		add("architectural consistency", "fail", "new Go module dependency contradicts the zero-dependency invariant")
		block(FindingContradiction, "new Go dependency vs zero-dep invariant")
	} else {
		add("architectural consistency", "pass", "no invariant contradiction detected")
	}

	// 8. complexity named in trade-offs when code is touched.
	if len(c.Artifacts) > 0 {
		joined := strings.Join(c.Tradeoffs, "\n")
		for _, a := range c.Alternatives {
			joined += "\n" + a.Tradeoff
		}
		if costWordRe.MatchString(joined) {
			add("complexity justified", "pass", "trade-offs name costs")
		} else {
			add("complexity justified", "fail", "code touched but no cost named in trade-offs")
			block(FindingUnjustifiedComplexity, "complexity introduced without a named cost")
		}
	} else {
		add("complexity justified", "pass", "no code touched")
	}

	// 9. impact: artifacts exist or the gap is named.
	if len(c.Artifacts) == 0 {
		add("impact stated", "abstain", "no artifacts listed; blast radius unknown")
		warn(FindingHiddenDependency, "no artifacts listed")
	} else {
		missing := false
		for _, a := range c.Artifacts {
			if !fileExists(filepath.Join(root, filepath.FromSlash(a))) {
				missing = true
			}
		}
		if missing {
			add("impact stated", "fail", "a listed artifact does not exist")
			block(FindingHiddenDependency, "artifact path resolves to nothing")
		} else {
			add("impact stated", "pass", "all artifacts resolve")
		}
	}

	// 10. reversibility with a way back when hard.
	if c.Reversibility == "hard" && strings.TrimSpace(c.Rollback) == "" {
		add("reversibility", "fail", "hard reversal needs a rollback note")
		block(FindingUnmitigatedRisk, "hard to reverse with no way back")
	} else {
		add("reversibility", "pass", c.Reversibility+" reversibility with rollback note where required")
	}

	// Unsupported assumptions: absolute language with nothing behind it.
	if absoluteRe.MatchString(c.Consequences+"\n"+strings.Join(c.Risks, "\n")+"\n"+c.Proposal) && resolvedCount == 0 {
		warn(FindingUnsupportedAssumption, "absolute claims with no resolving evidence")
	}

	// DECISION: recommend; abstain on insufficient evidence.
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
	return p
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
