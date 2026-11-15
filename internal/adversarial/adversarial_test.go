package adversarial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
)

// catalogEntry is one versioned adversarial scenario (NOV-41).
type catalogEntry struct {
	ID        string `json:"id"`
	Component string `json:"component"`
	Risk      string `json:"risk"`
	Scenario  string `json:"scenario"`
	Seed      string `json:"seed"`
	Expected  string `json:"expected"`
}

type catalog struct {
	Version int            `json:"version"`
	Entries []catalogEntry `json:"entries"`
}

func loadCatalog(t *testing.T) catalog {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog.json: %v", err)
	}
	var c catalog
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("parse catalog.json: %v", err)
	}
	if c.Version != 1 {
		t.Fatalf("catalog version must be pinned to 1, got %d", c.Version)
	}
	if len(c.Entries) < 6 || len(c.Entries) > 8 {
		t.Fatalf("catalog must hold 6-8 entries, got %d", len(c.Entries))
	}
	seen := map[string]bool{}
	for _, e := range c.Entries {
		if e.ID == "" || e.Component == "" || e.Risk == "" || e.Scenario == "" || e.Seed == "" || e.Expected == "" {
			t.Fatalf("catalog entry must fill every field, got %+v", e)
		}
		if seen[e.ID] {
			t.Fatalf("duplicate catalog id %q", e.ID)
		}
		seen[e.ID] = true
		info, err := os.Stat(corpusPath(t, e.Seed))
		if err != nil || info.Size() == 0 {
			t.Fatalf("catalog entry %q needs a non-empty corpus seed %q", e.ID, e.Seed)
		}
	}
	return c
}

// corpusPath anchors a corpus seed to this test file, never to the process
// cwd (the workspace runner executes compiled test binaries from the repo
// root while `go test` runs them from the package dir).
func corpusPath(t *testing.T, seed string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(thisFile), "corpus", seed)
}

// seedText returns the raw corpus seed for one entry.
func seedText(t *testing.T, e catalogEntry) string {
	t.Helper()
	data, err := os.ReadFile(corpusPath(t, e.Seed))
	if err != nil {
		t.Fatalf("read seed %q: %v", e.Seed, err)
	}
	return string(data)
}

// seedLines returns the non-empty trimmed lines of one corpus seed.
func seedLines(t *testing.T, e catalogEntry) []string {
	t.Helper()
	var out []string
	for _, ln := range strings.Split(seedText(t, e), "\n") {
		if s := strings.TrimSpace(ln); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatalf("seed %q holds no scenario lines", e.Seed)
	}
	return out
}

// stubRunner is a deterministic hermetic CmdRunner: no shell, no network.
type stubRunner struct {
	calls []string
	run   func(cmd string) (stdout, stderr string, code int)
}

func (s *stubRunner) Run(_ context.Context, command string) (string, string, int, error) {
	s.calls = append(s.calls, command)
	so, se, code := s.run(command)
	return so, se, code, nil
}

func okStub() *stubRunner {
	return &stubRunner{run: func(cmd string) (string, string, int) { return cmd + "\n", "", 0 }}
}

// TestAdversarialCatalog runs every catalog entry against the real code
// with its fixed seed: hostile input must be blocked/contained, never
// executed nor corrupting. Deterministic, hermetic (no shell, no net).
func TestAdversarialCatalog(t *testing.T) {
	c := loadCatalog(t)
	for _, e := range c.Entries {
		t.Run(e.ID, func(t *testing.T) {
			switch e.ID {
			case "framing-evasion-upper":
				input := strings.TrimSpace(seedText(t, e))
				if got := gates.Classify(input); got != gates.ClassificationDestructive {
					t.Fatalf("uppercase framing must classify destructive, got %q", got)
				}
				if gates.AllowedByAllowlist(input, []string{"echo"}) {
					t.Fatalf("uppercase framing must fail the allowlist")
				}
			case "framing-evasion-unicode":
				input := strings.TrimSpace(seedText(t, e))
				if got := gates.Classify(input); got != gates.ClassificationStandard {
					t.Fatalf("homoglyph input documents a classifier miss, got %q", got)
				}
				if gates.AllowedByAllowlist(input, []string{"echo"}) {
					t.Fatalf("homoglyph framing must be refused by the allowlist")
				}
			case "traversal-slug":
				root := t.TempDir()
				for _, task := range seedLines(t, e) {
					if _, err := evidence.TaskDirSafe(root, task); err == nil {
						t.Fatalf("task %q must be refused by the evidence jail", task)
					} else if !strings.Contains(err.Error(), "evidence: task escapes evidence dir") {
						t.Fatalf("task %q refusal must name the jail, got %v", task, err)
					}
				}
			case "gate-subshell":
				for _, line := range seedLines(t, e) {
					if gates.AllowedByAllowlist(line, []string{"echo"}) {
						t.Fatalf("subshell gate %q must be refused by the allowlist", line)
					}
					if strings.Contains(line, "$(") {
						if got := gates.Classify(line); got != gates.ClassificationDestructive {
							t.Fatalf("$(...) gate %q must classify destructive, got %q", line, got)
						}
					}
				}
			case "receipt-stdout-hostile":
				pattern := strings.TrimSpace(seedText(t, e))
				hostile := "\x1b[2J\x1b]0;pwned\a" + strings.Repeat(pattern+"\n", 512)
				if len(hostile) <= receipts.MaxCapture {
					t.Fatalf("hostile seed must exceed the capture cap")
				}
				stored, sha := receipts.Capture(hostile)
				sum := sha256.Sum256([]byte(hostile))
				if sha != hex.EncodeToString(sum[:]) {
					t.Fatalf("capture must pin the full output sha")
				}
				if !receipts.IsTruncated(stored) {
					t.Fatalf("oversized hostile stdout must be truncated")
				}
				dir := t.TempDir()
				se, esha := receipts.Capture("")
				path, err := receipts.Write(dir, receipts.Receipt{
					Task: "adv/R1", Gate: "hostile", Command: "echo",
					ExitCode: 0, Verdict: receipts.VerdictPass,
					StartedAt: time.Now().UTC(),
					Stdout:    stored, StdoutSHA: sha,
					Stderr: se, StderrSHA: esha,
				})
				if err != nil {
					t.Fatalf("write hostile receipt: %v", err)
				}
				back, err := receipts.ReadFile(path)
				if err != nil {
					t.Fatalf("read hostile receipt: %v", err)
				}
				if back.Stdout != stored || back.StdoutSHA != sha {
					t.Fatalf("hostile receipt must round-trip byte-identical")
				}
				if back.StdoutStoredSHA != receipts.StoredSHA(stored) {
					t.Fatalf("stored-text pin must verify")
				}
			case "jsonl-corrupto":
				root := t.TempDir()
				task := "adv/J1"
				dir := filepath.Join(root, ".regulus", "evidence", receipts.Slug(task))
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 2; i++ {
					if _, err := receipts.Write(dir, receipts.Receipt{
						Task: task, Gate: fmt.Sprintf("ok%d", i), Command: "echo ok",
						ExitCode: 0, Verdict: receipts.VerdictPass,
						StartedAt: time.Now().UTC().Add(time.Duration(i) * time.Second),
					}); err != nil {
						t.Fatal(err)
					}
				}
				corrupt := strings.TrimSpace(seedText(t, e))
				if err := os.WriteFile(filepath.Join(dir, "ghost-0.json"), []byte(corrupt), 0o644); err != nil {
					t.Fatal(err)
				}
				got, skipped, err := evidence.ListSkipping(root, task)
				if err != nil {
					t.Fatalf("tolerant list: %v", err)
				}
				if len(got) != 2 || skipped != 1 {
					t.Fatalf("tolerant list must keep 2 healthy and count 1 corrupt, got %d+%d", len(got), skipped)
				}
				if _, err := evidence.List(root, task); err == nil {
					t.Fatalf("strict list must abort on the corrupt receipt")
				}
			case "batch-falla-parcial":
				cmds := seedLines(t, e)
				if len(cmds) != 3 {
					t.Fatalf("batch seed must stage ok/fail/never, got %d lines", len(cmds))
				}
				var gs []gates.Gate
				for i, cmd := range cmds {
					gs = append(gs, gates.Gate{Name: fmt.Sprintf("b%d", i+1), Layer: i + 1, Command: cmd})
				}
				stub := &stubRunner{run: func(cmd string) (string, string, int) {
					if cmd == "exit 1" {
						return "", "boom\n", 1
					}
					return cmd + "\n", "", 0
				}}
				res, err := gates.Run(context.Background(), gates.Config{Gates: gs}, t.TempDir(), "adv/B1", "", stub)
				if err != nil {
					t.Fatalf("batch run: %v", err)
				}
				if !res.Failed || res.FailedGate != "b2" {
					t.Fatalf("batch must stop at b2, got %+v", res)
				}
				if res.Executed != 2 || res.Passed != 1 {
					t.Fatalf("batch must record honest partial 1/2, got %+v", res)
				}
				if len(stub.calls) != 2 || stub.calls[1] != "exit 1" {
					t.Fatalf("batch must never execute past the failure, calls=%q", stub.calls)
				}
				if len(res.Receipts) != 2 {
					t.Fatalf("batch must leave exactly 2 receipts, got %d", len(res.Receipts))
				}
				verdicts := map[string]int{}
				for _, p := range res.Receipts {
					rec, err := receipts.ReadFile(p)
					if err != nil {
						t.Fatal(err)
					}
					verdicts[rec.Verdict]++
				}
				if verdicts[receipts.VerdictPass] != 1 || verdicts[receipts.VerdictFail] != 1 {
					t.Fatalf("batch receipts must be 1 pass + 1 fail, got %v", verdicts)
				}
			case "timeout-cero":
				lines := seedLines(t, e)
				if len(lines) != 2 {
					t.Fatalf("timeout seed must hold zero+huge, got %q", lines)
				}
				var timeouts []int
				for _, ln := range lines {
					v, err := strconv.Atoi(strings.TrimSpace(ln))
					if err != nil {
						t.Fatalf("timeout seed line %q is not an int: %v", ln, err)
					}
					timeouts = append(timeouts, v)
				}
				if timeouts[0] != 0 || timeouts[1] <= gates.DefaultMaxTimeoutSec {
					t.Fatalf("timeout seed must stage 0 and >max, got %v", timeouts)
				}
				cfg := gates.Config{Gates: []gates.Gate{
					{Name: "t0", Layer: 1, Command: "echo ok", TimeoutSec: timeouts[0]},
					{Name: "tbig", Layer: 2, Command: "echo ok", TimeoutSec: timeouts[1]},
				}}
				res, err := gates.Run(context.Background(), cfg, t.TempDir(), "adv/T0", "", okStub())
				if err != nil {
					t.Fatalf("timeout run: %v", err)
				}
				if res.Failed || res.Passed != 2 {
					t.Fatalf("clamped timeouts must still pass, got %+v", res)
				}
				if res.Outcomes[0].TimeoutSec != gates.DefaultTimeoutSec {
					t.Fatalf("timeout 0 must clamp to default %d, got %d", gates.DefaultTimeoutSec, res.Outcomes[0].TimeoutSec)
				}
				if res.Outcomes[1].TimeoutSec != gates.DefaultMaxTimeoutSec {
					t.Fatalf("huge timeout must clamp to max %d, got %d", gates.DefaultMaxTimeoutSec, res.Outcomes[1].TimeoutSec)
				}
			default:
				t.Fatalf("catalog entry %q has no scenario driver", e.ID)
			}
		})
	}
}
