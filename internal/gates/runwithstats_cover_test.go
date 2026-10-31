package gates

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// C6 COVER runWithStats (cx12 cov35 -> >=70%): docker/no-docker, dir,
// envUnset, timeout/cap, exit codes. Comportamiento idéntico, solo cobertura.

func TestRunWithStatsPlainEcho(t *testing.T) {
	s := ShRunner{}
	out, stderr, code, cpu, iso, err := s.runWithStats(context.Background(), "echo hi", "", "", "")
	if err != nil || code != 0 {
		t.Fatalf("echo hi: out=%q code=%d err=%v stderr=%q", out, code, err, stderr)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("echo output = %q, want hi", out)
	}
	if iso != IsolationHostContained {
		t.Fatalf("isolation = %q, want host-contained", iso)
	}
	if cpu < 0 {
		t.Fatalf("cpu = %d", cpu)
	}
}

func TestRunWithStatsExitCodes(t *testing.T) {
	s := ShRunner{}
	_, _, code, _, _, err := s.runWithStats(context.Background(), "exit 3", "", "", "")
	if err != nil || code != 3 {
		t.Fatalf("exit 3: code=%d err=%v", code, err)
	}
	_, _, code, _, iso, err := s.runWithStats(context.Background(), "this-command-does-not-exist-xyz", "", "", "")
	if err != nil {
		t.Fatalf("not-found must complete with code, got err=%v", err)
	}
	if code != 127 && code != 1 {
		t.Fatalf("not-found code = %d, want 127 (sh) or 1 (cmd/pwsh mapped)", code)
	}
	if iso != IsolationHostContained {
		t.Fatalf("isolation = %q", iso)
	}
}

func TestRunWithStatsDir(t *testing.T) {
	dir := t.TempDir()
	s := ShRunner{Dir: dir}
	_, _, code, _, _, err := s.runWithStats(context.Background(), "echo hi", "", "", "")
	if err != nil || code != 0 {
		t.Fatalf("echo in dir: code=%d err=%v", code, err)
	}
	bad := ShRunner{Dir: filepath.Join(t.TempDir(), "does-not-exist-xyz")}
	_, _, _, _, _, err = bad.runWithStats(context.Background(), "echo hi", "", "", "")
	if err == nil {
		t.Fatal("nonexistent Dir must error")
	}
}

func TestRunWithStatsEnvUnset(t *testing.T) {
	t.Setenv("REGULUS_TEST_UNSET_ME", "secret-value")
	dir := t.TempDir()
	probe := "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() {\n\tif v, ok := os.LookupEnv(\"REGULUS_TEST_UNSET_ME\"); ok {\n\t\tfmt.Println(v)\n\t} else {\n\t\tfmt.Println(\"absent\")\n\t}\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "printenv_probe.go"), []byte(probe), 0o644); err != nil {
		t.Fatal(err)
	}
	s := ShRunner{Dir: dir, EnvUnset: []string{"REGULUS_TEST_UNSET_ME"}}
	out, _, code, _, _, err := s.runWithStats(context.Background(), "go run ./printenv_probe.go", "", "", "")
	if err != nil || code != 0 {
		t.Fatalf("probe filtered: out=%q code=%d err=%v", out, code, err)
	}
	if strings.Contains(out, "secret-value") || !strings.Contains(out, "absent") {
		t.Fatalf("EnvUnset must filter, got %q", out)
	}
	s2 := ShRunner{Dir: dir}
	out2, _, code, _, _, err := s2.runWithStats(context.Background(), "go run ./printenv_probe.go", "", "", "")
	if err != nil || code != 0 {
		t.Fatalf("probe kept: out=%q code=%d err=%v", out2, code, err)
	}
	if !strings.Contains(out2, "secret-value") {
		t.Fatalf("zero-value must keep environ, got %q", out2)
	}
}

func TestRunWithStatsTimeout(t *testing.T) {
	s := ShRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, _, code, _, _, err := s.runWithStats(ctx, "sleep 2", "", "", "")
	if err == nil || code != -1 || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout must refuse: code=%d err=%v", code, err)
	}
}

func writeStderrProbe(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prog := "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() {\n\tfmt.Println(\"out\")\n\tfmt.Fprintln(os.Stderr, \"prev\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "stderr_probe.go"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunWithStatsDockerNoImage(t *testing.T) {
	s := ShRunner{Dir: t.TempDir()}
	_, stderr, _, _, iso, err := s.runWithStats(context.Background(), "echo hi", "", "", "docker")
	if err != nil {
		t.Fatalf("docker-no-image echo must still run host fallback, got err=%v", err)
	}
	if iso != IsolationHostContained {
		t.Fatalf("isolation = %q, want host-contained", iso)
	}
	if !strings.Contains(stderr, "no docker image configured") || !strings.Contains(stderr, "ran host-contained") {
		t.Fatalf("fallback stderr must name missing image, got %q", stderr)
	}
}

func TestRunWithStatsDockerNoImageWithStderr(t *testing.T) {
	dir := writeStderrProbe(t)
	s := ShRunner{Dir: dir}
	_, stderr, code, _, iso, err := s.runWithStats(context.Background(), "go run ./stderr_probe.go", "", "", "docker")
	if err != nil || code != 0 {
		t.Fatalf("probe with stderr: code=%d err=%v stderr=%q", code, err, stderr)
	}
	if iso != IsolationHostContained {
		t.Fatalf("isolation = %q", iso)
	}
	if !strings.Contains(stderr, "prev") || !strings.Contains(stderr, "no docker image configured") {
		t.Fatalf("stderr must join prev and note, got %q", stderr)
	}
}

func TestRunWithStatsDockerFallbackEmpty(t *testing.T) {
	old := dockerProbe
	dockerProbe = func(ctx context.Context) bool { return false }
	defer func() { dockerProbe = old }()
	s := ShRunner{Dir: t.TempDir()}
	_, stderr, _, _, iso, err := s.runWithStats(context.Background(), "echo hi", "", "alpine:latest", "docker")
	if err != nil {
		t.Fatalf("docker fallback must run host, got err=%v", err)
	}
	if iso != IsolationHostContained {
		t.Fatalf("isolation = %q", iso)
	}
	if !strings.Contains(stderr, "docker daemon unavailable") || !strings.Contains(stderr, "ran host-contained") {
		t.Fatalf("fallback stderr must carry note, got %q", stderr)
	}
}

func TestRunWithStatsDockerFallbackWithStderr(t *testing.T) {
	old := dockerProbe
	dockerProbe = func(ctx context.Context) bool { return false }
	defer func() { dockerProbe = old }()
	dir := writeStderrProbe(t)
	s := ShRunner{Dir: dir}
	_, stderr, code, _, iso, err := s.runWithStats(context.Background(), "go run ./stderr_probe.go", "", "alpine:latest", "docker")
	if err != nil || code != 0 {
		t.Fatalf("probe fallback: code=%d err=%v stderr=%q", code, err, stderr)
	}
	if iso != IsolationHostContained {
		t.Fatalf("isolation = %q", iso)
	}
	if !strings.Contains(stderr, "prev") || !strings.Contains(stderr, "docker daemon unavailable") {
		t.Fatalf("stderr must join prev and note, got %q", stderr)
	}
}

func TestRunWithStatsDockerProbeTrueRoot(t *testing.T) {
	old := dockerProbe
	dockerProbe = func(ctx context.Context) bool { return true }
	defer func() { dockerProbe = old }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := t.TempDir()
	s := ShRunner{Dir: t.TempDir()}
	_, _, _, _, iso, _ := s.runWithStats(ctx, "echo hi", root, "alpine:latest", "docker")
	if iso != IsolationDocker {
		t.Fatalf("isolation = %q, want docker", iso)
	}
}

func TestRunWithStatsDockerProbeTrueDir(t *testing.T) {
	old := dockerProbe
	dockerProbe = func(ctx context.Context) bool { return true }
	defer func() { dockerProbe = old }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s := ShRunner{Dir: t.TempDir()}
	_, _, _, _, iso, _ := s.runWithStats(ctx, "echo hi", "", "alpine:latest", "docker")
	if iso != IsolationDocker {
		t.Fatalf("isolation = %q, want docker", iso)
	}
}

func TestRunWithStatsDockerProbeTrueWd(t *testing.T) {
	old := dockerProbe
	dockerProbe = func(ctx context.Context) bool { return true }
	defer func() { dockerProbe = old }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s := ShRunner{}
	_, _, _, _, iso, _ := s.runWithStats(ctx, "echo hi", "", "alpine:latest", "docker")
	if iso != IsolationDocker {
		t.Fatalf("isolation = %q, want docker", iso)
	}
}
