package fsx

// NOV-85 Paso 65: single place for operator-data file handling under
// .regulus/. Everything here is private by default (0700/0600); shared
// or versioned artifacts live outside .regulus and keep their modes.
// On Windows the mode bits are best-effort only (ACLs rule); the
// O_EXCL lockfile IS atomic there, so LockDir works everywhere.
// Documented in the FixPermissions warning, never silently claimed.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// lockStaleAfter mirrors the almanac store lock: a lockfile older
	// than this is considered orphaned and may be stolen.
	lockStaleAfter = 60 * time.Second
	// lockWait bounds acquisition: writers fail fast instead of
	// queueing behind a stuck holder.
	lockWait = 5 * time.Second
	lockPoll = 20 * time.Millisecond
	lockName = ".lock"
	lockPerm = 0o600
	dirPerm  = 0o700
	filePerm = 0o600
)

// ErrWindowsACL reports that private modes are unenforceable on
// Windows (ACLs, not mode bits). Callers print the guidance and
// continue: locking still works, privacy does not.
var ErrWindowsACL = errors.New("fsx: private modes unenforceable on Windows (ACLs rule); use a per-user profile and encrypted disk")

// MkdirPrivate creates dir (and parents) with 0700.
func MkdirPrivate(dir string) error {
	return os.MkdirAll(dir, dirPerm)
}

// WriteFilePrivate creates parents (0700) and writes with 0600.
func WriteFilePrivate(path string, data []byte) error {
	if err := MkdirPrivate(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, data, filePerm)
}

// AppendLine atomically appends one line to path: private dir,
// inter-process lock for the append, 0600 file. The caller marshals
// first; only complete lines ever hit disk.
func AppendLine(path string, line []byte) error {
	if err := MkdirPrivate(filepath.Dir(path)); err != nil {
		return err
	}
	release, err := LockDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer release()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(line); err != nil {
		return err
	}
	return f.Close()
}

// LockDir acquires an inter-process lock for dir via dir/.lock
// (O_CREATE|O_EXCL, pid-tagged, stale after lockStaleAfter, waits up
// to lockWait polling every lockPoll). Portable: no flock/x/sys.
// On Windows a racy exclusive create can surface as access denied
// rather than exists; that is treated as contention, not failure.
func LockDir(dir string) (release func(), err error) {
	if err := MkdirPrivate(dir); err != nil {
		return nil, err
	}
	lp := filepath.Join(dir, lockName)
	tag := fmt.Sprintf("%d", os.Getpid())
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(lp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, lockPerm)
		if err == nil {
			_, _ = fmt.Fprintln(f, tag)
			_ = f.Close()
			return func() { _ = os.Remove(lp) }, nil
		}
		if !os.IsExist(err) && !(runtime.GOOS == "windows" && os.IsPermission(err)) {
			return nil, err
		}
		if fi, serr := os.Stat(lp); serr == nil && time.Since(fi.ModTime()) > lockStaleAfter {
			_ = os.Remove(lp)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("fsx: lock busy: %s", lp)
		}
		time.Sleep(lockPoll)
	}
}

// permissionWant returns the private mode for an entry: 0700 for dirs,
// 0600 for files. Single decision point extracted from FixPermissions
// to keep the orchestrator under the CRAP budget.
func permissionWant(isDir bool) os.FileMode {
	if isDir {
		return os.FileMode(dirPerm)
	}
	return os.FileMode(filePerm)
}

// fixSingle stats one walked entry and fixes its mode when needed.
// Returns the slash-rel path to record, or "" when nothing changed.
// Never aborts the walk: every error is a skip, as before.
func fixSingle(p string, d os.DirEntry, root string) string {
	want := permissionWant(d.IsDir())
	fi, err := d.Info()
	if err != nil {
		return ""
	}
	if fi.Mode().Perm() == want {
		return ""
	}
	if err := os.Chmod(p, want); err != nil {
		return ""
	}
	rel, _ := filepath.Rel(root, p)
	return filepath.ToSlash(rel)
}

// slashTrim normalizes recorded rel paths (walk roots may prefix "./").
// Split out so the FixPermissions orchestrator stays under CRAP budget.
func slashTrim(paths []string) []string {
	for i, s := range paths {
		paths[i] = strings.TrimPrefix(s, "./")
	}
	return paths
}

// FixPermissions walks root/.regulus setting dirs to 0700 and files to
// 0600, returning the paths it changed. On Windows it returns
// ErrWindowsACL (callers print the guidance); locking still applies.
func FixPermissions(root string) ([]string, error) {
	if runtime.GOOS == "windows" {
		return nil, ErrWindowsACL
	}
	base := filepath.Join(root, ".regulus")
	var fixed []string
	err := filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if rel := fixSingle(p, d, root); rel != "" {
			fixed = append(fixed, rel)
		}
		return nil
	})
	if err != nil {
		return fixed, err
	}
	return slashTrim(fixed), nil
}
