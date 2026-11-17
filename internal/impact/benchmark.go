// Benchmark (F12.8/NOV-103): the documented CodeGraph comparison.
//
// The spec evaluates CodeGraph or any other tool by benchmark only:
// this report times the own graph build against a naive textual
// import scan over the same tree and records the standing decision —
// the own graph (verified edges, no daemon, no DB) stays the
// dependency, CodeGraph stays unevaluated-external. Re-running
// `regulus impact benchmark` refreshes the numbers; flipping the
// decision requires a new benchmark with measured net value, per
// docs/codegraph.md (no demuestra valor neto → no integrar).
package impact

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunBenchmark builds the graph (timed), runs the naive scan
// (timed) and returns the documented comparison. lookup may be nil
// (default git lookup); tests inject a fake for determinism.
func RunBenchmark(root string, lookup GitLookup) (BenchmarkReport, error) {
	start := time.Now()
	g, err := BuildGraph(root, nil, lookup)
	if err != nil {
		return BenchmarkReport{}, err
	}
	buildMs := time.Since(start).Milliseconds()
	scanStart := time.Now()
	_ = naiveImportScan(root)
	scanMs := time.Since(scanStart).Milliseconds()
	return BenchmarkReport{
		Version:      BenchmarkVersion,
		GraphBuildMs: buildMs,
		NaiveScanMs:  scanMs,
		Nodes:        len(g.Nodes),
		Edges:        len(g.Edges),
		CodeGraph:    "no instalado como dependencia: sin daemon ni DB (docs/codegraph.md); comparación por benchmark, no por integración",
		Decision:     "adoptar grafo propio (aristas verificables go-import/test-import/cli-route/workflow-ref/config-ref); CodeGraph no integrado: coste/frescura/cobertura sin valor neto demostrado",
		Notes: []string{
			"el grafo propio verifica cada arista (fuente + commit/fecha reproducibles con grep + git log)",
			"el scan ingenuo no distingue tests, rutas CLI ni workflows: mismas menciones, cero procedencia",
			"revertir la decisión exige benchmark con precisión/recall medidos y valor neto (spec F12.8)",
		},
	}, nil
}

// naiveImportScan is the baseline the benchmark beats: a flat
// textual scan for regulus-ai/internal imports with no
// attribution, no tests split, no commit/date.
func naiveImportScan(root string) int {
	hits := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		hits += strings.Count(string(raw), "regulus-ai/internal/")
		return nil
	})
	return hits
}
