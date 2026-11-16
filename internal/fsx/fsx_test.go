package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMkdirPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := MkdirPrivate(dir); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o, want 700", fi.Mode().Perm())
	}
}

func TestWriteAndAppendPrivate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "f.jsonl")
	if err := WriteFilePrivate(p, []byte("{\"a\":1}\n")); err != nil {
		t.Fatal(err)
	}
	if err := AppendLine(p, []byte("{\"a\":2}\n")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "{\"a\":1}\n{\"a\":2}\n" {
		t.Fatalf("appended content = %q", raw)
	}
	if runtime.GOOS == "windows" {
		return
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 600", fi.Mode().Perm())
	}
}

func TestLockDirAcquireReleaseReacquire(t *testing.T) {
	dir := t.TempDir()
	rel, err := LockDir(dir)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	rel()
	rel2, err := LockDir(dir)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	rel2()
	if _, err := os.Stat(filepath.Join(dir, ".lock")); !os.IsNotExist(err) {
		t.Fatalf("release must remove the lockfile, stat err = %v", err)
	}
}

func TestLockDirStealsStale(t *testing.T) {
	dir := t.TempDir()
	if err := MkdirPrivate(dir); err != nil {
		t.Fatal(err)
	}
	lp := filepath.Join(dir, ".lock")
	if err := os.WriteFile(lp, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-61 * time.Second)
	if err := os.Chtimes(lp, past, past); err != nil {
		t.Skipf("chtimes unsupported: %v", err)
	}
	rel, err := LockDir(dir)
	if err != nil {
		t.Fatalf("stale lock must be stolen, got %v", err)
	}
	rel()
}

func TestFixPermissions(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, ".regulus", "traces")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(base, "s.jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixed, err := FixPermissions(root)
	if runtime.GOOS == "windows" {
		if err != ErrWindowsACL {
			t.Fatalf("windows must report ACL limitation, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) == 0 {
		t.Fatal("must report at least the fixed paths")
	}
	for _, rel := range fixed {
		if strings.Contains(rel, "..") {
			t.Fatalf("fixed path escapes root: %q", rel)
		}
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 600", fi.Mode().Perm())
	}
}
