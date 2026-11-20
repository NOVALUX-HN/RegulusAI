// Tabla toolVersion (CRAP paydown): bin falso sin stub para el camino
// real de lanzamiento, y stub de toolVersionRun para Outcome!=Pass,
// primera linea, truncado a 120 y vacio. Portable: sin shell ni red.
package bench

import (
	"context"
	"strings"
	"testing"

	"regulus-ai/internal/pxexec"
)

func stubToolVersion(res pxexec.Result) func() {
	old := toolVersionRun
	toolVersionRun = func(context.Context, pxexec.Spec) pxexec.Result { return res }
	return func() { toolVersionRun = old }
}

func TestToolVersionTable(t *testing.T) {
	t.Run("bin falso", func(t *testing.T) {
		if _, err := toolVersion(context.Background(), "regulus-definitivamente-ausente-xyz"); err == nil ||
			!strings.Contains(err.Error(), "probe failed") {
			t.Fatalf("bin ausente debe fallar con probe failed, got %v", err)
		}
	})
	t.Run("exit status", func(t *testing.T) {
		defer stubToolVersion(pxexec.Result{Outcome: pxexec.OutcomeFail, ErrClass: pxexec.ErrFail, Exit: 3})()
		if _, err := toolVersion(context.Background(), "tool"); err == nil ||
			!strings.Contains(err.Error(), "exit status 3") {
			t.Fatalf("ErrFail debe citar el exit, got %v", err)
		}
	})
	t.Run("detalle", func(t *testing.T) {
		defer stubToolVersion(pxexec.Result{Outcome: pxexec.OutcomeNotRun, ErrClass: pxexec.ErrLaunch, Detail: "no se pudo lanzar"})()
		if _, err := toolVersion(context.Background(), "tool"); err == nil ||
			!strings.Contains(err.Error(), "no se pudo lanzar") {
			t.Fatalf("NOT_RUN debe propagar el detalle, got %v", err)
		}
	})
	t.Run("primera linea", func(t *testing.T) {
		defer stubToolVersion(pxexec.Result{Outcome: pxexec.OutcomePass, Combined: "v1.2.3\notra linea\n"})()
		got, err := toolVersion(context.Background(), "tool", "--version")
		if err != nil || got != "v1.2.3" {
			t.Fatalf("debe recortar a la primera linea, got %q, %v", got, err)
		}
	})
	t.Run("truncar", func(t *testing.T) {
		defer stubToolVersion(pxexec.Result{Outcome: pxexec.OutcomePass, Combined: strings.Repeat("v", 130) + "\nsegunda"})()
		got, err := toolVersion(context.Background(), "tool")
		if err != nil || len(got) != 120 {
			t.Fatalf("debe truncar a 120, got len=%d, %v", len(got), err)
		}
	})
	t.Run("vacio", func(t *testing.T) {
		defer stubToolVersion(pxexec.Result{Outcome: pxexec.OutcomePass, Combined: "  \n"})()
		if _, err := toolVersion(context.Background(), "tool"); err == nil ||
			!strings.Contains(err.Error(), "probe empty") {
			t.Fatalf("salida vacia debe fallar con probe empty, got %v", err)
		}
	})
}
