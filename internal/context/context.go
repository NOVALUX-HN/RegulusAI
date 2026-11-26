// Package context is the Predictive Context Engine (F12.3/NOV-98):
// the minimum sufficient, pertinent and up-to-date context per agent.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - input: orchestrate.TaskDescriptor (F12.1: domain, language,
//     complexity, risk, constraints, outputs, criteria) — validated
//     by orchestrate.ValidateTaskDescriptor, never redefined;
//   - tokens: the F11.5 proxy (chars/4, like trace.EstimateLocalTokens
//     over the stored fields) — unknown is never zero, every layer
//     measures;
//   - memory: almanac.All (the Almanac is the memory substrate with
//     provenance; observations keep source, date and expiry);
//   - scopes: the F11.8 closed scope vocabulary (mcp.Scope*): memory
//     fragments require memory:read, anything else declares its own
//     requirement or none;
//   - redaction marker: [redacted], mirroring almanac.Redacted;
//   - trace: the CLI records F11.1 exec events (RecordExec), the
//     engine itself is pure (no clock, no disk, no network beyond
//     its inputs).
//
// The pipeline is pure and deterministic: FilterByACL → Rank →
// Detect → AssignLayers (Select runs all four). Same corpus,
// descriptor, profile and options always print the same bytes.
// The conservative fallback never omits critical context: critical
// fragments stay essential under any budget, and an empty essential
// layer promotes instead of returning nothing.
package context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/mcp"
	"regulus-ai/internal/orchestrate"
)

// Fragment kinds: the closed inventory vocabulary (spec: code,
// symbols, dependencies, docs, ADRs, issues, memory).
const (
	KindCode       = "code"
	KindSymbol     = "symbol"
	KindDependency = "dependency"
	KindDoc        = "doc"
	KindADR        = "adr"
	KindIssue      = "issue"
	KindMemory     = "memory"
)

// Delivery layers: imprescindible first, bajo-demanda next,
// opcional last. Budgets trim from the back; essential is never
// trimmed (critical omission is a refusal-shaped warning, never a
// silent drop).
const (
	LayerEssential = "imprescindible"
	LayerOnDemand  = "bajo-demanda"
	LayerOptional  = "opcional"
)

// Trust levels: imported memory (almanac SourceImport) and caller
// declarations outside the trusted set read as untrusted — flagged
// and demoted, never silently trusted.
const (
	TrustTrusted   = "trusted"
	TrustUntrusted = "untrusted"
)

// Analysis flags: every detection the engine ran, printed on the
// fragment so the delivery explains itself.
const (
	FlagDuplicate     = "duplicado"
	FlagObsolete      = "obsoleto"
	FlagContradiction = "contradicción"
	FlagUntrusted     = "fuente-no-confiable"
	FlagInjection     = "inyección"
	FlagDenied        = "denegado"
	FlagRedacted      = "secreto-redactado"
	FlagFallback      = "fallback-conservador"
)

// Redacted mirrors almanac.Redacted: the placeholder that replaces
// secret-bearing values (structure kept, value gone).
const Redacted = "[redacted]"

// Fragment is one inventory unit: code, symbol, dependency, doc,
// ADR, issue or memory observation. Source is the repo-relative
// path or store locator (almanac:<id>), Version the git head or
// "worktree", Date the mtime or observation instant. Reason names
// why this fragment was scored, layered and flagged the way it
// was — no fragment ships without source, version/date and reason.
type Fragment struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Source        string    `json:"source"`
	Version       string    `json:"version"`
	Date          time.Time `json:"date,omitempty"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Domain        string    `json:"domain,omitempty"`
	Critical      bool      `json:"critical"`
	RequiredScope string    `json:"required_scope,omitempty"`
	Trust         string    `json:"trust,omitempty"`
	Contradicts   []string  `json:"contradicts,omitempty"`
	Expired       bool      `json:"expired,omitempty"`
	Tokens        int64     `json:"tokens"`
	Score         float64   `json:"score"`
	Reason        string    `json:"reason"`
	Layer         string    `json:"layer,omitempty"`
	Flags         []string  `json:"flags,omitempty"`
	DuplicateOf   string    `json:"duplicate_of,omitempty"`
}

// Corpus is the inventoried set the engine ranks.
type Corpus struct {
	Fragments []Fragment `json:"fragments"`
}

// Profile is the receiving agent: identity, held scopes (F11.8
// closed vocabulary) and domain affinities for explicable ranking.
type Profile struct {
	AgentID string   `json:"agent_id"`
	Scopes  []string `json:"scopes,omitempty"`
	Domains []string `json:"domains,omitempty"`
}

// Options bounds one selection: token budget (<= 0 means unset),
// obsolescence horizon (<= 0 means the 180-day default) and the
// reference instant, plus the caller-declared untrusted sources.
type Options struct {
	BudgetTokens     int64
	MaxAge           time.Duration
	Now              time.Time
	UntrustedSources []string
}

// DefaultMaxAge is the obsolescence horizon when Options leaves it
// unset: a fragment older than this (with a known date) reads as
// obsolete — flagged and demoted, never silently dropped.
const DefaultMaxAge = 180 * 24 * time.Hour

// Selection is the layered delivery with its token/quality report.
type Selection struct {
	Essential []Fragment `json:"essential"`
	OnDemand  []Fragment `json:"bajo_demanda"`
	Optional  []Fragment `json:"opcional"`
	Report    Report     `json:"report"`
	Warnings  []string   `json:"warnings"`
}

// Report measures tokens and quality before/after (F11.5 honesty:
// unknown is never zero, every figure recomputes from the corpus).
// SavedPct is the share saved by delivering essential instead of
// the whole visible corpus; CriticalRecall must be 1 — anything
// less is a bug, reported, never hidden.
type Report struct {
	CorpusTokens    int64   `json:"corpus_tokens"`
	DeniedTokens    int64   `json:"denied_tokens"`
	VisibleTokens   int64   `json:"visible_tokens"`
	EssentialTokens int64   `json:"essential_tokens"`
	OnDemandTokens  int64   `json:"ondemand_tokens"`
	SavedPct        float64 `json:"saved_pct"`
	CriticalTotal   int     `json:"critical_total"`
	CriticalKept    int     `json:"critical_kept"`
	CriticalRecall  float64 `json:"critical_recall"`
	Duplicates      int     `json:"duplicates"`
	Obsolete        int     `json:"obsolete"`
	Contradictions  int     `json:"contradictions"`
	Untrusted       int     `json:"untrusted"`
	Injections      int     `json:"injections"`
	Denied          int     `json:"denied"`
	Redacted        int     `json:"redacted"`
	BudgetTrimmed   int     `json:"budget_trimmed"`
	// Loss and recovery are measured from the same selection that produced the
	// context, so the number can never disagree with the decision (F19.3).
	Omitted     int `json:"omitted"`
	LossPct     int `json:"loss_pct"`
	Recoverable int `json:"recoverable"`
	RecoveryPct int `json:"recovery_rate_pct"`
}

// CountTokens measures the F11.5 proxy over free text: chars/4
// rounded up (the same definition trace.EstimateLocalTokens applies
// to the stored fields). Zero means empty, never unknown.
func CountTokens(s string) int64 {
	n := int64(len(s))
	if n <= 0 {
		return 0
	}
	return (n + 3) / 4
}

// effectiveNow resolves the reference instant (explicit, else now).
func (o Options) effectiveNow() time.Time {
	if o.Now.IsZero() {
		return time.Now().UTC()
	}
	return o.Now
}

// effectiveMaxAge resolves the obsolescence horizon.
func (o Options) effectiveMaxAge() time.Duration {
	if o.MaxAge <= 0 {
		return DefaultMaxAge
	}
	return o.MaxAge
}

// FilterByACL splits the corpus on the F11.8 closed vocabulary: a
// fragment with no required scope is public; otherwise the profile
// must hold the scope. Denied fragments are returned separately —
// excluded from every layer, counted in the report, never ranked.
func FilterByACL(frags []Fragment, p Profile) (visible, denied []Fragment) {
	held := map[string]bool{}
	for _, s := range p.Scopes {
		held[strings.TrimSpace(s)] = true
	}
	for _, f := range frags {
		if f.RequiredScope != "" && !held[f.RequiredScope] {
			f.Flags = appendFlag(f.Flags, FlagDenied)
			f.Reason = fmt.Sprintf("denegado: exige scope %q que el perfil no tiene (F11.8: sin scope no hay entrega)", f.RequiredScope)
			denied = append(denied, f)
			continue
		}
		visible = append(visible, f)
	}
	return visible, denied
}

// appendFlag adds a flag once (deterministic order: call order).
func appendFlag(flags []string, f string) []string {
	for _, e := range flags {
		if e == f {
			return flags
		}
	}
	return append(flags, f)
}

// stopWords keeps the term match on content words (Spanish and
// English glue never scores).
var stopWords = map[string]bool{
	"el": true, "la": true, "los": true, "las": true, "de": true,
	"del": true, "en": true, "y": true, "o": true, "a": true,
	"con": true, "por": true, "para": true, "que": true, "se": true,
	"un": true, "una": true, "the": true, "and": true, "or": true,
	"of": true, "to": true, "in": true, "for": true, "with": true,
	"on": true, "is": true, "are": true, "as": true,
}

// wordRe splits free text into lowercase alphanumeric tokens.
var wordRe = regexp.MustCompile(`[a-z0-9áéíóúñü]+`)

// descriptorTerms folds the F12.1 descriptor into distinct content
// terms: domain, language, constraints, outputs, criteria and
// required capabilities — the vocabulary relevance is measured
// against.
func descriptorTerms(d orchestrate.TaskDescriptor) []string {
	var raw []string
	raw = append(raw, wordRe.FindAllString(strings.ToLower(d.Domain+" "+d.Language), -1)...)
	for _, lst := range [][]string{d.Constraints, d.Outputs, d.Criteria, d.RequiredCapabilities} {
		for _, v := range lst {
			raw = append(raw, wordRe.FindAllString(strings.ToLower(v), -1)...)
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, w := range raw {
		if len(w) < 2 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// scoreFragment prices one fragment against the descriptor terms:
// the share of distinct terms present in title, content, source or
// domain, plus a 0.15 profile-domain affinity capped at 1. The
// reason names every matched term (up to 8, then "+N más"), so the
// rank explains itself.
func scoreFragment(f *Fragment, terms []string, p Profile) {
	hay := strings.ToLower(f.Title + "\n" + f.Content + "\n" + f.Source + "\n" + f.Domain)
	var matched []string
	for _, t := range terms {
		if strings.Contains(hay, t) {
			matched = append(matched, t)
		}
	}
	score := 0.0
	if len(terms) > 0 {
		score = float64(len(matched)) / float64(len(terms))
	}
	var notes []string
	if len(matched) == 0 {
		notes = append(notes, fmt.Sprintf("sin coincidencia con el descriptor (0 de %d términos)", len(terms)))
	} else {
		show := matched
		extra := ""
		if len(matched) > 8 {
			show, extra = matched[:8], fmt.Sprintf(" (+%d más)", len(matched)-8)
		}
		notes = append(notes, fmt.Sprintf("términos: %s%s (%d de %d)", strings.Join(show, ", "), extra, len(matched), len(terms)))
	}
	for _, dom := range p.Domains {
		if dom != "" && strings.EqualFold(strings.TrimSpace(dom), strings.TrimSpace(f.Domain)) {
			score += 0.15
			notes = append(notes, fmt.Sprintf("afinidad de perfil: dominio %q (+0.15)", dom))
			break
		}
	}
	if score > 1 {
		score = 1
	}
	f.Score = score
	f.Reason = strings.Join(notes, "; ")
}

// RankVisible scores every visible fragment and sorts by score
// (ties by id): the deterministic relevance order AssignLayers
// consumes. It validates the descriptor through F12.1 first — an
// undescribable task ranks nothing.
func RankVisible(frags []Fragment, d orchestrate.TaskDescriptor, p Profile) ([]Fragment, error) {
	if err := orchestrate.ValidateTaskDescriptor(d); err != nil {
		return nil, err
	}
	terms := descriptorTerms(d)
	out := make([]Fragment, len(frags))
	copy(out, frags)
	for i := range out {
		scoreFragment(&out[i], terms, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// injectionRes quarantines instruction-shaped content: the fragment
// is DATA (like almanac context), so directives inside it are an
// attack, never orders. Matches stay narrow (explicit override
// verbs) so quality prose never quarantines.
var injectionRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous\s+|prior\s+)?instructions`),
	regexp.MustCompile(`(?i)disregard\s+.*instructions`),
	regexp.MustCompile(`(?i)reveal\s+.*system\s+prompt`),
	regexp.MustCompile(`(?i)\bexfiltrat\w*`),
	regexp.MustCompile(`(?i)send\s+.*\s+to\s+https?://`),
}

// secretRes scrubs credential-shaped values before delivery (the
// same deny-list discipline as almanac redaction; structure kept,
// value replaced by Redacted).
var secretRes = []*regexp.Regexp{
	regexp.MustCompile(`(?is)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(api_key|apikey|secret|token|password|credential|authorization)\b\s*([:=])\s*("[^"]*"|'[^']*'|[^\s,}]+)`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9\-._~+/=]+`),
	regexp.MustCompile(`\bghp_[A-Za-z0-9]+`),
}

// contentHash identifies duplicates: sha256 over the normalized
// (lowercased, trimmed, whitespace-collapsed) content.
func contentHash(content string) string {
	norm := strings.Join(strings.Fields(strings.ToLower(content)), " ")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

// DetectIssues runs every detector over rank order: secrets are
// redacted first (delivery never leaks), then duplicates,
// injection, obsolescence, untrusted sources and contradictions.
// Flags accumulate on the fragment and extend its reason — the
// report counts derive from these flags, never from a recount.
func DetectIssues(frags []Fragment, opts Options) []Fragment {
	now := opts.effectiveNow()
	maxAge := opts.effectiveMaxAge()
	untrusted := map[string]bool{}
	for _, s := range opts.UntrustedSources {
		untrusted[s] = true
	}
	out := make([]Fragment, len(frags))
	copy(out, frags)
	// Deterministic canonical order for duplicates: lowest id wins.
	byID := make([]int, len(out))
	for i := range out {
		byID[i] = i
	}
	sort.Slice(byID, func(a, b int) bool { return out[byID[a]].ID < out[byID[b]].ID })
	seen := map[string]string{}
	for _, i := range byID {
		h := contentHash(out[i].Content)
		if first, dup := seen[h]; dup {
			out[i].Flags = appendFlag(out[i].Flags, FlagDuplicate)
			out[i].DuplicateOf = first
			out[i].Reason += fmt.Sprintf("; duplicado de %s (mismo contenido; la entrega usa el canónico)", first)
		} else {
			seen[h] = out[i].ID
		}
	}
	byFragID := map[string]int{}
	for i := range out {
		byFragID[out[i].ID] = i
	}
	for i := range out {
		f := &out[i]
		scrubbed := f.Title + "\n" + f.Content
		for _, re := range secretRes {
			scrubbed = re.ReplaceAllString(scrubbed, Redacted)
		}
		if scrubbed != f.Title+"\n"+f.Content {
			parts := strings.SplitN(scrubbed, "\n", 2)
			f.Title, f.Content = parts[0], parts[1]
			f.Flags = appendFlag(f.Flags, FlagRedacted)
			f.Reason += "; secreto redactado antes de entregar (ACLs/secretos: el valor nunca viaja)"
		}
		for _, re := range injectionRes {
			if re.MatchString(f.Title + "\n" + f.Content) {
				f.Flags = appendFlag(f.Flags, FlagInjection)
				f.Reason += "; posible inyección de prompt en cuarentena: el contenido es DATO, nunca instrucción (capa máxima opcional)"
				break
			}
		}
		if f.Expired || (!f.Date.IsZero() && now.Sub(f.Date) > maxAge) {
			f.Flags = appendFlag(f.Flags, FlagObsolete)
			if f.Expired {
				f.Reason += "; obsoleto: caducidad vencida (memoria con ExpiresAt pasado)"
			} else {
				f.Reason += fmt.Sprintf("; obsoleto: fecha %s supera el horizonte %s", f.Date.Format("2006-01-02"), maxAge)
			}
		}
		if f.Trust == TrustUntrusted || untrusted[f.Source] {
			f.Flags = appendFlag(f.Flags, FlagUntrusted)
			f.Reason += "; fuente no confiable: se entrega degradada y siempre verificada contra otra fuente"
		}
	}
	// Contradictions name both sides (like almanac conflicts: read
	// both before trusting either) — flagged, never hidden.
	paired := map[[2]string]bool{}
	for i := range out {
		for _, other := range out[i].Contradicts {
			j, ok := byFragID[other]
			if !ok || i == j {
				continue
			}
			a, b := out[i].ID, other
			if b < a {
				a, b = b, a
			}
			if paired[[2]string{a, b}] {
				continue
			}
			paired[[2]string{a, b}] = true
			out[i].Flags = appendFlag(out[i].Flags, FlagContradiction)
			out[i].Reason += fmt.Sprintf("; contradicción con %s: leer ambos antes de confiar en ninguno", other)
			out[j].Flags = appendFlag(out[j].Flags, FlagContradiction)
			out[j].Reason += fmt.Sprintf("; contradicción con %s: leer ambos antes de confiar en ninguno", out[i].ID)
		}
	}
	return out
}

// EssentialThreshold is the score that promotes a fragment to the
// essential layer on relevance alone (critical fragments always
// qualify via the fallback, whatever their score).
const EssentialThreshold = 0.35

// demote drops one layer (essential → on-demand; anything else →
// optional): the shape obsolescence and untrusted sources share.
func demote(layer string) string {
	if layer == LayerEssential {
		return LayerOnDemand
	}
	return LayerOptional
}

// hasAnyFlag reports whether the fragment carries any of the flags.
func hasAnyFlag(f Fragment, flags ...string) bool {
	for _, want := range flags {
		for _, got := range f.Flags {
			if got == want {
				return true
			}
		}
	}
	return false
}

// AssignLayers delivers every analyzed fragment to a layer. Rules,
// in order: duplicates and injections cap at optional (quarantine,
// never essential); critical is always essential (fallback
// conservador: flagged when relevance did not earn it); score >=
// threshold is essential, score > 0 on-demand, else optional;
// obsolete and untrusted demote one layer unless critical (flagged
// but kept — better more than omitting). An empty essential layer
// promotes the best ranked fragment instead of delivering nothing.
func AssignLayers(frags []Fragment) []Fragment {
	out := make([]Fragment, len(frags))
	copy(out, frags)
	for i := range out {
		f := &out[i]
		switch {
		case f.Critical:
			f.Layer = LayerEssential
			if f.Score < EssentialThreshold {
				f.Flags = appendFlag(f.Flags, FlagFallback)
				f.Reason += "; fallback conservador: crítico sin coincidencia suficiente, se entrega igual (mejor de más que omitir)"
			} else {
				f.Reason += "; crítico y pertinente: capa imprescindible"
			}
		case f.Score >= EssentialThreshold:
			f.Layer = LayerEssential
			f.Reason += "; pertinente: capa imprescindible"
		case f.Score > 0:
			f.Layer = LayerOnDemand
			f.Reason += "; parcialmente pertinente: bajo demanda"
		default:
			f.Layer = LayerOptional
			f.Reason += "; sin pertinencia: opcional"
		}
		if hasAnyFlag(*f, FlagDuplicate, FlagInjection) {
			if f.Layer == LayerEssential {
				f.Layer = LayerOptional
				f.Reason += "; degradado a opcional por cuarentena (duplicado/inyección nunca imprescindible)"
			} else if f.Layer == LayerOnDemand {
				f.Layer = LayerOptional
				f.Reason += "; degradado a opcional por cuarentena"
			}
		}
		if !f.Critical && hasAnyFlag(*f, FlagObsolete, FlagUntrusted) {
			before := f.Layer
			f.Layer = demote(f.Layer)
			if f.Layer != before {
				f.Reason += "; degradado una capa por obsolescencia/fuente no confiable"
			}
		}
	}
	essential := false
	for _, f := range out {
		if f.Layer == LayerEssential {
			essential = true
			break
		}
	}
	if !essential && len(out) > 0 {
		// Fallback conservador: rank order is score-desc/id-asc, so
		// the head is the best candidate — promote the best one
		// that is neither quarantined nor stale (duplicates,
		// injections, obsolete and untrusted fragments are never
		// promoted: their flags already explain the demotion).
		// When nothing qualifies, essential stays honestly empty
		// (the caller warns) instead of laundering a bad fragment.
		for i := range out {
			if hasAnyFlag(out[i], FlagDuplicate, FlagInjection, FlagObsolete, FlagUntrusted) {
				continue
			}
			out[i].Layer = LayerEssential
			out[i].Flags = appendFlag(out[i].Flags, FlagFallback)
			out[i].Reason += "; fallback conservador: nada alcanzó imprescindible, se promociona lo mejor rankeado"
			essential = true
			break
		}
	}
	return out
}

// Select runs the whole engine: ACL split, explicable ranking,
// detection, layering and budget trim. Critical fragments bypass
// the budget (a blown budget warns, never omits). It fails only
// when the descriptor itself is undescribable (F12.1 validation);
// every other anomaly is a flag, a demotion or a warning.
func Select(corpus Corpus, d orchestrate.TaskDescriptor, p Profile, opts Options) Selection {
	var sel Selection
	visible, denied := FilterByACL(corpus.Fragments, p)
	for _, f := range corpus.Fragments {
		sel.Report.CorpusTokens += CountTokens(f.Title + "\n" + f.Content)
		if f.Critical {
			sel.Report.CriticalTotal++
		}
	}
	for _, f := range denied {
		sel.Report.DeniedTokens += CountTokens(f.Title + "\n" + f.Content)
	}
	sel.Report.Denied = len(denied)
	ranked, err := RankVisible(visible, d, p)
	if err != nil {
		sel.Warnings = append(sel.Warnings, fmt.Sprintf("descriptor inválido: %v (sin ranking; se entrega fallback conservador)", err))
		// Undescribable task: everything critical stays essential,
		// the rest optional — better more than omitting.
		layered := AssignLayers(DetectIssues(visible, opts))
		return finishSelection(sel, layered, opts)
	}
	layered := AssignLayers(DetectIssues(ranked, opts))
	return finishSelection(sel, layered, opts)
}

// finishSelection applies the budget trim (back to front, essential
// never trimmed) and folds the token/quality report.
func finishSelection(sel Selection, layered []Fragment, opts Options) Selection {
	keptOnDemand := layered[:0:0]
	var keptOptional []Fragment
	budget := opts.BudgetTokens
	essentialTokens := int64(0)
	for _, f := range layered {
		if f.Layer == LayerEssential {
			essentialTokens += CountTokens(f.Title + "\n" + f.Content)
		}
	}
	used := essentialTokens
	for _, f := range layered {
		switch f.Layer {
		case LayerEssential:
			sel.Essential = append(sel.Essential, f)
		case LayerOnDemand:
			t := CountTokens(f.Title + "\n" + f.Content)
			if budget > 0 && used+t > budget {
				f.Layer = LayerOptional
				f.Reason += "; recortado por budget de tokens: queda en opcional, recuperable bajo demanda"
				sel.Report.BudgetTrimmed++
				keptOptional = append(keptOptional, f)
				continue
			}
			used += t
			keptOnDemand = append(keptOnDemand, f)
		default:
			keptOptional = append(keptOptional, f)
		}
	}
	sel.OnDemand = keptOnDemand
	sel.Optional = keptOptional
	sel.Report.VisibleTokens = sel.Report.CorpusTokens - sel.Report.DeniedTokens
	sel.Report.EssentialTokens = essentialTokens
	for _, f := range sel.OnDemand {
		sel.Report.OnDemandTokens += CountTokens(f.Title + "\n" + f.Content)
	}
	if sel.Report.VisibleTokens > 0 {
		sel.Report.SavedPct = 100 * (1 - float64(essentialTokens)/float64(sel.Report.VisibleTokens))
	}
	for _, f := range layered {
		switch {
		case hasAnyFlag(f, FlagDuplicate):
			sel.Report.Duplicates++
		case hasAnyFlag(f, FlagInjection):
			sel.Report.Injections++
		}
		if hasAnyFlag(f, FlagObsolete) {
			sel.Report.Obsolete++
		}
		if hasAnyFlag(f, FlagUntrusted) {
			sel.Report.Untrusted++
		}
		if hasAnyFlag(f, FlagRedacted) {
			sel.Report.Redacted++
		}
		if f.Critical && f.Layer == LayerEssential {
			sel.Report.CriticalKept++
		}
	}
	// Contradictions count pairs (each side flags, one pair counts
	// once): halve the side count.
	sides := 0
	for _, f := range layered {
		if hasAnyFlag(f, FlagContradiction) {
			sides++
		}
	}
	sel.Report.Contradictions = sides / 2

	// Measure what the cut cost and whether it can be reached again. A critical
	// fragment in the optional layer is reported as a warning, never folded into
	// the percentage where it would look like a saving.
	loss := MeasureLoss(sel)
	recovery := MeasureRecovery(sel)
	sel.Report.Omitted = loss.Omitted
	sel.Report.LossPct = loss.LossPct
	sel.Report.Recoverable = recovery.Recoverable
	sel.Report.RecoveryPct = recovery.RatePct
	if loss.EssentialOmitted > 0 {
		sel.Warnings = append(sel.Warnings, "critical fragments landed in the optional layer: that is a selector bug, not a saving")
	}
	if sel.Report.CriticalTotal > 0 {
		sel.Report.CriticalRecall = float64(sel.Report.CriticalKept) / float64(sel.Report.CriticalTotal)
	} else {
		sel.Report.CriticalRecall = 1
	}
	if len(sel.Essential) == 0 && sel.Report.VisibleTokens > 0 {
		sel.Warnings = append(sel.Warnings, "nada alcanzó imprescindible (cuarentena/obsolescencia): se entrega bajo-demanda/opcional sin promocionar")
	}
	if sel.Report.CriticalKept < sel.Report.CriticalTotal {
		sel.Warnings = append(sel.Warnings, fmt.Sprintf("contexto crítico omitido: %d de %d fuera de imprescindible (fallo del fallback, revisar)",
			sel.Report.CriticalTotal-sel.Report.CriticalKept, sel.Report.CriticalTotal))
	}
	if budget > 0 && essentialTokens > budget {
		sel.Warnings = append(sel.Warnings, fmt.Sprintf("budget de %d tokens excedido por imprescindible (%d): no se omite contexto crítico (fallback conservador)", budget, essentialTokens))
	}
	if sel.Report.Injections > 0 {
		sel.Warnings = append(sel.Warnings, fmt.Sprintf("%d fragmento(s) en cuarentena por posible inyección (contenido = DATO, nunca instrucción)", sel.Report.Injections))
	}
	if sel.Report.Contradictions > 0 {
		sel.Warnings = append(sel.Warnings, fmt.Sprintf("%d contradicci(ón/ones) viva(s): leer ambos lados antes de confiar", sel.Report.Contradictions))
	}
	return sel
}

// ValidKind reports whether kind belongs to the closed inventory
// vocabulary. Anything else fails closed at inventory time.
func ValidKind(kind string) bool {
	switch kind {
	case KindCode, KindSymbol, KindDependency, KindDoc, KindADR, KindIssue, KindMemory:
		return true
	}
	return false
}

// RequiredScopeFor defaults the ACL requirement per kind: memory
// needs memory:read (F11.8), everything else is public unless the
// caller declares otherwise.
func RequiredScopeFor(kind string) string {
	if kind == KindMemory {
		return mcp.ScopeMemoryRead
	}
	return ""
}
