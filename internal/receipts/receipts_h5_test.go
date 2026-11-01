package receipts

// H5: frontera exacta de MaxCapture, colisión mismo-nanos y bordes de
// fileNameSafe — escenarios Given/When/Then que matan los 2 LIVED de
// `gremlins unleash ./internal/receipts` (receipts.go:87 frontera
// `len(s) > MaxCapture`, receipts.go:132 avance `ts++`) y cubren los
// 10 NOT COVERED de `fileNameSafe` (receipts.go:176 bordes + fallback
// `"gate"`).
//
// Alcance de mutación (H5-Then, justificación medida 2026-11-01, sin
// cambio en regulus.json:12):
//   - `gremlins unleash` acepta UN solo path (dos paths = "accepts at
//     most 1 arg(s)"); el padre común arrastraría todo ./internal/*.
//   - Dry-run envfacts: 80 RUNNABLE + 15 NOT COVERED = 95 mutantes y
//     solo el análisis (sin ejecutar tests) tardó 203s; el unleash
//     completo con --workers 1 superaría el timeout de 600s del gate.
//   - receipts: 15 RUNNABLE; los 2 LIVED previos (:87 frontera,
//     :132 ts++) mueren con estos tests (verificado por simulación:
//     aplicar cada mutante hace fallar su test, revertido después),
//     efficacy 86.67% → ~100% esperado, ≥70 sostenido.
//   - fileNameSafe: `go tool cover` da 100.0% en la función y cada
//     brazo de la condición se ejecuta en ambos sentidos (ver tabla:
//     fronteras ":" 0x3A y "~" 0x7E incluidas); el dry-run sigue
//     listando los 10 fragmentos de receipts.go:176 como NOT COVERED
//     por mapeo de bloques del coverprofile (condiciones multi-brazo
//     en una sola línea), artefacto excluido del denominador de
//     efficacy en cualquier caso.
//   - envfacts queda cubierto por unit-tests (envfacts_test.go,
//     scan_verdict_test.go, scan_corrupt_test.go, admit_h3_test.go)
//     + coverage-floor + spec; reevaluar la ampliación cuando el
//     gate disponga de más presupuesto o gremlins soporte multi-path.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Given un contenido de exactamente MaxCapture bytes
// When se captura
// Then se almacena verbatim sin marcador (frontera `>` estricta:
// `>=` truncaría aquí y este test lo mataría).
func TestCaptureExactMaxCaptureIsVerbatim(t *testing.T) {
	s := strings.Repeat("a", MaxCapture)
	text, sha := Capture(s)
	if text != s {
		t.Fatalf("exact-MaxCapture output must be stored verbatim, got len %d want %d", len(text), len(s))
	}
	if IsTruncated(text) {
		t.Fatal("exact-MaxCapture output must not carry the truncation marker")
	}
	if sha != shaOf(s) {
		t.Fatal("sha must pin the content")
	}
}

// Given un contenido de MaxCapture+1 bytes
// When se captura
// Then se trunca con marcador y el sha sigue fijando el FULL original.
func TestCaptureOneOverMaxCaptureTruncates(t *testing.T) {
	s := strings.Repeat("b", MaxCapture+1)
	text, sha := Capture(s)
	if !IsTruncated(text) {
		t.Fatal("MaxCapture+1 output must carry the truncation marker")
	}
	if want := s[:MaxCapture] + truncMarker; text != want {
		t.Fatalf("over-cap output must be prefix plus marker, got len %d want %d", len(text), len(want))
	}
	if sha != shaOf(s) {
		t.Fatal("sha must pin the FULL content, not the truncation")
	}
	if got := StoredSHA(text); got == sha {
		t.Fatal("stored-text pin must differ from the full-output pin on truncation")
	}
}

// Given dos receipts con el mismo StartedAt (mismo nanos)
// When se escriben en el mismo dir
// Then avanzan al siguiente slot (ts+1, nunca atrás ni el mismo),
// reciben IDs distintos ligados a su slot y el primero queda intacto.
// Mata el LIVED de `ts++` (p.ej. `ts--` caería en ts-1 y fallaría el
// nombre esperado) y el de `r.ID = ""` (sin reset ambos IDs serían
// iguales).
func TestWriteSameNanosAdvancesSlotAndReassignsID(t *testing.T) {
	dir := t.TempDir()
	start := time.Unix(1700000000, 0).UTC()
	ts := start.UnixNano()
	mk := func() Receipt {
		return Receipt{Task: "t", Gate: "lint", Layer: 1, Command: "x", Verdict: VerdictPass, ExitCode: 0, StartedAt: start}
	}

	p1, err := Write(dir, mk())
	if err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(p1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := Write(dir, mk())
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatal("same-slot write must advance to a new file")
	}
	want1 := filepath.Join(dir, fmt.Sprintf("lint-%d.json", ts))
	if p1 != want1 {
		t.Fatalf("first write must land on its nanos slot, got %q want %q", p1, want1)
	}
	want2 := filepath.Join(dir, fmt.Sprintf("lint-%d.json", ts+1))
	if p2 != want2 {
		t.Fatalf("collision must advance to ts+1, got %q want %q", p2, want2)
	}
	r1, err := ReadFile(p1)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := ReadFile(p2)
	if err != nil {
		t.Fatal(err)
	}
	wantID1 := "rec_" + strconv.FormatInt(ts, 16)
	wantID2 := "rec_" + strconv.FormatInt(ts+1, 16)
	if r1.ID != wantID1 {
		t.Fatalf("first id must tie to its slot, got %q want %q", r1.ID, wantID1)
	}
	if r2.ID != wantID2 {
		t.Fatalf("collided write must reassign the id to its slot, got %q want %q", r2.ID, wantID2)
	}
	if r1.ID == r2.ID {
		t.Fatalf("same-nanos writes must yield distinct ids, both %q", r1.ID)
	}
	now, err := os.ReadFile(p1)
	if err != nil {
		t.Fatal(err)
	}
	if string(orig) != string(now) {
		t.Fatal("the first receipt must be unchanged (append-only)")
	}
}

// Given nombres de gate en los bordes del alfabeto conservativo
// When se sanitizan
// Then [a-z0-9-_] sobreviven, mayúsculas se pliegan y el resto es `-`,
// con vacío → `gate` (cubre cada rama de receipts.go:176).
func TestFileNameSafeBoundaries(t *testing.T) {
	cases := map[string]string{
		"lint":         "lint",
		"unit-tests":   "unit-tests",
		"abcxyz":       "abcxyz",
		"0129":         "0129",
		"a-b_c":        "a-b_c",
		"-":            "-",
		"_":            "_",
		"ABC":          "abc",
		"A-Z_0":        "a-z_0",
		" ":            "-",
		"/":            "-",
		":":            "-", // 0x3A: >= '0' true, <= '9' false (frontera alta de dígito)
		".":            "-",
		"!":            "-",
		"~":            "-", // 0x7E: >= 'a' true, <= 'z' false (frontera alta de letra)
		"Weird Name/2": "weird-name-2",
		"UPPER CASE!":  "upper-case-",
		"a/b.c d":      "a-b-c-d",
		"":             "gate",
	}
	for in, want := range cases {
		if got := fileNameSafe(in); got != want {
			t.Errorf("fileNameSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

// Given un gate vacío
// When se escribe el receipt
// Then el fallback `gate` nombra el fichero (integración Write).
func TestWriteEmptyGateUsesFallback(t *testing.T) {
	p, err := Write(t.TempDir(), Receipt{
		Gate: "", Verdict: VerdictPass, StartedAt: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if name := filepath.Base(p); !strings.HasPrefix(name, "gate-") {
		t.Fatalf("empty gate must fall back to %q prefix, got %q", "gate-", name)
	}
}
