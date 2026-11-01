// D1 collections: paginated read-only receipts, observations with
// search, lessons/candidates, trace sessions and entries, budget
// history — plus redaction and 405s on every new route.
package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/budget"
	"regulus-ai/internal/library"
	"regulus-ai/internal/mcp"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

func decodePage(t *testing.T, body []byte) (total, limit, offset int, items []any) {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, body)
	}
	if env["ok"] != true {
		t.Fatalf("ok = %v, want true (%s)", env["ok"], body)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %v, want object (%s)", env["data"], body)
	}
	total = int(data["total"].(float64))
	limit = int(data["limit"].(float64))
	offset = int(data["offset"].(float64))
	raw, _ := json.Marshal(data["items"])
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("items decode: %v (%s)", err, body)
	}
	return total, limit, offset, items
}

func seedReceipts(t *testing.T, dir string, task string, n int) {
	t.Helper()
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		_, err := receipts.Write(dir+"/.regulus/evidence/"+receipts.Slug(task),
			receipts.Receipt{
				Task: task, Gate: "unit-tests", Verdict: receipts.VerdictPass,
				StartedAt: now.Add(time.Duration(i) * time.Second),
				Stdout:    "ok", Stderr: "",
			})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestReceiptsPagination(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	seedReceipts(t, dir, "demo/T1", 5)
	srv := newTestServer()

	code, body := get(t, srv, "/v1/receipts?limit=2&offset=1")
	if code != 200 {
		t.Fatalf("GET /v1/receipts = %d, want 200 (%s)", code, body)
	}
	total, limit, offset, items := decodePage(t, body)
	if total != 5 || limit != 2 || offset != 1 || len(items) != 2 {
		t.Fatalf("page = total %d limit %d offset %d items %d, want 5/2/1/2 (%s)", total, limit, offset, len(items), body)
	}

	// task filter narrows; unknown task is an empty page, still 200.
	code, body = get(t, srv, "/v1/receipts?task=demo%2FT1")
	if code != 200 {
		t.Fatalf("GET task filter = %d, want 200 (%s)", code, body)
	}
	if total, _, _, _ := decodePage(t, body); total != 5 {
		t.Fatalf("task filter total = %d, want 5 (%s)", total, body)
	}
	code, body = get(t, srv, "/v1/receipts?task=nope%2Fnone")
	if code != 200 {
		t.Fatalf("GET unknown task = %d, want 200 (%s)", code, body)
	}
	if total, _, _, items := decodePage(t, body); total != 0 || len(items) != 0 {
		t.Fatalf("unknown task page = total %d items %d, want 0/0 (%s)", total, len(items), body)
	}

	// offset past the end is an empty page with the total preserved.
	code, body = get(t, srv, "/v1/receipts?offset=99")
	if code != 200 {
		t.Fatalf("GET offset past end = %d, want 200 (%s)", code, body)
	}
	if total, _, _, items := decodePage(t, body); total != 5 || len(items) != 0 {
		t.Fatalf("past-end page = total %d items %d, want 5/0 (%s)", total, len(items), body)
	}
}

func TestCollectionsBadPagination(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := newTestServer()
	for _, path := range []string{
		"/v1/receipts?limit=0", "/v1/receipts?limit=999", "/v1/receipts?limit=abc", "/v1/receipts?offset=-1",
		"/v1/observations?limit=0", "/v1/lessons?limit=999", "/v1/traces?offset=-2", "/v1/budget?limit=nope",
		"/v1/observations?type=bogus", "/v1/lessons?status=bogus", "/v1/lessons?domain=..",
	} {
		code, body := get(t, srv, path)
		if code != http.StatusBadRequest {
			t.Fatalf("GET %s = %d, want 400 (%s)", path, code, body)
		}
		env := decodeEnvelope(t, body)
		if env["ok"] != false {
			t.Fatalf("GET %s ok = %v, want false (%s)", path, env["ok"], body)
		}
		errObj := env["error"].(map[string]any)
		if int(errObj["code"].(float64)) != mcp.CodeInvalidParams {
			t.Fatalf("GET %s code = %v, want %d", path, errObj["code"], mcp.CodeInvalidParams)
		}
	}
}

func TestObservationsSearch(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeDecision, Title: "postgres pooling", Content: "pool size matters",
		SavedAt: now,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: "unrelated", Content: "nothing to see",
		SavedAt: now.Add(time.Second),
	}, now); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer()
	code, body := get(t, srv, "/v1/observations?q=postgres")
	if code != 200 {
		t.Fatalf("GET search = %d, want 200 (%s)", code, body)
	}
	total, _, _, items := decodePage(t, body)
	if total != 1 || len(items) != 1 {
		t.Fatalf("search total = %d items %d, want 1/1 (%s)", total, len(items), body)
	}
	if !strings.Contains(string(body), "postgres") {
		t.Fatalf("search must return the matching observation (%s)", body)
	}

	// type filter narrows to the decision only.
	code, body = get(t, srv, "/v1/observations?type=decision")
	if code != 200 {
		t.Fatalf("GET type filter = %d, want 200 (%s)", code, body)
	}
	if total, _, _, _ := decodePage(t, body); total != 1 {
		t.Fatalf("type filter total = %d, want 1 (%s)", total, body)
	}
}

func TestCollectionsRedactSecret(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	secret := "SECRET-ABC-123"
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: "leak", Content: "api_key=" + secret,
		SavedAt: now,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.Write(dir+"/.regulus/evidence/"+receipts.Slug("demo/T9"),
		receipts.Receipt{
			Task: "demo/T9", Gate: "unit-tests", Verdict: receipts.VerdictPass,
			StartedAt: now, Stdout: "token=" + secret, Stderr: "bearer " + secret,
			Command: "run with password=" + secret,
		}); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.Record(dir, trace.Entry{
		Session: "ses1", Message: "token=" + secret, Call: "c1", Tool: "read", Status: trace.StatusCompleted,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := budget.Record(dir, budget.Entry{
		Kind: budget.KindProbe, Timestamp: now, Task: "demo/T9",
		Outcome: budget.OutcomeProbeRun, Detail: "token=" + secret,
	}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer()
	for _, path := range []string{
		"/v1/receipts", "/v1/observations", "/v1/traces?session=ses1", "/v1/budget",
	} {
		_, body := get(t, srv, path)
		if strings.Contains(string(body), secret) {
			t.Fatalf("GET %s leaks the test secret (%s)", path, body)
		}
		if !strings.Contains(string(body), "[redacted]") {
			t.Fatalf("GET %s must carry the redaction marker (%s)", path, body)
		}
	}
}

func TestLessonsAndBudgetAndTraces(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	if err := library.Admit(dir, library.Lesson{
		Domain: "testdom", Task: "demo/T1", Cue: "cue one", Lesson: "lesson one",
		Evidence: []string{"r1"}, Confidence: library.ConfidenceHigh,
		GitHead: "abc", CreatedAt: now,
	}, "guardian"); err != nil {
		t.Fatal(err)
	}
	if err := library.Propose(dir, library.Lesson{
		ID: "les_cand1", Domain: "testdom", Task: "demo/T2",
		Cue: "cue two", Lesson: "lesson two", Evidence: []string{"r2"},
		Confidence: library.ConfidenceLow, GitHead: "abc", CreatedAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer()
	if code, body := get(t, srv, "/v1/lessons"); code != 200 {
		t.Fatalf("GET /v1/lessons = %d, want 200 (%s)", code, body)
	} else if total, _, _, _ := decodePage(t, body); total != 2 {
		t.Fatalf("lessons total = %d, want 2 (%s)", total, body)
	}
	if _, body := get(t, srv, "/v1/lessons?status=admitted"); !strings.Contains(string(body), "lesson one") || strings.Contains(string(body), "lesson two") {
		t.Fatalf("admitted filter must return only the admitted lesson (%s)", body)
	}
	if _, body := get(t, srv, "/v1/lessons?status=candidate"); !strings.Contains(string(body), "lesson two") || strings.Contains(string(body), "lesson one") {
		t.Fatalf("candidate filter must return only the candidate (%s)", body)
	}
	if _, body := get(t, srv, "/v1/lessons?q=two"); !strings.Contains(string(body), "lesson two") {
		t.Fatalf("lessons q must match the candidate cue/lesson (%s)", body)
	}

	if _, err := trace.Record(dir, trace.Entry{
		Session: "s1", Message: "m1", Call: "c1", Tool: "read", Status: trace.StatusCompleted, Task: "demo/T1",
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.Record(dir, trace.Entry{
		Session: "s1", Message: "m2", Call: "c2", Tool: "write", Status: trace.StatusError,
	}, now); err != nil {
		t.Fatal(err)
	}
	if code, body := get(t, srv, "/v1/traces"); code != 200 {
		t.Fatalf("GET /v1/traces = %d, want 200 (%s)", code, body)
	} else if total, _, _, _ := decodePage(t, body); total != 1 {
		t.Fatalf("traces sessions total = %d, want 1 (%s)", total, body)
	}
	if _, body := get(t, srv, "/v1/traces?session=s1"); !strings.Contains(string(body), "m1") {
		t.Fatalf("trace entries must list session calls (%s)", body)
	}
	if _, body := get(t, srv, "/v1/traces?session=s1&task=demo%2FT1"); strings.Contains(string(body), "m2") {
		t.Fatalf("trace task filter must drop the unlinked entry (%s)", body)
	}

	if err := budget.Record(dir, budget.Entry{
		Kind: budget.KindProbe, Timestamp: now, Task: "demo/T1",
		Outcome: budget.OutcomeProbeRun, Detail: "first",
	}); err != nil {
		t.Fatal(err)
	}
	if err := budget.Record(dir, budget.Entry{
		Kind: budget.KindReflect, Timestamp: now, Task: "demo/T2",
		Outcome: budget.OutcomeCandidateProposed, Detail: "second",
	}); err != nil {
		t.Fatal(err)
	}
	if code, body := get(t, srv, "/v1/budget"); code != 200 {
		t.Fatalf("GET /v1/budget = %d, want 200 (%s)", code, body)
	} else if total, _, _, _ := decodePage(t, body); total != 2 {
		t.Fatalf("budget total = %d, want 2 (%s)", total, body)
	}
	if _, body := get(t, srv, "/v1/budget?task=demo%2FT1"); !strings.Contains(string(body), "first") || strings.Contains(string(body), "second") {
		t.Fatalf("budget task filter must return only the matching entry (%s)", body)
	}
}

func TestCollectionsMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	paths := []string{"/v1/receipts", "/v1/observations", "/v1/lessons", "/v1/traces", "/v1/budget"}
	for _, path := range paths {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			code, body := doMethod(t, srv, method, path)
			if code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s = %d, want 405 (%s)", method, path, code, body)
			}
			env := decodeEnvelope(t, body)
			if env["ok"] != false {
				t.Fatalf("%s %s ok = %v, want false", method, path, env["ok"])
			}
			errObj := env["error"].(map[string]any)
			if int(errObj["code"].(float64)) != mcp.CodeInvalidRequest {
				t.Fatalf("%s %s code = %v, want %d", method, path, errObj["code"], mcp.CodeInvalidRequest)
			}
		}
	}
}
