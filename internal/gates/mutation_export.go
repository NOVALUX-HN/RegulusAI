package gates

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/executil"
	"regulus-ai/internal/verdict"
)

// Mutation export notes: the receipt must indicate whether gremlins ran on
// a clean HEAD export (hermetic, no untracked bloat) or fell back in-situ.
// Notes travel on the existing stderr channel via appendGateNote so the
// receipt schema is unchanged.
const (
	MutationNoteCleanExport = "mutation: clean export via git archive HEAD (hermetic, no untracked files)"
	MutationNoteInSitu      = "warning: mutation without clean export (not a git checkout)"
)

// isMutationGate reports whether a gate is the mutation layer: the gate
// named "mutation", or any gate whose command runs `gremlins unleash`.
// Name match is case-insensitive; command match checks the first token
// (basename, .exe-tolerant via firstToken) plus an `unleash` argument.
func isMutationGate(g Gate) bool {
	if strings.EqualFold(strings.TrimSpace(g.Name), "mutation") {
		return true
	}
	fields := strings.Fields(g.Command)
	if len(fields) == 0 {
		return false
	}
	if !strings.EqualFold(firstToken(g.Command), "gremlins") {
		return false
	}
	for _, f := range fields[1:] {
		if strings.Trim(f, `"'`) == "unleash" {
			return true
		}
	}
	return strings.Contains(g.Command, "unleash")
}

// mutationSrcDir resolves the checkout the export is taken from: the
// runner's pinned directory, or the process cwd when unset.
func mutationSrcDir(sh ShRunner) string {
	if strings.TrimSpace(sh.Dir) != "" {
		return sh.Dir
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		return wd
	}
	return "."
}

// exportCleanTree exports HEAD (tracked files only, no untracked bloat such
// as node_modules) into a fresh temp dir via `git archive HEAD` (tar format)
// extracted in pure Go (archive/tar, no POSIX tar/mktemp). Portable:
// os.MkdirTemp + os/exec + HideWindow. Returns the tmp dir and its cleanup
// (defer RemoveAll). Any failure (not a git checkout, git absent, empty
// archive, untar error) returns an error so the caller falls back in-situ.
func exportCleanTree(ctx context.Context, srcDir string) (string, func(), error) {
	noop := func() {}
	// Bound the export well inside the gate budget: HEAD archives finish in
	// seconds; a hung git must not eat the mutation timeout.
	ectx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ectx, "git", "archive", "HEAD")
	if srcDir != "" {
		cmd.Dir = srcDir
	}
	executil.HideWindow(cmd)
	var tarBuf bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &tarBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(errBuf.String())
		if detail != "" {
			return "", noop, fmt.Errorf("git archive HEAD: %w: %s", err, detail)
		}
		return "", noop, fmt.Errorf("git archive HEAD: %w", err)
	}
	if tarBuf.Len() == 0 {
		return "", noop, fmt.Errorf("git archive HEAD: empty archive")
	}
	tmp, err := os.MkdirTemp("", "regulus-mutation-*")
	if err != nil {
		return "", noop, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	if err := untar(tarBuf.Bytes(), tmp); err != nil {
		cleanup()
		return "", noop, err
	}
	return tmp, cleanup, nil
}

// untar extracts a tar archive into dest. The loop stays trivial; each
// entry dispatches to extractTarEntry so every function keeps a small
// complexity (the crap gate caps CRAP at 30).
func untar(data []byte, dest string) error {
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := extractTarEntry(dest, tr, hdr); err != nil {
			return err
		}
	}
}

// extractTarEntry materializes one tar header under dest. Empty and dot
// names skip; anything escaping dest is an error so the caller falls back
// instead of writing outside tmp; unknown types skip.
func extractTarEntry(dest string, tr *tar.Reader, hdr *tar.Header) error {
	name := filepath.FromSlash(hdr.Name)
	if name == "" || name == "." {
		return nil
	}
	target := filepath.Join(dest, name)
	if !withinDir(dest, target) {
		return fmt.Errorf("tar entry escapes destination: %q", hdr.Name)
	}
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0o755)
	case tar.TypeReg, tar.TypeRegA:
		return writeTarFile(target, tr, hdr)
	case tar.TypeSymlink, tar.TypeLink:
		return linkTarEntry(dest, target, hdr)
	default:
		return nil
	}
}

// withinDir reports whether target stays inside dest (no abs, no ..).
func withinDir(dest, target string) bool {
	rel, err := filepath.Rel(dest, target)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

// writeTarFile creates parents and streams one regular entry to disk,
// keeping its mode bits on a best-effort chmod.
func writeTarFile(target string, tr *tar.Reader, hdr *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, tr)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	_ = os.Chmod(target, hdr.FileInfo().Mode())
	return nil
}

// linkTarEntry recreates a symlink/hardlink entry. Platforms that refuse
// symlinks (Windows without privilege) skip instead of failing the export.
func linkTarEntry(dest, target string, hdr *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	_ = os.Remove(target)
	linkTarget := hdr.Linkname
	if hdr.Typeflag == tar.TypeLink {
		linkTarget = filepath.Join(dest, filepath.FromSlash(linkTarget))
	}
	if err := os.Symlink(linkTarget, target); err != nil {
		return nil
	}
	return nil
}

// execMutationSh runs the mutation gate hermetically: HEAD is exported to a
// fresh tmp dir and the gate command runs there (cmd.Dir = tmp), so
// untracked bloat such as node_modules never reaches gremlins. On any
// export failure (not a git checkout, git absent) it falls back in-situ and
// records the in-situ warning. Either way the receipt stderr carries the
// mode note via appendGateNote (no schema change) and the verdict still
// comes from efficacy/exit facts. Tmp is always removed.
func execMutationSh(ctx context.Context, g Gate, sh ShRunner, timeout int, dockerImage string, execEnvUnset []string) execResult {
	mergedEnv := append(append([]string(nil), sh.EnvUnset...), execEnvUnset...)
	srcDir := mutationSrcDir(sh)
	gctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	start := time.Now().UTC()
	tmp, cleanup, exportErr := exportCleanTree(gctx, srcDir)
	var so, se string
	var c int
	var cpu int64
	var iso string
	var rerr error
	var modeNote string
	if exportErr == nil {
		defer cleanup()
		shTmp := ShRunner{Dir: tmp, EnvUnset: mergedEnv}
		so, se, c, cpu, iso, rerr = shTmp.runWithStats(gctx, g.Command, tmp, dockerImage, g.Isolate)
		modeNote = MutationNoteCleanExport
	} else {
		shIn := ShRunner{Dir: sh.Dir, EnvUnset: mergedEnv}
		so, se, c, cpu, iso, rerr = shIn.runWithStats(gctx, g.Command, sh.Dir, dockerImage, g.Isolate)
		modeNote = MutationNoteInSitu
	}
	dur := time.Since(start)
	cancel()
	out := execResult{stdout: so, stderr: se, code: c, cpuMs: cpu, isolation: iso, rerr: rerr, start: start, dur: dur, exit: c}
	if rerr != nil {
		if out.exit == 0 {
			out.exit = -1
		}
		out.stderr = appendGateNote(out.stderr, "gate could not run: "+rerr.Error())
		out.facts = runnerFacts(gctx, rerr)
	} else if c != 0 {
		out.facts = verdict.Facts{Executed: true, ExitCode: c}
	} else {
		out.facts = verdict.Facts{Executed: true, ExitCode: 0}
	}
	out.stderr = appendGateNote(out.stderr, modeNote)
	return out
}
