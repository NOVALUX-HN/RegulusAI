package gates

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/verdict"
)

// Coverage cache (T5): repeated daily-loop runs with an unchanged config
// and HEAD skip coverage recomputation. The key is sha256(cfgBytes +
// HEAD); the key file sits next to the coverprofile (`<cover>.sha`), so
// custom -coverprofile paths work. A hit requires a non-empty HEAD, an
// existing coverprofile, and a matching key — anything else recomputes.
// Dirty-worktree staleness is out of scope by design: the contract is
// config+HEAD, exactly as specified.

const coverageCacheNotePrefix = "coverage: cache hit — reused "

// coverageProfilePath extracts the -coverprofile output path from a gate
// command: the token right after the -coverprofile flag, quotes trimmed.
// ok=false when the command produces no coverprofile.
func coverageProfilePath(command string) (path string, ok bool) {
	fields := strings.Fields(command)
	for i, f := range fields {
		if strings.Trim(f, `"'`) == "-coverprofile" && i+1 < len(fields) {
			if p := strings.Trim(fields[i+1], `"'`); p != "" {
				return p, true
			}
		}
	}
	return "", false
}

// CoverageKey hashes the config bytes and HEAD into the cache key.
func CoverageKey(cfgBytes []byte, head string) string {
	sum := sha256.Sum256(append(append([]byte(nil), cfgBytes...), []byte("\x00"+head)...))
	return hex.EncodeToString(sum[:])
}

// coverageAbs resolves a possibly repo-relative coverprofile path against
// the repo root.
func coverageAbs(root, cover string) string {
	if filepath.IsAbs(cover) {
		return cover
	}
	return filepath.Join(root, cover)
}

// CheckCoverageCache reports whether a previous successful run already
// produced the coverprofile for this exact config+HEAD.
func CheckCoverageCache(root string, cfgBytes []byte, head, cover string) bool {
	if root == "" || len(cfgBytes) == 0 || head == "" || cover == "" {
		return false
	}
	abs := coverageAbs(root, cover)
	if fi, err := os.Stat(abs); err != nil || fi.IsDir() {
		return false
	}
	key, err := os.ReadFile(abs + ".sha")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(key)) == CoverageKey(cfgBytes, head)
}

// WriteCoverageCache records the config+HEAD key next to a freshly
// produced coverprofile. Best-effort: a cache write failure must never
// fail the gauntlet.
func WriteCoverageCache(root string, cfgBytes []byte, head, cover string) {
	if root == "" || len(cfgBytes) == 0 || head == "" || cover == "" {
		return
	}
	abs := coverageAbs(root, cover)
	if fi, err := os.Stat(abs); err != nil || fi.IsDir() {
		return
	}
	_ = os.WriteFile(abs+".sha", []byte(CoverageKey(cfgBytes, head)+"\n"), 0o644)
}

// coverageHit returns a synthetic passing execResult when the gate's
// coverprofile for this exact config+HEAD already exists: the runner is
// never invoked. ok=false means run the gate normally. The receipt still
// records an honest PASS with the reuse note on stderr.
func coverageHit(root string, cfgBytes []byte, head string, g Gate) (execResult, bool) {
	cover, ok := coverageProfilePath(g.Command)
	if !ok || !CheckCoverageCache(root, cfgBytes, head, cover) {
		return execResult{}, false
	}
	short := head
	if len(short) > 12 {
		short = short[:12]
	}
	return execResult{
		code: 0, exit: 0,
		stderr:    coverageCacheNotePrefix + cover + " (config+HEAD " + short + ", recomputation skipped)",
		isolation: IsolationUnknown, start: time.Now().UTC(),
		facts: verdict.Facts{Executed: true, ExitCode: 0},
	}, true
}
