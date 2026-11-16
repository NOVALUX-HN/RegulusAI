package receipts

import (
	"strings"
	"testing"
)

// NOV-81: redaction keeps stored bytes and stored sha consistent by
// construction (redact before Capture); these pins prove the value
// replacement and the known token shapes.
func TestRedactReplacesSecretValues(t *testing.T) {
	got := Redact("key=abc123-visible and token=abc123-visible end", []string{"abc123-visible"})
	if strings.Contains(got, "abc123-visible") {
		t.Fatalf("secret value must be replaced, got %q", got)
	}
	if c := strings.Count(got, "[redacted]"); c != 2 {
		t.Fatalf("both occurrences must be replaced, got %q", got)
	}
}

func TestRedactKnownTokenShapes(t *testing.T) {
	got := Redact("push ghp_abcdefghijklmnopqrstuvwxyz0123456789 end", nil)
	if strings.Contains(got, "ghp_") {
		t.Fatalf("github token shape must be redacted, got %q", got)
	}
	if Redact("", []string{"x"}) != "" || Redact("plain", nil) != "plain" {
		t.Fatal("empty input and empty secrets must be no-ops")
	}
}
