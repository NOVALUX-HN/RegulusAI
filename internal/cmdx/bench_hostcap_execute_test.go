// Host journey execution cover (CRAP paydown sin cambiar
// comportamiento): helpers mkwork+fixture vs runSteps+classify con
// JourneyStep fake y journeyBin inyectable. Portable: t.TempDir, sin
// red, sin shell, sin binarios reales.
package cmdx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

func TestSetupJourneyWork(t *testing.T) {
	t.Run("ok materializes the fixture", func(t *testing.T) {
		work, err := setupJourneyWork(os.MkdirTemp, bench.JourneyFixture)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		defer os.RemoveAll(work)
		if _, err := os.Stat(filepath.Join(work, "charts", "demo.md")); err != nil {
			t.Fatalf("fixture chart must exist: %v", err)
		}
	})
	t.Run("mktemp refusal", func(t *testing.T) {
		if _, err := setupJourneyWork(
			func(string, string) (string, error) { return "", errors.New("no temp") },
			bench.JourneyFixture,
		); err == nil || !strings.Contains(err.Error(), "no temp") {
			t.Fatalf("mktemp failure must surface, got %v", err)
		}
	})
	t.Run("fixture refusal cleans the workdir", func(t *testing.T) {
		var created string
		mk := func(_, _ string) (string, error) {
			dir := t.TempDir()
			created = dir
			return dir, nil
		}
		fixture := func(string) error { return errors.New("no fixture") }
		if _, err := setupJourneyWork(mk, fixture); err == nil {
			t.Fatal("fixture failure must refuse")
		}
		if _, err := os.Stat(created); !os.IsNotExist(err) {
			t.Fatalf("failed fixture must remove the workdir, stat = %v", err)
		}
	})
}

func fakeJourneyStep(name string, calls *int, err error) bench.JourneyStep {
	return bench.JourneyStep{
		Name: name,
		Run: func(context.Context, string) (string, error) {
			*calls++
			if err != nil {
				return "detail-" + name, err
			}
			return "ok-" + name, nil
		},
	}
}

func TestRunJourneySteps(t *testing.T) {
	ctx := context.Background()
	t.Run("all green", func(t *testing.T) {
		var calls int
		steps := []bench.JourneyStep{
			fakeJourneyStep("install", &calls, nil),
			fakeJourneyStep("doctor", &calls, nil),
		}
		if name, detail, err := runJourneySteps(ctx, t.TempDir(), steps); err != nil || name != "" || detail != "" {
			t.Fatalf("green run = (%q,%q,%v), want empty nil", name, detail, err)
		}
		if calls != 2 {
			t.Fatalf("both steps must run, got %d calls", calls)
		}
	})
	t.Run("first failure stops the journey", func(t *testing.T) {
		var calls int
		boom := errors.New("gate blew up")
		steps := []bench.JourneyStep{
			fakeJourneyStep("install", &calls, nil),
			fakeJourneyStep("gate", &calls, boom),
			fakeJourneyStep("close", &calls, nil),
		}
		name, detail, err := runJourneySteps(ctx, t.TempDir(), steps)
		if err == nil || name != "gate" || detail != "detail-gate" {
			t.Fatalf("failure = (%q,%q,%v), want gate stopped", name, detail, err)
		}
		if calls != 2 {
			t.Fatalf("steps after the failure must not run, got %d calls", calls)
		}
	})
}

func TestApplyJourneyFailureClassify(t *testing.T) {
	t.Run("fail keeps host level", func(t *testing.T) {
		r := bench.HostJourneyOutcome{Host: "h", Level: bench.LevelHost}
		applyJourneyFailure(&r, "gate", "ev", errors.New("boom"))
		if r.Status != bench.AgentFail || r.Level != bench.LevelHost {
			t.Fatalf("plain failure must stay host-tested fail, got %+v", r)
		}
		if !strings.Contains(r.Reason, "gate:") || r.Evidence != "ev" {
			t.Fatalf("reason must name the step and keep evidence, got %+v", r)
		}
	})
	t.Run("quota degrades to contract", func(t *testing.T) {
		r := bench.HostJourneyOutcome{Host: "h", Level: bench.LevelHost}
		applyJourneyFailure(&r, "gate", "ev", errors.New("host reports insufficient balance"))
		if r.Status != bench.AgentNotRun || r.Level != bench.LevelContract {
			t.Fatalf("quota must read contract not-run, got %+v", r)
		}
	})
	t.Run("doctor degrades to contract", func(t *testing.T) {
		r := bench.HostJourneyOutcome{Host: "h", Level: bench.LevelHost}
		applyJourneyFailure(&r, "doctor", "ev", errors.New("doctor refused"))
		if r.Status != bench.AgentNotRun || r.Level != bench.LevelContract {
			t.Fatalf("doctor refusal must read contract not-run, got %+v", r)
		}
		if !strings.Contains(r.Reason, "doctor refused") {
			t.Fatalf("doctor reason must stay advisory, got %q", r.Reason)
		}
	})
}

func TestJourneyBinInjectable(t *testing.T) {
	t.Setenv("REGULUS_BIN", "/fake/regulus-bin")
	if got := journeyBin(); got != "/fake/regulus-bin" {
		t.Fatalf("REGULUS_BIN must win, got %q", got)
	}
	// Unset: falls back to the running binary or the PATH name,
	// always non-empty and hermetic (no spawn, only env+exe).
	os.Unsetenv("REGULUS_BIN")
	if got := journeyBin(); got == "" {
		t.Fatal("fallback binary must never be empty")
	}
}
