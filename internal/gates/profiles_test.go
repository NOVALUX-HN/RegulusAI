package gates

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/receipts"
)

// RED: every gate carries ID, category, execution conditions, inputs,
// failure policy and profile membership — with defaults that preserve
// today's behavior exactly.

func TestGateDefaultsPreserveBehavior(t *testing.T) {
	cfg := Config{Gates: []Gate{{Name: "Build It", Layer: 1, Command: "echo ok"}}}
	if err := validate(cfg); err != nil {
		t.Fatal(err)
	}
	g := cfg.Gates[0]
	if got := g.EffectiveID(); got != "build-it" {
		t.Fatalf("default ID must slug the name, got %q", got)
	}
	if !g.IsRequired() {
		t.Fatal("gates are required by default: mandatory policy can never be omitted by omission")
	}
	if g.EffectiveOnFail() != "halt" {
		t.Fatalf("default failure policy must be halt, got %q", g.EffectiveOnFail())
	}
	if len(g.Profiles) != 0 || len(g.When) != 0 {
		t.Fatalf("no conditions by default, got %+v", g)
	}
}

func TestValidateNewGateFields(t *testing.T) {
	for _, gate := range []Gate{
		{Name: "a", Layer: 1, Command: "x", OnFail: "explode"},
		{Name: "a", Layer: 1, Command: "x", Profiles: []string{""}},
	} {
		if err := validate(Config{Gates: []Gate{gate}}); err == nil {
			t.Fatalf("must reject %+v", gate)
		}
	}
	ok := Gate{Name: "a", Layer: 1, Command: "x", OnFail: "advisory", Category: "security", When: []string{"*.go"}, Profiles: []string{"full", "security"}}
	if err := validate(Config{Gates: []Gate{ok}}); err != nil {
		t.Fatalf("valid metadata must pass, got %v", err)
	}
}

func TestSelectGatesProfiles(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "req", Layer: 1, Command: "echo req"},
		{Name: "opt", Layer: 2, Command: "echo opt", Required: boolPtr(false), Profiles: []string{"full"}},
	}}
	// Default (no profile): everything, exactly today's behavior.
	all, err := SelectGates(cfg, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("default must select all, got %v/%v", all, err)
	}
	// quick: required only.
	quick, err := SelectGates(cfg, "quick")
	if err != nil || len(quick) != 1 || quick[0].Name != "req" {
		t.Fatalf("quick must select required only, got %v/%v", quick, err)
	}
	// full: required + members.
	full, err := SelectGates(cfg, "full")
	if err != nil || len(full) != 2 {
		t.Fatalf("full must select required + members, got %v/%v", full, err)
	}
	// Unknown custom profile: error, never a silent subset.
	if _, err := SelectGates(cfg, "nope"); err == nil {
		t.Fatal("unknown profile must be rejected")
	}
	// Declared custom profile works.
	secCfg := Config{Gates: []Gate{
		{Name: "s", Layer: 1, Command: "x", Required: boolPtr(false), Profiles: []string{"security"}},
		{Name: "req2", Layer: 2, Command: "y"},
	}}
	sec, err := SelectGates(secCfg, "security")
	if err != nil || len(sec) != 2 {
		t.Fatalf("declared custom profile must select required + members, got %v/%v", sec, err)
	}
}

func TestAdvisoryFailureContinues(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "a", Layer: 1, Command: "ok-a"},
		{Name: "b", Layer: 2, Command: "fail-b", OnFail: "advisory"},
		{Name: "c", Layer: 3, Command: "ok-c"},
	}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"ok-a":   {stdout: "a"},
		"fail-b": {stdout: "b", code: 1},
		"ok-c":   {stdout: "c"},
	}}
	dir := t.TempDir()
	res, err := Run(context.Background(), cfg, dir, "t", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatal("advisory failure must not fail the run")
	}
	if res.Executed != 3 || len(res.Receipts) != 3 {
		t.Fatalf("advisory failure must not stop execution, got %+v", res)
	}
	var advisory *Outcome
	for i, o := range res.Outcomes {
		if o.Name == "b" {
			advisory = &res.Outcomes[i]
		}
	}
	if advisory == nil || !advisory.Advisory || advisory.Status != "FAIL" {
		t.Fatalf("advisory outcome must be marked, got %+v", advisory)
	}
	rec, err := receipts.ReadFile(advisory.ReceiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Verdict != receipts.VerdictFail {
		t.Fatalf("advisory receipt stays honest FAIL, got %q", rec.Verdict)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("all three gates must execute, got %v", fr.calls)
	}
}

func TestHaltStillStopsAfterAdvisory(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "a", Layer: 1, Command: "fail-a", OnFail: "advisory"},
		{Name: "b", Layer: 2, Command: "fail-b"},
		{Name: "c", Layer: 3, Command: "ok-c"},
	}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"fail-a": {code: 1},
		"fail-b": {code: 2},
	}}
	dir := t.TempDir()
	res, err := Run(context.Background(), cfg, dir, "t", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed || res.FailedGate != "b" {
		t.Fatalf("halt failure must stop the run, got %+v", res)
	}
	if res.Executed != 2 {
		t.Fatalf("gate c must never run, got %+v", res)
	}
}

func TestSkipUnmatchedOptionalGate(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "go-check", Layer: 1, Command: "check-go"},
		{Name: "docs-check", Layer: 2, Command: "check-docs", Required: boolPtr(false), When: []string{"*.md"}},
	}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {stdout: " M main.go\x00"},
		"check-go": {stdout: "go ok"},
	}}
	dir := t.TempDir()
	res, err := Run(context.Background(), cfg, dir, "t", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed || res.Executed != 1 {
		t.Fatalf("unmatched optional gate must skip, got %+v", res)
	}
	if res.Skipped == nil || len(res.Skipped) != 1 || res.Skipped[0] != "docs-check" {
		t.Fatalf("skip must be recorded, got %+v", res.Skipped)
	}
	var skipped *Outcome
	for i, o := range res.Outcomes {
		if o.Name == "docs-check" {
			skipped = &res.Outcomes[i]
		}
	}
	if skipped == nil || !skipped.Skipped {
		t.Fatalf("skipped outcome must be marked, got %+v", res.Outcomes)
	}
	for _, p := range res.Receipts {
		if strings.Contains(filepath.Base(p), "docs-check") {
			t.Fatalf("skipped gates write no receipts, got %s", p)
		}
	}
	for _, c := range fr.calls {
		if c == "check-docs" {
			t.Fatalf("skipped gate must not execute, calls: %v", fr.calls)
		}
	}
}

func TestRequiredIgnoresWhen(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "req", Layer: 1, Command: "check-req", When: []string{"*.go"}},
	}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {stdout: " M README.md\x00"},
		"check-req": {stdout: "ok"},
	}}
	dir := t.TempDir()
	res, err := Run(context.Background(), cfg, dir, "t", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if res.Executed != 1 || len(res.Skipped) != 0 {
		t.Fatalf("required gates always run, got %+v", res)
	}
}

func TestUnevaluableConditionRunsWithNote(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "opt", Layer: 1, Command: "check-opt", Required: boolPtr(false), When: []string{"*.go"}},
	}}
	fr := &fakeRunner{results: map[string]fakeResult{
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {err: errors.New("no git")},
		"check-opt": {stdout: "ok"},
	}}
	dir := t.TempDir()
	res, err := Run(context.Background(), cfg, dir, "t", "head", fr)
	if err != nil {
		t.Fatal(err)
	}
	if res.Executed != 1 {
		t.Fatalf("unevaluable conditions fail open (run), got %+v", res)
	}
	rec, err := receipts.ReadFile(res.Receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Stderr, "unevaluable") {
		t.Fatalf("fail-open must be recorded in the receipt, got %q", rec.Stderr)
	}
}

func boolPtr(b bool) *bool { return &b }

func TestRunTaskRecordsSkippedInManifest(t *testing.T) {
	cfg := Config{Gates: []Gate{
		{Name: "req", Layer: 1, Command: "check-req"},
		{Name: "docs", Layer: 2, Command: "check-docs", Required: boolPtr(false), When: []string{"*.md"}},
	}}
	root := t.TempDir()
	r := &fakeRunner{results: map[string]fakeResult{
		"git rev-parse --verify HEAD":                                {stdout: "abc123\n"},
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {stdout: " M main.go\x00"},
		"git diff HEAD -- .":                                         {stdout: "DIFF"},
		"check-req":                                                  {stdout: "ok"},
	}}
	res, mpath, err := RunTask(context.Background(), cfg, []byte(`{}`), root, "feat/T1", "v", "c", "", r)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "docs" {
		t.Fatalf("run must record the skip, got %+v", res.Skipped)
	}
	data, err := os.ReadFile(mpath)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Skipped []string `json:"skipped"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Skipped) != 1 || decoded.Skipped[0] != "docs" {
		t.Fatalf("manifest must record the skip decision, got %v", decoded.Skipped)
	}
}
