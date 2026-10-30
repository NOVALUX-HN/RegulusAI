package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeOpencodeJSON(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile("opencode.json", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestYoloStatusMissingFileIsOff(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	for _, args := range [][]string{{"yolo"}, {"yolo", "status"}, {"yolo", "off"}} {
		if code := c.Run(args); code != 0 {
			t.Fatalf("%v exit = %d, want 0", args, code)
		}
	}
	if !strings.Contains(out.String(), "yolo: off (no opencode.json") {
		t.Fatalf("output = %q, want off-with-hint", out.String())
	}
}

func TestYoloOnCreatesTemplate(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "on"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "yolo: on (created opencode.json") {
		t.Fatalf("output = %q, want creation note", out.String())
	}
	data, err := os.ReadFile("opencode.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"rm *": "deny"`) {
		t.Fatalf("template must keep the destructive denies, got:\n%s", data)
	}
}

func TestYoloOnOffPreservesDeniesAndOrder(t *testing.T) {
	// Scrambled on purpose: deny before the catch-all, plus extra keys.
	write := func() {
		t.Chdir(t.TempDir())
		writeOpencodeJSON(t, `{"$schema":"x","permission":{"bash":{"rm *":"deny","*":"ask"},"edit":"allow","doom_loop":"ask"}}`)
	}
	write()
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "on"}); code != 0 {
		t.Fatalf("on exit = %d", code)
	}
	data, _ := os.ReadFile("opencode.json")
	text := string(data)
	if !strings.Contains(text, `"*": "allow"`) || !strings.Contains(text, `"rm *": "deny"`) {
		t.Fatalf("on must flip only the catch-all, got:\n%s", text)
	}
	if strings.Index(text, `"*": "allow"`) > strings.Index(text, `"rm *": "deny"`) {
		t.Fatalf("catch-all must marshal first (last-match-wins), got:\n%s", text)
	}
	if !strings.Contains(text, `"doom_loop": "ask"`) {
		t.Fatalf("unrelated keys must survive, got:\n%s", text)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"yolo", "status"}); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	if !strings.Contains(out2.String(), "yolo: on") {
		t.Fatalf("status = %q, want on", out2.String())
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"yolo", "off"}); code != 0 {
		t.Fatalf("off exit = %d", code)
	}
	if !strings.Contains(out3.String(), "yolo: off") {
		t.Fatalf("off = %q", out3.String())
	}
	data, _ = os.ReadFile("opencode.json")
	if !strings.Contains(string(data), `"*": "ask"`) || !strings.Contains(string(data), `"rm *": "deny"`) {
		t.Fatalf("off must restore ask and keep denies, got:\n%s", data)
	}
	_ = out
}

func TestYoloRefusals(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"yolo", "maybe"}); code != 2 {
		t.Fatalf("bad mode exit = %d, want 2", code)
	}
	writeOpencodeJSON(t, `{"permission": {}} // not json`)
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"yolo", "on"}); code != 1 {
		t.Fatalf("invalid json exit = %d, want 1", code)
	}
	if !strings.Contains(errb2.String(), "not plain JSON") {
		t.Fatalf("stderr = %q, want plain-JSON guidance", errb2.String())
	}
	writeOpencodeJSON(t, `{"permission":{"bash":"allow"}}`)
	c3, _, errb3 := newCLI(nil)
	if code := c3.Run([]string{"yolo", "on"}); code != 1 {
		t.Fatalf("string bash exit = %d, want 1", code)
	}
	if !strings.Contains(errb3.String(), "not a pattern map") {
		t.Fatalf("stderr = %q, want pattern-map guidance", errb3.String())
	}
	_ = errb
}

func TestYoloTemplateDeniesDestructiveGitAndEnvAsk(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "on"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	data, _ := os.ReadFile("opencode.json")
	text := string(data)
	for _, deny := range []string{`"git rm*": "deny"`, `"git checkout --*": "deny"`, `"git restore*": "deny"`, `"git rebase*": "deny"`} {
		if !strings.Contains(text, deny) {
			t.Fatalf("template must deny %s, got:\n%s", deny, text)
		}
	}
	if !strings.Contains(text, `"*.env"`) {
		t.Fatalf("template must ask for .env edits, got:\n%s", text)
	}
}

func TestYoloOffFlipsEditToAskAndStatusReportsEdit(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOpencodeJSON(t, `{"permission":{"bash":{"*":"allow"},"edit":"allow"}}`)
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "off"}); code != 0 {
		t.Fatalf("off exit = %d", code)
	}
	data, _ := os.ReadFile("opencode.json")
	if strings.Contains(string(data), `"edit": "allow"`) {
		t.Fatalf("off must not leave edits auto-approved, got:\n%s", data)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"yolo", "status"}); code != 0 {
		t.Fatalf("status exit = %d", code)
	}
	if !strings.Contains(out2.String(), "edit") {
		t.Fatalf("status must report edit mode, got %q", out2.String())
	}
}

func TestMemorySaveRejectsAbsoluteAndTraversalProvenance(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "save", "--type", "decision", "--title", "t", "--path", "/etc/passwd", "body"}); code == 0 {
		t.Fatal("absolute --path must be refused")
	}
	if !strings.Contains(errb.String(), "provenance") {
		t.Fatalf("stderr = %q, want provenance guidance", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"memory", "save", "--type", "decision", "--title", "t", "--receipt", "../outside.json", "body"}); code == 0 {
		t.Fatal("traversal --receipt must be refused")
	}
	if !strings.Contains(errb2.String(), "provenance") {
		t.Fatalf("stderr = %q, want provenance guidance", errb2.String())
	}
}

func TestYoloHelpListsCommand(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"help"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "yolo [on|off|status]") {
		t.Fatalf("help must list yolo, got:\n%s", out.String())
	}
}

func TestYoloAlreadyThere(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOpencodeJSON(t, `{"permission":{"bash":{"*":"allow","rm *":"deny"}}}`)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "on"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "already on") {
		t.Fatalf("output = %q, want already-on", out.String())
	}
	_ = filepath.Separator
}

func writeWorkerMD(t *testing.T, bashBlock string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(".opencode", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\n" +
		"description: worker fixture\n" +
		"mode: subagent\n" +
		"permission:\n" +
		"  bash:\n" +
		bashBlock +
		"---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(".opencode", "agents", "regulus-worker.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readWorkerMD(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(".opencode", "agents", "regulus-worker.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestYoloOnFlipsWorkerCatchAllAllowKeepsDeniesFirst(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOpencodeJSON(t, `{"permission":{"bash":{"*":"ask","rm *":"deny"},"edit":"ask"}}`)
	writeWorkerMD(t, "    \"go *\": allow\n    \"rm *\": deny\n    \"*\": ask\n")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "on"}); code != 0 {
		t.Fatalf("on exit = %d", code)
	}
	if !strings.Contains(out.String(), "yolo: on") {
		t.Fatalf("output = %q, want on", out.String())
	}
	if !strings.Contains(out.String(), "/reload") {
		t.Fatalf("on must print the /reload reminder, got %q", out.String())
	}
	text := readWorkerMD(t)
	if !strings.Contains(text, `"*": allow`) {
		t.Fatalf("worker catch-all must flip to allow, got:\n%s", text)
	}
	if !strings.Contains(text, `"rm *": deny`) {
		t.Fatalf("worker must preserve denies, got:\n%s", text)
	}
	if strings.Index(text, `"*": allow`) > strings.Index(text, `"rm *": deny`) {
		t.Fatalf("worker catch-all must come first (last-match-wins), got:\n%s", text)
	}
	for _, deny := range []string{`"git rm*": deny`, `"git restore*": deny`, `"git rebase*": deny`} {
		if !strings.Contains(text, deny) {
			t.Fatalf("worker on must ensure destructive deny %s, got:\n%s", deny, text)
		}
	}
	if !strings.Contains(text, `"go *": allow`) {
		t.Fatalf("worker must keep specific allows, got:\n%s", text)
	}
}

func TestYoloOffRestoresWorkerAsk(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOpencodeJSON(t, `{"permission":{"bash":{"*":"allow","rm *":"deny"},"edit":"allow"}}`)
	writeWorkerMD(t, "    \"*\": allow\n    \"go *\": allow\n    \"rm *\": deny\n")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "off"}); code != 0 {
		t.Fatalf("off exit = %d", code)
	}
	if !strings.Contains(out.String(), "/reload") {
		t.Fatalf("off must print the /reload reminder, got %q", out.String())
	}
	text := readWorkerMD(t)
	if !strings.Contains(text, `"*": ask`) {
		t.Fatalf("worker catch-all must restore to ask, got:\n%s", text)
	}
	if strings.Contains(text, `"*": allow`) {
		t.Fatalf("worker must not stay allowed, got:\n%s", text)
	}
	if !strings.Contains(text, `"rm *": deny`) {
		t.Fatalf("worker off must keep denies, got:\n%s", text)
	}
}

func TestYoloLeavesPrimaryAndGuardiansAlone(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOpencodeJSON(t, `{"permission":{"bash":{"*":"ask"},"edit":"ask"}}`)
	writeWorkerMD(t, "    \"*\": ask\n")
	primary := "---\ndescription: p\nmode: primary\npermission:\n  bash:\n    \"*\": allow\n    \"rm *\": deny\n---\n\nbody\n"
	guardian := "---\ndescription: g\nmode: subagent\npermission:\n  edit: deny\n  bash:\n    \"regulus *\": allow\n    \"*\": deny\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(".opencode", "agents", "regulus.md"), []byte(primary), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".opencode", "agents", "regulus-gate-verifier.md"), []byte(guardian), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "on"}); code != 0 {
		t.Fatalf("on exit = %d", code)
	}
	got, _ := os.ReadFile(filepath.Join(".opencode", "agents", "regulus.md"))
	if string(got) != primary {
		t.Fatalf("primary must stay untouched, got:\n%s", got)
	}
	got, _ = os.ReadFile(filepath.Join(".opencode", "agents", "regulus-gate-verifier.md"))
	if string(got) != guardian {
		t.Fatalf("guardian must stay deny, got:\n%s", got)
	}
}

func TestYoloOnWithoutWorkerFileStillSucceeds(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOpencodeJSON(t, `{"permission":{"bash":{"*":"ask"},"edit":"ask"}}`)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"yolo", "on"}); code != 0 {
		t.Fatalf("on exit = %d", code)
	}
	if !strings.Contains(out.String(), "yolo: on") {
		t.Fatalf("output = %q, want on", out.String())
	}
}
