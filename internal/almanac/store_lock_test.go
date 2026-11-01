package almanac

// M2 guardian condition (a): no flock in portable stdlib — the lock is
// a lockfile (O_CREATE|O_EXCL) + tmp+Rename discipline for the files,
// stale-lock stealing for crashed holders.

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestLockSecondWriterRefuses(t *testing.T) {
	root := t.TempDir()
	release, err := acquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := acquireLock(root); !errors.Is(err, ErrStoreLocked) {
		t.Fatalf("second writer must refuse with ErrStoreLocked, got %v", err)
	}
	// Release frees the next writer.
	release()
	release2, err := acquireLock(root)
	if err != nil {
		t.Fatalf("released lock must be acquirable: %v", err)
	}
	release2()
	if _, err := os.Stat(LockPath(root)); !os.IsNotExist(err) {
		t.Fatalf("release must remove the lockfile, stat err = %v", err)
	}
}

func TestLockStaleIsStolen(t *testing.T) {
	root := t.TempDir()
	_, err := acquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crashed holder: keep the file (never released), age it
	// past the bound.
	stale := time.Now().Add(-2 * lockStaleAfter)
	if err := os.Chtimes(LockPath(root), stale, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(LockPath(root)); err != nil {
		t.Fatalf("crash simulation must keep the lockfile: %v", err)
	}
	release2, err := acquireLock(root)
	if err != nil {
		t.Fatalf("stale lock must be stolen, got %v", err)
	}
	defer release2()
	if pid := lockHolderPID(root); pid != os.Getpid() {
		t.Fatalf("stolen lock must carry our pid, got %d", pid)
	}
}

func TestNoFlockInStore(t *testing.T) {
	// Guardian condition (a) pineado: flock no existe en stdlib
	// portable/Windows — el store no debe nombrarlo jamás.
	matches := 0
	for _, f := range []string{"store_lock.go", "vindex.go", "almanac.go", "prune.go", "search_indexed.go"} {
		raw, err := os.ReadFile(f)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range splitLines(string(raw)) {
			trimmed := line
			for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
				trimmed = trimmed[1:]
			}
			if len(trimmed) >= 2 && trimmed[:2] == "//" {
				continue
			}
			if containsFold(line, "flock") || containsFold(line, "syscall.") {
				t.Errorf("%s: portable store must not use flock/syscall (found %q)", f, line)
				matches++
			}
		}
		if matches > 5 {
			t.Fatal("too many violations")
		}
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func containsFold(hay, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			a, b := hay[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
