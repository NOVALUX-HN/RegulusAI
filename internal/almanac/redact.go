// Secret redaction before Almanac persistence (NOV-22, spec S12).
//
// Save and appendImported are the choke points: every observation
// enters through one of them (note/save/reflect/autonote/mine via
// Save, import via appendImported), so scrubbing here covers all
// writers without touching callers. Receipts and traces are NOT
// redacted: their bytes are evidence and their hash integrity forbids
// mutation (see docs/memory.md).
package almanac

import (
	"regexp"
	"strings"
)

// Redacted replaces secret-bearing values, mirroring
// observability.Redacted without importing it (almanac stays
// dependency-free).
const Redacted = "[redacted]"

// sensitiveKeys is the deny-list for key-name matching
// (case-insensitive substring), mirroring observability/redact.go.
// Extend this list to cover new secret kinds — the key=value and
// "key": "value" patterns below derive from it, so no other literal
// needs to change.
var sensitiveKeys = []string{
	"api_key",
	"apikey",
	"secret",
	"token",
	"password",
	"credential",
	"authorization",
	"system_prompt",
	"private_prompt",
	"prompt",
	"private_key",
}

// secretTokenPatterns matches standalone credential prefixes: GitHub
// PATs, OpenAI-style live/project keys and AWS access key ids.
// Extend this list for new token shapes — the combined pattern below
// derives from it, so call sites never carry literals.
var secretTokenPatterns = []string{
	`\bghp_[A-Za-z0-9]+`,
	`\bgho_[A-Za-z0-9]+`,
	`\bsk-live_[A-Za-z0-9_-]+`,
	`\bsk-proj_[A-Za-z0-9_-]+`,
	`\bAKIA[0-9A-Z]{16}\b`,
}

var (
	sensitiveAlt = strings.Join(sensitiveKeys, "|")

	bearerRe = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9\-._~+/=]+`)
	jsonKVRe = regexp.MustCompile(`(?i)"([^"]*(?:` + sensitiveAlt + `)[^"]*)"\s*:\s*"[^"]*"`)
	bareKVRe = regexp.MustCompile(`(?i)\b(` + sensitiveAlt + `)\b\s*([:=])\s*("[^"]*"|'[^']*'|[^\s,}]+)`)

	// PEM private-key blocks, multiline, non-greedy.
	privateKeyRe = regexp.MustCompile(`(?is)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
	// URLs carrying credentials: scheme://user:pass@host.
	urlCredRe = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://)[^/\s:@]+:[^/\s@]+@`)
	// Standalone prefixed tokens, combined from secretTokenPatterns.
	secretTokenRe = regexp.MustCompile(strings.Join(secretTokenPatterns, "|"))
)

// redactString scrubs one free-text value for every credential shape.
// The replacement is always Redacted, preserving the surrounding
// structure (key names, JSON quotes, URL scheme/host, bearer scheme).
// It is idempotent: scrubbing twice yields the same output as once.
func redactString(s string) string {
	s = privateKeyRe.ReplaceAllString(s, Redacted)
	s = urlCredRe.ReplaceAllString(s, `$1`+Redacted+`@`)
	s = secretTokenRe.ReplaceAllString(s, Redacted)
	s = bearerRe.ReplaceAllString(s, "bearer "+Redacted)
	s = jsonKVRe.ReplaceAllString(s, `"$1": "`+Redacted+`"`)
	s = bareKVRe.ReplaceAllString(s, `$1$2`+Redacted)
	return s
}

// RedactObservation returns a copy of o with secret-bearing free-text
// fields scrubbed: Title, Content, TopicKey, Domain, the lesson
// Cue/Lesson/Evidence block and the provenance Paths. Structural and
// identity fields (ID, type, confidence, dimension, task, git_head,
// receipts, source, origin, timestamps) pass through untouched, as do
// receipts and traces themselves, which keep exact bytes for evidence
// integrity.
func RedactObservation(o Observation) Observation {
	o.Title = redactString(o.Title)
	o.Content = redactString(o.Content)
	o.TopicKey = redactString(o.TopicKey)
	o.Domain = redactString(o.Domain)
	if o.Lesson != nil {
		lesson := *o.Lesson
		lesson.Cue = redactString(lesson.Cue)
		lesson.Lesson = redactString(lesson.Lesson)
		if lesson.Evidence != nil {
			evidence := make([]string, len(lesson.Evidence))
			for i, e := range lesson.Evidence {
				evidence[i] = redactString(e)
			}
			lesson.Evidence = evidence
		}
		o.Lesson = &lesson
	}
	if o.Provenance != nil {
		prov := *o.Provenance
		if prov.Paths != nil {
			paths := make([]string, len(prov.Paths))
			for i, p := range prov.Paths {
				paths[i] = redactString(p)
			}
			prov.Paths = paths
		}
		o.Provenance = &prov
	}
	return o
}
