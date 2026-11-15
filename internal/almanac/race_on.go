//go:build race

package almanac

// raceDetectorOn reports whether the race detector instruments this
// build. The -race flag sets the race build tag automatically, so the
// wall-clock perf verdicts (meaningless under ~10x instrumentation
// slowdown plus shared CI vCPUs) can stand down while the searches
// still execute for race coverage.
const raceDetectorOn = true
