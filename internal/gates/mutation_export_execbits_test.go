package gates

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSanitizeTarFileMode pins the S3 mapping: tar exec bits never reach
// disk; the base is 0o644 and restrictive modes are preserved.
func TestSanitizeTarFileMode(t *testing.T) {
	cases := []struct {
		in   os.FileMode
		want os.FileMode
	}{
		{0o755, 0o644},
		{0o777, 0o644},
		{0o644, 0o644},
		{0o600, 0o600},
		{0o666, 0o644},
		{0o444, 0o444},
		{0o700, 0o600},
		{0, 0},
	}
	for _, tc := range cases {
		if got := sanitizeTarFileMode(tc.in); got != tc.want {
			t.Fatalf("sanitizeTarFileMode(%04o) = %04o, want %04o", tc.in, got, tc.want)
		}
	}
	// No output may carry any exec bit, whatever the input perm.
	for perm := 0; perm < 0o1000; perm++ {
		if got := sanitizeTarFileMode(os.FileMode(perm)); got&0o111 != 0 {
			t.Fatalf("sanitizeTarFileMode(%04o) = %04o, carries exec bits", perm, got)
		}
	}
}

// buildTarWithModes archives entries with explicit modes (no git, no disk
// fixtures). Dirs use hdr mode 0755; file modes are per-entry.
func buildTarWithModes(t *testing.T, entries []struct {
	name string
	body string
	mode int64
	flag byte
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, e := range entries {
		flag := e.flag
		if flag == 0 {
			flag = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{Name: e.name, Mode: mode, Size: int64(len(e.body)), Typeflag: flag}
		if flag == tar.TypeDir {
			hdr.Size = 0
		}
		if err := w.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if flag == tar.TypeReg || flag == tar.TypeRegA {
			if _, err := w.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestUntarHeaderModesSanitizeTo0644 proves the S3 mapping through the real
// tar-decoding path on every platform (including Windows, where chmod Perm
// bits are not observable): headers decoded from a synthetic tar with
// 0755/0777/0644 yield 0644 once passed through the same sanitizer
// writeTarFile applies.
func TestUntarHeaderModesSanitizeTo0644(t *testing.T) {
	data := buildTarWithModes(t, []struct {
		name string
		body string
		mode int64
		flag byte
	}{
		{name: "exec755.sh", body: "x", mode: 0o755},
		{name: "exec777.sh", body: "x", mode: 0o777},
		{name: "plain.txt", body: "x", mode: 0o644},
	})
	tr := tar.NewReader(bytes.NewReader(data))
	seen := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if got := sanitizeTarFileMode(hdr.FileInfo().Mode()); got != 0o644 {
			t.Fatalf("%s tar mode %04o sanitizes to %04o, want 0644", hdr.Name, hdr.FileInfo().Mode().Perm(), got)
		}
		seen[hdr.Name] = true
	}
	for _, name := range []string{"exec755.sh", "exec777.sh", "plain.txt"} {
		if !seen[name] {
			t.Fatalf("tar entry %s not decoded", name)
		}
	}
}

// TestUntarStripsExecBits is the S3 scenario: a tar with 0755/0777 entries
// extracts without exec bits (0644 base) and stays readable.
func TestUntarStripsExecBits(t *testing.T) {
	data := buildTarWithModes(t, []struct {
		name string
		body string
		mode int64
		flag byte
	}{
		{name: "exec755.sh", body: "#!/bin/sh\necho hi\n", mode: 0o755},
		{name: "exec777.sh", body: "#!/bin/sh\necho hi\n", mode: 0o777},
		{name: "plain.txt", body: "plain\n", mode: 0o644},
		{name: "sub/", mode: 0o755, flag: tar.TypeDir},
		{name: "sub/nested.txt", body: "nested\n", mode: 0o755},
	})
	dest := t.TempDir()
	if err := untar(data, dest); err != nil {
		t.Fatal(err)
	}
	// Contents survive the extraction.
	for name, want := range map[string]string{
		"exec755.sh":     "#!/bin/sh\necho hi",
		"exec777.sh":     "#!/bin/sh\necho hi",
		"plain.txt":      "plain",
		"sub/nested.txt": "nested",
	} {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
		if err != nil || strings.TrimSpace(string(got)) != want {
			t.Fatalf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
	// Dirs stay traversable: reaching the nested file above already proves
	// it, and the dir itself must exist as a directory.
	fi, err := os.Stat(filepath.Join(dest, "sub"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("sub dir = %v, %v; want directory", fi, err)
	}
	if runtime.GOOS == "windows" {
		// Windows chmod only models read-only: Perm() cannot show the
		// stripped exec bits, so the pure mapping test above is the
		// receipt there; here we only prove content + traversability.
		t.Skip("exec-bit Perm assertions need POSIX chmod")
	}
	for name := range map[string]bool{
		"exec755.sh":     true,
		"exec777.sh":     true,
		"plain.txt":      true,
		"sub/nested.txt": true,
	} {
		fi, err := os.Stat(filepath.Join(dest, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != 0o644 {
			t.Fatalf("%s mode = %04o, want 0644", name, got)
		}
		if got := fi.Mode().Perm(); got&0o111 != 0 {
			t.Fatalf("%s mode = %04o, carries exec bits", name, got)
		}
	}
}

// TestMutationExportStrippedStillRuns proves the second half of S3: with an
// executable bit committed (git archive carries 0755), the export lands
// without exec bits and the mutation gate still runs green.
func TestMutationExportStrippedStillRuns(t *testing.T) {
	repo := initProbeRepo(t, map[string]string{"tracked.txt": "hi\n"})
	script := filepath.Join(repo, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-m", "script")
	// Record the exec bit the way git stores it (100755), portable even
	// when the OS cannot chmod +x.
	runGit(t, repo, "update-index", "--chmod=+x", "run.sh")
	runGit(t, repo, "commit", "-m", "exec bit")
	tmp, cleanup, err := exportCleanTree(context.Background(), repo)
	if err != nil {
		t.Fatalf("export must succeed: %v", err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(tmp, "run.sh")); err != nil {
		t.Fatalf("export must contain run.sh: %v", err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(tmp, "run.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got&0o111 != 0 {
			t.Fatalf("exported run.sh mode = %04o, must carry no exec bits", got)
		}
	}
	cfg := Config{Gates: []Gate{{Name: "mutation", Layer: 1, Command: "echo hello"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "probe/T1", "abc", ShRunner{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed || res.Passed != 1 {
		t.Fatalf("mutation over the stripped export must stay green: %+v", res)
	}
}
