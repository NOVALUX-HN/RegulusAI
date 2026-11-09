package gates

import (
	"regexp"
	"strings"
)

// ExecutionConfig bounds gate execution (Phase 1, S4).
type ExecutionConfig struct {
	MaxTimeoutSec    int      `json:"max_timeout_sec,omitempty"`
	AllowDestructive bool     `json:"allow_destructive,omitempty"`
	Allowlist        []string `json:"allowlist,omitempty"`
	EnvUnset         []string `json:"env_unset,omitempty"`
	DockerImage      string   `json:"docker_image,omitempty"`
}

// DefaultMaxTimeoutSec caps a gate when no execution block configures one.
const DefaultMaxTimeoutSec = 3600

// Classification vocabulary.
const (
	ClassificationStandard    = "standard"
	ClassificationPrivileged  = "privileged"
	ClassificationDestructive = "destructive"
)

// Isolation vocabulary.
const (
	IsolationHostContained = "host-contained"
	IsolationHostDegraded  = "host-contained:degraded-docker-unavailable"
	IsolationDocker        = "docker"
	IsolationUnknown       = "unknown"
)

func maxTimeout(cfg Config) int {
	if cfg.Execution == nil || cfg.Execution.MaxTimeoutSec <= 0 {
		return DefaultMaxTimeoutSec
	}
	return cfg.Execution.MaxTimeoutSec
}

func effectiveTimeout(gateTimeout, max int) int {
	base := gateTimeout
	if base <= 0 {
		base = DefaultTimeoutSec
	}
	if max <= 0 {
		max = DefaultMaxTimeoutSec
	}
	if base > max {
		base = max
	}
	if base < 1 {
		base = 1
	}
	return base
}

// Classify maps a shell command to standard|privileged|destructive.
// Quoted spans are stripped first so `echo "rm -rf /"` stays standard;
// the remainder is split on shell separators and each segment is scanned
// for the catastrophic blocklist (destructive wins) then privilege markers.
func Classify(command string) string {
	stripped := stripQuoted(command)
	segs := splitSegments(stripped)
	for _, s := range segs {
		if isDestructiveSegment(s) {
			return ClassificationDestructive
		}
	}
	for _, s := range segs {
		if isPrivilegedSegment(s) {
			return ClassificationPrivileged
		}
	}
	return ClassificationStandard
}

func stripQuoted(s string) string {
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if (quote == '"' || quote == '`') && r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
				b.WriteByte(' ')
			}
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			quote = r
			continue
		}
		b.WriteRune(r)
	}
	if quote != 0 {
		b.WriteByte(' ')
	}
	return b.String()
}

func splitSegments(s string) []string {
	tmp := strings.ReplaceAll(s, "&&", ";")
	tmp = strings.ReplaceAll(tmp, "||", ";")
	tmp = strings.ReplaceAll(tmp, "\r\n", "\n")
	f := func(r rune) bool { return r == ';' || r == '\n' || r == '|' }
	parts := strings.FieldsFunc(tmp, f)
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 && strings.TrimSpace(s) != "" {
		return []string{s}
	}
	return out
}

var (
	reWordRM       = regexp.MustCompile(`\brm\b`)
	reRecursive    = regexp.MustCompile(`(^|\s)-[a-zA-Z]*r[a-zA-Z]*\b`)
	reNoPreserve   = regexp.MustCompile(`--no-preserve-root`)
	reHomeVar      = regexp.MustCompile(`\$home\b`)
	reTilde        = regexp.MustCompile(`(^|\s)~(/|\s|$)`)
	reRootPath     = regexp.MustCompile(`(^|\s)/(\s|$)`)
	reDangerPrefix = regexp.MustCompile(`(^|\s)/(root|home|etc|usr|var)(/|\s|$)`)
	reWordRD       = regexp.MustCompile(`\brd\b`)
	reSlashS       = regexp.MustCompile(`/s\b`)
	reMKFS         = regexp.MustCompile(`\bmkfs[\w.\-]*\b`)
	reDDWord       = regexp.MustCompile(`\bdd\b`)
	reDDOf         = regexp.MustCompile(`\bof\s*=\s*/dev/`)
	reDevSD        = regexp.MustCompile(`>\s*/dev/sd[a-z]`)
	reChmod        = regexp.MustCompile(`\bchmod\b`)
	reChown        = regexp.MustCompile(`\bchown\b`)
	reSudo         = regexp.MustCompile(`\bsudo\b`)
	reDoas         = regexp.MustCompile(`\bdoas\b`)
	reRunas        = regexp.MustCompile(`\brunas\b`)
	reSU           = regexp.MustCompile(`\bsu\b`)
	reSetfacl      = regexp.MustCompile(`\bsetfacl\b`)
	reIcacls       = regexp.MustCompile(`\bicacls\b`)
	reDiskpart     = regexp.MustCompile(`\bdiskpart\b`)
	reFormat       = regexp.MustCompile(`\bformat\b`)
	reBcdedit      = regexp.MustCompile(`\bbcdedit\b`)
	reRegAddDel    = regexp.MustCompile(`\breg\s+(add|delete)\b`)
)

func hasRMRecursive(lower string) bool {
	if !reWordRM.MatchString(lower) {
		return false
	}
	if strings.Contains(lower, "--recursive") {
		return true
	}
	return reRecursive.MatchString(lower)
}

func rmDangerousTarget(lower string) bool {
	if reNoPreserve.MatchString(lower) {
		return true
	}
	if strings.Contains(lower, "/*") {
		return true
	}
	if reHomeVar.MatchString(lower) {
		return true
	}
	if reTilde.MatchString(lower) {
		return true
	}
	if reRootPath.MatchString(lower) {
		return true
	}
	if reDangerPrefix.MatchString(lower) {
		return true
	}
	return false
}

func isDestructiveSegment(seg string) bool {
	lower := strings.ToLower(seg)
	if strings.TrimSpace(lower) == "" {
		return false
	}
	// fork bomb
	if strings.Contains(lower, ":(){") || strings.Contains(lower, ":|:&") {
		return true
	}
	// mkfs*
	if reMKFS.MatchString(lower) {
		return true
	}
	// dd of=/dev/*
	if reDDWord.MatchString(lower) && reDDOf.MatchString(lower) {
		return true
	}
	// >/dev/sd*
	nospace := strings.ReplaceAll(lower, " ", "")
	_ = nospace
	if reDevSD.MatchString(lower) || strings.Contains(nospace, ">/dev/sd") {
		return true
	}
	// rd /s (Windows)
	if reWordRD.MatchString(lower) && reSlashS.MatchString(lower) {
		return true
	}
	// chmod/chown -R on /
	if (reChmod.MatchString(lower) || reChown.MatchString(lower)) &&
		(strings.Contains(lower, "--recursive") || reRecursive.MatchString(lower)) &&
		(strings.Contains(lower, "/*") || reRootPath.MatchString(lower)) {
		return true
	}
	// rm -rf dangerous
	if hasRMRecursive(lower) && rmDangerousTarget(lower) {
		return true
	}
	return false
}

func isPrivilegedSegment(seg string) bool {
	lower := strings.ToLower(seg)
	switch {
	case reSudo.MatchString(lower),
		reDoas.MatchString(lower),
		reRunas.MatchString(lower),
		reSU.MatchString(lower),
		reChmod.MatchString(lower),
		reChown.MatchString(lower),
		reSetfacl.MatchString(lower),
		reIcacls.MatchString(lower),
		reDiskpart.MatchString(lower),
		reFormat.MatchString(lower),
		reBcdedit.MatchString(lower),
		reRegAddDel.MatchString(lower):
		return true
	}
	return false
}

// allowlistRefused reports whether any executable segment of command falls
// outside the allowlist (S1: cada segmento contra allowlist).
// Every `splitSegments` segment of the quote-stripped command contributes
// its first token; command substitutions (`$(...)` outside single quotes
// and backtick spans) are scanned recursively because the shell executes
// them even when the outer token is allowlisted. A leading `$`
// (variable/indirection such as `$RM`, `${RM}`) refuses closed: the shell
// would expand it to an unresolved command name. Empty segments are
// ignored. Matching stays exact (`firstToken` basename/`.exe` rules).
func allowlistRefused(command string, allowlist []string) bool {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, a := range allowlist {
		allowed[a] = struct{}{}
	}
	// Command substitutions execute even when the outer token is allowed.
	for _, inner := range extractSubstInners(command) {
		if allowlistRefused(inner, allowlist) {
			return true
		}
	}
	masked := maskSubstitutions(command)
	stripped := stripQuoted(masked)
	segs := splitSegments(stripped)
	// `splitSegments` splits `;`, `||`, `&&`, `|` and newlines; a lone `&`
	// backgrounds too, so split it here without touching Classify's table.
	var flat []string
	for _, s := range segs {
		for _, p := range strings.Split(s, "&") {
			if t := strings.TrimSpace(p); t != "" {
				flat = append(flat, t)
			}
		}
	}
	if len(flat) == 0 {
		if strings.TrimSpace(command) == "" {
			return false
		}
		// Fully-quoted or substitution-only input: fall back to the outer
		// first token so `"hello"` still refuses when `hello` is not listed.
		// (Substitution-only input was already judged via its inners above;
		// the fallback keeps the verdict closed.)
		ft := firstToken(command)
		if ft == "" {
			ft = firstToken(masked)
		}
		if ft == "" {
			return false
		}
		if strings.HasPrefix(ft, "$") {
			return true
		}
		_, ok := allowed[ft]
		return !ok
	}
	for _, s := range flat {
		ft := firstToken(s)
		if ft == "" {
			continue
		}
		if strings.HasPrefix(ft, "$") {
			return true
		}
		if _, ok := allowed[ft]; !ok {
			return true
		}
	}
	return false
}

// substSpan is one `$(...)` or backtick span to mask (delimiters included).
type substSpan struct{ start, end int }

// substQuote tracks single/double-quote plus backslash state while scanning
// shell text. The shell treats single-quoted text as literal but still
// executes `$(...)` and backticks inside double quotes.
type substQuote struct {
	single, double, esc bool
}

// step consumes one byte inside `$(...)`; it reports quoted=true when the
// byte must not participate in paren matching (escape, quote toggle, or
// quoted text).
func (q *substQuote) step(c byte) (quoted bool) {
	if q.esc {
		q.esc = false
		return true
	}
	switch {
	case c == '\\' && !q.single:
		q.esc = true
		return true
	case c == '\'' && !q.double:
		q.single = !q.single
		return true
	case c == '"' && !q.single:
		q.double = !q.double
		return true
	}
	return q.single || q.double
}

// consumeControl consumes s[i] when it is outer quote/escape control or
// single-quoted literal text; the caller advances past it. It reports false
// for bytes that may open a substitution (`$`, backtick, ordinary text).
func (q *substQuote) consumeControl(c byte) bool {
	if q.esc {
		q.esc = false
		return true
	}
	switch {
	case c == '\\' && !q.single:
		q.esc = true
		return true
	case c == '\'' && !q.double:
		q.single = !q.single
		return true
	case c == '"' && !q.single:
		q.double = !q.double
		return true
	case q.single:
		return true
	}
	return false
}

// findSubstSpans locates command-substitution spans in s: `$(...)` (with
// nesting) and backtick pairs. Single-quoted text is literal for the shell,
// so spans inside single quotes are ignored; double-quoted `$(...)` and
// backticks DO execute and are collected. `$((...))` arithmetic and
// `${...}` parameter expansion are not commands and are skipped.
func findSubstSpans(s string) []substSpan {
	var spans []substSpan
	var q substQuote
	i := 0
	for i < len(s) {
		if q.consumeControl(s[i]) {
			i++
			continue
		}
		if end, ok := scanDollarEnd(s, i); ok {
			spans = append(spans, substSpan{start: i, end: end})
			i = end
			continue
		}
		// Skip `${...}` / `$((...))` openers without scanning: they are not
		// commands, and scanDollarEnd already refused them.
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {
			i += 2
			continue
		}
		if end, ok := scanBacktickEnd(s, i); ok {
			spans = append(spans, substSpan{start: i, end: end})
			i = end
			continue
		}
		i++
	}
	return spans
}

// scanDollarEnd scans a `$(...)` opener at s[i] (`$` followed by `(`) and
// returns the exclusive end on a balanced close. It refuses `${...}` and
// `$((...))` / `$()` (parameter/arithmetic/empty): those are not commands.
func scanDollarEnd(s string, i int) (int, bool) {
	if i+1 >= len(s) || s[i] != '$' || s[i+1] != '(' {
		return 0, false
	}
	if i+2 < len(s) && (s[i+2] == '(' || s[i+2] == ')') {
		return 0, false
	}
	var q substQuote
	depth := 0
	for j := i + 1; j < len(s); j++ {
		if q.step(s[j]) {
			continue
		}
		switch s[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return j + 1, true
			}
		}
	}
	return 0, false
}

// scanBacktickEnd scans a backtick opener at s[i] and returns the exclusive
// end on its unescaped close.
func scanBacktickEnd(s string, i int) (int, bool) {
	if i >= len(s) || s[i] != '`' {
		return 0, false
	}
	j := i + 1
	bEsc := false
	for j < len(s) {
		d := s[j]
		if bEsc {
			bEsc = false
			j++
			continue
		}
		if d == '\\' {
			bEsc = true
			j++
			continue
		}
		if d == '`' {
			return j + 1, true
		}
		j++
	}
	return 0, false
}

// extractSubstInners returns the payload inside each substitution span
// (`$(`...`)` without the outer delimiters, backtick content as-is).
func extractSubstInners(s string) []string {
	spans := findSubstSpans(s)
	out := make([]string, 0, len(spans))
	for _, sp := range spans {
		raw := s[sp.start:sp.end]
		if strings.HasPrefix(raw, "$(") && strings.HasSuffix(raw, ")") {
			out = append(out, raw[2:len(raw)-1])
		} else if strings.HasPrefix(raw, "`") && strings.HasSuffix(raw, "`") && len(raw) >= 2 {
			out = append(out, raw[1:len(raw)-1])
		}
	}
	return out
}

// maskSubstitutions blanks substitution spans (spaces, same length) so the
// outer quote-strip/split judges only the outer command; inners are judged
// separately via recursion in allowlistRefused.
func maskSubstitutions(s string) string {
	spans := findSubstSpans(s)
	if len(spans) == 0 {
		return s
	}
	b := []byte(s)
	for _, sp := range spans {
		for k := sp.start; k < sp.end && k < len(b); k++ {
			b[k] = ' '
		}
	}
	return string(b)
}

func firstToken(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	first := strings.Trim(fields[0], `"'`)
	// basename on both separators
	if i := strings.LastIndex(first, "/"); i >= 0 {
		first = first[i+1:]
	}
	if i := strings.LastIndex(first, `\`); i >= 0 {
		first = first[i+1:]
	}
	// strip drive prefix remnants and .exe suffix
	first = strings.TrimSuffix(first, ".exe")
	return first
}
