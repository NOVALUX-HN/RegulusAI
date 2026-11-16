package receipts

// NOV-81 Paso 61: redaction of secrets stored in receipts.
//
// Capture stores truncated bytes plus the sha256 of WHAT IS STORED, so
// redacting BEFORE Capture keeps verification consistent: the stored
// sha always matches the stored bytes. Never redact after Write.
import (
	"regexp"
	"strings"
)

var tokenShapes = []*regexp.Regexp{
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{82}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`xox[bap]-.[\w-]*`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
}

// Redact replaces secret values with [redacted]. Empty input or an
// empty secret list returns the input untouched (no-op, never nil-dirty).
func Redact(s string, secrets []string) string {
	if s == "" {
		return s
	}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		if strings.Contains(s, secret) {
			s = strings.ReplaceAll(s, secret, "[redacted]")
		}
	}
	for _, re := range tokenShapes {
		if re.MatchString(s) {
			s = re.ReplaceAllString(s, "[redacted]")
		}
	}
	return s
}
