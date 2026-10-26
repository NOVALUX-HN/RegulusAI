package gates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/receipts"
)

type fakeResult struct {
	stdout, stderr string
	code           int
	err            error
	delay          time.Duration
}

type fakeRunner struct {
	results map[string]fakeResult
	calls   []string
}

func (f *fakeRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	f.calls = append(f.calls, command)
	res, ok := f.results[command]
	if !ok {
		res = fakeResult{stdout: "ok"}
	}
	if res.delay > 0 {
		select {
		case <-time.After(res.delay):
		case <-ctx.Done():
			return "", "", -1, ctx.Err()
		}
	}
	return res.stdout, res.stderr, res.code, res.err
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "regulus.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "regulus.json"))
	if err == nil || !strings.Contains(err.Error(), "no regulus.json") {
		t.Fatalf("want missing-config error, got %v", err)
	}
}

func TestLoadMalformed(t *testing.T) {
	_, err := Load(writeConfig(t, "{not json"))
	if err == nil || !strings.Contains(err.Error(), "malformed regulus.json") {
		t.Fatalf("want malformed-config error, got %v", err)
	}
}

func TestLoadValidation(t *testing.T) {
	cases := []struct {
		name, json, want string
	}{
		{"no gates", `{"gates":[]}`, "no gates configured"},
		{"duplicate layer", `{"gates":[{"name":"a","layer":1,"command":"x"},{"name":"b","layer":1,"command":"y"}]}`, "duplicate layer"},
		{"zero layer", `{"gates":[{"name":"a","layer":0,"command":"x"}]}`, "layer must be"},
		{"empty name", `{"gates":[{"name":"","layer":1,"command":"x"}]}`, "must not be empty"},
		{"empty command", `{"gates":[{"name":"a","layer":1,"command":""}]}`, "must not be empty"},
		{"duplicate name", `{"gates":[{"name":"a","layer":1,"command":"x"},{"name":"a","layer":2,"command":"y"}]}`, "duplicate gate name"},
		{"bad metric operator", `{"gates":[{"name":"a","layer":1,"command":"x","metric":{"extract":"last-line-float","threshold":80,"operator":"<"}}]}`, "metric operator"},
		{"bad metric extract", `{"gates":[{"name":"a","layer":1,"command":"x","metric":{"extract":"magic","threshold":80,"operator":">="}}]}`, "metric extract"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tc.json))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestRunAllPassInLayerOrder(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "unit-tests", Layer: 4, Command: "cmd-d"},
		{Name: "typecheck", Layer: 1, Command: "cmd-a"},
		{Name: "lint", Layer: 2, Command: "cmd-b"},
		{Name: "build", Layer: 3, Command: "cmd-c"},
	}}
	r := &fakeRunner{}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t/x", "abc", r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Executed != 4 || res.Passed != 4 || res.Failed {
		t.Fatalf("res = %+v", res)
	}
	want := []string{"cmd-a", "cmd-b", "cmd-c", "cmd-d"}
	if len(r.calls) != len(want) {
		t.Fatalf("calls = %v", r.calls)
	}
	for i, c := range want {
		if r.calls[i] != c {
			t.Fatalf("gates must run in ascending layer order, calls = %v", r.calls)
		}
	}
	if len(res.Receipts) != 4 {
		t.Fatalf("receipts = %v", res.Receipts)
	}
}

func TestRunStopsAtFirstFailure(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "typecheck", Layer: 1, Command: "ok-cmd"},
		{Name: "lint", Layer: 2, Command: "fail-cmd"},
		{Name: "build", Layer: 3, Command: "never-cmd"},
	}}
	r := &fakeRunner{results: map[string]fakeResult{
		"fail-cmd": {code: 2, stderr: "boom"},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "abc", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.FailedGate != "lint" || res.Executed != 2 || res.Passed != 1 {
		t.Fatalf("res = %+v", res)
	}
	for _, c := range r.calls {
		if c == "never-cmd" {
			t.Fatal("no gate may run after a failure")
		}
	}
	if len(res.Receipts) != 2 {
		t.Fatalf("receipts = %v", res.Receipts)
	}
}

func TestRunReceiptFieldsAndHashes(t *testing.T) {
	long := strings.Repeat("x", 20*1024)
	cfg := Config{Gates: []Gate{{Name: "unit-tests", Layer: 4, Command: "big-cmd"}}}
	r := &fakeRunner{results: map[string]fakeResult{
		"big-cmd": {stdout: long},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "my-task", "abc123", r)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(res.Receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	var rec receipts.Receipt
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Task != "my-task" || rec.Gate != "unit-tests" || rec.Layer != 4 || rec.Verdict != receipts.VerdictPass || rec.ExitCode != 0 {
		t.Fatalf("receipt = %+v", rec)
	}
	if rec.GitHead != "abc123" {
		t.Fatalf("git head = %q", rec.GitHead)
	}
	if len(rec.Stdout) >= len(long) {
		t.Fatal("long stdout must be truncated in the receipt")
	}
	if !strings.Contains(rec.Stdout, "truncated") {
		t.Fatalf("truncated capture must carry the marker, got %q", rec.Stdout)
	}
	sum := sha256.Sum256([]byte(long))
	if rec.StdoutSHA != hex.EncodeToString(sum[:]) {
		t.Fatal("stdout sha must pin the FULL output")
	}
}

func TestRunGateCouldNotRun(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "typecheck", Layer: 1, Command: "spawn-error"}}}
	r := &fakeRunner{results: map[string]fakeResult{
		"spawn-error": {err: errors.New("sh: not found")},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("a gate that cannot run must fail the gauntlet")
	}
	data, err := os.ReadFile(res.Receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	var rec receipts.Receipt
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Verdict != receipts.VerdictFail || rec.ExitCode != -1 {
		t.Fatalf("receipt = %+v", rec)
	}
	if !strings.Contains(rec.Stderr, "could not run") {
		t.Fatalf("receipt stderr must name the runner failure, got %q", rec.Stderr)
	}
}

func TestRunTimeoutFailsGate(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "slow", Layer: 1, Command: "sleep-cmd", TimeoutSec: 1}}}
	r := &fakeRunner{results: map[string]fakeResult{
		"sleep-cmd": {delay: 1500 * time.Millisecond},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.FailedGate != "slow" {
		t.Fatalf("res = %+v", res)
	}
}

func TestRunAppendOnlyAcrossRuns(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "lint", Layer: 1, Command: "ok"}}}
	dir := t.TempDir()
	res1, err := Run(context.Background(), cfg, dir, "t", "", &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(res1.Receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	res2, err := Run(context.Background(), cfg, dir, "t", "", &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if res1.Receipts[0] == res2.Receipts[0] {
		t.Fatal("a second run must never overwrite the first receipt")
	}
	after, err := os.ReadFile(res1.Receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("the first receipt is immutable")
	}
}

func TestGitHead(t *testing.T) {
	r := &fakeRunner{results: map[string]fakeResult{
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	if got := GitHead(context.Background(), r); got != "abc123" {
		t.Fatalf("git head = %q, want abc123", got)
	}
	rErr := &fakeRunner{results: map[string]fakeResult{
		"git rev-parse --verify HEAD": {err: errors.New("exit 128")},
	}}
	if got := GitHead(context.Background(), rErr); got != "" {
		t.Fatalf("git head outside a repo must be empty, got %q", got)
	}
}

func readReceiptFile(t *testing.T, path string) receipts.Receipt {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rec receipts.Receipt
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestRunMetricFloorPass(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "coverage-floor", Layer: 5, Command: "cover-cmd",
		Metric: &Metric{Extract: ExtractLastLineFloat, Threshold: 80, Operator: OpAtLeast},
	}}}
	r := &fakeRunner{results: map[string]fakeResult{"cover-cmd": {stdout: "85.3"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "abc", r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed || res.Passed != 1 {
		t.Fatalf("res = %+v", res)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.MetricValue == nil || *rec.MetricValue != 85.3 {
		t.Fatalf("metric value = %v", rec.MetricValue)
	}
	if rec.MetricThreshold == nil || *rec.MetricThreshold != 80 {
		t.Fatalf("metric threshold = %v", rec.MetricThreshold)
	}
	if rec.MetricOperator != OpAtLeast {
		t.Fatalf("metric operator = %q", rec.MetricOperator)
	}
}

func TestRunMetricFloorFails(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "coverage-floor", Layer: 5, Command: "cover-cmd",
		Metric: &Metric{Extract: ExtractLastLineFloat, Threshold: 80, Operator: OpAtLeast},
	}}}
	r := &fakeRunner{results: map[string]fakeResult{"cover-cmd": {stdout: "72.5"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.FailedGate != "coverage-floor" {
		t.Fatalf("res = %+v", res)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.MetricValue == nil || *rec.MetricValue != 72.5 {
		t.Fatalf("metric value = %v", rec.MetricValue)
	}
	if !strings.Contains(rec.Stderr, "does not meet") {
		t.Fatalf("receipt stderr must name the threshold miss, got %q", rec.Stderr)
	}
}

func TestRunMetricPercentExtract(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "coverage", Layer: 5, Command: "cover-cmd",
		Metric: &Metric{Extract: ExtractLastLinePercent, Threshold: 80, Operator: OpAtLeast},
	}}}
	r := &fakeRunner{results: map[string]fakeResult{
		"cover-cmd": {stdout: "total:\t(statements)\t81.2%"},
	}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("81.2%% must pass an 80 floor: %+v", res)
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.MetricValue == nil || *rec.MetricValue != 81.2 {
		t.Fatalf("metric value = %v", rec.MetricValue)
	}
}

func TestRunMetricExtractionRefused(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "coverage", Layer: 5, Command: "cover-cmd",
		Metric: &Metric{Extract: ExtractLastLineFloat, Threshold: 80, Operator: OpAtLeast},
	}}}
	r := &fakeRunner{results: map[string]fakeResult{"cover-cmd": {stdout: "no number here"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("an unextractable metric must fail the gate")
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.MetricValue != nil {
		t.Fatal("MetricValue must stay nil when extraction is refused")
	}
	if !strings.Contains(rec.Stderr, "could not extract") {
		t.Fatalf("receipt stderr must name the extraction failure, got %q", rec.Stderr)
	}
}

func TestRunMetricAtMostPass(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "crap", Layer: 7, Command: "crap-cmd",
		Metric: &Metric{Extract: ExtractLastLineFloat, Threshold: 30, Operator: OpAtMost},
	}}}
	r := &fakeRunner{results: map[string]fakeResult{"crap-cmd": {stdout: "CRAP max: 12.5"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("12.5 must pass a <=30 gate: %+v", res)
	}
}

func TestRunMetricGateStillFailsOnExitCode(t *testing.T) {
	cfg := Config{Gates: []Gate{{
		Name: "coverage", Layer: 5, Command: "cover-cmd",
		Metric: &Metric{Extract: ExtractLastLineFloat, Threshold: 80, Operator: OpAtLeast},
	}}}
	r := &fakeRunner{results: map[string]fakeResult{"cover-cmd": {code: 1, stdout: "85.3"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "t", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatal("a non-zero exit must fail a metric gate regardless of the value")
	}
	rec := readReceiptFile(t, res.Receipts[0])
	if rec.MetricValue != nil {
		t.Fatal("MetricValue must not be set when the command itself failed")
	}
}

// TestShRunnerRealExec pins the REAL runner semantics against a real sh:
// a completed run (even non-zero, even command-not-found 127) returns its
// exit code with a nil error; only spawn failures and timeouts refuse with
// an error.
func TestShRunnerRealExec(t *testing.T) {
	r := ShRunner{}
	out, _, code, err := r.Run(context.Background(), "echo hello")
	if err != nil || code != 0 || !strings.Contains(out, "hello") {
		t.Fatalf("echo: out=%q code=%d err=%v", out, code, err)
	}
	_, _, code, err = r.Run(context.Background(), "exit 3")
	if err != nil || code != 3 {
		t.Fatalf("exit 3: code=%d err=%v", code, err)
	}
	_, _, code, err = r.Run(context.Background(), "this-command-does-not-exist-xyz")
	if err != nil || code != 127 {
		t.Fatalf("not-found is a completed run with the shell's exit code: code=%d err=%v", code, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, _, code, err = r.Run(ctx, "sleep 2")
	if err == nil || code != -1 || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout must refuse with an error: code=%d err=%v", code, err)
	}
}
