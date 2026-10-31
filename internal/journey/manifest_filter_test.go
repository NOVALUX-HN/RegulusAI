package journey

import (
	"path/filepath"
	"strings"
)

// withoutManifests drops candidate-manifest index files from evidence globs:
// the manifest shares the task dir but is not a receipt.
func withoutManifests(paths []string) []string {
	var out []string
	for _, p := range paths {
		if strings.HasPrefix(filepath.Base(p), "manifest-") {
			continue
		}
		out = append(out, p)
	}
	return out
}
