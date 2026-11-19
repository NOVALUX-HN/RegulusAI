// Cross-agent learning (F12.6/NOV-101): typed learn knowledge with
// provenance, ACL-scoped retrieval, contradiction resolution,
// invalidation and quality metrics — on the Almanac substrate, never
// beside it.
//
// Reuse map (no parallel stores, no parallel shapes):
//   - storage: Observation (almanac.go) carries the five new learn
//     kinds plus decision; revision chains (History), pins and the
//     contentHash idempotency + vector dedup (dedup.go) apply unchanged;
//   - retirement: supersedes/invalidated_by relations (events.go) and
//     REJECTED governance (status.go) hide retired knowledge from
//     default retrieval — invalidation is a status event, the audit
//     trail is status.jsonl;
//   - permissions: the F11.8 closed vocabulary — project/task learn
//     knowledge requires memory:read (MemoryReadScope mirrors
//     mcp.ScopeMemoryRead without importing mcp; the Almanac stays
//     dependency-free);
//   - ranking: the deterministic order (sortHits: pinned, confidence,
//     recency) — the same decider the F12.3 engine and HybridSearch
//     share;
//   - audit: the F11.1 exec log (observability.RecordExec) via the CLI,
//     the engine itself stays pure.
package almanac

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Learn knowledge kinds. Decision reuses the Almanac decision type
// (one substrate, typed views); the other five are new observation
// types admitted by validType.
const (
	KindVerified   = "verified-solution"
	KindPattern    = "pattern"
	KindDecision   = TypeDecision
	KindError      = "error"
	KindWorkaround = "workaround"
	KindHypothesis = "hypothesis"
)

// Learn visibility scopes: private (author only), task (same task or
// author, plus memory:read), project (memory:read), shared (public —
// the cross-agent pool; secrets are still redacted at save).
const (
	ScopePrivate = "private"
	ScopeTask    = "task"
	ScopeProject = "project"
	ScopeShared  = "shared"
)

// MemoryReadScope is the F11.8 scope project/task learn knowledge
// requires. It mirrors mcp.ScopeMemoryRead as a literal so the
// Almanac stays dependency-free (redact.go); a test pins the
// equality, so a rename breaks loudly instead of silently opening.
const MemoryReadScope = "memory:read"

// IsLearnKind reports whether an observation type is cross-agent
// learn knowledge (the five new kinds plus decision).
func IsLearnKind(t string) bool {
	switch t {
	case KindVerified, KindPattern, KindDecision, KindError, KindWorkaround, KindHypothesis:
		return true
	}
	return false
}

// ValidScope reports whether s is a known visibility scope.
func ValidScope(s string) bool {
	switch s {
	case ScopePrivate, ScopeTask, ScopeProject, ScopeShared:
		return true
	}
	return false
}

// validConfidence reports whether c is a known self-assessed
// confidence (lessons keep their own inline check; learn knowledge
// funnels here).
func validConfidence(c string) bool {
	switch c {
	case "low", "medium", "high":
		return true
	}
	return false
}

// validateLearn gates learn knowledge at the Save/Import choke
// point (called from validate): scope, confidence and author are
// mandatory provenance; verified-solution additionally needs cited
// evidence — "verificado" without proof never enters. Legacy types
// keep their shape: scope/confidence are only checked when set.
//
// Decision compatibility: bare decisions (no learn fields at all)
// stay legacy — memory save owns them and existing tests/logs keep
// working. Setting any learn field (scope, author, confidence,
// evidence) opts the decision into full learn validation.
func validateLearn(o *Observation) error {
	if !IsLearnKind(o.Type) {
		if o.Scope != "" && !ValidScope(o.Scope) {
			return fmt.Errorf("scope %q is not one of private, task, project, shared", o.Scope)
		}
		if o.Confidence != "" && !validConfidence(o.Confidence) {
			return fmt.Errorf("confidence must be low, medium or high, got %q", o.Confidence)
		}
		return nil
	}
	if o.Type == KindDecision && o.Scope == "" && strings.TrimSpace(o.Author) == "" && o.Confidence == "" && len(o.Evidence) == 0 {
		return nil
	}
	if !ValidScope(o.Scope) {
		return fmt.Errorf("scope %q is not one of private, task, project, shared (learn knowledge separates private/task/project/shared)", o.Scope)
	}
	if !validConfidence(o.Confidence) {
		return fmt.Errorf("confidence must be low, medium or high, got %q", o.Confidence)
	}
	if strings.TrimSpace(o.Author) == "" {
		return errors.New("learn knowledge needs an author (autor/agente)")
	}
	if o.Type == KindVerified && len(o.Evidence) == 0 {
		return errors.New("verified-solution needs evidence: cite at least one test or proof reference (--evidence)")
	}
	return nil
}

// LearnSave is the learn write choke point: only learn kinds enter
// here (memory save owns the rest), then the atomic SaveChecked
// path — contentHash idempotency plus the vector near-duplicate bar
// (deduplication inherited, not reimplemented).
func LearnSave(root string, o *Observation, now time.Time) (Observation, error) {
	if o == nil {
		return Observation{}, errors.New("learn save needs an observation")
	}
	if !IsLearnKind(o.Type) {
		return Observation{}, fmt.Errorf("type %q is not learn knowledge (verified-solution, pattern, decision, error, workaround, hypothesis) — regulus memory save owns the rest", o.Type)
	}
	return SaveChecked(root, o, now)
}

// evidenceGatedKind reports whether a learn type verifies only
// against cited top-level evidence (F12.6): the five new kinds.
// Decisions keep their legacy reviewer-judgment path (existing tests
// and logs verify bare decisions with a note) — the machine gate
// guards the new semantics without rewriting legacy governance.
func isEvidenceGatedKind(t string) bool {
	switch t {
	case KindVerified, KindPattern, KindError, KindWorkaround, KindHypothesis:
		return true
	}
	return false
}

// LearnViewer is the retrieving agent: identity, held F11.8 scopes
// and current task. Zero value is the anonymous reader (shared only).
type LearnViewer struct {
	AgentID string
	Scopes  []string
	Task    string
}

// CanRead is the ACL decider: private reads author-only; task reads
// author or same provenance task (both need memory:read); project
// reads memory:read; shared is public; unknown scopes deny closed.
func CanRead(o Observation, v LearnViewer) bool {
	held := map[string]bool{}
	for _, s := range v.Scopes {
		held[strings.TrimSpace(s)] = true
	}
	switch o.Scope {
	case ScopeShared:
		return true
	case ScopePrivate:
		return v.AgentID != "" && v.AgentID == o.Author
	case ScopeTask:
		if v.AgentID != "" && v.AgentID == o.Author {
			return true
		}
		if !held[MemoryReadScope] {
			return false
		}
		if o.Provenance == nil || strings.TrimSpace(o.Provenance.Task) == "" || strings.TrimSpace(v.Task) == "" {
			return false
		}
		return o.Provenance.Task == v.Task
	case "", ScopeProject:
		return held[MemoryReadScope]
	default:
		return false
	}
}

// LearnQuery is one learn retrieval: relevance terms, kind/scope
// filters, the verified-only gate (hypotheses never pass it —
// neither does anything unverified), staleness/supersede handling
// (hidden by default, like Search) and the viewer for ACLs.
type LearnQuery struct {
	Terms             []string
	Kinds             []string
	Scopes            []string
	VerifiedOnly      bool
	IncludeStale      bool
	IncludeSuperseded bool
	Limit             int
	Viewer            LearnViewer
}

// LearnHit is one retrieved observation with its explicit label:
// Verified (status VERIFIED/ACCEPTED against cited evidence) or
// Hypothesis (never a fact, even when it matches). The label is the
// hypothesis≠fact guarantee at the retrieval boundary.
type LearnHit struct {
	Observation Observation
	Verified    bool
	Hypothesis  bool
}

// LearnLabel renders one hit's status line tag: hypotheses warn they
// are not facts; verified knowledge says so; the rest reads honestly
// unverified.
func LearnLabel(o Observation, verified bool) string {
	if o.Type == KindHypothesis {
		return "hipótesis — no es un hecho"
	}
	if verified {
		return "verificado"
	}
	return "no verificado"
}

// verifiedLearnIDs folds the governance log to the learn-kind
// observations whose latest state is VERIFIED or ACCEPTED —
// evidence-gated by RecordStatus (hypotheses can never land here,
// unverified kinds never qualify).
func verifiedLearnIDs(all []Observation, ends map[string]string) map[string]bool {
	out := map[string]bool{}
	for _, o := range all {
		if !IsLearnKind(o.Type) {
			continue
		}
		if ends[o.ID] == StatusVerified || ends[o.ID] == StatusAccepted {
			out[o.ID] = true
		}
	}
	return out
}

// validateLearnQuery rejects unknown kinds and scopes before any
// disk read. Pure: no I/O, table-testable.
func validateLearnQuery(q LearnQuery) error {
	for _, k := range q.Kinds {
		if !IsLearnKind(k) {
			return fmt.Errorf("kind %q is not learn knowledge (verified-solution, pattern, decision, error, workaround, hypothesis)", k)
		}
	}
	for _, s := range q.Scopes {
		if !ValidScope(s) {
			return fmt.Errorf("scope %q is not one of private, task, project, shared", s)
		}
	}
	return nil
}

// learnSearchState is the loaded retrieval state: every observation
// plus the pin, retirement and governance folds and the want-sets.
// The expiry clock is captured after the loads, like the single
// function was before the split.
type learnSearchState struct {
	all          []Observation
	pins         map[string]bool
	superseded   map[string]bool
	stale        map[string]bool
	ends         map[string]string
	wantKind     map[string]bool
	wantScope    map[string]bool
	includeStale bool
	now          time.Time
}

// loadLearnSearchState reads the Almanac substrate once: the log,
// the pins, the retirement folds (skipped under the include flags)
// and the governance ends, plus the kind/scope want-sets.
func loadLearnSearchState(root string, q LearnQuery) (*learnSearchState, error) {
	s := &learnSearchState{includeStale: q.IncludeStale}
	var err error
	if s.all, err = All(root); err != nil {
		return nil, err
	}
	if s.pins, err = PinnedIDs(root); err != nil {
		return nil, err
	}
	if !q.IncludeSuperseded {
		if s.superseded, err = SupersededIDs(root); err != nil {
			return nil, err
		}
	}
	if !q.IncludeStale {
		if s.stale, err = staleIDs(root); err != nil {
			return nil, err
		}
	}
	if s.ends, err = statusEnds(root); err != nil {
		return nil, err
	}
	s.wantKind = map[string]bool{}
	for _, k := range q.Kinds {
		s.wantKind[k] = true
	}
	s.wantScope = map[string]bool{}
	for _, sc := range q.Scopes {
		s.wantScope[sc] = true
	}
	s.now = time.Now().UTC()
	return s, nil
}

// kindScopeOK reports whether one observation passes the kind and
// scope filters. Pure.
func (s *learnSearchState) kindScopeOK(o Observation) bool {
	if len(s.wantKind) > 0 && !s.wantKind[o.Type] {
		return false
	}
	return len(s.wantScope) == 0 || s.wantScope[o.Scope]
}

// retirementHidden reports whether one observation hides under the
// retirement folds: superseded targets, stale (invalidated/
// rejected) ids and expired observations — the last only while the
// caller does not include stale. Pure.
func (s *learnSearchState) retirementHidden(o Observation) bool {
	if s.superseded[o.ID] {
		return true
	}
	if s.stale[o.ID] {
		return true
	}
	return o.ExpiresAt != nil && !s.now.Before(*o.ExpiresAt) && !s.includeStale
}

// verifiedPass is the VerifiedOnly gate: hypotheses are excluded
// unconditionally (imported status history restores verbatim, so
// the exclusion never trusts the status alone) and unverified
// knowledge never passes. Pure.
func verifiedPass(o Observation, verified, verifiedOnly bool) bool {
	return !verifiedOnly || (o.Type != KindHypothesis && verified)
}

// collectLearnMatches folds one pass over the log: kind and scope
// filters, term match (AND over title+content), the VerifiedOnly
// gate, retirement hiding and the ACL split. Observations that
// match everything but the ACL count as denied.
func collectLearnMatches(s *learnSearchState, q LearnQuery) (matched []Observation, denied int) {
	for _, o := range s.all {
		if !IsLearnKind(o.Type) {
			continue
		}
		if !s.kindScopeOK(o) {
			continue
		}
		if s.retirementHidden(o) {
			continue
		}
		if !matchTerms(o, q.Terms, false) {
			continue
		}
		st := s.ends[o.ID]
		verified := st == StatusVerified || st == StatusAccepted
		if !verifiedPass(o, verified, q.VerifiedOnly) {
			continue
		}
		if !CanRead(o, q.Viewer) {
			denied++
			continue
		}
		matched = append(matched, o)
	}
	return matched, denied
}

// learnHitsFrom tags the ordered matches with their explicit label:
// Verified (status VERIFIED/ACCEPTED against cited evidence) or
// Hypothesis (never a fact, even when it matches). Pure.
func learnHitsFrom(matched []Observation, ends map[string]string) []LearnHit {
	var hits []LearnHit
	for _, o := range matched {
		st := ends[o.ID]
		hits = append(hits, LearnHit{
			Observation: o,
			Verified:    st == StatusVerified || st == StatusAccepted,
			Hypothesis:  o.Type == KindHypothesis,
		})
	}
	return hits
}

// LearnSearch retrieves learn knowledge by relevance: kind and scope
// filters, term match (AND over title+content), the VerifiedOnly
// gate (hypotheses excluded unconditionally — imported status
// history restores verbatim, so the exclusion never trusts the
// status alone), retirement hiding (superseded/invalidated/rejected/
// expired) and the ACL split. Order is the shared deterministic
// order (pinned, confidence, recency); Limit keeps the first N.
// The denied count names observations that matched everything but
// the ACL — retrieval respects permissions and says so.
func LearnSearch(root string, q LearnQuery) (hits []LearnHit, denied int, err error) {
	if err := validateLearnQuery(q); err != nil {
		return nil, 0, err
	}
	s, err := loadLearnSearchState(root, q)
	if err != nil {
		return nil, 0, err
	}
	matched, denied := collectLearnMatches(s, q)
	sortHits(s.pins, matched)
	if q.Limit > 0 && len(matched) > q.Limit {
		matched = matched[:q.Limit]
	}
	return learnHitsFrom(matched, s.ends), denied, nil
}

// InvalidateKnowledge retires knowledge with a recorded reason: the
// REJECTED transition (actor + reason) is the invalidation, and the
// status log is the audit. Invalidated knowledge hides from default
// retrieval (staleIDs) and reads back with --include-stale.
func InvalidateKnowledge(root, id, by, reason string, now time.Time) error {
	if strings.TrimSpace(by) == "" {
		return errors.New("invalidation needs an actor (by)")
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("invalidation needs a reason")
	}
	return RecordStatus(root, id, StatusRejected, by, "invalidated: "+reason, now)
}

// ResolveContradiction settles one live contradiction: the winner
// supersedes the loser (attributed to the resolver), so the loser
// hides from default retrieval while history keeps both. Both ids
// must exist and contradict each other while active — resolving
// settled or unrelated knowledge is a usage error, not a write.
func ResolveContradiction(root, winner, loser, by string, now time.Time) error {
	if strings.TrimSpace(by) == "" {
		return errors.New("resolution needs an actor (by)")
	}
	live, err := Contradictions(root, now)
	if err != nil {
		return err
	}
	joined := false
	for _, c := range live {
		if (c.A == winner && c.B == loser) || (c.A == loser && c.B == winner) {
			joined = true
			break
		}
	}
	if !joined {
		return fmt.Errorf("no live contradiction between %q and %q (resolve settles active conflicts_with only)", winner, loser)
	}
	return RelateAttributed(root, winner, RelationSupersedes, loser, by, now)
}

// LearnMetrics is the quality report over learn knowledge: counts
// plus the four spec rates — precision (verified over decided),
// useful recall (verified still active over verified), obsolescence
// (retired over total) and noise (rejected+invalidated+live
// contradiction sides over total). Empty denominators read clean
// (precision/recall 1, rates 0): nothing wrong yet, never a divide
// by zero. HypothesesVerified must stay 0 — the metric watches the
// invariant the retrieval boundary enforces.
type LearnMetrics struct {
	Total              int     `json:"total"`
	Verified           int     `json:"verified"`
	VerifiedEver       int     `json:"verified_ever"`
	Hypotheses         int     `json:"hypotheses"`
	HypothesesVerified int     `json:"hypotheses_verified"`
	Active             int     `json:"active"`
	Obsolete           int     `json:"obsolete"`
	Rejected           int     `json:"rejected"`
	Invalidated        int     `json:"invalidated"`
	Superseded         int     `json:"superseded"`
	Expired            int     `json:"expired"`
	Contradictions     int     `json:"contradictions"`
	Precision          float64 `json:"precision"`
	UsefulRecall       float64 `json:"useful_recall"`
	ObsoleteRate       float64 `json:"obsolescence"`
	NoiseRate          float64 `json:"noise"`
}

// isVerifiedStatus reports whether a governance end state counts as
// verified knowledge: VERIFIED or ACCEPTED against cited evidence.
// Pure: shared by the metrics folds so the spec gate reads one way.
func isVerifiedStatus(st string) bool {
	return st == StatusVerified || st == StatusAccepted
}

// learnExpired reports whether an observation is past its expiry at
// now. Pure: the clock is the caller's, like the metrics fold.
func learnExpired(o Observation, now time.Time) bool {
	return o.ExpiresAt != nil && !now.Before(*o.ExpiresAt)
}

// learnKindObservations filters the log to learn knowledge.
// Pure: no disk, table-testable.
func learnKindObservations(all []Observation) []Observation {
	var learn []Observation
	for _, o := range all {
		if IsLearnKind(o.Type) {
			learn = append(learn, o)
		}
	}
	return learn
}

// verifiedEverIDs folds every status history to the observations that
// were ever VERIFIED or ACCEPTED. Histories load in log order; the
// first verified transition marks the id, like the inline loop did.
func verifiedEverIDs(root string, learn []Observation) (map[string]bool, error) {
	ever := map[string]bool{}
	for _, o := range learn {
		hist, err := StatusHistory(root, o.ID)
		if err != nil {
			return nil, err
		}
		for _, ev := range hist {
			if isVerifiedStatus(ev.To) {
				ever[o.ID] = true
				break
			}
		}
	}
	return ever, nil
}

// learnContradictionSides counts the live contradiction pairs that
// touch learn knowledge and names the distinct learn-kind sides, so
// noise counts each contradicted observation once. Pure.
func learnContradictionSides(live []Conflict, isLearn map[string]bool) (int, map[string]bool) {
	sides := map[string]bool{}
	count := 0
	for _, c := range live {
		if isLearn[c.A] || isLearn[c.B] {
			count++
			if isLearn[c.A] {
				sides[c.A] = true
			}
			if isLearn[c.B] {
				sides[c.B] = true
			}
		}
	}
	return count, sides
}

// learnFoldCounts is the per-observation tally the metrics fold
// accumulates before the four spec rates divide it.
type learnFoldCounts struct {
	verifiedLatest     int
	verifiedActive     int
	hypotheses         int
	hypothesesVerified int
	active             int
	obsolete           int
	rejected           int
	invalidated        int
	superseded         int
	expired            int
}

// foldLearnObservations tallies one pass over the learn-kind log:
// hypothesis counts (plus the invariant watch), latest-verified,
// still-useful verified, retirement (superseded/invalidated/
// rejected/expired) and the rejected/invalidated/superseded split.
// Pure: every store fold (ends, sup, inv, verifiedEver) loads above.
func foldLearnObservations(learn []Observation, ends map[string]string, sup, inv map[string]bool, verifiedEver map[string]bool, now time.Time) learnFoldCounts {
	var c learnFoldCounts
	for _, o := range learn {
		st := ends[o.ID]
		latest := isVerifiedStatus(st)
		expired := learnExpired(o, now)
		retired := sup[o.ID] || inv[o.ID] || st == StatusRejected || expired
		if o.Type == KindHypothesis {
			c.hypotheses++
			if verifiedEver[o.ID] {
				c.hypothesesVerified++
			}
		}
		if latest {
			c.verifiedLatest++
		}
		if verifiedEver[o.ID] && !retired {
			c.verifiedActive++
		}
		if retired {
			c.obsolete++
		} else {
			c.active++
		}
		switch {
		case st == StatusRejected:
			c.rejected++
		case inv[o.ID]:
			c.invalidated++
		case sup[o.ID]:
			c.superseded++
		}
		if expired {
			c.expired++
		}
	}
	return c
}

// learnPrecisionRate is verified over decided (verified plus
// rejected plus invalidated). Empty denominators read clean (1):
// nothing wrong yet, never a divide by zero. Pure.
func learnPrecisionRate(verifiedLatest, rejected, invalidated int) float64 {
	decided := verifiedLatest + rejected + invalidated
	if decided == 0 {
		return 1
	}
	return float64(verifiedLatest) / float64(decided)
}

// learnRecallRate is verified-still-active over verified-ever.
// Empty denominators read clean (1). Pure.
func learnRecallRate(verifiedActive, verifiedEver int) float64 {
	if verifiedEver == 0 {
		return 1
	}
	return float64(verifiedActive) / float64(verifiedEver)
}

// learnObsoleteRate is retired over total. Pure.
func learnObsoleteRate(obsolete, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(obsolete) / float64(total)
}

// learnNoiseRate is rejected plus invalidated plus distinct
// contradicted sides over total. Pure.
func learnNoiseRate(rejected, invalidated, contradicted, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(rejected+invalidated+contradicted) / float64(total)
}

// ComputeLearnMetrics computes the quality report over learn-kind
// observations at now. Contradiction pairs count when at least one
// endpoint is learn knowledge; noise counts distinct learn-kind
// sides.
func ComputeLearnMetrics(root string, now time.Time) (LearnMetrics, error) {
	var m LearnMetrics
	all, err := All(root)
	if err != nil {
		return m, err
	}
	learn := learnKindObservations(all)
	m.Total = len(learn)
	if m.Total == 0 {
		m.Precision, m.UsefulRecall = 1, 1
		return m, nil
	}
	sup, err := SupersededIDs(root)
	if err != nil {
		return m, err
	}
	inv, err := InvalidatedIDs(root)
	if err != nil {
		return m, err
	}
	ends, err := statusEnds(root)
	if err != nil {
		return m, err
	}
	verifiedEver, err := verifiedEverIDs(root, learn)
	if err != nil {
		return m, err
	}
	live, err := Contradictions(root, now)
	if err != nil {
		return m, err
	}
	isLearn := map[string]bool{}
	for _, o := range learn {
		isLearn[o.ID] = true
	}
	contradictions, contrSides := learnContradictionSides(live, isLearn)
	fold := foldLearnObservations(learn, ends, sup, inv, verifiedEver, now)
	m.Contradictions = contradictions
	m.Verified = fold.verifiedLatest
	m.VerifiedEver = len(verifiedEver)
	m.Hypotheses = fold.hypotheses
	m.HypothesesVerified = fold.hypothesesVerified
	m.Active = fold.active
	m.Obsolete = fold.obsolete
	m.Rejected = fold.rejected
	m.Invalidated = fold.invalidated
	m.Superseded = fold.superseded
	m.Expired = fold.expired
	m.Precision = learnPrecisionRate(fold.verifiedLatest, fold.rejected, fold.invalidated)
	m.UsefulRecall = learnRecallRate(fold.verifiedActive, len(verifiedEver))
	m.ObsoleteRate = learnObsoleteRate(fold.obsolete, m.Total)
	m.NoiseRate = learnNoiseRate(fold.rejected, fold.invalidated, len(contrSides), m.Total)
	return m, nil
}
