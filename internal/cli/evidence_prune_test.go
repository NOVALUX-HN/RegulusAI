package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writePruneReceipt(t *testing.T, dir, gate string, at, mtime time.Time) string {
	t.Helper()
	path := writeVerifyReceipt(t, dir, gate, at, "out-"+gate)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return path
}

func setupPruneTask(t *testing.T, slug string, n int) (taskDir string, receipts []string) {
	t.Helper()
	taskDir = filepath.Join(".regulus", "evidence", slug)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := time.Unix(1700000000, 0).UTC()
	for i := 0; i < n; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		receipts = append(receipts, writePruneReceipt(t, taskDir, "gate"+string(rune('a'+i)), at, at))
	}
	return taskDir, receipts
}

func writePruneManifest(t *testing.T, taskDir, name string) string {
	t.Helper()
	p := filepath.Join(taskDir, name)
	if err := os.WriteFile(p, []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writePruneChart(t *testing.T, name, body string) {
	t.Helper()
	if err := os.MkdirAll("charts", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("charts", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEvidencePruneDryRunListsWithoutDeleting(t *testing.T) {
	t.Chdir(t.TempDir())
	taskDir, _ := setupPruneTask(t, "feat__T1", 3)
	man := writePruneManifest(t, taskDir, "manifest-run1.json")
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune", "--task", "feat/T1"}); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "dry run: 3 receipt(s) would be removed, 1 manifest(s) kept") {
		t.Fatalf("dry-run line missing, got:\n%s", out.String())
	}
	// Every receipt is listed, nothing deleted.
	for _, want := range []string{"gatea-", "gateb-", "gatec-"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("dry-run list must contain %q, got:\n%s", want, out.String())
		}
	}
	found, _ := filepath.Glob(filepath.Join(taskDir, "gate*.json"))
	if len(found) != 3 {
		t.Fatalf("dry-run must delete nothing, found %v", found)
	}
	if _, err := os.Stat(man); err != nil {
		t.Fatalf("manifest must survive dry-run: %v", err)
	}
}

func TestEvidencePruneKeepRespectsN(t *testing.T) {
	t.Chdir(t.TempDir())
	taskDir, receipts := setupPruneTask(t, "feat__T1", 3)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune", "--task", "feat/T1", "--keep", "2", "--yes"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "pruned 1") {
		t.Fatalf("output = %q, want pruned 1", out.String())
	}
	// Oldest gone, two newest kept.
	if _, err := os.Stat(receipts[0]); !os.IsNotExist(err) {
		t.Fatalf("oldest receipt must be deleted: %s", receipts[0])
	}
	for _, p := range receipts[1:] {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("newest receipt must be kept %s: %v", p, err)
		}
	}
	found, _ := filepath.Glob(filepath.Join(taskDir, "gate*.json"))
	if len(found) != 2 {
		t.Fatalf("want 2 receipts kept, got %v", found)
	}
}

func TestEvidencePruneManifestsAndCorruptIntact(t *testing.T) {
	t.Chdir(t.TempDir())
	taskDir, _ := setupPruneTask(t, "feat__T1", 2)
	man := writePruneManifest(t, taskDir, "manifest-run1.json")
	qdir := filepath.Join(taskDir, "corrupt")
	if err := os.MkdirAll(qdir, 0o755); err != nil {
		t.Fatal(err)
	}
	qfile := filepath.Join(qdir, "old-1.json")
	if err := os.WriteFile(qfile, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune", "--task", "feat/T1", "--yes"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "pruned 2") {
		t.Fatalf("output = %q", out.String())
	}
	if _, err := os.Stat(man); err != nil {
		t.Fatalf("manifest must never be deleted: %v", err)
	}
	if _, err := os.Stat(qfile); err != nil {
		t.Fatalf("corrupt/ must never be touched: %v", err)
	}
}

func TestEvidencePruneRefusesClosedWithoutForce(t *testing.T) {
	t.Chdir(t.TempDir())
	taskDir, receipts := setupPruneTask(t, "feat__T1", 2)
	writePruneManifest(t, taskDir, "manifest-run1.json")
	writePruneChart(t, "feat.md", "# feat\n\n## Log\n\n- L2: T1 CLOSED — done\n")
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune", "--task", "feat/T1", "--yes"}); code != 1 {
		t.Fatalf("exit = %d, want 1 (refuse CLOSED)", code)
	}
	if !strings.Contains(errb.String(), "refused: evidence of closed task") || !strings.Contains(errb.String(), "--force") {
		t.Fatalf("stderr must refuse with authorization hint, got %q", errb.String())
	}
	for _, p := range receipts {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("refused prune must delete nothing: %s missing", p)
		}
	}
}

func TestEvidencePruneDryRunRefusesClosedWithoutForce(t *testing.T) {
	t.Chdir(t.TempDir())
	_, _ = setupPruneTask(t, "feat__T1", 2)
	writePruneChart(t, "feat.md", "# feat\n\n## Log\n\n- L2: T1 CLOSED — done\n")
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune", "--task", "feat/T1"}); code != 1 {
		t.Fatalf("dry-run on CLOSED without --force must refuse: exit = %d", code)
	}
	if !strings.Contains(errb.String(), "refused: evidence of closed task") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestEvidencePruneForceDeletesClosed(t *testing.T) {
	t.Chdir(t.TempDir())
	taskDir, _ := setupPruneTask(t, "feat__T1", 2)
	man := writePruneManifest(t, taskDir, "manifest-run1.json")
	writePruneChart(t, "feat.md", "# feat\n\n## Log\n\n- L2: T1 CLOSED — done\n")
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune", "--task", "feat/T1", "--yes", "--force"}); code != 0 {
		t.Fatalf("exit = %d, want 0; stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "pruned 2") {
		t.Fatalf("output = %q", out.String())
	}
	if !strings.Contains(errb.String(), "--force") {
		t.Fatalf("force prune must note --force on stderr, got %q", errb.String())
	}
	if _, err := os.Stat(man); err != nil {
		t.Fatalf("manifest must survive forced prune: %v", err)
	}
	found, _ := filepath.Glob(filepath.Join(taskDir, "gate*.json"))
	if len(found) != 0 {
		t.Fatalf("forced prune must delete receipts, left %v", found)
	}
}

func TestEvidencePruneRangeGuardEnDash(t *testing.T) {
	t.Chdir(t.TempDir())
	_, _ = setupPruneTask(t, "win-hardening__H2", 1)
	writePruneChart(t, "win-hardening.md", "# w\n\n## Log\n\n- L3: H1–H4 CLOSED 2026-10-31 — all done\n")
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune", "--task", "win-hardening/H2", "--yes"}); code != 1 {
		t.Fatalf("exit = %d, want 1 (range guard)", code)
	}
	if !strings.Contains(errb.String(), "refused: evidence of closed task") {
		t.Fatalf("stderr = %q", errb.String())
	}
	// Slash form and slug form map to the same dir.
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"evidence", "prune", "--task", "win-hardening__H2", "--yes"}); code != 1 {
		t.Fatalf("slug form must also refuse: exit = %d, stderr %q", code, errb2.String())
	}
}

func TestEvidencePruneChartWideGuard(t *testing.T) {
	t.Chdir(t.TempDir())
	_, _ = setupPruneTask(t, "arch__X1", 1)
	writePruneChart(t, "arch.md", "# arch\n\n## Log\n\n- L5: Chart CERRADO.\n")
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune", "--task", "arch/X1", "--yes"}); code != 1 {
		t.Fatalf("exit = %d, want 1 (chart-wide guard)", code)
	}
	if !strings.Contains(errb.String(), "refused: evidence of closed task") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestEvidencePruneMissingTaskIsCleanError(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune", "--task", "nope/T9", "--yes"}); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "no evidence for task") {
		t.Fatalf("stderr must name the missing task, got %q", errb.String())
	}
}

func TestEvidencePruneUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence", "prune"}); code != 2 {
		t.Fatalf("missing --task: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evidence prune") {
		t.Fatalf("stderr = %q", errb.String())
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"evidence", "prune", "--task", "a/B", "--keep", "-1", "--yes"}); code != 2 {
		t.Fatalf("negative keep: exit = %d, want 2", code)
	}
	c3, _, _ := newCLI(nil)
	if code := c3.Run([]string{"evidence", "prune", "--task", "a/B", "extra"}); code != 2 {
		t.Fatalf("stray positional: exit = %d, want 2", code)
	}
}

func TestEvidenceUsageNamesPrune(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evidence"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "prune") {
		t.Fatalf("evidence usage must name prune, got %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"evidence", "frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), "prune") {
		t.Fatalf("unknown subcommand usage must name prune, got %q", errb2.String())
	}
}
