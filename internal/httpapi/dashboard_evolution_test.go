package httpapi

// Evolution surface for the dashboard: one read-only aggregate over
// existing stores — admitted lessons, proxy tokens, rot count, pending
// candidates, last tick position. Counts only, never collections (D3
// aggregates policy); free text never leaves this endpoint.
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"regulus-ai/internal/library"
)

func TestEvolutionSummaryCounts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	if err := library.Admit(dir, library.Lesson{
		Domain: "go", Task: "", Cue: "keep facts pure",
		Lesson:     "Pure helpers stay testable.",
		Evidence:   []string{"rec_x/1"},
		Confidence: "high", CreatedAt: now,
	}, "test-guardian"); err != nil {
		t.Fatal(err)
	}
	if err := library.Propose(dir, library.Lesson{
		Domain: "go", Task: "", Cue: "pending idea",
		Lesson:     "Maybe useful.",
		Evidence:   []string{"ses_a"},
		Confidence: "low", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/v1/evolution", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/evolution = %d, want 200", rec.Code)
	}
	var body struct {
		Result struct {
			Admitted    int   `json:"admitted"`
			ProxyTokens int64 `json:"proxy_tokens"`
			Rot         int   `json:"rot"`
			Pending     int   `json:"pending"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("evolution response is not an envelope: %v\n%s", err, rec.Body.String())
	}
	if body.Result.Admitted != 1 || body.Result.Pending != 1 {
		t.Fatalf("evolution = %+v, want 1 admitted 1 pending", body.Result)
	}
	if body.Result.ProxyTokens <= 0 {
		t.Fatalf("proxy tokens must be measured, got %+v", body.Result)
	}
}

func TestEvolutionSummaryEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/v1/evolution", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/evolution = %d, want 200", rec.Code)
	}
	if !json.Valid(rec.Body.Bytes()) {
		t.Fatal("evolution response must be JSON")
	}
}
