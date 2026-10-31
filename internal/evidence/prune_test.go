package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeNReceipts(t *testing.T, dir string, n int, base time.Time) []string {
	t.Helper()
	var paths []string
	for i := 0; i < n; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		p := writeReceipt(t, dir, "gate", at, nil)
		// Pin mtime order explicitly: filesystem now is not ordered.
		ts := base.Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(p, ts, ts); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

func TestPlanKeepsNewestByMtime(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := writeNReceipts(t, dir, 5, t0)
	toDelete, toKeep, manifests, err := Plan(root, "feat/T1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if manifests != 0 {
		t.Fatalf("manifests = %d, want 0", manifests)
	}
	if len(toDelete) != 3 || len(toKeep) != 2 {
		t.Fatalf("want 3 to delete + 2 to keep, got %d + %d", len(toDelete), len(toKeep))
	}
	// Oldest three (paths[0:3]) must be deleted (mtime order).
	for i := 0; i < 3; i++ {
		found := false
		for _, d := range toDelete {
			if d == paths[i] {
				found = true
			}
		}
		if !found {
			t.Fatalf("oldest receipt %q must be in toDelete %v", paths[i], toDelete)
		}
	}
	for i := 3; i < 5; i++ {
		found := false
		for _, k := range toKeep {
			if k == paths[i] {
				found = true
			}
		}
		if !found {
			t.Fatalf("newest receipt %q must be kept %v", paths[i], toKeep)
		}
	}
	// Planning deletes nothing.
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("plan must not delete %s: %v", p, err)
		}
	}
}

func TestPlanKeepZeroDeletesAllReceipts(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeNReceipts(t, dir, 3, t0)
	toDelete, toKeep, _, err := Plan(root, "feat/T1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(toDelete) != 3 || len(toKeep) != 0 {
		t.Fatalf("keep 0 must delete all 3, got delete=%d keep=%d", len(toDelete), len(toKeep))
	}
}

func TestPlanKeepBeyondCountDeletesNothing(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeNReceipts(t, dir, 3, t0)
	toDelete, toKeep, _, err := Plan(root, "feat/T1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(toDelete) != 0 || len(toKeep) != 3 {
		t.Fatalf("keep beyond count must delete nothing, got delete=%d keep=%d", len(toDelete), len(toKeep))
	}
}

func TestPlanNeverListsManifestsAndSkipsCorrupt(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeNReceipts(t, dir, 2, t0)
	manA := filepath.Join(dir, "manifest-run1.json")
	manB := filepath.Join(dir, "manifest-run2.json")
	for _, m := range []string{manA, manB} {
		if err := os.WriteFile(m, []byte(`{"schema_version":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	qdir := filepath.Join(dir, "corrupt")
	if err := os.MkdirAll(qdir, 0o755); err != nil {
		t.Fatal(err)
	}
	qfile := filepath.Join(qdir, "old-1.json")
	if err := os.WriteFile(qfile, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	toDelete, _, manifests, err := Plan(root, "feat/T1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if manifests != 2 {
		t.Fatalf("manifests = %d, want 2", manifests)
	}
	if len(toDelete) != 2 {
		t.Fatalf("must delete only the 2 receipts, got %v", toDelete)
	}
	for _, d := range toDelete {
		if strings.HasPrefix(filepath.Base(d), "manifest-") {
			t.Fatalf("manifest must never be listed for deletion: %q", d)
		}
		if strings.Contains(filepath.ToSlash(d), "/corrupt/") {
			t.Fatalf("corrupt/ must never be listed: %q", d)
		}
	}
	n, err := Delete(toDelete)
	if err != nil || n != 2 {
		t.Fatalf("Delete = %d, %v; want 2, nil", n, err)
	}
	for _, m := range []string{manA, manB, qfile} {
		if _, err := os.Stat(m); err != nil {
			t.Fatalf("protected file must survive prune: %s: %v", m, err)
		}
	}
}

func TestPlanMissingTaskIsCleanError(t *testing.T) {
	root := t.TempDir()
	_, _, _, err := Plan(root, "nope/T9", 0)
	if err == nil || !strings.Contains(err.Error(), "no evidence for task") {
		t.Fatalf("want clean missing-task error, got %v", err)
	}
}

func TestPlanNegativeKeepRefused(t *testing.T) {
	root := t.TempDir()
	_, _, _, err := Plan(root, "feat/T1", -1)
	if err == nil || !strings.Contains(err.Error(), "keep must be") {
		t.Fatalf("want keep validation error, got %v", err)
	}
}

func TestPlanAcceptsSlashAndSlugForms(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "feat__T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeNReceipts(t, dir, 2, t0)
	d1, _, _, err := Plan(root, "feat/T1", 0)
	if err != nil {
		t.Fatal(err)
	}
	d2, _, _, err := Plan(root, "feat__T1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(d1) != 2 || len(d2) != 2 || d1[0] != d2[0] || d1[1] != d2[1] {
		t.Fatalf("slash and slug forms must map to the same dir: %v vs %v", d1, d2)
	}
}

func TestDeleteRefusesManifestAndCorrupt(t *testing.T) {
	if _, err := Delete([]string{filepath.Join("d", "manifest-x.json")}); err == nil {
		t.Fatal("Delete must refuse manifests")
	}
	if _, err := Delete([]string{filepath.Join("d", "corrupt", "old.json")}); err == nil {
		t.Fatal("Delete must refuse corrupt/")
	}
}

func writeChart(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "charts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsClosedDirect(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat.md", "# feat\n\n## Log\n\n- L2: T1 CLOSED — done\n")
	closed, err := IsClosed(root, "feat/T1")
	if err != nil || !closed {
		t.Fatalf("feat/T1 must be closed: %v, %v", closed, err)
	}
	closed, err = IsClosed(root, "feat/T2")
	if err != nil || closed {
		t.Fatalf("feat/T2 must be open: %v, %v", closed, err)
	}
}

func TestIsClosedRangeCoversMiddle(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "win-hardening.md", "# w\n\n## Log\n\n- L3: H1–H4 CLOSED 2026-10-31 — all done\n")
	for _, id := range []string{"H1", "H2", "H3", "H4"} {
		closed, err := IsClosed(root, "win-hardening/"+id)
		if err != nil || !closed {
			t.Fatalf("win-hardening/%s must be closed via range: %v, %v", id, closed, err)
		}
	}
	closed, err := IsClosed(root, "win-hardening/H5")
	if err != nil || closed {
		t.Fatalf("win-hardening/H5 must be open: %v, %v", closed, err)
	}
}

func TestIsClosedHyphenRange(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "crap-paydown.md", "# c\n\n## Log\n\n- L3: C1-C7 CLOSED + V1 PASS\n")
	closed, err := IsClosed(root, "crap-paydown/C6")
	if err != nil || !closed {
		t.Fatalf("C6 must be closed via hyphen range: %v, %v", closed, err)
	}
}

func TestIsClosedChartWide(t *testing.T) {
	for _, body := range []string{
		"# a\n\n## Log\n\n- L5: Chart CERRADO.\n",
		"# a\n\n## Log\n\n- L5: Chart CLOSED.\n",
		"# a\n\n## Log\n\n- L5: Chart arch CERRADO.\n",
	} {
		root := t.TempDir()
		writeChart(t, root, "arch.md", body)
		closed, err := IsClosed(root, "arch/anything")
		if err != nil || !closed {
			t.Fatalf("chart-wide %q must close every id: %v, %v", body, closed, err)
		}
	}
}

func TestIsClosedIgnoresUnrelatedChart(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "other.md", "# other\n\n## Log\n\n- L2: H1 CLOSED — other chart\n")
	closed, err := IsClosed(root, "feat/H1")
	if err != nil || closed {
		t.Fatalf("H1 in other.md must not close feat/H1: %v, %v", closed, err)
	}
}

func TestIsClosedPlusList(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "devin-agents.md", "# d\n\n## Log\n\n- L2: A1+A2 CLOSED + PROVE\n")
	for _, id := range []string{"A1", "A2"} {
		closed, err := IsClosed(root, "devin-agents/"+id)
		if err != nil || !closed {
			t.Fatalf("devin-agents/%s must be closed via plus list: %v, %v", id, closed, err)
		}
	}
}

func TestIsClosedNoChartsDirIsOpen(t *testing.T) {
	closed, err := IsClosed(t.TempDir(), "feat/T1")
	if err != nil || closed {
		t.Fatalf("missing charts/ must be open: %v, %v", closed, err)
	}
}

func TestIsClosedWordBoundary(t *testing.T) {
	root := t.TempDir()
	writeChart(t, root, "feat.md", "# feat\n\n## Log\n\n- L2: H10 CLOSED\n")
	closed, err := IsClosed(root, "feat/H1")
	if err != nil || closed {
		t.Fatalf("H10 must not close H1: %v, %v", closed, err)
	}
}
