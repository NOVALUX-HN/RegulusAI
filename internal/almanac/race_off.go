//go:build !race

package almanac

// raceDetectorOn reports whether the race detector instruments this
// build (see race_on.go).
const raceDetectorOn = false
