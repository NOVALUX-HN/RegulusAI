// QualityBaselineCmd: tabla de exit codes con IO{Dir: tmp}.
// Cubre el parse (flag desconocido, requeridos ausentes, posicional
// extra), thresholds ilegibles, window ausente o malformada,
// segment sin match, out no escribible, y los caminos ok en texto,
// json y con thresholds. Los mensajes preservan el prefijo
// "quality baseline:". Portable: solo t.TempDir(), sin shell ni git.
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

func writeBaselineWindow(t *testing.T, dir, name string) string {
	t.Helper()
	raw := `[{"source":"window","run_id":"r1","success":true,"retries":0,"latency_ms":120,"latency_known":true,"tokens_in":10,"tokens_out":20,"tokens_known":true},` +
		`{"source":"window","run_id":"r2","success":true,"retries":1,"latency_ms":140,"latency_known":true,"tokens_in":12,"tokens_out":22,"tokens_known":true}]`
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestQualityBaselineCmdTable(t *testing.T) {
	dir := t.TempDir()
	window := writeBaselineWindow(t, dir, "window.json")
	goodTh := filepath.Join(dir, "thresholds.json")
	if err := os.WriteFile(goodTh, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	badWindow := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badWindow, []byte(`{no es json`), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "no-existe")
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	okOut := filepath.Join(dir, "base")
	okOutJSON := filepath.Join(dir, "base-json")

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
		{name: "parse sin args", args: nil, code: 2, errHas: []string{"regulus quality baseline"}},
		{name: "parse falta out", args: []string{"--window", window}, code: 2, errHas: []string{"regulus quality baseline"}},
		{name: "parse falta window", args: []string{"--out", okOut}, code: 2, errHas: []string{"regulus quality baseline"}},
		{name: "parse posicional extra", args: []string{"--window", window, "--out", okOut, "extra"}, code: 2, errHas: []string{"regulus quality baseline"}},
		{name: "thresholds ilegibles", args: []string{"--window", window, "--out", okOut, "--thresholds", missing}, code: 1, errHas: []string{"quality baseline:"}},
		{name: "window ausente", args: []string{"--window", missing, "--out", okOut}, code: 1, errHas: []string{"quality baseline:"}},
		{name: "window malformada", args: []string{"--window", badWindow, "--out", okOut}, code: 1, errHas: []string{"quality baseline:"}},
		{name: "segment sin match", args: []string{"--window", window, "--out", okOut, "--task", "nope"}, code: 1, errHas: []string{"quality baseline:"}},
		{name: "out no escribible", args: []string{"--window", window, "--out", filepath.Join(blocker, "sub")}, code: 1, errHas: []string{"quality baseline:"}},
		{name: "texto ok", args: []string{"--window", window, "--out", okOut}, code: 0, outHas: []string{"quality: baseline"}},
		{
			name: "json ok",
			args: []string{"--window", window, "--out", okOutJSON, "--json"},
			code: 0,
			check: func(t *testing.T, stdout string) {
				var b quality.Baseline
				if err := json.Unmarshal([]byte(stdout), &b); err != nil {
					t.Fatalf("json debe decodificar el baseline: %v\n%s", err, stdout)
				}
				if b.N != 2 {
					t.Fatalf("baseline n = %d, want 2", b.N)
				}
			},
		},
		{name: "thresholds ok", args: []string{"--window", window, "--out", filepath.Join(dir, "base-th"), "--thresholds", goodTh}, code: 0, outHas: []string{"quality: baseline"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			io, out, errb := cmdIO(dir)
			if code := io.QualityBaselineCmd(tc.args); code != tc.code {
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

func TestBuildQualityBaselinePure(t *testing.T) {
	dir := t.TempDir()
	window := writeBaselineWindow(t, dir, "w.json")
	th := quality.DefaultThresholds()
	b, err := buildQualityBaseline(quality.Segment{}, window, "", th, time.Now())
	if err != nil {
		t.Fatalf("build puro: %v", err)
	}
	if b.N != 2 || b.Stats.SuccessRate != 1 {
		t.Fatalf("baseline puro inesperado: %+v", b)
	}
	if _, err := buildQualityBaseline(quality.Segment{Task: "nope"}, window, "", th, time.Now()); err == nil {
		t.Fatal("segment sin match debe fallar")
	}
}
