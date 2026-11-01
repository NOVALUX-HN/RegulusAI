// P2: scoping ?project= por NOMBRE contra la allowlist de arranque +
// jail defensivo + tests de traversal. El query nunca es un path: la
// resolución es un lookup exacto por almanac.ProjectName(dir), así el
// traversal muere antes de tocar disco.
//
// Given todos los handlers leen stores con root fijo "."
// When se pide /v1/receipts?project=.., ?project=/abs,
//
//	?project=otro-fuera-de-allowlist o falta project con allowlist activa
//
// Then traversal y absolutos responden 400 sin tocar disco fuera de
//
//	allowlist, proyecto desconocido responde 404, y proyecto válido se
//	resuelve solo dentro de su root (jail); ningún test escribe fuera
//	de t.TempDir.
package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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

// p2Project crea un subdirectorio con nombre fijo dentro de base (nombres
// estables => ProjectName determinista por basename en minúsculas).
func p2Project(t *testing.T, base, name string) string {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// p2Seed llena un proyecto con una huella distinguible por endpoint:
// 2 receipts, 1 observación, 1 lección admitida, 1 sesión de trace y
// 1 entrada de budget.
func p2Seed(t *testing.T, root, marker string) {
	t.Helper()
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		if _, err := receipts.Write(filepath.Join(root, ".regulus", "evidence", receipts.Slug("demo/T1")),
			receipts.Receipt{
				Task: "demo/T1", Gate: "unit-tests", Verdict: receipts.VerdictPass,
				StartedAt: now.Add(time.Duration(i) * time.Second),
				Stdout:    "ok " + marker, Stderr: "",
			}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := almanac.Save(root, &almanac.Observation{
		Type: almanac.TypeDecision, Title: "choice " + marker, Content: "content " + marker,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := library.Admit(root, library.Lesson{
		Domain: "p2dom", Task: "demo/T1", Cue: "cue " + marker, Lesson: "lesson " + marker,
		Evidence: []string{"r1"}, Confidence: library.ConfidenceHigh,
		GitHead: "abc", CreatedAt: now,
	}, "guardian"); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.Record(root, trace.Entry{
		Session: "s1", Message: "m1 " + marker, Call: "c1", Tool: "read", Status: trace.StatusCompleted,
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := budget.Record(root, budget.Entry{
		Kind: budget.KindProbe, Timestamp: now, Task: "demo/T1",
		Outcome: budget.OutcomeProbeRun, Detail: "detail " + marker,
	}); err != nil {
		t.Fatal(err)
	}
}

// p2Server levanta un Server allowlistado a alpha+beta (alpha es default).
func p2Server(t *testing.T, alpha, beta string) (Server, string, string) {
	t.Helper()
	srv := New("test", testProbe(), stubRunner{code: 1}, alpha, beta)
	if len(srv.Projects) != 2 {
		t.Fatalf("allowlist = %v, want exactly alpha+beta", srv.Projects)
	}
	nameA := almanac.ProjectName(alpha)
	nameB := almanac.ProjectName(beta)
	if nameA == nameB {
		t.Fatalf("fixture names collide: %q", nameA)
	}
	if srv.Projects[0].Name != nameA || srv.Projects[0].Root != alpha {
		t.Fatalf("default must be alpha first, got %+v", srv.Projects)
	}
	return srv, nameA, nameB
}

var p2Endpoints = []string{
	"/v1/status", "/v1/version", "/v1/metrics", "/v1/almanac/summary",
	"/v1/receipts", "/v1/observations", "/v1/lessons", "/v1/traces", "/v1/budget",
}

// p2TraversalValues son intentos de escape: parent, absoluto unix,
// absolutos Windows (drive+backslash, drive+slash, drive-relativo),
// home-expansion y mezcla con el nombre real de un proyecto vecino.
func p2TraversalValues(neighborName string) []string {
	return []string{
		"..", "../" + neighborName, "..\\" + neighborName,
		"/abs", "/etc/passwd", `C:\x`, "C:/x", "C:x", ".", "~/x",
	}
}

// traversal y absolutos responden 400 en TODOS los handlers, sin tocar
// disco fuera de allowlist.
func TestP2TraversalIs400Everywhere(t *testing.T) {
	base := t.TempDir()
	alpha := p2Project(t, base, "proj-alpha")
	beta := p2Project(t, base, "proj-beta")
	sibling := p2Project(t, base, "proj-outside")
	p2Seed(t, sibling, "OUTSIDE")
	srv, _, _ := p2Server(t, alpha, beta)

	before := p2Walk(t, base)
	for _, ep := range p2Endpoints {
		for _, evil := range p2TraversalValues("proj-outside") {
			code, body := get(t, srv, ep+"?project="+evilURL(evil))
			if code != http.StatusBadRequest {
				t.Fatalf("GET %s?project=%q = %d, want 400 (%s)", ep, evil, code, body)
			}
			env := decodeEnvelope(t, body)
			if env["ok"] != false {
				t.Fatalf("GET %s?project=%q ok = true, want false", ep, evil)
			}
			if got := int(env["error"].(map[string]any)["code"].(float64)); got != mcp.CodeInvalidParams {
				t.Fatalf("GET %s?project=%q code = %d, want %d", ep, evil, got, mcp.CodeInvalidParams)
			}
		}
	}
	// Sin I/O fuera de allowlist: el vecino no allowlistado queda intacto.
	if after := p2Walk(t, base); before != after {
		t.Fatalf("evil ?project= touched disk outside the allowlist:\nbefore %s\nafter %s", before, after)
	}
	if total := p2Receipts(t, sibling); total != 2 {
		t.Fatalf("outside project receipts = %d, want 2 (untouched)", total)
	}
}

func evilURL(s string) string {
	r := strings.NewReplacer(
		"/", "%2F", "\\", "%5C", "~", "%7E", ":", "%3A", ".", "%2E",
	)
	return r.Replace(s)
}

// proyecto desconocido (nombre plano, no path) responde 404 en TODOS los
// handlers.
func TestP2UnknownProjectIs404Everywhere(t *testing.T) {
	base := t.TempDir()
	alpha := p2Project(t, base, "proj-alpha")
	bosque := p2Project(t, base, "proj-beta")
	srv, _, _ := p2Server(t, alpha, bosque)
	for _, ep := range p2Endpoints {
		code, body := get(t, srv, ep+"?project=otro-fuera-de-allowlist")
		if code != http.StatusNotFound {
			t.Fatalf("GET %s?project=otro-fuera-de-allowlist = %d, want 404 (%s)", ep, code, body)
		}
		env := decodeEnvelope(t, body)
		if env["ok"] != false {
			t.Fatalf("GET %s unknown project ok = true, want false", ep)
		}
		if got := int(env["error"].(map[string]any)["code"].(float64)); got != mcp.CodeMethodNotFound {
			t.Fatalf("GET %s unknown project code = %d, want %d", ep, got, mcp.CodeMethodNotFound)
		}
	}
}

// proyecto válido sirve solo su .regulus/; ausente = proyecto actual
// (default = primer allowlistado).
func TestP2ValidProjectServesItsOwnRoot(t *testing.T) {
	base := t.TempDir()
	alpha := p2Project(t, base, "proj-alpha")
	beta := p2Project(t, base, "proj-beta")
	p2Seed(t, alpha, "ALPHA")
	srv, nameA, nameB := p2Server(t, alpha, beta)

	// receipts: alpha=2, beta=0, default(alpha)=2.
	if total := p2Total(t, srv, "/v1/receipts?project="+nameA); total != 2 {
		t.Fatalf("alpha receipts = %d, want 2", total)
	}
	if total := p2Total(t, srv, "/v1/receipts?project="+nameB); total != 0 {
		t.Fatalf("beta receipts = %d, want 0 (empty, never alpha's)", total)
	}
	if total := p2Total(t, srv, "/v1/receipts"); total != 2 {
		t.Fatalf("default receipts = %d, want 2 (alpha, the first allowlisted)", total)
	}
	if _, body := get(t, srv, "/v1/receipts?project="+nameA); !strings.Contains(string(body), "ALPHA") {
		t.Fatalf("alpha receipts must carry the alpha marker (%s)", body)
	}
	if _, body := get(t, srv, "/v1/receipts?project="+nameB); strings.Contains(string(body), "ALPHA") {
		t.Fatalf("beta receipts must never leak alpha data (%s)", body)
	}

	// El resto de colecciones distingue igual por proyecto.
	for _, tc := range []struct{ ep, marker string }{
		{"/v1/observations?q=choice", "ALPHA"},
		{"/v1/lessons", "ALPHA"},
		{"/v1/traces", "s1"},
		{"/v1/budget", "ALPHA"},
		{"/v1/almanac/summary", ""},
		{"/v1/metrics", ""},
	} {
		sep := "?"
		if strings.Contains(tc.ep, "?") {
			sep = "&"
		}
		_, bodyA := get(t, srv, tc.ep+sep+"project="+nameA)
		_, bodyB := get(t, srv, tc.ep+sep+"project="+nameB)
		if string(bodyA) == string(bodyB) {
			t.Fatalf("GET %s alpha == beta, want per-project stores (%s)", tc.ep, bodyA)
		}
		if tc.marker != "" && !strings.Contains(string(bodyA), tc.marker) {
			t.Fatalf("GET %s alpha must carry %q (%s)", tc.ep, tc.marker, bodyA)
		}
		if tc.marker != "" && strings.Contains(string(bodyB), "ALPHA") {
			t.Fatalf("GET %s beta must never leak alpha data (%s)", tc.ep, bodyB)
		}
	}
}

// runCLI corre con el root resuelto: /v1/status?project=beta nombra a
// beta y cuenta SUS receipts, aunque el cwd sea otro.
func TestP2StatusRunsWithResolvedRoot(t *testing.T) {
	base := t.TempDir()
	alpha := p2Project(t, base, "proj-alpha")
	beta := p2Project(t, base, "proj-beta")
	p2Seed(t, alpha, "ALPHA")
	p2Seed(t, beta, "BETA")
	// cwd deliberadamente fuera de ambos para probar que el root viene
	// del lookup, no del proceso.
	t.Chdir(t.TempDir())
	srv, nameA, nameB := p2Server(t, alpha, beta)

	gotA := p2StatusProject(t, srv, "?project="+nameA)
	if gotA != almanac.ProjectName(alpha) {
		t.Fatalf("status alpha project = %q, want %q", gotA, almanac.ProjectName(alpha))
	}
	gotB := p2StatusProject(t, srv, "?project="+nameB)
	if gotB != almanac.ProjectName(beta) {
		t.Fatalf("status beta project = %q, want %q", gotB, almanac.ProjectName(beta))
	}
	gotDef := p2StatusProject(t, srv, "")
	if gotDef != almanac.ProjectName(alpha) {
		t.Fatalf("status default project = %q, want alpha %q", gotDef, almanac.ProjectName(alpha))
	}
	// Beta tiene 2 receipts propios: el stdout scoped los cuenta.
	if _, body := get(t, srv, "/v1/status?project="+nameB); !strings.Contains(string(body), "receipts:   2 on disk") {
		t.Fatalf("beta status must count beta's 2 receipts (%s)", body)
	}
}

func p2StatusProject(t *testing.T, srv Server, query string) string {
	t.Helper()
	code, body := get(t, srv, "/v1/status"+query)
	if code != 200 {
		t.Fatalf("GET /v1/status%s = %d, want 200 (%s)", query, code, body)
	}
	return almanacData(t, body)["project"].(string)
}

// Sin flag (allowlist = solo ".", P3 intacto): todo funciona idéntico a
// hoy y el default es el cwd.
func TestP2NoFlagDefaultsToCurrentProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	p2Seed(t, dir, "HERE")
	srv := New("test", testProbe(), stubRunner{code: 1})
	if len(srv.Projects) != 1 || srv.Projects[0].Root != "." {
		t.Fatalf("no-flag allowlist = %+v, want exactly [.]", srv.Projects)
	}
	if total := p2Total(t, srv, "/v1/receipts"); total != 2 {
		t.Fatalf("default receipts = %d, want 2", total)
	}
	if got := p2StatusProject(t, srv, ""); got != almanac.ProjectName(".") {
		t.Fatalf("default project = %q, want %q", got, almanac.ProjectName("."))
	}
}

// --- helpers ---

func p2Total(t *testing.T, srv Server, path string) int {
	t.Helper()
	code, body := get(t, srv, path)
	if code != 200 {
		t.Fatalf("GET %s = %d, want 200 (%s)", path, code, body)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil || !env.OK {
		t.Fatalf("GET %s bad envelope (%s): %v", path, body, err)
	}
	return env.Data.Total
}

func p2Walk(t *testing.T, base string) string {
	t.Helper()
	var names []string
	err := filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, p)
		names = append(names, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(names, "\n")
}

func p2Receipts(t *testing.T, root string) int {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(root, ".regulus", "evidence", "*", "*.json"))
	n := 0
	for _, p := range paths {
		if !strings.HasPrefix(filepath.Base(p), "manifest-") {
			n++
		}
	}
	return n
}
