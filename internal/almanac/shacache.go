package almanac

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
)

// Process-local source-sha cache (almanac-elite T1): every freshness
// check used to ReadFile the whole observations.jsonl and hash it —
// per query, several times (hybrid + indexed + vectors). The CLI is
// stateless across spawns, but a single command performs multiple
// freshness checks over the same unchanged log, so a stat-gated
// in-process cache turns repeat checks into one stat, not one full
// read+hash. Keyed by log path; entry valid only while size+mtime
// match. A torn read (log rewritten between stat and read) caches
// under the old stat and misses next call — fail-safe, never stale.
type shaCacheEntry struct {
	size          int64
	mtimeUnixNano int64
	sha           string
}

var (
	shaCacheMu sync.Mutex
	shaCache   = map[string]shaCacheEntry{}
)

// cachedSourceSHA returns the hex sha256 of the observation log,
// serving repeat checks on an unchanged file from a stat-validated
// cache entry. A missing log returns the stat error; callers keep
// their existing missing-file behavior.
func cachedSourceSHA(root string) (string, error) {
	path := File(root)
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	size := st.Size()
	mtime := st.ModTime().UnixNano()
	shaCacheMu.Lock()
	if e, ok := shaCache[path]; ok && e.size == size && e.mtimeUnixNano == mtime {
		sha := e.sha
		shaCacheMu.Unlock()
		return sha, nil
	}
	shaCacheMu.Unlock()
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	hexSum := hex.EncodeToString(sum[:])
	shaCacheMu.Lock()
	shaCache[path] = shaCacheEntry{size: size, mtimeUnixNano: mtime, sha: hexSum}
	shaCacheMu.Unlock()
	return hexSum, nil
}

// clearSHACache drops the process-local entries (tests only).
func clearSHACache() {
	shaCacheMu.Lock()
	defer shaCacheMu.Unlock()
	shaCache = map[string]shaCacheEntry{}
}
