package hostcfg

import (
	"path/filepath"
	"strings"
	"testing"
)

// Covers HostConfigPaths for every host branch, portable via filepath.
func TestHostConfigPathsCoverage(t *testing.T) {
	home := t.TempDir()
	cases := map[string]string{
		"opencode": "opencode.json",
		"claude":   ".claude.json",
		"codex":    "config.toml",
		"devin":    ".devin/",
	}
	for host, tail := range cases {
		got := HostConfigPaths(host, home)
		if len(got) != 1 {
			t.Fatalf("%s paths = %v, want 1 entry", host, got)
		}
		if host == "devin" {
			if got[0] != ".devin/" {
				t.Fatalf("devin path = %q, want repo-relative", got[0])
			}
			continue
		}
		if !strings.HasSuffix(got[0], tail) {
			t.Fatalf("%s path = %q, want suffix %q", host, got[0], tail)
		}
		if !filepath.IsAbs(got[0]) {
			t.Fatalf("%s path = %q, want absolute under home", host, got[0])
		}
	}
	// Empty home falls back to slash-joined relative form.
	rel := HostConfigPaths("opencode", "")
	if len(rel) != 1 || strings.Contains(rel[0], "\\") {
		t.Fatalf("empty home must stay slash-joined, got %v", rel)
	}
	// Unknown host is explicit, never empty.
	unk := HostConfigPaths("nope", home)
	if len(unk) != 1 || !strings.Contains(unk[0], "unknown host") {
		t.Fatalf("unknown host must explain, got %v", unk)
	}
}
