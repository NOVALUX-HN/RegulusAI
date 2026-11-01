package almanac

// Portable store lock (M2 guardian condition (a)): the Almanac is a
// single-writer WAL (the CLI), but Save and Reindex can still race two
// processes (two terminals, a hook plus a hand-run command). `flock`
// does not exist in portable stdlib (and means nothing on Windows), so
// writers serialize on a lockfile instead: exclusive create
// (`O_CREATE|O_EXCL`, atomic on POSIX and Windows) plus stale-lock
// stealing by mtime. No `x/sys`, no cgo, zero-deps intact.
//
// Pins/relations/status stay on the pre-M2 single-writer convention
// (one CLI process appends one line with O_APPEND); the lock protects
// the WAL writers (Save, Import, ApplyPrune) and the derived index
// (Reindex) — those are the writers M2 serializes.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrStoreLocked reports a second writer holding the Almanac lock:
// retry the command instead of running two writers at once.
var ErrStoreLocked = errors.New("almanac: store is locked by another writer — retry the command")

// lockStaleAfter bounds a crashed holder: a lockfile older than this is
// stolen, not waited on. A successful Save/Reindex holds the lock for
// milliseconds; 60s means the holder died mid-write.
const lockStaleAfter = 60 * time.Second

// StoreLockWait bounds how long a writer waits for a live lock before
// refusing with ErrStoreLocked (poll every lockRetryStep). Waiting (not
// failing fast) preserves the pre-M2 availability: two concurrent CLI
// invocations serialize instead of one losing its write. Tests set it
// to zero for immediate-refusal assertions.
var StoreLockWait = 5 * time.Second

// lockRetryStep is the poll interval while waiting for a live lock.
const lockRetryStep = 20 * time.Millisecond

// LockPath returns the lockfile path under a project root.
func LockPath(root string) string {
	return filepath.Join(root, ".regulus", "almanac", ".lock")
}

// withStoreLock runs fn holding the portable lockfile, releasing it
// afterwards (even on failure). A live lock waits up to StoreLockWait
// (writers serialize); a stale lock is stolen; an unclaimed lock after
// the budget refuses with ErrStoreLocked.
func withStoreLock(root string, fn func() error) error {
	deadline := time.Now().Add(StoreLockWait)
	for {
		release, err := acquireLock(root)
		if err == nil {
			defer release()
			return fn()
		}
		if !errors.Is(err, ErrStoreLocked) {
			return err
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(lockRetryStep)
	}
}

// acquireLock creates the lockfile exclusively and returns its
// release. On EEXIST it steals the lock only when the file is older
// than lockStaleAfter; otherwise it refuses with ErrStoreLocked.
func acquireLock(root string) (func(), error) {
	path := LockPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// Best-effort owner tag: the pid helps a human `ls` the holder.
	// A failed write still holds the lock (the file exists) — the tag
	// is DATA, never a trust decision; staleness comes from mtime.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
		_ = f.Close()
		return func() { _ = os.Remove(path) }, nil
	}
	if !os.IsExist(err) {
		return nil, err
	}
	st, statErr := os.Stat(path)
	if statErr != nil {
		// Lost a race with a release: retry once as a fresh acquire.
		if os.IsNotExist(statErr) {
			return acquireLock(root)
		}
		return nil, statErr
	}
	if time.Since(st.ModTime()) < lockStaleAfter {
		return nil, fmt.Errorf("%w (holder since %s)", ErrStoreLocked, st.ModTime().UTC().Format(time.RFC3339))
	}
	// Stale: the holder died mid-write. Remove and take it; a lost
	// race with another stealer surfaces as EEXIST → ErrStoreLocked.
	if rmErr := os.Remove(path); rmErr != nil && !os.IsNotExist(rmErr) {
		return nil, rmErr
	}
	f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("%w (lost a steal race)", ErrStoreLocked)
		}
		return nil, err
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	_ = f.Close()
	return func() { _ = os.Remove(path) }, nil
}

// lockHolderPID parses the owner tag for diagnostics (0 = unreadable).
// The pid is never a trust decision — see acquireLock.
func lockHolderPID(root string) int {
	raw, err := os.ReadFile(LockPath(root))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0
	}
	return n
}
