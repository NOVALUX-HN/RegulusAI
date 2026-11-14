// S18 (NOV-33): el dashboard local es exacto. Los agregados del
// servidor concilian numéricamente con los stores (mismos totales y
// tasas que las fixtures), y las ventanas de 7 días del cliente
// reconcilian con esos mismos totales/tasas. Vacío = 0 + unmeasured
// (nunca NaN); store corrupto = ok:false.
package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/library"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

// clientWindow7d mirrors dashboard_ui.js visionWindowCounts over local
// midnights: per-day counts for the last 7 days plus last/prev 7d
// sums. The server test feeds it the same started_at values the
// client buckets, so equal inputs must give equal windows.
func clientWindow7d(times []time.Time, now time.Time) (perDay []int, last, prev int) {
	now = now.Local()
	mid := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	days := make([]time.Time, 7)
	for i := range days {
		days[i] = mid.AddDate(0, 0, i-6)
	}
	lastStart := days[0]
	prevStart := lastStart.AddDate(0, 0, -7)
	key := func(d time.Time) string {
		d = d.Local()
		return d.Format("2006-1-2")
	}
	slots := map[string]int{}
	for i, d := range days {
		slots[key(d)] = i
	}
	perDay = make([]int, 7)
	for _, t := range times {
		t = t.Local()
		if i, ok := slots[key(t)]; ok {
			perDay[i]++
		}
		if !t.Before(lastStart) {
			last++
		} else if !t.Before(prevStart) {
			prev++
		}
	}
	return perDay, last, prev
}

// s18Meta asserts the S18 envelope on one aggregate payload:
// generated_at parses as RFC3339 and project/root travel with it.
func s18Meta(t *testing.T, path string, data map[string]any) {
	t.Helper()
	raw, ok := data["generated_at"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		t.Fatalf("GET %s generated_at = %v, want RFC3339 stamp (%v)", path, data["generated_at"], data)
	}
	if _, err := time.Parse(time.RFC3339, raw); err != nil {
		t.Fatalf("GET %s generated_at = %q, want RFC3339 (%v)", path, raw, err)
	}
	if strings.TrimSpace(data["project"].(string)) == "" {
		t.Fatalf("GET %s project is empty (%v)", path, data)
	}
	if strings.TrimSpace(data["root"].(string)) == "" {
		t.Fatalf("GET %s root is empty (%v)", path, data)
	}
	if p, ok := data["partial"]; ok && p != nil {
		part, ok := p.(map[string]any)
		if !ok {
			t.Fatalf("GET %s partial = %v, want {cap,total} (%v)", path, p, data)
		}
		if part["cap"] == nil || part["total"] == nil {
			t.Fatalf("GET %s partial = %v, want cap+total (%v)", path, part, data)
		}
	}
}

// S18: fixture con N receipts/traces/lessons: GET
// /v1/metrics|/v1/executions|/v1/almanac/summary dan los mismos
// totales/tasas que la fixture, y la ventana de 7 días del cliente
// reconcilia con ellos.
func TestDashboardConsistencyMatchesFixture(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	now := time.Now().UTC()

	// 5 receipts en 3 ciclos: runA 2/2 pass, runB 1/2 (un fail), y un
	// ciclo viejo fuera de la ventana de 7 días pero dentro de la previa.
	writeRunReceipt(t, dir, "demo/T1", "run_s18aaa00000001", "lint", receipts.VerdictPass, now.Add(-24*time.Hour), 100)
	writeRunReceipt(t, dir, "demo/T1", "run_s18aaa00000001", "unit-tests", receipts.VerdictPass, now.Add(-23*time.Hour), 200)
	writeRunReceipt(t, dir, "demo/T2", "run_s18bbb00000002", "lint", receipts.VerdictPass, now.Add(-48*time.Hour), 50)
	writeRunReceipt(t, dir, "demo/T2", "run_s18bbb00000002", "unit-tests", receipts.VerdictFail, now.Add(-47*time.Hour), 70)
	oldStart := now.Add(-10 * 24 * time.Hour)
	writeRunReceipt(t, dir, "demo/T0", "run_s18old00000000", "lint", receipts.VerdictPass, oldStart, 10)
	writeRunManifest(t, dir, "demo/T1", "run_s18aaa00000001", "feature-s18")

	// 2 observaciones (una en ventana, una en la previa) + 1 admitida
	// en ventana + 1 candidata en la previa.
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: "recent", Content: "one",
	}, now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := almanac.Save(dir, &almanac.Observation{
		Type: almanac.TypeDecision, Title: "older", Content: "two",
	}, now.Add(-10*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := library.Admit(dir, library.Lesson{
		Domain: "s18dom", Task: "demo/T1", Cue: "keep", Lesson: "pure",
		Evidence: []string{"r1"}, Confidence: library.ConfidenceHigh,
		GitHead: "abc", CreatedAt: now.Add(-24 * time.Hour),
	}, "test-guardian"); err != nil {
		t.Fatal(err)
	}
	if err := library.Propose(dir, library.Lesson{
		ID: "les_s18cand", Domain: "s18dom", Task: "demo/T2",
		Cue: "maybe", Lesson: "later", Evidence: []string{"r2"},
		Confidence: library.ConfidenceLow, GitHead: "abc",
		CreatedAt: now.Add(-10 * 24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	// 4 entradas de traza en 2 sesiones.
	for i, msg := range []string{"m1", "m2", "m3"} {
		if _, err := trace.Record(dir, trace.Entry{
			Session: "ses_s18a", Message: msg, Call: "c" + string(rune('1'+i)),
			Tool: "read", Status: trace.StatusCompleted, Task: "demo/T1",
		}, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := trace.Record(dir, trace.Entry{
		Session: "ses_s18b", Message: "m4", Call: "c4",
		Tool: "write", Status: trace.StatusCompleted, Task: "demo/T2",
	}, now); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer()

	// /v1/metrics: 5 receipts, 4 pass, tasa 4/5, 4 tool calls, meta S18.
	code, mbody := get(t, srv, "/v1/metrics")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/metrics = %d, want 200 (%s)", code, mbody)
	}
	mdata := almanacData(t, mbody)
	s18Meta(t, "/v1/metrics", mdata)
	m, ok := mdata["metrics"].(map[string]any)
	if !ok {
		t.Fatalf("metrics = %v, want object (%s)", mdata["metrics"], mbody)
	}
	if m["receipts_total"].(float64) != 5 {
		t.Fatalf("receipts_total = %v, want 5 (%s)", m["receipts_total"], mbody)
	}
	if m["receipts_pass"].(float64) != 4 {
		t.Fatalf("receipts_pass = %v, want 4 (%s)", m["receipts_pass"], mbody)
	}
	if rate := m["verification_pass_rate"].(float64); rate < 0.8-1e-9 || rate > 0.8+1e-9 {
		t.Fatalf("verification_pass_rate = %v, want 0.8 (%s)", rate, mbody)
	}
	if m["tool_calls"].(float64) != 4 {
		t.Fatalf("tool_calls = %v, want 4 trace entries (%s)", m["tool_calls"], mbody)
	}

	// /v1/executions: 3 ciclos; la suma de compuertas concilia con
	// receipts_total/pass de /v1/metrics.
	code, ebody := get(t, srv, "/v1/executions")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/executions = %d, want 200 (%s)", code, ebody)
	}
	total, _, _, items := decodePage(t, ebody)
	if total != 3 || len(items) != 3 {
		t.Fatalf("executions total = %d items %d, want 3/3 (%s)", total, len(items), ebody)
	}
	var gatesPass, gatesTotal int
	var results []string
	for _, it := range items {
		cycle := it.(map[string]any)
		gatesPass += int(cycle["gates_pass"].(float64))
		gatesTotal += int(cycle["gates_total"].(float64))
		results = append(results, cycle["result"].(string))
	}
	if gatesTotal != 5 || gatesPass != 4 {
		t.Fatalf("cycles gates = %d/%d, want 4/5 like metrics (%s)", gatesPass, gatesTotal, ebody)
	}
	if len(results) != 3 || results[0] != "pass" || results[1] != "fail" || results[2] != "pass" {
		t.Fatalf("cycles results = %v, want [pass fail pass] oldest first (%s)", results, ebody)
	}

	// /v1/almanac/summary: 2 observaciones, 1 admitida, 1 pendiente, meta S18.
	code, sbody := get(t, srv, "/v1/almanac/summary")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/almanac/summary = %d, want 200 (%s)", code, sbody)
	}
	sdata := almanacData(t, sbody)
	s18Meta(t, "/v1/almanac/summary", sdata)
	if sdata["total"].(float64) != 2 {
		t.Fatalf("summary total = %v, want 2 (%s)", sdata["total"], sbody)
	}
	if sdata["lessons_admitted"].(float64) != 1 {
		t.Fatalf("lessons_admitted = %v, want 1 (%s)", sdata["lessons_admitted"], sbody)
	}
	if sdata["candidates_pending"].(float64) != 1 {
		t.Fatalf("candidates_pending = %v, want 1 (%s)", sdata["candidates_pending"], sbody)
	}

	// /v1/evolution: la admitida y la pendiente de la fixture, meta S18.
	code, vbody := get(t, srv, "/v1/evolution")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/evolution = %d, want 200 (%s)", code, vbody)
	}
	vdata := almanacData(t, vbody)
	s18Meta(t, "/v1/evolution", vdata)
	if vdata["admitted"].(float64) != 1 {
		t.Fatalf("evolution admitted = %v, want 1 (%s)", vdata["admitted"], vbody)
	}
	if vdata["pending"].(float64) != 1 {
		t.Fatalf("evolution pending = %v, want 1 (%s)", vdata["pending"], vbody)
	}

	// /v1/git: identidad + meta S18.
	code, gbody := get(t, srv, "/v1/git")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/git = %d, want 200 (%s)", code, gbody)
	}
	gdata := almanacData(t, gbody)
	s18Meta(t, "/v1/git", gdata)
	if gdata["branch"] == nil || gdata["head"] == nil {
		t.Fatalf("git must carry branch+head (%s)", gbody)
	}

	// Ventana del cliente sobre los mismos started_at del servidor:
	// última semana 4, previa 1 (reconcilia con receipts_total 5), y la
	// tasa en ventana (3/4) sale de los mismos veredictos.
	code, rbody := get(t, srv, "/v1/receipts?limit=200")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/receipts = %d, want 200 (%s)", code, rbody)
	}
	_, _, _, ritems := decodePage(t, rbody)
	var times []time.Time
	var winPass, winTotal int
	for _, it := range ritems {
		rc := it.(map[string]any)["receipt"].(map[string]any)
		st, err := time.Parse(time.RFC3339, rc["started_at"].(string))
		if err != nil {
			t.Fatalf("receipt started_at = %v, want RFC3339 (%s)", rc["started_at"], rbody)
		}
		times = append(times, st)
		winTotal++
		if rc["verdict"] == receipts.VerdictPass {
			winPass++
		}
	}
	perDay, last, prev := clientWindow7d(times, now)
	if last != 4 || prev != 1 {
		t.Fatalf("client window last/prev = %d/%d, want 4/1 (%s)", last, prev, rbody)
	}
	sum := 0
	for _, n := range perDay {
		sum += n
	}
	if sum != last {
		t.Fatalf("client per-day sum = %d, want last = %d", sum, last)
	}
	if last+prev != winTotal || winTotal != 5 {
		t.Fatalf("window last+prev = %d, want receipts_total 5", last+prev)
	}
	if winPass != 4 {
		t.Fatalf("window pass = %d, want 4 like metrics receipts_pass", winPass)
	}

	// Ventana de lecciones sobre CreatedAt del servidor: 1 nueva, 1 previa.
	code, lbody := get(t, srv, "/v1/lessons?status=all&limit=200")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/lessons = %d, want 200 (%s)", code, lbody)
	}
	_, _, _, litems := decodePage(t, lbody)
	var ltimes []time.Time
	for _, it := range litems {
		l := it.(map[string]any)
		raw, _ := l["CreatedAt"].(string)
		if raw == "" {
			raw, _ = l["created_at"].(string)
		}
		ct, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t.Fatalf("lesson CreatedAt = %q, want RFC3339 (%s)", raw, lbody)
		}
		ltimes = append(ltimes, ct)
	}
	_, llast, lprev := clientWindow7d(ltimes, now)
	if llast != 1 || lprev != 1 {
		t.Fatalf("lessons window last/prev = %d/%d, want 1/1 (%s)", llast, lprev, lbody)
	}
}

// S18: proyecto vacío = 0 + unmeasured (tasas 0, nunca NaN).
func TestDashboardConsistencyEmptyIsZeroUnmeasured(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_ = dir
	srv := newTestServer()

	code, mbody := get(t, srv, "/v1/metrics")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/metrics = %d, want 200 (%s)", code, mbody)
	}
	if strings.Contains(string(mbody), "NaN") {
		t.Fatalf("empty metrics contains NaN (%s)", mbody)
	}
	mdata := almanacData(t, mbody)
	s18Meta(t, "/v1/metrics", mdata)
	m := mdata["metrics"].(map[string]any)
	if m["receipts_total"].(float64) != 0 || m["receipts_pass"].(float64) != 0 {
		t.Fatalf("empty receipts = %v/%v, want 0/0 (%s)", m["receipts_pass"], m["receipts_total"], mbody)
	}
	for _, k := range []string{"verification_pass_rate", "task_completion_rate"} {
		if m[k].(float64) != 0 {
			t.Fatalf("empty %s = %v, want 0 (%s)", k, m[k], mbody)
		}
	}
	unmeasured, ok := m["unmeasured"].([]any)
	if !ok || len(unmeasured) == 0 {
		t.Fatalf("empty unmeasured = %v, want the honest-zero notes (%s)", m["unmeasured"], mbody)
	}
	want := map[string]bool{
		"guardian_disagreement_rate": false,
		"token_usage":                false,
		"model_costs":                false,
	}
	for _, u := range unmeasured {
		want[u.(string)] = true
	}
	for k, seen := range want {
		if !seen {
			t.Fatalf("empty unmeasured lacks %q (%s)", k, mbody)
		}
	}

	if code, ebody := get(t, srv, "/v1/executions"); code != http.StatusOK {
		t.Fatalf("GET /v1/executions = %d, want 200 (%s)", code, ebody)
	} else if total, _, _, items := decodePage(t, ebody); total != 0 || len(items) != 0 {
		t.Fatalf("empty executions = total %d items %d, want 0/0 (%s)", total, len(items), ebody)
	}

	if code, sbody := get(t, srv, "/v1/almanac/summary"); code != http.StatusOK {
		t.Fatalf("GET /v1/almanac/summary = %d, want 200 (%s)", code, sbody)
	} else {
		sdata := almanacData(t, sbody)
		s18Meta(t, "/v1/almanac/summary", sdata)
		if sdata["total"].(float64) != 0 {
			t.Fatalf("empty summary total = %v, want 0 (%s)", sdata["total"], sbody)
		}
		if _, ok := sdata["oldest"]; ok {
			t.Fatalf("empty summary must omit oldest (%s)", sbody)
		}
		if _, ok := sdata["newest"]; ok {
			t.Fatalf("empty summary must omit newest (%s)", sbody)
		}
	}
}

// S18: store corrupto = ok:false (el error nombra el store, sin payload
// parcial que parezca medido).
func TestDashboardConsistencyCorruptIsNotOK(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	srv := newTestServer()

	// observations.jsonl como directorio rompe almanac.All.
	target := almanac.File(dir)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	code, body := get(t, srv, "/v1/almanac/summary")
	if code != http.StatusInternalServerError {
		t.Fatalf("corrupt almanac GET = %d, want 500 (%s)", code, body)
	}
	env := decodeEnvelope(t, body)
	if env["ok"] != false {
		t.Fatalf("corrupt almanac ok = %v, want false (%s)", env["ok"], body)
	}
	if env["data"] != nil {
		t.Fatalf("corrupt almanac must not carry data (%s)", body)
	}
	msg := env["error"].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "almanac") {
		t.Fatalf("corrupt error must name the almanac store (%s)", body)
	}

	// Línea malformada en una traza rompe el conteo de ejecuciones
	// (CountByExecution falla como List: nunca un conteo parcial).
	if _, err := trace.Record(dir, trace.Entry{
		Session: "ses_s18bad", Message: "m", Call: "c1",
		Tool: "read", Status: trace.StatusCompleted,
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(trace.File(dir, "ses_s18bad"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\nnot-json{{{"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	code, ebody := get(t, srv, "/v1/executions")
	if code != http.StatusBadRequest {
		t.Fatalf("corrupt trace GET /v1/executions = %d, want 400 (%s)", code, ebody)
	}
	eenv := decodeEnvelope(t, ebody)
	if eenv["ok"] != false {
		t.Fatalf("corrupt trace ok = %v, want false (%s)", eenv["ok"], ebody)
	}
}
