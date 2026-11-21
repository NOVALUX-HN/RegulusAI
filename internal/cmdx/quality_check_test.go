// QualityCheckCmd: tabla de exit codes con IO{Dir: tmp}.
// Cubre el parse (flag desconocido, requeridos ausentes, posicional
// extra), baseline ausente, thresholds ilegibles, window ausente o
// malformada, out no escribible, y los caminos ok en texto, json, con
// thresholds y con out. Los mensajes se preservan: los casos de uso
// exigen "usage:" y cada fallo exige el prefijo "quality check:".
// La rama de fallo de marshal es inalcanzable (Diagnosis siempre
// serializa) y queda documentada aquí como excepción de cobertura.
// Portable: solo t.TempDir(), sin shell ni git.
package cmdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/quality"
)

func writeCheckWindow(t *testing.T, dir, name string) string {
	t.Helper()
	raw := `[{"source":"window","run_id":"r1","success":true,"retries":0,"latency_ms":120,"latency_known":true,"tokens_in":10,"tokens_out":20,"tokens_known":true},` +
		`{"source":"window","run_id":"r2","success":true,"retries":1,"latency_ms":140,"latency_known":true,"tokens_in":12,"tokens_out":22,"tokens_known":true}]`
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeCheckBaseline(t *testing.T, dir string) string {
	t.Helper()
	obs := []quality.Observation{
		{Source: quality.SourceWindow, RunID: "r0", Success: true, LatencyMS: 100, LatencyKnown: true, TokensIn: 10, TokensOut: 20, TokensKnown: true},
		{Source: quality.SourceWindow, RunID: "r1", Success: true, LatencyMS: 110, LatencyKnown: true, TokensIn: 11, TokensOut: 21, TokensKnown: true},
	}
	b, err := quality.BuildBaseline(quality.Segment{}, obs, quality.DefaultThresholds(),
		[]quality.EvidenceRef{{Kind: "window-file", Path: "w", SHA256: "abc"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := quality.WriteBaselineEvidence(dir, b, "deadbeef"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestQualityCheckCmdTable(t *testing.T) {
	dir := t.TempDir()
	baseline := writeCheckBaseline(t, filepath.Join(dir, "base"))
	window := writeCheckWindow(t, dir, "window.json")
	goodTh := filepath.Join(dir, "thresholds.json")
	if err := os.WriteFile(goodTh, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	badWindow := filepath.Join(dir, "bad-window.json")
	if err := os.WriteFile(badWindow, []byte(`{no es json`), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "no-existe")
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	okOut := filepath.Join(dir, "diag")
	okOutJSON := filepath.Join(dir, "diag-json")

	cases := []struct {
		name    string
		args    []string
		code    int
		outHas  []string
		errHas  []string
		wantErr bool
		check   func(t *testing.T, stdout string)
	}{
		{name: "parse flag desconocido", args: []string{"--nope"}, code: 2, wantErr: true},
		{name: "parse sin args", args: nil, code: 2, errHas: []string{"regulus quality check"}},
		{name: "parse falta window", args: []string{"--baseline", baseline}, code: 2, errHas: []string{"regulus quality check"}},
		{name: "parse falta baseline", args: []string{"--window", window}, code: 2, errHas: []string{"regulus quality check"}},
		{name: "parse posicional extra", args: []string{"--baseline", baseline, "--window", window, "extra"}, code: 2, errHas: []string{"regulus quality check"}},
		{name: "baseline ausente", args: []string{"--baseline", missing, "--window", window}, code: 1, errHas: []string{"quality check:"}},
		{name: "thresholds ilegibles", args: []string{"--baseline", baseline, "--window", window, "--thresholds", missing}, code: 1, errHas: []string{"quality check:"}},
		{name: "window ausente", args: []string{"--baseline", baseline, "--window", missing}, code: 1, errHas: []string{"quality check:"}},
		{name: "window malformada", args: []string{"--baseline", baseline, "--window", badWindow}, code: 1, errHas: []string{"quality check:"}},
		{name: "out no escribible", args: []string{"--baseline", baseline, "--window", window, "--out", filepath.Join(blocker, "sub")}, code: 1, errHas: []string{"quality check:"}},
		{name: "texto ok", args: []string{"--baseline", baseline, "--window", window}, code: 0, outHas: []string{"quality ", "segment"}},
		{
			name: "json ok",
			args: []string{"--baseline", baseline, "--window", window, "--json"},
			code: 0,
			check: func(t *testing.T, stdout string) {
				var d quality.Diagnosis
				if err := json.Unmarshal([]byte(stdout), &d); err != nil {
					t.Fatalf("json debe decodificar el diagnóstico: %v\n%s", err, stdout)
				}
				if d.Version != quality.QualityVersion || d.WindowN != 2 {
					t.Fatalf("diagnóstico inesperado: %+v", d)
				}
			},
		},
		{name: "thresholds ok", args: []string{"--baseline", baseline, "--window", window, "--thresholds", goodTh}, code: 0, outHas: []string{"quality "}},
		{
			name:   "out ok",
			args:   []string{"--baseline", baseline, "--window", window, "--out", okOut},
			code:   0,
			outHas: []string{"quality "},
			check: func(t *testing.T, _ string) {
				for _, f := range []string{"diagnosis.json", "manifest.json"} {
					if _, err := os.Stat(filepath.Join(okOut, f)); err != nil {
						t.Fatalf("out debe escribir %s: %v", f, err)
					}
				}
			},
		},
		{
			name: "json y out ok",
			args: []string{"--baseline", baseline, "--window", window, "--json", "--out", okOutJSON},
			code: 0,
			check: func(t *testing.T, stdout string) {
				var d quality.Diagnosis
				if err := json.Unmarshal([]byte(stdout), &d); err != nil {
					t.Fatalf("json debe decodificar: %v\n%s", err, stdout)
				}
				if _, err := os.Stat(filepath.Join(okOutJSON, "diagnosis.json")); err != nil {
					t.Fatalf("out debe escribir el diagnóstico: %v", err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			io, out, errb := cmdIO(dir)
			if code := io.QualityCheckCmd(tc.args); code != tc.code {
				t.Fatalf("exit = %d, want %d (stderr=%q)", code, tc.code, errb.String())
			}
			for _, w := range tc.outHas {
				if !strings.Contains(out.String(), w) {
					t.Errorf("stdout debe contener %q, got %q", w, out.String())
				}
			}
			for _, w := range tc.errHas {
				if !strings.Contains(errb.String(), w) {
					t.Errorf("stderr debe contener %q, got %q", w, errb.String())
				}
			}
			if tc.wantErr && errb.Len() == 0 {
				t.Error("stderr debe explicar el fallo de parse")
			}
			if tc.check != nil {
				tc.check(t, out.String())
			}
		})
	}
}
