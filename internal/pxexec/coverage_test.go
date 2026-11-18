// Cobertura T1 (coverage-80/S1-S4): caminos del executor que la suite
// de contrato no pisa en todos los SO. Todo hermético y portable:
// el hijo es el propio binario de test (TestHelperProcess), los
// shells se resuelven por LookPath con t.Skip justificado si
// faltan, y un BLOCKED del host salta con motivo (cubre CI).
package pxexec

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// resolveShell resuelve un shell por nombre portátil (bare name para
// que shellFlag lo clasifique): pwsh preferido, powershell fallback
// Windows, sh fallback POSIX. Skip justificado si no hay ninguno:
// ejercita passthrough, nunca el núcleo (cubierto sin subproceso).
func resolveShell(t *testing.T) string {
	t.Helper()
	for _, n := range []string{"pwsh", "powershell", "sh"} {
		if _, err := exec.LookPath(n); err == nil {
			return n
		}
	}
	t.Skip("sin shell disponible (pwsh/powershell/sh): passthrough no testeable aquí")
	return ""
}

func shellKind(shell string) string {
	switch strings.ToLower(shell) {
	case "pwsh", "powershell":
		return "pwsh"
	default:
		return "sh"
	}
}

func TestValidateVariants(t *testing.T) {
	cases := []struct {
		name    string
		spec    Spec
		wantErr string // "" = válido
	}{
		{"empty name", Spec{}, "command name"},
		{"blank name", Spec{Name: "   "}, "command name"},
		{"unknown env mode", Spec{Name: "x", EnvMode: "bogus"}, "unknown env mode"},
		{"negative cap", Spec{Name: "x", MaxOutput: -1}, "negative output cap"},
		{"shell sin justify", Spec{Name: "x", Shell: "sh"}, "without a written justification"},
		{"shell blank es no-shell", Spec{Name: "x", Shell: "   "}, ""},
		{"timeout negativo", Spec{Name: "x", Timeout: -time.Second}, "unbounded"},
		{"inherit ok", Spec{Name: "x", EnvMode: "inherit"}, ""},
		{"minimal ok", Spec{Name: "x", EnvMode: "minimal"}, ""},
		{"exact ok", Spec{Name: "x", EnvMode: "exact"}, ""},
		{"shell con justify ok", Spec{Name: "x", Shell: "sh", Justify: "glob de fixtures"}, ""},
		{"skip cortocircuita todo", Spec{EnvMode: "bogus", MaxOutput: -5, Shell: "sh", Timeout: -time.Second, SkipReason: "sin toolchain"}, ""},
	}
	for _, c := range cases {
		err := c.spec.Validate()
		if c.wantErr == "" {
			if err != nil {
				t.Fatalf("%s: válido esperado, got %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Fatalf("%s: error con %q esperado, got %v", c.name, c.wantErr, err)
		}
	}
}

func TestDefaultEnvShape(t *testing.T) {
	env := DefaultEnv()
	allowed := map[string]bool{"SystemRoot": true, "TEMP": true, "TMP": true, "PATH": true, "PATHEXT": true}
	for _, kv := range env {
		eq := strings.Index(kv, "=")
		if eq <= 0 {
			t.Fatalf("entrada malformada: %q", kv)
		}
		if !allowed[kv[:eq]] {
			t.Fatalf("clave inesperada en env mínimo: %q", kv[:eq])
		}
	}
}

func TestRenderEdgeCases(t *testing.T) {
	if got := (Spec{Name: "x"}).Render(); got != "x" {
		t.Fatalf("render sin args: %q", got)
	}
	// Secreto case-insensitive y con espacios recortados.
	s := Spec{Name: "deploy", Args: []string{"token=abc123", "MYKEY=hunter2", "justaword"}, Secrets: []string{"TOKEN", "  mykey  "}}
	out := s.Render()
	if strings.Contains(out, "abc123") || strings.Contains(out, "hunter2") {
		t.Fatalf("secretos deben redactarse: %q", out)
	}
	if !strings.Contains(out, "token=[redacted]") || !strings.Contains(out, "justaword") {
		t.Fatalf("redacción exacta + benigno: %q", out)
	}
	// Sin Secrets, scrubCredentials igual filtra bearer/json.
	s2 := Spec{Name: "call", Args: []string{"bearer abc123", `{"secret": "v"}`}}
	if got := s2.Render(); strings.Contains(got, "abc123") || strings.Contains(got, `"v"`) {
		t.Fatalf("bearer/json deben filtrarse aun sin Secrets: %q", got)
	}
}

func TestShellFlagVariants(t *testing.T) {
	pwsh := []string{"-NoProfile", "-NonInteractive", "-Command"}
	for _, sh := range []string{"pwsh", "PWSH", " pwsh ", "powershell", "PowerShell"} {
		got := strings.Join(shellFlag(sh), " ")
		if got != strings.Join(pwsh, " ") {
			t.Fatalf("shellFlag(%q) = %q", sh, got)
		}
	}
	if got := shellFlag("cmd"); len(got) != 1 || got[0] != "/C" {
		t.Fatalf("shellFlag(cmd) = %q", got)
	}
	if got := shellFlag("CMD"); len(got) != 1 || got[0] != "/C" {
		t.Fatalf("shellFlag(CMD) = %q", got)
	}
	for _, sh := range []string{"sh", "bash", "", "zsh", "  "} {
		got := shellFlag(sh)
		if len(got) != 1 || got[0] != "-c" {
			t.Fatalf("shellFlag(%q) debe ser [-c], got %q", sh, got)
		}
	}
	// Rutas absolutas (p. ej. de LookPath) clasifican por base, no caen a -c.
	for sh, want := range map[string]string{
		`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`: "-NoProfile -NonInteractive -Command",
		"/usr/bin/pwsh": "-NoProfile -NonInteractive -Command",
		"/bin/bash":     "-c",
	} {
		if got := strings.Join(shellFlag(sh), " "); got != want {
			t.Fatalf("shellFlag(%q) = %q, want %q", sh, got, want)
		}
	}
}

func TestBlockTextVectors(t *testing.T) {
	for _, out := range []string{
		"Control de aplicaciones bloqueó este archivo",
		"BLOCKED THIS FILE by policy",
		"Device Guard denegó el lanzamiento",
		"smart app control refusal",
		"falló con 0xC0E90002",
		"  Device   Guard  ",
	} {
		if !blockText(out) {
			t.Fatalf("señal de bloqueo no detectada: %q", out)
		}
	}
	for _, out := range []string{"", "everything ok", "exit 0", "blocking socket ready", "the guard device passed"} {
		if blockText(out) {
			t.Fatalf("falso positivo de bloqueo: %q", out)
		}
	}
}

func TestIsPolicyDenied(t *testing.T) {
	if isPolicyDenied(nil) {
		t.Fatal("nil nunca es denegación de policy")
	}
	if isPolicyDenied(errors.New("boom")) {
		t.Fatal("error genérico no es denegación de policy")
	}
	if !isPolicyDenied(os.ErrPermission) {
		t.Fatal("ErrPermission debe ser denegación de policy")
	}
	if !isPolicyDenied(&os.PathError{Op: "open", Path: "x", Err: os.ErrPermission}) {
		t.Fatal("PathError con permiso debe ser denegación de policy")
	}
	if isPolicyDenied(&os.PathError{Op: "open", Path: "x", Err: os.ErrNotExist}) {
		t.Fatal("ENOENT no es denegación de policy")
	}
}

func TestBlockHintWithAndWithoutFile(t *testing.T) {
	// Binario resoluble: incluye ruta LookPath + query de eventos.
	hint := blockHint(Spec{Name: "go"})
	for _, want := range []string{"file: go", "check: Get-WinEvent"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("hint de go debe contener %q: %q", want, hint)
		}
	}
	// Archivo existente legible: incluye sha256 corto.
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool.exe")
	if err := os.WriteFile(tool, []byte("fake-binary-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	hintFile := blockHint(Spec{Name: tool})
	if !strings.Contains(hintFile, "sha256=") || !strings.Contains(hintFile, "check: Get-WinEvent") {
		t.Fatalf("hint con archivo debe llevar sha256 y query: %q", hintFile)
	}
	// Archivo inexistente: sin sha, con query (nada se excluye ni cambia).
	hintMissing := blockHint(Spec{Name: filepath.Join(dir, "absent-xyz")})
	if strings.Contains(hintMissing, "sha256=") {
		t.Fatalf("hint sin archivo no debe llevar sha: %q", hintMissing)
	}
	if !strings.Contains(hintMissing, "check: Get-WinEvent") {
		t.Fatalf("hint sin archivo debe llevar query: %q", hintMissing)
	}
	// Nombre irresoluble: sin paréntesis LookPath, con query.
	hintBare := blockHint(Spec{Name: "definitely-not-a-binary-xyz"})
	if strings.Contains(hintBare, "(") {
		t.Fatalf("hint irresoluble no debe llevar ruta: %q", hintBare)
	}
}

func TestFirstLineVectors(t *testing.T) {
	cases := map[string]string{
		"a\nb":          "a",
		"  padded  \nx": "padded",
		"single":        "single",
		"":              "",
		"  \n  ":        "",
		"first\r\nnext": "first",
	}
	for in, want := range cases {
		if got := firstLine(in); got != want {
			t.Fatalf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScrubCredentialsExtended(t *testing.T) {
	vectors := map[string]string{
		"Bearer XYZabc123":         "bearer [redacted]",
		`{"password":"hunter2"}`:   `{"password": "[redacted]"}`,
		`{"other": "keep"}`:        `{"other": "keep"}`,
		"token=abc123":             "token=[redacted]",
		"password: hunter2":        "password:[redacted]",
		`secret="quoted value"`:    `secret=[redacted]`,
		"token='abc'":              "token=[redacted]",
		"api_key=AB-12_cd":         "api_key=[redacted]",
		"authorization=tok123":     "authorization=[redacted]",
		"nothing to hide here":     "nothing to hide here",
		"pathname=/usr/bin/foo":    "pathname=/usr/bin/foo",
		"the token pouch is here":  "the token pouch is here",
		"prompt without separator": "prompt without separator",
	}
	for in, want := range vectors {
		if got := scrubCredentials(in); got != want {
			t.Fatalf("scrub(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLimitedWriter(t *testing.T) {
	// Escritura simple dentro del tope.
	var w bytes.Buffer
	lw := &limitedWriter{w: &w, n: 10}
	if n, err := lw.Write([]byte("abcd")); err != nil || n != 4 {
		t.Fatalf("write simple = %d,%v", n, err)
	}
	// Escritura parcial: solo cabe room en el buffer, pero retorna
	// len(p) con nil para que exec no lo lea como fallo de
	// lanzamiento ("short write"). El truncado se reporta aparte.
	if n, err := lw.Write([]byte("0123456789")); err != nil || n != 10 {
		t.Fatalf("write parcial = %d,%v", n, err)
	}
	if w.Len() != 10 || w.String() != "abcd012345" {
		t.Fatalf("buffer = %q", w.String())
	}
	// Lleno: descarta sin fallar, retorna len(p).
	if n, err := lw.Write([]byte("zz")); err != nil || n != 2 {
		t.Fatalf("write lleno = %d,%v", n, err)
	}
	if w.Len() != 10 {
		t.Fatalf("buffer lleno mutó: %q", w.String())
	}
	// Tee con tope propio: primario completo, tee truncado.
	var w2, tee bytes.Buffer
	lw2 := &limitedWriter{w: &w2, n: 100, tee: &tee, tn: 5}
	if _, err := lw2.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	if w2.String() != "0123456789" || tee.String() != "01234" {
		t.Fatalf("tee = %q/%q", w2.String(), tee.String())
	}
	// Tee lleno: no falla el primario.
	if _, err := lw2.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if w2.String() != "0123456789ab" || tee.String() != "01234" {
		t.Fatalf("tee lleno = %q/%q", w2.String(), tee.String())
	}
	// Tee nil: sin pánico.
	lw3 := &limitedWriter{w: &bytes.Buffer{}, n: 4}
	if _, err := lw3.Write([]byte("hey")); err != nil {
		t.Fatal(err)
	}
}

func TestRunDefaultTimeoutUsesDefault(t *testing.T) {
	// Timeout 0 = DefaultTimeout: el branch por defecto también corre.
	r := Run(context.Background(), Spec{Name: "go", Args: []string{"version"}})
	if r.Outcome != OutcomePass || r.Exit != 0 {
		t.Fatalf("timeout 0 debe usar el default y pasar: %+v", r)
	}
	if r.Mechanism != "go-native" || r.PID <= 0 || r.Truncated {
		t.Fatalf("mecanismo/PID/truncate: %+v", r)
	}
	if !strings.Contains(r.Combined, "go version") {
		t.Fatalf("combined debe llevar stdout+stderr: %q", r.Combined)
	}
}

func TestRunInvalidSpecs(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		want string
	}{
		{"vacío", Spec{}, "command name"},
		{"shell sin justify", Spec{Name: "echo", Shell: "sh"}, "justification"},
		{"cap negativo", Spec{Name: "go", MaxOutput: -1}, "output cap"},
		{"timeout negativo", Spec{Name: "go", Timeout: -time.Second}, "unbounded"},
		{"env mode malo", Spec{Name: "go", EnvMode: "bogus"}, "env mode"},
	}
	for _, c := range cases {
		r := Run(context.Background(), c.spec)
		if r.Outcome != OutcomeNotRun || r.ErrClass != ErrLaunch || r.Exit != -1 || r.Mechanism != "none" {
			t.Fatalf("%s: NOT_RUN/launch esperado, got %+v", c.name, r)
		}
		if !strings.Contains(r.Detail, c.want) {
			t.Fatalf("%s: detail con %q, got %q", c.name, c.want, r.Detail)
		}
	}
}

func TestRunDirAndCombined(t *testing.T) {
	dir := t.TempDir()
	r := Run(context.Background(), Spec{Name: "go", Args: []string{"version"}, Dir: dir, Timeout: 30 * time.Second})
	if r.Outcome != OutcomePass || r.Duration < 0 {
		t.Fatalf("Dir válido debe pasar: %+v", r)
	}
	if !strings.Contains(r.Combined, "go version") || r.Truncated {
		t.Fatalf("combined sin truncar: %+v", r)
	}
}

func TestRunEnvModes(t *testing.T) {
	const probe = "PXEXEC_COVER_PROBE"
	for _, c := range []struct {
		name string
		mode string
		env  []string
		want string
	}{
		{"inherit suma Env", "inherit", []string{probe + "=inherit-ok"}, "inherit-ok"},
		{"minimal sobre DefaultEnv", "minimal", []string{probe + "=minimal-ok"}, "minimal-ok"},
		{"exact solo Env", "exact", append(DefaultEnv(), probe+"=exact-ok"), "exact-ok"},
	} {
		s := helperSpec(t, "env", c.env, func(s *Spec) { s.EnvMode = c.mode; s.Timeout = 30 * time.Second })
		r := Run(context.Background(), s)
		if r.Outcome == OutcomeBlocked {
			t.Skip("host bloquea hijos; cubierto en CI")
		}
		if r.Outcome != OutcomePass {
			t.Fatalf("%s: debe pasar: %+v", c.name, r)
		}
		if !strings.Contains(r.Stdout, c.want) {
			t.Fatalf("%s: stdout debe llevar %q: %q", c.name, c.want, r.Stdout)
		}
	}
}

func TestRunWithStdin(t *testing.T) {
	// Portable: go ignora stdin pero el branch Stdin se ejecuta.
	r := Run(context.Background(), Spec{Name: "go", Args: []string{"version"}, Stdin: []byte("hello-stdin"), Timeout: 30 * time.Second})
	if r.Outcome != OutcomePass {
		t.Fatalf("stdin con go version debe pasar: %+v", r)
	}
	r2 := Run(context.Background(), helperSpec(t, "echo", nil, func(s *Spec) { s.Stdin = []byte("hello-stdin"); s.Timeout = 30 * time.Second }))
	if r2.Outcome == OutcomeBlocked {
		t.Skip("host bloquea hijos; cubierto en CI")
	}
	if r2.Outcome != OutcomePass || !strings.Contains(r2.Stdout, "hello-stdin") {
		t.Fatalf("eco debe devolver stdin: %+v", r2)
	}
}

func TestRunTruncation(t *testing.T) {
	// El helper vuelca 32K: con tope 64 hay truncado reportado.
	s := helperSpec(t, "big", nil, func(s *Spec) { s.MaxOutput = 64; s.Timeout = 60 * time.Second })
	r := Run(context.Background(), s)
	if r.Outcome == OutcomeBlocked {
		t.Skip("host bloquea hijos; cubierto en CI")
	}
	if r.Outcome != OutcomePass || !r.Truncated {
		t.Fatalf("salida grande debe truncarse: %+v", r)
	}
	if !strings.Contains(r.Stdout, TruncateNotice) || !strings.Contains(r.Stderr, TruncateNotice) {
		t.Fatalf("aviso en ambos streams: %+v", r)
	}
}

func TestRunTimeoutExpires(t *testing.T) {
	s := helperSpec(t, "sleep", nil, func(s *Spec) { s.Timeout = 500 * time.Millisecond })
	r := Run(context.Background(), s)
	if r.Outcome != OutcomeNotRun || r.ErrClass != ErrTimeout {
		t.Fatalf("expirado debe ser NOT_RUN/timeout: %+v", r)
	}
	if !strings.Contains(r.Detail, "timeout") {
		t.Fatalf("detail debe explicar timeout: %q", r.Detail)
	}
}

func TestRunShellMechanism(t *testing.T) {
	shell := resolveShell(t)
	var blocked, chatter string
	if shellKind(shell) == "pwsh" {
		blocked = "Write-Error 'device guard simulated'; exit 1"
		chatter = "Write-Output 'device guard (simulated log line)'"
	} else {
		blocked = "echo device guard simulated >&2; exit 1"
		chatter = "echo 'device guard (simulated log line)'"
	}
	rb := Run(context.Background(), Spec{Shell: shell, Justify: "cobertura: passthrough con causa", Name: blocked, Timeout: 60 * time.Second})
	if rb.Outcome != OutcomeBlocked || rb.ErrClass != ErrPolicy {
		t.Fatalf("señal policy vía shell debe ser BLOCKED: %+v", rb)
	}
	if rb.Mechanism != "shell:"+shell || !strings.Contains(rb.BlockHint, "Get-WinEvent") {
		t.Fatalf("mecanismo/hint: %+v", rb)
	}
	rp := Run(context.Background(), Spec{Shell: shell, Justify: "cobertura: passthrough con causa", Name: chatter, Timeout: 60 * time.Second})
	if rp.Outcome != OutcomePass {
		t.Fatalf("exit 0 con palabras policy sigue PASS: %+v", rp)
	}
}
