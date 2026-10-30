package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// VersionLine compares the stamped binary version against the project's
// package.json and returns the one-line verdict for `regulus doctor`.
// A match (or nothing to compare against) is informational; anything
// else is a WARNING naming the rebuild that fixes it — a stale binary
// silently runs old gates, tools and hooks (the F2 lesson).
func VersionLine(dir, binaryVersion string) string {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return fmt.Sprintf("version:    %s (no package.json — nothing to compare)", binaryVersion)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Version == "" {
		return fmt.Sprintf("version:    %s (package.json has no version — nothing to compare)", binaryVersion)
	}
	if binaryVersion == pkg.Version {
		return fmt.Sprintf("version:    %s (matches package.json)", binaryVersion)
	}
	if binaryVersion == "dev" {
		return fmt.Sprintf("version:    WARNING — unstamped dev build (package.json says %s): rebuild with npm run build (scripts/build.bash stamps bin/regulus)", pkg.Version)
	}
	return fmt.Sprintf("version:    WARNING — binary says %s but package.json says %s: rebuild with npm run build (scripts/build.bash stamps bin/regulus)", binaryVersion, pkg.Version)
}
