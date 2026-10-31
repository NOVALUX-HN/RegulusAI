package gates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"regulus-ai/internal/manifest"
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
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, "", err
	}
	runID, err := manifest.NewRunID()
	if err != nil {
		return Result{}, "", err
	}
	max := maxTimeout(cfg)
	allowDestr := cfg.Execution != nil && cfg.Execution.AllowDestructive
	allowEnforced := cfg.Execution != nil && len(cfg.Execution.Allowlist) > 0
	isolation := IsolationHostContained
	if cfg.Execution != nil && cfg.Execution.DockerImage != "" {
		isolation = IsolationDocker
	}
	var specs []manifest.GateSpec
	for _, g := range cfg.Gates {
		rep := manifest.ParseReproducibility(g.Reproducibility)
		if rep == "" {
			return Result{}, "", fmt.Errorf("manifest: gate %q: invalid reproducibility %q", g.Name, g.Reproducibility)
		}
		specs = append(specs, manifest.GateSpec{
			Name:            g.Name,
			Layer:           g.Layer,
			Command:         g.Command,
			TimeoutSec:      g.TimeoutSec,
			Reproducibility: rep,
			Category:        g.Category,
		})
	}
	params := manifest.Params{
		Root:              absRoot,
		Task:              task,
		RunID:             runID,
		RegulusVersion:    version,
		Contract:          contract,
		ConfigBytes:       cfgBytes,
		Gates:             specs,
		MaxTimeoutSec:     max,
		AllowDestructive:  allowDestr,
		AllowlistEnforced: allowEnforced,
		Isolation:         isolation,
	}
	m, err := manifest.Build(ctx, manifestRunnerAdapter{r: r}, params)
	if err != nil {
		return Result{}, "", err
	}
	evidenceDir := filepath.Join(absRoot, ".regulus", "evidence", receipts.Slug(task))
	head := GitHead(ctx, r)
	rForRun := r
	if sh, ok := asShRunner(r); ok {
		merged := append(append([]string(nil), sh.EnvUnset...), func() []string {
			if cfg.Execution != nil {
				return cfg.Execution.EnvUnset
			}
			return nil
		}()...)
		// cwd pin: gates execute in the repo root
		rForRun = ShRunner{Dir: absRoot, EnvUnset: merged}
	}
	res, err := run(ctx, cfg, evidenceDir, task, head, rForRun, runStamp{runID: runID, manifestFile: manifest.FileName(runID)}, profile)
	if err != nil {
		return Result{}, "", err
	}
	var refs []manifest.ArtifactRef
	for _, p := range res.Receipts {
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return res, "", rerr
		}
		sum := sha256.Sum256(data)
		rec, rerr := receipts.ReadFile(p)
		if rerr != nil {
			return res, "", rerr
		}
		refs = append(refs, manifest.ArtifactRef{
			ReceiptID: rec.ID,
			File:      filepath.Base(p),
			SHA256:    hex.EncodeToString(sum[:]),
			Gate:      rec.Gate,
			Status:    rec.Status,
			Reason:    rec.Reason,
		})
	}
	manifest.Attach(&m, refs)
	m.Skipped = append([]string(nil), res.Skipped...)
	// The manifest lives alongside its receipts in the task evidence dir:
	// that locality is what evidence.Verify cross-checks.
	mpath, err := manifest.Write(evidenceDir, m)
	if err != nil {
		return res, "", err
	}
	return res, mpath, nil
}
