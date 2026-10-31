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
