// D3: the embedded visual is served with a restrictive CSP, aggregates
// never dump collections, and every JSON surface redacts secrets
// (status/version stdout, all collections, store error details).
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/budget"
	"regulus-ai/internal/library"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

// D3: / and /dashboard serve HTML with a restrictive same-origin CSP
// (inline style/script allowed because the UI is one inline <style>
// plus one inline <script>, nothing external) and nosniff.
func TestDashboardServesRestrictiveCSP(t *testing.T) {
	srv := newTestServer()
	for _, path := range []string{"/", "/dashboard"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		want := "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
		if got := rec.Header().Get("Content-Security-Policy"); got != want {
			t.Fatalf("GET %s CSP = %q, want %q", path, got, want)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("GET %s X-Content-Type-Options = %q, want nosniff", path, got)
		}
	}
}

// D3: the old aggregate endpoints return counts only — they never dump
// whole collections, so no listing can bypass the pagination caps.
func TestAggregatesNeverDumpCollections(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	const marker = "D3-AGGREGATE-CONTENT-UNIQUE"
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeDecision, Title: "agg", Content: marker,
		SavedAt: now,
	}, now); err != nil {
		t.Fatal(err)
	}
	seedReceipts(t, dir, "demo/D3agg", 3)
	srv := newTestServer()
	for _, path := range []string{"/v1/metrics", "/v1/almanac/summary"} {
		code, body := get(t, srv, path)
		if code != 200 {
			t.Fatalf("GET %s = %d, want 200 (%s)", path, code, body)
		}
		data := almanacData(t, body)
		if _, ok := data["items"]; ok {
			t.Fatalf("GET %s must not carry an items collection (%s)", path, body)
		}
		if strings.Contains(string(body), marker) {
			t.Fatalf("GET %s leaks stored free text (%s)", path, body)
		}
	}
}

// D3: every JSON surface redacts the test secret — the five paginated
// collections, the two aggregates and the verbatim status/version
// readouts — while the collections that hold it carry the marker.
func TestEverySurfaceRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	const secret = "D3-SECRET-XYZ-789"
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: "leak", Content: "api_key=" + secret,
		SavedAt: now,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.Write(dir+"/.regulus/evidence/"+receipts.Slug("demo/D3"),
		receipts.Receipt{
			Task: "demo/D3", Gate: "unit-tests", Verdict: receipts.VerdictPass,
			StartedAt: now, Stdout: "token=" + secret, Stderr: "bearer " + secret,
			Command: "run with password=" + secret,
		}); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.Record(dir, trace.Entry{
		Session: "sesD3", Message: "token=" + secret, Call: "c1", Tool: "read",
		Status: trace.StatusCompleted,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := budget.Record(dir, budget.Entry{
		Kind: budget.KindProbe, Timestamp: now, Task: "demo/D3",
		Outcome: budget.OutcomeProbeRun, Detail: "token=" + secret,
	}); err != nil {
		t.Fatal(err)
	}
	if err := library.Admit(dir, library.Lesson{
		Domain: "testdom", Task: "demo/D3", Cue: "cue api_key=" + secret, Lesson: "lesson body",
		Evidence: []string{"r1"}, Confidence: library.ConfidenceHigh,
		GitHead: "abc", CreatedAt: now,
	}, "guardian"); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer()
	paths := []string{
		"/v1/receipts", "/v1/observations", "/v1/lessons",
		"/v1/traces?session=sesD3", "/v1/budget",
		"/v1/metrics", "/v1/almanac/summary", "/v1/status", "/v1/version",
	}
	for _, path := range paths {
		_, body := get(t, srv, path)
		if strings.Contains(string(body), secret) {
			t.Fatalf("GET %s leaks the test secret (%s)", path, body)
		}
	}
	for _, path := range []string{
		"/v1/receipts", "/v1/observations", "/v1/lessons",
		"/v1/traces?session=sesD3", "/v1/budget",
	} {
		_, body := get(t, srv, path)
		if !strings.Contains(string(body), "[redacted]") {
			t.Fatalf("GET %s must carry the redaction marker (%s)", path, body)
		}
	}
}
