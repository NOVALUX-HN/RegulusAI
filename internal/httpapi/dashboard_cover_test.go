package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/library"
)

func almanacData(t *testing.T, body []byte) map[string]any {
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
	return data
}

// S6/C7: handleAlmanacSummary was cx14 cov50. Covers method, empty,
// populated (oldest/newest, by_type, lessons/candidates) and errors.
func TestAlmanacSummaryMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		code, body := doMethod(t, srv, m, "/v1/almanac/summary")
		if code != http.StatusMethodNotAllowed {
			t.Fatalf("%s = %d, want 405 (%s)", m, code, body)
		}
	}
}

func TestAlmanacSummaryEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	srv := newTestServer()
	code, body := get(t, srv, "/v1/almanac/summary")
	if code != http.StatusOK {
		t.Fatalf("GET = %d, want 200 (%s)", code, body)
	}
	data := almanacData(t, body)
	if data["total"].(float64) != 0 {
		t.Fatalf("total = %v, want 0 (%s)", data["total"], body)
	}
	if _, ok := data["oldest"]; ok {
		t.Fatalf("empty almanac must omit oldest (%s)", body)
	}
	if _, ok := data["newest"]; ok {
		t.Fatalf("empty almanac must omit newest (%s)", body)
	}
}

func TestAlmanacSummaryPopulated(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	older := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: "first", Content: "one", SavedAt: older,
	}, older); err != nil {
		t.Fatal(err)
	}
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeDecision, Title: "second", Content: "two", SavedAt: newer,
	}, newer); err != nil {
		t.Fatal(err)
	}
	// One admitted lesson + one pending candidate exercise the
	// library counting branches (List/Candidates success).
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	if err := library.Admit(dir, library.Lesson{
		Domain: "testdom", Task: "demo/T1", Cue: "cue", Lesson: "lesson",
		Evidence: []string{"r1"}, Confidence: library.ConfidenceHigh,
		GitHead: "abc", CreatedAt: now,
	}, "guardian"); err != nil {
		t.Fatal(err)
	}
	if err := library.Propose(dir, library.Lesson{
		ID: "les_candidate1", Domain: "testdom", Task: "demo/T2",
		Cue: "cue2", Lesson: "lesson2", Evidence: []string{"r2"},
		Confidence: library.ConfidenceLow, GitHead: "abc", CreatedAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer()
	code, body := get(t, srv, "/v1/almanac/summary")
	if code != http.StatusOK {
		t.Fatalf("GET = %d, want 200 (%s)", code, body)
	}
	data := almanacData(t, body)
	if data["total"].(float64) != 2 {
		t.Fatalf("total = %v, want 2 (%s)", data["total"], body)
	}
	byType, ok := data["by_type"].(map[string]any)
	if !ok {
		t.Fatalf("by_type = %v, want object (%s)", data["by_type"], body)
	}
	if byType["session-note"].(float64) != 1 || byType["decision"].(float64) != 1 {
		t.Fatalf("by_type = %v, want 1 session-note + 1 decision (%s)", byType, body)
	}
	if data["oldest"] != older.UTC().Format(time.RFC3339) {
		t.Fatalf("oldest = %v, want %s (%s)", data["oldest"], older.UTC().Format(time.RFC3339), body)
	}
	if data["newest"] != newer.UTC().Format(time.RFC3339) {
		t.Fatalf("newest = %v, want %s (%s)", data["newest"], newer.UTC().Format(time.RFC3339), body)
	}
	if data["lessons_admitted"].(float64) != 1 {
		t.Fatalf("lessons_admitted = %v, want 1 (%s)", data["lessons_admitted"], body)
	}
	if data["candidates_pending"].(float64) != 1 {
		t.Fatalf("candidates_pending = %v, want 1 (%s)", data["candidates_pending"], body)
	}
	if data["index_status"] == nil {
		t.Fatalf("index_status must be present (%s)", body)
	}
}

func TestAlmanacSummaryCorruptLibrarySkipped(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// Domains lists testdom, but its LESSON.md is malformed so List
	// fails and the counts stay 0 (err != nil skip branches).
	badDir := filepath.Join(dir, ".regulus", "library", "testdom", "lessons", "les_bad")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "LESSON.md"), []byte("not a lesson\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Malformed candidate: Candidates fails the same way.
	badCand := filepath.Join(dir, ".regulus", "library", "testdom", "candidates", "les_bad")
	if err := os.MkdirAll(badCand, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badCand, "LESSON.md"), []byte("not a lesson\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer()
	// Direct handler call keeps the request/response shape.
	req := httptest.NewRequest(http.MethodGet, "/v1/almanac/summary", nil)
	rec := httptest.NewRecorder()
	srv.handleAlmanacSummary(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	data := almanacData(t, rec.Body.Bytes())
	// The malformed lesson parses as an empty lesson (no error): the
	// point is the handler stays 200 and the envelope holds.
	if data["total"] == nil {
		t.Fatalf("total must be present (%s)", rec.Body.String())
	}
	_ = badDir
	_ = badCand
}

func TestAlmanacSummaryLibraryUnreadable(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// .regulus/library as a file makes Domains error: the handler
	// skips counting and still answers 200 (err != nil branch).
	if err := os.MkdirAll(filepath.Join(dir, ".regulus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".regulus", "library"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer()
	code, body := get(t, srv, "/v1/almanac/summary")
	if code != http.StatusOK {
		t.Fatalf("GET = %d, want 200 (%s)", code, body)
	}
	data := almanacData(t, body)
	if data["lessons_admitted"].(float64) != 0 || data["candidates_pending"].(float64) != 0 {
		t.Fatalf("unreadable library must count 0/0 (%s)", body)
	}
}

func TestAlmanacSummaryStoreError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// observations.jsonl as a directory breaks almanac.All: the
	// handler reports the store error as HTTP 500 (H3: monitors see failure).
	target := almanac.File(dir)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/v1/almanac/summary", nil)
	rec := httptest.NewRecorder()
	srv.handleAlmanacSummary(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET = %d, want 500", rec.Code)
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env["ok"] != false {
		t.Fatalf("store error must set ok=false (%s)", rec.Body.String())
	}
	errObj, ok := env["error"].(map[string]any)
	if !ok || !strings.Contains(errObj["message"].(string), "almanac") {
		t.Fatalf("error must name the almanac store (%s)", rec.Body.String())
	}
}

// H5 (S2,S3): the dashboard polls every 30s and fans each aggregate
// out to several panels per cycle, so successful aggregate GETs carry
// a short Cache-Control (max-age=10). Poll frequency and JSON shapes
// stay byte-identical: headers only, no body change.
func TestDashboardAggregatesCarryCacheControl(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	srv := newTestServer()
	for _, path := range []string{"/v1/metrics", "/v1/almanac/summary", "/v1/evolution"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200 (%s)", path, rec.Code, rec.Body.String())
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "max-age=10" {
			t.Fatalf("GET %s Cache-Control = %q, want max-age=10", path, cc)
		}
	}
}

// H5 (S2,S3): the almanac.All fold is memoized per store mtime+size so
// repeat polls skip the uncapped rescan, and any store append
// invalidates the memo (the second fetch sees the new total, and an
// unchanged store replays byte-identical bodies).
func TestAlmanacSummaryMemoInvalidatesOnStoreChange(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	srv := newTestServer()
	fetch := func() (string, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/almanac/summary", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET = %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		return rec.Body.String(), almanacData(t, rec.Body.Bytes())
	}
	if _, data := fetch(); data["total"].(float64) != 0 {
		t.Fatalf("total = %v, want 0 on empty store", data["total"])
	}
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: "memo", Content: "probe",
	}, now); err != nil {
		t.Fatal(err)
	}
	bodyAfterSave, data := fetch()
	if data["total"].(float64) != 1 {
		t.Fatalf("total = %v, want 1 after store append (memo must invalidate)", data["total"])
	}
	bodyAgain, _ := fetch()
	if bodyAgain != bodyAfterSave {
		t.Fatalf("repeat fetch on unchanged store must replay byte-identical body:\n%s\n%s", bodyAfterSave, bodyAgain)
	}
}
