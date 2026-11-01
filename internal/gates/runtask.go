package gates

import (
	"context"
	"path/filepath"

	"regulus-ai/internal/receipts"
)

type manifestRunnerAdapter struct {
	r CmdRunner
}

func (a manifestRunnerAdapter) Run(ctx context.Context, command string) (string, string, int, error) {
	return a.r.Run(ctx, command)
}

// RunTask runs the gauntlet for one task and links the run to a candidate
// manifest. It returns the gauntlet result and the manifest path. The
// manifest is written even when the gauntlet failed (evidence); no Run
// happens when validation fails, so no manifest is written then.
func RunTask(ctx context.Context, cfg Config, cfgBytes []byte, root, task, version, contract, profile string, r CmdRunner) (Result, string, error) {
	absRoot, runID, err := startTaskRun(root)
	if err != nil {
		return Result{}, "", err
	}
	m, err := buildTaskManifest(ctx, cfg, cfgBytes, absRoot, task, runID, version, contract, r)
	if err != nil {
		return Result{}, "", err
	}
	evidenceDir := filepath.Join(absRoot, ".regulus", "evidence", receipts.Slug(task))
	res, err := executeTaskRun(ctx, cfg, absRoot, task, runID, profile, r, evidenceDir)
	if err != nil {
		return Result{}, "", err
	}
	return finishTaskManifest(&m, res, evidenceDir)
}
