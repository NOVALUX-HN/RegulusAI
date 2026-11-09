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

// startTaskRun resolves the repo root and mints the run id.
func startTaskRun(root string) (string, string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	runID, err := manifest.NewRunID()
	if err != nil {
		return "", "", err
	}
	return absRoot, runID, nil
}

// buildGateSpecs validates reproducibility and pins each gate's spec.
func buildGateSpecs(cfg Config) ([]manifest.GateSpec, error) {
	var specs []manifest.GateSpec
	for _, g := range cfg.Gates {
		rep := manifest.ParseReproducibility(g.Reproducibility)
		if rep == "" {
			return nil, fmt.Errorf("manifest: gate %q: invalid reproducibility %q", g.Name, g.Reproducibility)
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
	return specs, nil
}

// buildManifestParams assembles the manifest inputs for one task run.
func buildManifestParams(absRoot, task, runID, version, contract string, cfgBytes []byte, cfg Config, specs []manifest.GateSpec) manifest.Params {
	isolation := IsolationHostContained
	if cfg.Execution != nil && cfg.Execution.DockerImage != "" {
		isolation = IsolationDocker
	}
	return manifest.Params{
		Root:              absRoot,
		Task:              task,
		RunID:             runID,
		RegulusVersion:    version,
		Contract:          contract,
		ConfigBytes:       cfgBytes,
		Gates:             specs,
		MaxTimeoutSec:     maxTimeout(cfg),
		AllowDestructive:  cfg.Execution != nil && cfg.Execution.AllowDestructive,
		AllowlistEnforced: cfg.Execution != nil && len(cfg.Execution.Allowlist) > 0,
		Isolation:         isolation,
	}
}

// buildTaskManifest validates the gate specs and pins the candidate.
func buildTaskManifest(ctx context.Context, cfg Config, cfgBytes []byte, absRoot, task, runID, version, contract string, r CmdRunner) (manifest.Manifest, error) {
	specs, err := buildGateSpecs(cfg)
	if err != nil {
		return manifest.Manifest{}, err
	}
	params := buildManifestParams(absRoot, task, runID, version, contract, cfgBytes, cfg, specs)
	return manifest.Build(ctx, manifestRunnerAdapter{r: r}, params)
}

// taskGitHead bounds the git HEAD probe (S3 hygiene): the gauntlet
// itself relies on per-gate timeouts, so only the git call gets a
// deadline, never the whole RunTask.
func taskGitHead(ctx context.Context, r CmdRunner) string {
	gctx, cancel := context.WithTimeout(ctx, GitHeadTimeout)
	defer cancel()
	return GitHead(gctx, r)
}

// resolveRunRunner pins gate execution to the repo root for ShRunner.
func resolveRunRunner(cfg Config, absRoot string, r CmdRunner) CmdRunner {
	sh, ok := asShRunner(r)
	if !ok {
		return r
	}
	merged := append(append([]string(nil), sh.EnvUnset...), func() []string {
		if cfg.Execution != nil {
			return cfg.Execution.EnvUnset
		}
		return nil
	}()...)
	// cwd pin: gates execute in the repo root
	return ShRunner{Dir: absRoot, EnvUnset: merged}
}

// executeTaskRun runs the gauntlet with a bounded git probe and a
// root-pinned runner. cfgBytes pins the coverage cache key: an unchanged
// config+HEAD reuses the existing coverprofile instead of recomputing it.
func executeTaskRun(ctx context.Context, cfg Config, cfgBytes []byte, absRoot, task, runID, profile string, r CmdRunner, evidenceDir string) (Result, error) {
	head := taskGitHead(ctx, r)
	rForRun := resolveRunRunner(cfg, absRoot, r)
	return run(ctx, cfg, cfgBytes, evidenceDir, task, head, rForRun, runStamp{runID: runID, manifestFile: manifest.FileName(runID)}, profile, absRoot)
}

// collectArtifactRefs fingerprints every written receipt for the manifest.
func collectArtifactRefs(receiptPaths []string) ([]manifest.ArtifactRef, error) {
	var refs []manifest.ArtifactRef
	for _, p := range receiptPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		rec, err := receipts.ReadFile(p)
		if err != nil {
			return nil, err
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
	return refs, nil
}

// attachRefs links the written receipts (and skips) to the manifest.
func attachRefs(m *manifest.Manifest, refs []manifest.ArtifactRef, skipped []string) {
	manifest.Attach(m, refs)
	m.Skipped = append([]string(nil), skipped...)
}

// finishTaskManifest attaches the receipts and writes the manifest
// alongside them in the task evidence dir (evidence.Verify locality).
func finishTaskManifest(m *manifest.Manifest, res Result, evidenceDir string) (Result, string, error) {
	refs, err := collectArtifactRefs(res.Receipts)
	if err != nil {
		return res, "", err
	}
	attachRefs(m, refs, res.Skipped)
	mpath, err := manifest.Write(evidenceDir, *m)
	if err != nil {
		return res, "", err
	}
	return res, mpath, nil
}
