// Package portability hosts the Go mirror of the G5 bash version
// fallback (scripts/build.bash + scripts/install-devin.bash):
// when node is absent, the version resolves with pure grep/sed over
// package.json and fails loud only when no version exists.
package portability

import "regexp"

var versionRe = regexp.MustCompile(`"version"\s*:\s*"([^"]*)"`)

// FallbackVersion mirrors pkg_version_fallback: first "version"
// match, "" when absent (callers fail loud on "").
func FallbackVersion(data []byte) string {
	m := versionRe.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return string(m[1])
}
