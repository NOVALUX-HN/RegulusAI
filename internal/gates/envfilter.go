package gates

// NOV-81 Paso 61: single place for gate environment policy.
//
// Before: RunStructured ignored EnvUnset whenever cmd.Env was set, and
// runShell carried its own copy of the filter loop. Both now go through
// buildEnv: filter first (unset always wins), gate-declared vars last.

import (
	"os"
	"runtime"
	"strings"
)

// foldName normalizes a variable name for comparison: Windows treats
// environment names case-insensitively (Path vs PATH), other platforms
// are exact. Values are never folded, only names for matching.
func foldName(n string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(n)
	}
	return n
}

// filterEnv rebuilds env without the unset names (exact match, trimmed).
// Values of removed entries are NOT kept: callers capture secrets first
// via SecretValues when they need redaction.
func filterEnv(env []string, unset []string) []string {
	if len(unset) == 0 {
		return env
	}
	drop := map[string]bool{}
	for _, n := range unset {
		if n = strings.TrimSpace(n); n != "" {
			drop[foldName(n)] = true
		}
	}
	if len(drop) == 0 {
		return env
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 && drop[foldName(kv[:i])] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// allowPrefixes are always visible to gates (harness/CI namespaces).
var allowPrefixes = []string{
	"REGULUS_",    // harness-own config (STRICT_ENV, splits, evidence dirs)
	"GITHUB_",     // CI context gates branch on
	"RUNNER_",     // CI runner context
	"BUN_",        // bun toolchain
	"NPM_CONFIG_", // npm/bun registry config
	"npm_config_",
}

// defaultAllowlist is the minimal environment gates may see: process
// startup + toolchain caches + CI context. Everything else is denied
// unless the gate declares it (extra keys are always appended) or the
// operator opts that gate into full inheritance (FullEnv / inherit_env).
func defaultAllowlist() []string {
	base := []string{
		// exec resolution
		"PATH", "PATHEXT",
		// home / temp
		"HOME", "USERPROFILE", "TEMP", "TMP", "TMPDIR",
		// Windows process startup (nothing spawns without these)
		"SYSTEMROOT", "WINDIR", "COMSPEC",
		// Go toolchain caches and behavior
		"GOCACHE", "GOMODCACHE", "GOPATH", "GOENV",
		"GOFLAGS", "GOPROXY", "GOTOOLCHAIN", "GOSUMDB",
		// CI context (plain names; prefixed ones via allowPrefixes)
		"CI",
		// locale / time (deterministic output, timestamps)
		"LANG", "LC_ALL", "LC_CTYPE", "TZ",
		// proxies (module downloads behind one break without these)
		"NO_PROXY", "no_proxy",
		"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy",
	}
	if runtime.GOOS == "windows" {
		base = append(base, "USERDOMAIN", "USERNAME", "COMPUTERNAME")
	} else {
		base = append(base, "USER", "LOGNAME", "SHELL")
	}
	return base
}

func allowedName(name string, extra map[string]bool) bool {
	if extra[foldName(name)] {
		return true
	}
	for _, p := range allowPrefixes {
		if strings.HasPrefix(foldName(name), foldName(p)) {
			return true
		}
	}
	for _, n := range defaultAllowlist() {
		if n == foldName(name) {
			return true
		}
	}
	return false
}

// buildEnv returns the child environment for one gate invocation.
//
//   - extra: gate-declared vars (appended last, so they win and are
//     always visible even when not allowlisted).
//   - unset: names removed everywhere (filter first: unset always wins
//     over inheritance; extras are appended after filtering).
//   - allow: additional visible names for this runner.
//   - fullEnv: legacy escape hatch — inherit everything minus unset.
//     Default false: allowlist mode.
func buildEnv(extra, unset, allow []string, fullEnv bool) []string {
	names := func(kv string) (string, bool) {
		if i := strings.IndexByte(kv, '='); i > 0 {
			return kv[:i], true
		}
		return "", false
	}
	if fullEnv {
		return append(filterEnv(os.Environ(), unset), extra...)
	}
	extraSet := map[string]bool{}
	for _, kv := range extra {
		if n, ok := names(kv); ok {
			extraSet[foldName(n)] = true
		}
	}
	for _, n := range allow {
		if n = strings.TrimSpace(n); n != "" {
			extraSet[foldName(n)] = true
		}
	}
	drop := map[string]bool{}
	for _, n := range unset {
		if n = strings.TrimSpace(n); n != "" {
			drop[foldName(n)] = true
		}
	}
	out := make([]string, 0, len(os.Environ())+len(extra))
	for _, kv := range os.Environ() {
		n, ok := names(kv)
		if !ok || drop[foldName(n)] || !allowedName(n, extraSet) {
			continue
		}
		out = append(out, kv)
	}
	return append(out, extra...)
}

// secretName reports whether an env name looks like it holds a secret.
func secretName(name string) bool {
	u := strings.ToUpper(name)
	for _, sub := range []string{"SECRET", "TOKEN", "PASSWORD", "PASSWD", "PASS", "CREDENTIAL", "_KEY", "KEY_", "PAT_"} {
		if strings.Contains(u, sub) {
			return true
		}
	}
	return false
}

// SecretValues collects values worth redacting from receipts: values of
// unset names (removed from the child env but still leakable through
// output) plus values of secret-looking names present in env.
func SecretValues(env []string, unset []string) []string {
	want := map[string]bool{}
	for _, n := range unset {
		if n = strings.TrimSpace(n); n != "" {
			want[foldName(n)] = true
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		n, v := kv[:i], kv[i+1:]
		if v == "" || seen[v] {
			continue
		}
		if want[foldName(n)] || secretName(n) {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
