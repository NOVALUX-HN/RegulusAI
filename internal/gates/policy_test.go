package gates

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"regulus-ai/internal/receipts"
)

// RED: classification must exist and handle false positives.

func TestClassifyStandardVsDestructive(t *testing.T) {
	if got := Classify("rm -rf ./build"); got != "standard" {
		t.Fatalf("rm -rf ./build = %q, want standard", got)
	}
	if got := Classify(`echo "rm -rf /"`); got != "standard" {
		t.Fatalf("quoted rm -rf / = %q, want standard", got)
	}
	if got := Classify("rm -rf /"); got != "destructive" {
		t.Fatalf("rm -rf / = %q, want destructive", got)
	}
}

func TestClassifyDestructiveVariants(t *testing.T) {
	destructive := []string{
		"rm -rf /*",
		"rm -rf / --no-preserve-root",
		"rm -rf ~",
		"rm -rf $HOME",
		"rm -rf /root",
		"rm -rf /home",
		"rm -rf /etc",
		"rm -rf /usr",
		"rm -rf /var",
		"rd /s C:\\",
		"mkfs.ext4 /dev/sda1",
		"dd if=/dev/zero of=/dev/sda",
		":(){ :|:& };:",
		"echo hi > /dev/sda",
		"chmod -R 777 /",
		"chown -R root /",
		"sudo rm -rf /",
		"rm -rf /; echo hi",
	}
	for _, cmd := range destructive {
		if got := Classify(cmd); got != "destructive" {
			t.Fatalf("Classify(%q) = %q, want destructive", cmd, got)
		}
	}
}

func TestClassifyPrivileged(t *testing.T) {
	priv := []string{
		"sudo apt update",
		"doas pkg_add foo",
		"runas /user:admin cmd",
		"chmod 755 file",
		"chown root file",
		"setfacl -m u:alice:r file",
		"icacls file /grant alice:F",
		"diskpart /s script.txt",
		"format D: /FS:NTFS",
		"bcdedit /enum",
		"reg add HKLM\\Software /v x",
		"reg delete HKLM\\Software /v x",
	}
	for _, cmd := range priv {
		if got := Classify(cmd); got != "privileged" {
			t.Fatalf("Classify(%q) = %q, want privileged", cmd, got)
		}
	}
	// chmod without -R on non-root stays privileged, not destructive
	if got := Classify("chmod -R 755 ./build"); got != "privileged" {
		t.Fatalf("chmod -R ./build = %q, want privileged", got)
	}
}

func TestAllowlistAllowAndRefuse(t *testing.T) {
	cfg := Config{
		Gates: []Gate{{Name: "a", Layer: 1, Command: "go test ./..."}},
		Execution: &ExecutionConfig{
			Allowlist: []string{"go"},
		},
	}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("allowlisted go must pass: %+v", res)
	}
	cfg2 := Config{
		Gates: []Gate{{Name: "a", Layer: 1, Command: "go test ./..."}},
		Execution: &ExecutionConfig{
			Allowlist: []string{"npm"},
		},
	}
	r2 := &fakeRunner{}
	res2, err := Run(context.Background(), cfg2, t.TempDir(), "t", "", r2)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Failed {
		t.Fatal("non-allowlisted command must be refused")
	}
	if len(r2.calls) != 0 {
		t.Fatalf("refused gate must execute NOTHING, calls=%v", r2.calls)
	}
	if res2.Status != "BLOCKED" || res2.Reason != "policy_violation" {
		t.Fatalf("refused result = %q/%q, want BLOCKED/policy_violation", res2.Status, res2.Reason)
	}
	rec := readReceiptFile(t, res2.Receipts[0])
	if rec.Status != "BLOCKED" || rec.Reason != "policy_violation" {
		t.Fatalf("refused receipt = %q/%q", rec.Status, rec.Reason)
	}
}

func TestDestructiveRefusedBlocksAndStops(t *testing.T) {
	cfg := Config{
		Gates: []Gate{
			{Name: "bad", Layer: 1, Command: "rm -rf /"},
			{Name: "next", Layer: 2, Command: "echo ok"},
		},
	}
	r := &fakeRunner{}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.FailedGate != "bad" {
		t.Fatalf("destructive must stop at bad: %+v", res)
	}
	if res.Status != "BLOCKED" || res.Reason != "policy_violation" {
		t.Fatalf("status=%q reason=%q", res.Status, res.Reason)
	}
	for _, c := range r.calls {
		if c == "echo ok" {
			t.Fatal("no gate may run after a refusal")
		}
	}
	if len(r.calls) != 0 {
		t.Fatalf("refused gate must not execute, calls=%v", r.calls)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.Status != "BLOCKED" || rec.Reason != "policy_violation" {
		t.Fatalf("receipt status=%q reason=%q", rec.Status, rec.Reason)
	}
	if rec.Verdict != receipts.VerdictFail {
		t.Fatalf("legacy verdict must stay fail, got %q", rec.Verdict)
	}
}

func TestTimeoutClampKillsAroundOneSecond(t *testing.T) {
	cfg := Config{
		Gates:     []Gate{{Name: "slow", Layer: 1, Command: "sleep-cmd", TimeoutSec: 100}},
		Execution: &ExecutionConfig{MaxTimeoutSec: 1},
	}
	r := &fakeRunner{results: map[string]fakeResult{
		"sleep-cmd": {delay: 2 * time.Second},
	}}
	start := time.Now()
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if !res.Failed {
		t.Fatal("clamped timeout must fail")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("clamped to 1s but took %v", elapsed)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.Status != "BLOCKED" || rec.Reason != "infrastructure_failure" {
		t.Fatalf("timeout receipt = %q/%q", rec.Status, rec.Reason)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0].TimeoutSec != 1 {
		t.Fatalf("applied timeout must be 1, got %+v", res.Outcomes)
	}
}

func TestEffectiveTimeoutDefaultsAndClamp(t *testing.T) {
	if got := effectiveTimeout(0, 0); got != 600 {
		t.Fatalf("default timeout = %d, want 600", got)
	}
	if got := effectiveTimeout(100, 1); got != 1 {
		t.Fatalf("clamp 100 to max 1 = %d", got)
	}
	if got := effectiveTimeout(-5, 3600); got != 600 {
		t.Fatalf("negative gate timeout defaults to 600, got %d", got)
	}
}

func TestValidateNewFields(t *testing.T) {
	badRepro := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Reproducibility: "bogus"}}}
	if err := validate(badRepro); err == nil {
		t.Fatal("invalid reproducibility must fail validation")
	}
	badIso := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Isolate: "vm"}}}
	if err := validate(badIso); err == nil {
		t.Fatal("invalid isolate must fail validation")
	}
	badAllow := Config{
		Gates:     []Gate{{Name: "a", Layer: 1, Command: "x"}},
		Execution: &ExecutionConfig{Allowlist: []string{""}},
	}
	if err := validate(badAllow); err == nil {
		t.Fatal("empty allowlist entry must fail validation")
	}
	ok := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "x", Reproducibility: "exact", Isolate: "docker"}}}
	if err := validate(ok); err != nil {
		t.Fatalf("valid new fields must pass: %v", err)
	}
}

func TestShRunnerDirHonoredAndMissing(t *testing.T) {
	dir := t.TempDir()
	r := ShRunner{Dir: dir}
	out, _, code, err := r.Run(context.Background(), "pwd")
	if err != nil || code != 0 {
		t.Fatalf("pwd in dir: out=%q code=%d err=%v", out, code, err)
	}
	bad := ShRunner{Dir: filepath.Join(t.TempDir(), "does-not-exist-xyz")}
	_, _, _, err = bad.Run(context.Background(), "echo hi")
	if err == nil {
		t.Fatal("nonexistent Dir must error")
	}
}

func TestShRunnerEnvUnsetFilter(t *testing.T) {
	t.Setenv("REGULUS_TEST_UNSET_ME", "secret-value")
	r := ShRunner{EnvUnset: []string{"REGULUS_TEST_UNSET_ME"}}
	out, _, code, err := r.Run(context.Background(), "echo ${REGULUS_TEST_UNSET_ME:-absent}")
	if err != nil || code != 0 {
		t.Fatalf("env unset run: out=%q code=%d err=%v", out, code, err)
	}
	if strings.Contains(out, "secret-value") {
		t.Fatalf("EnvUnset must filter the var, got %q", out)
	}
	// zero value keeps current behavior
	r2 := ShRunner{}
	out2, _, _, err := r2.Run(context.Background(), "echo ${REGULUS_TEST_UNSET_ME:-absent}")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out2, "secret-value") {
		t.Fatalf("zero-value ShRunner must keep environ, got %q", out2)
	}
}

func TestDockerFallbackNote(t *testing.T) {
	old := dockerProbe
	dockerProbe = func(ctx context.Context) bool { return false }
	defer func() { dockerProbe = old }()
	cfg := Config{
		Gates:     []Gate{{Name: "a", Layer: 1, Command: "echo hi", Isolate: "docker"}},
		Execution: &ExecutionConfig{DockerImage: "alpine:latest"},
	}
	r := ShRunner{Dir: t.TempDir()}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("host fallback must still pass echo hi: %+v", res)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.Isolation != "host-contained" {
		t.Fatalf("fallback isolation = %q, want host-contained", rec.Isolation)
	}
	if !strings.Contains(rec.Stderr, "isolation: docker requested but unavailable") || !strings.Contains(rec.Stderr, "ran host-contained") {
		t.Fatalf("fallback stderr must carry the note, got %q", rec.Stderr)
	}
}

func TestNonShRunnerIsolationUnknownAndCPUZero(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "ok"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.Isolation != "unknown" {
		t.Fatalf("fake runner isolation = %q, want unknown", rec.Isolation)
	}
	if rec.CPUTimeMS != 0 {
		t.Fatalf("fake runner cpu = %d, want 0", rec.CPUTimeMS)
	}
}

func TestParallelRunDistinctTasks(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task := filepath.Join("p", string(rune('a'+i)))
			cfg := Config{Gates: []Gate{{Name: "a", Layer: 1, Command: "ok"}}}
			_, err := Run(context.Background(), cfg, dir, task, "", &fakeRunner{})
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

// RED: unbounded gate output must be killed at the cap, not buffered forever.
func TestCapWriterBoundsAndKillsOnce(t *testing.T) {
	kills := 0
	w := &capWriter{limit: 10, kill: func() { kills++ }}
	if n, err := w.Write([]byte("0123456789ABCDEF")); n != 16 || err != nil {
		t.Fatalf("capped writer must accept-and-discard, got n=%d err=%v", n, err)
	}
	if !w.overflow || w.buf.Len() != 10 || w.total != 16 {
		t.Fatalf("writer must record overflow with bounded buffer, got %+v len=%d total=%d", w.overflow, w.buf.Len(), w.total)
	}
	if _, err := w.Write([]byte("more")); err != nil {
		t.Fatalf("further writes must still succeed, got %v", err)
	}
	if kills != 1 {
		t.Fatalf("kill must fire exactly once, got %d", kills)
	}
}

func TestRunShellCapEnforcesTinyBudget(t *testing.T) {
	name, args := shellName("echo hello"), shellArgs("echo hello")
	cmd := exec.CommandContext(context.Background(), name, args...)
	// "hello\n" (6 bytes) against a 3-byte cap must overflow.
	out, _, code, _, err := runShellCap(cmd, context.Background(), name, 3)
	if err == nil {
		t.Fatal("output beyond the cap must fail")
	}
	var capErr *outputCapError
	if !errors.As(err, &capErr) {
		t.Fatalf("want *outputCapError, got %T (%v)", err, err)
	}
	if code != -1 {
		t.Fatalf("capped run must not carry an exit code, got %d", code)
	}
	if len(out) > 3 {
		t.Fatalf("returned output must respect the cap, got %q", out)
	}
}

func TestRunMapsCapErrorToBlockedInfrastructure(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "flood", Layer: 1, Command: "flood-cmd"}}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"flood-cmd": {err: &outputCapError{bytes: MaxOutputBytes + 1}},
	}}
	dir := t.TempDir()
	res, err := Run(context.Background(), cfg, dir, "t", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("capped gate must fail the gauntlet")
	}
	if len(res.Outcomes) != 1 {
		t.Fatalf("want one outcome, got %v", res.Outcomes)
	}
	o := res.Outcomes[0]
	if o.Status != "BLOCKED" || o.Reason != "infrastructure_failure" {
		t.Fatalf("want BLOCKED/infrastructure_failure, got %s/%s", o.Status, o.Reason)
	}
	if res.Status != "BLOCKED" || res.Reason != "infrastructure_failure" {
		t.Fatalf("overall result must carry the block, got %s/%s", res.Status, res.Reason)
	}
	rec, err := receipts.ReadFile(res.Receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "BLOCKED" || rec.Reason != "infrastructure_failure" || rec.Verdict != receipts.VerdictFail {
		t.Fatalf("receipt must record the block with legacy fail, got %+v", rec)
	}
}
