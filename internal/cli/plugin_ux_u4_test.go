package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// U4 (plugin-ux S4): yolo status derives the headline from BOTH bash
// and edit. Agreement reports on/off; disagreement reports mixed with
// the per-side words plus the /reload hint. on/off already remind
// /reload — pinned here so the honesty contract cannot regress.
func TestYoloStatusDerivesHeadlineFromBoth(t *testing.T) {
	cases := []struct {
		name     string
		bash     string
		edit     string
		headline string
		mixed    bool
	}{
		{"both on", "allow", `"allow"`, "yolo: on (bash on, edit on)", false},
		{"both off", "ask", `"ask"`, "yolo: off (bash off, edit off)", false},
		{"bash on edit off", "allow", `"ask"`, "yolo: mixed (bash on, edit off)", true},
		{"bash off edit on", "ask", `"allow"`, "yolo: mixed (bash off, edit on)", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			writeOpencodeJSON(t, `{"permission":{"bash":{"*":"`+tc.bash+`"},"edit":`+tc.edit+`}}`)
			c, out, _ := newCLI(nil)
			if code := c.Run([]string{"yolo", "status"}); code != 0 {
				t.Fatalf("status exit = %d, want 0", code)
			}
			if !strings.Contains(out.String(), tc.headline) {
				t.Fatalf("status = %q, want headline %q", out.String(), tc.headline)
			}
			if tc.mixed {
				if !strings.Contains(out.String(), "/reload") {
					t.Fatalf("mixed status must hint /reload, got %q", out.String())
				}
			}
		})
	}
}

func TestYoloStatusMixedGranularEdit(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOpencodeJSON(t, `{"permission":{"bash":{"*":"allow"},"edit":{"*":"ask","*.env":"ask"}}}`)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "status"}); code != 0 {
		t.Fatalf("status exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "yolo: mixed (bash on, edit off)") {
		t.Fatalf("granular edit ask must read as off, got %q", out.String())
	}
	if !strings.Contains(out.String(), "/reload") {
		t.Fatalf("mixed must hint /reload, got %q", out.String())
	}
}

func TestYoloOnOffRemindReload(t *testing.T) {
	for _, mode := range []string{"on", "off"} {
		t.Chdir(t.TempDir())
		writeOpencodeJSON(t, `{"permission":{"bash":{"*":"ask"},"edit":"ask"}}`)
		c, out, _ := newCLI(nil)
		if code := c.Run([]string{"yolo", mode}); code != 0 {
			t.Fatalf("%s exit = %d", mode, code)
		}
		if !strings.Contains(out.String(), "/reload") {
			t.Fatalf("%s must remind /reload, got %q", mode, out.String())
		}
	}
}

// U4 (plugin-ux S4): trace files/show distinguish a never-recorded
// session (no trace file) from a recorded trace with no files — both
// exit 0 with distinct honest messages.
func TestTraceFilesDistinguishesMissingFromNoFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "files", "--session", "ghost"}); code != 0 {
		t.Fatalf("missing exit = %d, want 0", code)
	}
	missing := out.String()
	if !strings.Contains(missing, "no trace file for session ghost") || !strings.Contains(missing, "never recorded") {
		t.Fatalf("missing files = %q, want never-recorded wording", missing)
	}
	// Record one entry with no --files: the trace file now exists but
	// carries no touched paths.
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"trace", "record", "--session", "ghost", "--message", "m", "--call", "c", "--tool", "read", "--status", "completed"}); code != 0 {
		t.Fatalf("record exit = %d", code)
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"trace", "files", "--session", "ghost"}); code != 0 {
		t.Fatalf("empty exit = %d, want 0", code)
	}
	empty := out3.String()
	if !strings.Contains(empty, "session ghost has no files") {
		t.Fatalf("empty files = %q, want has-no-files wording", empty)
	}
	if empty == missing {
		t.Fatalf("messages must differ: missing %q vs empty %q", missing, empty)
	}
}

func TestTraceShowDistinguishesMissingFromEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "show", "--session", "ghost"}); code != 0 {
		t.Fatalf("missing exit = %d, want 0", code)
	}
	missing := out.String()
	if !strings.Contains(missing, "no trace file for session ghost") || !strings.Contains(missing, "never recorded") {
		t.Fatalf("missing show = %q, want never-recorded wording", missing)
	}
	// An existing-but-empty trace file (zero entries) is recorded yet
	// empty — honest message differs from never-recorded, still exit 0.
	if err := os.MkdirAll(filepath.Join(".regulus", "traces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".regulus", "traces", "ghost.jsonl"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"trace", "show", "--session", "ghost"}); code != 0 {
		t.Fatalf("empty exit = %d, want 0", code)
	}
	empty := out2.String()
	if !strings.Contains(empty, "has no files") {
		t.Fatalf("empty show = %q, want has-no-files wording", empty)
	}
	if empty == missing {
		t.Fatalf("messages must differ: missing %q vs empty %q", missing, empty)
	}
}
