// V3 (charts/dashboard-vision.md): numbered execution cycles from
// manifests+receipts, the git identity endpoint, pagination, 405s and
// the Unknown ("—") fallbacks — all against real stores, never fixtures
// of the implementation.
package httpapi

import (
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/manifest"
	"regulus-ai/internal/mcp"
	"regulus-ai/internal/receipts"
)

func writeRunReceipt(t *testing.T, dir, task, runID, gate, verdict string, start time.Time, dur int64) {
	t.Helper()
	if _, err := receipts.Write(
		filepath.Join(dir, ".regulus", "evidence", receipts.Slug(task)),
		receipts.Receipt{
			Task: task, Gate: gate, RunID: runID, Verdict: verdict,
			StartedAt: start, DurationMS: dur, Stdout: "ok",
		}); err != nil {
		t.Fatal(err)
	}
}

func writeRunManifest(t *testing.T, dir, task, runID, branch string) {
	t.Helper()
	m := manifest.Manifest{
		Schema: manifest.SchemaVersion, RunID: runID, Task: task,
		CreatedAt: time.Now().UTC(), Branch: branch,
	}
	if _, err := manifest.Write(evidence.TaskDir(dir, task), m); err != nil {
		t.Fatal(err)
	}
}

func execItem(t *testing.T, items []any, i int) map[string]any {
	t.Helper()
	if i >= len(items) {
		t.Fatalf("item %d missing (have %d)", i, len(items))
	}
	it, ok := items[i].(map[string]any)
	if !ok {
		t.Fatalf("item %d = %v, want object", i, items[i])
	}
	return it
}

// V3: receipts group by run_id into sequential cycles (1 = oldest) with
// task, result, gate counts, summed duration, latest date and the
// manifest branch; missing manifests and pre-V3 manifests show "—".
func TestExecutionsGroupsCycles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

	writeRunReceipt(t, dir, "demo/T1", "run_aaa000000000001", "lint", receipts.VerdictPass, base, 100)
	writeRunReceipt(t, dir, "demo/T1", "run_aaa000000000001", "unit-tests", receipts.VerdictPass, base.Add(time.Minute), 200)
	writeRunManifest(t, dir, "demo/T1", "run_aaa000000000001", "feature-x")

	writeRunReceipt(t, dir, "demo/T2", "run_bbb000000000002", "lint", receipts.VerdictPass, base.Add(time.Hour), 50)
	writeRunReceipt(t, dir, "demo/T2", "run_bbb000000000002", "unit-tests", receipts.VerdictFail, base.Add(time.Hour+2*time.Minute), 70)

	writeRunReceipt(t, dir, "demo/T3", "run_ccc000000000003", "lint", receipts.VerdictPass, base.Add(2*time.Hour), 10)
	writeRunManifest(t, dir, "demo/T3", "run_ccc000000000003", "")

	srv := newTestServer()
	code, body := get(t, srv, "/v1/executions")
	if code != 200 {
		t.Fatalf("GET /v1/executions = %d, want 200 (%s)", code, body)
	}
	total, _, _, items := decodePage(t, body)
	if total != 3 || len(items) != 3 {
		t.Fatalf("executions total = %d items %d, want 3/3 (%s)", total, len(items), body)
	}

	first := execItem(t, items, 0)
	if int(first["n"].(float64)) != 1 || first["task"] != "demo/T1" || first["result"] != "pass" {
		t.Fatalf("cycle 1 = %v, want n=1 task=demo/T1 result=pass", first)
	}
	if int(first["gates_pass"].(float64)) != 2 || int(first["gates_total"].(float64)) != 2 {
		t.Fatalf("cycle 1 gates = %v/%v, want 2/2", first["gates_pass"], first["gates_total"])
	}
	if int64(first["duration_ms"].(float64)) != 300 {
		t.Fatalf("cycle 1 duration = %v, want 300 (100+200)", first["duration_ms"])
	}
	if first["date"] != base.Add(time.Minute).UTC().Format(time.RFC3339) {
		t.Fatalf("cycle 1 date = %v, want max started_at", first["date"])
	}
	if first["branch"] != "feature-x" {
		t.Fatalf("cycle 1 branch = %v, want manifest branch feature-x", first["branch"])
	}

	second := execItem(t, items, 1)
	if int(second["n"].(float64)) != 2 || second["result"] != "fail" {
		t.Fatalf("cycle 2 = %v, want n=2 result=fail (one gate failed)", second)
	}
	if int(second["gates_pass"].(float64)) != 1 || int(second["gates_total"].(float64)) != 2 {
		t.Fatalf("cycle 2 gates = %v/%v, want 1/2", second["gates_pass"], second["gates_total"])
	}
	if second["branch"] != manifest.Unknown {
		t.Fatalf("cycle 2 branch = %v, want %q without manifest", second["branch"], manifest.Unknown)
	}

	third := execItem(t, items, 2)
	if int(third["n"].(float64)) != 3 || third["branch"] != manifest.Unknown {
		t.Fatalf("cycle 3 = %v, want n=3 branch %q (pre-V3 manifest)", third, manifest.Unknown)
	}
}

// V3: receipts without run_id group by task plus UTC hour window and
// always show "—" for the branch.
func TestExecutionsLegacyWithoutRunID(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	base := time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)

	writeRunReceipt(t, dir, "demo/old", "", "lint", receipts.VerdictPass, base, 5)
	writeRunReceipt(t, dir, "demo/old", "", "unit-tests", receipts.VerdictPass, base.Add(30*time.Minute), 7)
	writeRunReceipt(t, dir, "demo/old", "", "lint", receipts.VerdictPass, base.Add(2*time.Hour), 9)

	srv := newTestServer()
	code, body := get(t, srv, "/v1/executions")
	if code != 200 {
		t.Fatalf("GET /v1/executions = %d, want 200 (%s)", code, body)
	}
	total, _, _, items := decodePage(t, body)
	if total != 2 || len(items) != 2 {
		t.Fatalf("legacy executions total = %d items %d, want 2/2 (same-hour pair + later one) (%s)", total, len(items), body)
	}
	for i, wantN := range []int{1, 2} {
		it := execItem(t, items, i)
		if int(it["n"].(float64)) != wantN || it["task"] != "demo/old" || it["branch"] != manifest.Unknown {
			t.Fatalf("legacy cycle %d = %v, want n=%d task=demo/old branch —", i, it, wantN)
		}
	}
	if int64(execItem(t, items, 0)["duration_ms"].(float64)) != 12 {
		t.Fatalf("legacy cycle 1 duration = %v, want 12 (5+7)", execItem(t, items, 0)["duration_ms"])
	}
}

// V3: empty store is an empty page, pagination clips like the other
// collections, and bad pagination is a 400.
func TestExecutionsPagination(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	base := time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		writeRunReceipt(t, dir, "demo/T1", "run_pag00000000000"+string(rune('a'+i)), "lint", receipts.VerdictPass, base.Add(time.Duration(i)*time.Hour), 1)
	}
	srv := newTestServer()

	if code, body := get(t, srv, "/v1/executions?limit=2"); code != 200 {
		t.Fatalf("GET paged = %d, want 200 (%s)", code, body)
	} else if total, limit, offset, items := decodePage(t, body); total != 3 || limit != 2 || offset != 0 || len(items) != 2 {
		t.Fatalf("paged = total %d limit %d offset %d items %d, want 3/2/0/2 (%s)", total, limit, offset, len(items), body)
	}
	if code, body := get(t, srv, "/v1/executions?limit=2&offset=2"); code != 200 {
		t.Fatalf("GET offset = %d, want 200 (%s)", code, body)
	} else if _, _, _, items := decodePage(t, body); len(items) != 1 || int(items[0].(map[string]any)["n"].(float64)) != 3 {
		t.Fatalf("offset page must hold only cycle n=3 (%s)", body)
	}
	if code, body := get(t, srv, "/v1/executions?offset=99"); code != 200 {
		t.Fatalf("GET past end = %d, want 200 (%s)", code, body)
	} else if total, _, _, items := decodePage(t, body); total != 3 || len(items) != 0 {
		t.Fatalf("past-end page = total %d items %d, want 3/0 (%s)", total, len(items), body)
	}
	for _, path := range []string{
		"/v1/executions?limit=0", "/v1/executions?limit=999", "/v1/executions?limit=abc", "/v1/executions?offset=-1",
	} {
		code, body := get(t, srv, path)
		if code != http.StatusBadRequest {
			t.Fatalf("GET %s = %d, want 400 (%s)", path, code, body)
		}
	}
}

// V3: both routes stay read-only and jailed — non-GET is 405,
// path-shaped ?project= is 400, unknown names are 404.
func TestExecutionsAndGitGuards(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := newTestServer()
	for _, path := range []string{"/v1/executions", "/v1/git"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			code, body := doMethod(t, srv, method, path)
			if code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s = %d, want 405 (%s)", method, path, code, body)
			}
			env := decodeEnvelope(t, body)
			if int(env["error"].(map[string]any)["code"].(float64)) != mcp.CodeInvalidRequest {
				t.Fatalf("%s %s code = %v, want %d", method, path, env["error"], mcp.CodeInvalidRequest)
			}
		}
		if code, _ := get(t, srv, path+"?project=.."); code != http.StatusBadRequest {
			t.Fatalf("GET %s?project=.. = %d, want 400", path, code)
		}
		if code, _ := get(t, srv, path+"?project=no-such-project"); code != http.StatusNotFound {
			t.Fatalf("GET %s?project=no-such = %d, want 404", path, code)
		}
	}
}

// V3: outside git both fields fall back to "—" with a 200.
func TestGitOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := newTestServer()
	code, body := get(t, srv, "/v1/git")
	if code != 200 {
		t.Fatalf("GET /v1/git = %d, want 200 (%s)", code, body)
	}
	env := decodeEnvelope(t, body)
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("git data = %v, want object (%s)", env["data"], body)
	}
	if data["branch"] != manifest.Unknown || data["head"] != manifest.Unknown {
		t.Fatalf("git outside repo = %v, want branch/head %q", data, manifest.Unknown)
	}
}

// V3: inside the checkout /v1/git reports the real branch and short
// HEAD the manifest writer would pin.
func TestGitInRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	srv := newTestServer()
	code, body := get(t, srv, "/v1/git")
	if code != 200 {
		t.Fatalf("GET /v1/git = %d, want 200 (%s)", code, body)
	}
	env := decodeEnvelope(t, body)
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("git data = %v, want object (%s)", env["data"], body)
	}
	want := func(args ...string) string {
		cmd := exec.Command("git", args...)
		out, err := cmd.Output()
		if err != nil {
			t.Skipf("git %v unavailable: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	if data["branch"] != want("rev-parse", "--abbrev-ref", "HEAD") {
		t.Fatalf("git branch = %v, want the checkout branch", data["branch"])
	}
	if data["head"] != want("rev-parse", "--short", "HEAD") {
		t.Fatalf("git head = %v, want the short HEAD", data["head"])
	}
}

// V3: an empty evidence store is an empty executions page, still 200.
func TestExecutionsEmptyStore(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := newTestServer()
	code, body := get(t, srv, "/v1/executions")
	if code != 200 {
		t.Fatalf("GET /v1/executions = %d, want 200 (%s)", code, body)
	}
	if total, _, _, items := decodePage(t, body); total != 0 || len(items) != 0 {
		t.Fatalf("empty store page = total %d items %d, want 0/0 (%s)", total, len(items), body)
	}
}
