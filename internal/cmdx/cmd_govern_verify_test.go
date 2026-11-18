// Governance + verification handler tests: the two largest cmdx
// files. Lifecycle round-trips over real stores in t.TempDir(),
// role-separation refusals (the executor never self-governs),
// corrupt-store handling, and the verify plan/run/report/rerun
// evidence chain. Portable: a stub gates.CmdRunner stands in for
// git (no shell, no env reads); sleeps are unnecessary (all
// synchronous).
package cmdx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/govern"
	"regulus-ai/internal/verify"
)

// errRunner refuses every command: handlers that only need a HEAD
// for provenance fall back to "worktree" instead of hanging or
// spawning a shell.
type errRunner struct{ msg string }

func (r errRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	return "", "", 1, errors.New(r.msg)
}

func cmdIO(dir string) (IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return New(&out, &errb, dir, "test", errRunner{"no git in tests"}), &out, &errb
}

func TestGovernMatrix(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	if code := io.GovernMatrixCmd(nil); code != 0 {
		t.Fatalf("matrix exit = %d", code)
	}
	if !strings.Contains(out.String(), "standard") {
		t.Fatalf("matrix must quote the standard profile, got %q", out.String())
	}
	out.Reset()
	if code := io.GovernMatrixCmd([]string{"--json"}); code != 0 {
		t.Fatalf("matrix --json exit = %d", code)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("matrix --json must decode: %v\n%s", err, out.String())
	}
	if _, ok := doc["profiles"]; !ok {
		t.Fatalf("matrix --json must carry profiles, got %q", out.String())
	}
}

func TestGovernAuthorizeExitMatchesVerdict(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := missionIO(dir)
	// Text form: the decision is printed, never silent.
	if code := io.GovernAuthorizeCmd([]string{"--kind", "read", "--surface", "docs"}); code != 0 && code != 1 {
		t.Fatalf("authorize exit = %d, want 0 or 1", code)
	}
	if !strings.Contains(out.String(), "govern:") {
		t.Fatalf("authorize must print the verdict, got %q", out.String())
	}
	// JSON form: exit code and verdict must agree, or a regression
	// is laundering denials into clean exits.
	out.Reset()
	code := io.GovernAuthorizeCmd([]string{"--kind", "read", "--surface", "docs", "--json"})
	var dec struct {
		Allow bool   `json:"allow"`
		Risk  string `json:"risk"`
	}
	if err := json.Unmarshal(out.Bytes(), &dec); err != nil {
		t.Fatalf("authorize --json must decode: %v\n%s", err, out.String())
	}
	if dec.Risk == "" {
		t.Fatalf("authorize --json must name the risk, got %q", out.String())
	}
	if dec.Allow != (code == 0) {
		t.Fatalf("allow=%v but exit=%d (must agree)", dec.Allow, code)
	}
	// What-if profile: evaluated without touching the stored policy.
	out.Reset()
	errb.Reset()
	code = io.GovernAuthorizeCmd([]string{"--kind", "deploy", "--surface", "core", "--profile", "locked"})
	if code != 0 && code != 1 {
		t.Fatalf("what-if authorize exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "what-if@locked") {
		t.Fatalf("what-if must name the profile, got %q", out.String())
	}
	// Unknown kind/surface fails closed, never allowed.
	if code := io.GovernAuthorizeCmd([]string{"--kind", "bogus", "--surface", "docs"}); code != 1 {
		t.Fatalf("unknown kind exit = %d, want 1", code)
	}
	// MCP cross-check: an unknown tool denies even when mission
	// autonomy allows, and the denial pins the exit.
	out.Reset()
	if code := io.GovernAuthorizeCmd([]string{"--kind", "read", "--surface", "docs", "--mcp-tool", "no-such-tool"}); code != 1 {
		t.Fatalf("mcp-denied authorize exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "mcp: DENY") {
		t.Fatalf("must show the MCP denial, got %q", out.String())
	}
}

func governRequest(t *testing.T, io IO, out, errb *bytes.Buffer) string {
	t.Helper()
	out.Reset()
	errb.Reset()
	args := []string{"--kind", "read", "--surface", "docs",
		"--summary", "low risk docs read for audit", "--scope", "docs:read",
		"--risks", "stale wording", "--rollback", "revert the wording",
		"--cost", "negligible", "--plan", "v1",
		"--proposer", "alice", "--reviewer", "bob", "--executor", "carol",
		"--diff", "deadbeef", "--json"}
	if code := io.GovernRequestCmd(args); code != 0 {
		t.Fatalf("request exit = %d stderr = %q", code, errb.String())
	}
	var req struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(out.Bytes(), &req); err != nil || req.ID == "" {
		t.Fatalf("request --json must carry the id: %v %q", err, out.String())
	}
	if req.State != govern.ApprovalRequested {
		t.Fatalf("fresh request state = %q, want REQUESTED", req.State)
	}
	return req.ID
}

func TestGovernLifecycleRoundTrip(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := missionIO(dir)
	id := governRequest(t, io, out, errb)
	// Pause/resume: only proposer or reviewer may freeze or lift.
	if code := io.GovernPauseCmd([]string{"--id", id, "--by", "carol", "--reason", "hold"}); code != 1 {
		t.Fatalf("executor pause exit = %d, want 1 (never freezes what it runs)", code)
	}
	if code := io.GovernPauseCmd([]string{"--id", id, "--by", "alice", "--reason", "hold for review"}); code != 0 {
		t.Fatalf("proposer pause exit = %d stderr = %q", code, errb.String())
	}
	if code := io.GovernResumeCmd([]string{"--id", id, "--by", "alice", "--reason", "review done"}); code != 0 {
		t.Fatalf("resume exit = %d stderr = %q", code, errb.String())
	}
	// Approve: no self-approval across the four duties.
	if code := io.GovernApproveCmd([]string{"--id", id, "--by", "carol", "--artifact", "deadbeef", "--reason", "fine"}); code != 1 {
		t.Fatalf("executor approve exit = %d, want 1 (four distinct duties)", code)
	}
	out.Reset()
	if code := io.GovernApproveCmd([]string{"--id", id, "--by", "diana", "--artifact", "deadbeef", "--reason", "accept low risk"}); code != 0 {
		t.Fatalf("approve exit = %d stderr = %q", code, errb.String())
	}
	// Check: bound artifact and stored policy validate the approval.
	out.Reset()
	if code := io.GovernCheckCmd([]string{"--id", id, "--artifact", "deadbeef"}); code != 0 {
		t.Fatalf("check exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "VALID") {
		t.Fatalf("check must declare VALID, got %q", out.String())
	}
	out.Reset()
	if code := io.GovernCheckCmd([]string{"--id", id, "--artifact", "deadbeef", "--json"}); code != 0 {
		t.Fatalf("check --json exit = %d stderr = %q", code, errb.String())
	}
	var chk struct {
		Valid bool `json:"valid"`
	}
	if err := json.Unmarshal(out.Bytes(), &chk); err != nil || !chk.Valid {
		t.Fatalf("check --json must decode valid: %v %q", err, out.String())
	}
	errb.Reset()
	if code := io.GovernCheckCmd([]string{"--id", id, "--artifact", "changed"}); code != 1 {
		t.Fatalf("material change exit = %d, want 1 (never warns-and-continues)", code)
	}
	// Revoke: the executor never un-signs what authorizes it.
	if code := io.GovernRevokeCmd([]string{"--id", id, "--by", "carol", "--reason", "stop"}); code != 1 {
		t.Fatalf("executor revoke exit = %d, want 1", code)
	}
	if code := io.GovernRevokeCmd([]string{"--id", id, "--by", "alice", "--reason", "superseded by v2"}); code != 0 {
		t.Fatalf("revoke exit = %d stderr = %q", code, errb.String())
	}
	// Show/list/audit quote the sealed trail.
	out.Reset()
	if code := io.GovernShowCmd([]string{"--id", id}); code != 0 {
		t.Fatalf("show exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "revoked") {
		t.Fatalf("show must quote the revoked state, got %q", out.String())
	}
	out.Reset()
	if code := io.GovernListCmd([]string{"--state", "revoked"}); code != 0 {
		t.Fatalf("list --state exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), id) {
		t.Fatalf("filtered list must carry the id, got %q", out.String())
	}
	out.Reset()
	if code := io.GovernAuditCmd([]string{"--id", id}); code != 0 {
		t.Fatalf("audit exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), id) {
		t.Fatalf("audit must cite the request, got %q", out.String())
	}
	// Policy: human-owned switch, then every read sees it.
	// The human rule: automation identities never edit the rules.
	if code := io.GovernPolicyCmd([]string{"--set-profile", "wide", "--by", "bot"}); code != 1 {
		t.Fatalf("policy switch by a bot exit = %d, want 1", code)
	}
	errb.Reset()
	if code := io.GovernPolicyCmd([]string{"--set-profile", "locked", "--by", "diana"}); code != 0 {
		t.Fatalf("human policy switch exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := io.GovernPolicyCmd([]string{"--show"}); code != 0 {
		t.Fatalf("policy --show exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "locked") {
		t.Fatalf("policy must quote the active posture, got %q", out.String())
	}
	// A second request rejected by the reviewer ends the trail.
	id2 := governRequest(t, io, out, errb)
	if code := io.GovernRejectCmd([]string{"--id", id2, "--by", "bob", "--reason", "out of scope this quarter"}); code != 0 {
		t.Fatalf("reject exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := io.GovernShowCmd([]string{"--id", id2}); code != 0 {
		t.Fatalf("show rejected exit = %d", code)
	}
	if !strings.Contains(out.String(), "rejected") {
		t.Fatalf("show must quote the rejected state, got %q", out.String())
	}
}

func TestGovernPolicyHumanRule(t *testing.T) {
	dir := t.TempDir()
	io, _, errb := missionIO(dir)
	if code := io.GovernPolicyCmd([]string{"--set-profile", "wide"}); code != 2 {
		t.Fatalf("switch without owner exit = %d, want 2", code)
	}
	if code := io.GovernPolicyCmd([]string{"--set-profile", "nope", "--by", "diana"}); code != 1 {
		t.Fatalf("unknown profile exit = %d, want 1 (fails closed)", code)
	}
	if !strings.Contains(errb.String(), "policy") {
		t.Fatalf("refusal must name policy, got %q", errb.String())
	}
}

func TestGovernListDetectsCorrupt(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := missionIO(dir)
	id := governRequest(t, io, out, errb)
	// A hand-edited approval file is reported explicitly and fails
	// the list: tampered governance never reads clean.
	bad := filepath.Join(govern.ApprovalsDir(dir), "tampered.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	if code := io.GovernListCmd(nil); code != 1 {
		t.Fatalf("list with a corrupt entry exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "corrupt") {
		t.Fatalf("list must name the corruption, got %q", errb.String())
	}
	out.Reset()
	if code := io.GovernListCmd([]string{"--state", "revoked", "--json"}); code != 1 {
		t.Fatalf("filtered list with corruption exit = %d, want 1 (never filtered silent)", code)
	}
	_ = id
}

func TestGovernAuthorizeExitTable(t *testing.T) {
	allow := govern.Decision{Allow: true}
	if got := governAuthorizeExit(allow, ""); got != 0 {
		t.Fatalf("allow exit = %d, want 0", got)
	}
	if got := governAuthorizeExit(allow, "mcp: DENY (unknown tool)"); got != 1 {
		t.Fatalf("mcp-denied exit = %d, want 1 (both gates must open)", got)
	}
	deny := govern.Decision{Allow: false, NeedsApproval: true}
	if got := governAuthorizeExit(deny, ""); got != 1 {
		t.Fatalf("approval-routed exit = %d, want 1", got)
	}
	if got := errString(nil); got != "" {
		t.Fatalf("errString(nil) = %q, want empty", got)
	}
	if got := errString(errors.New("boom")); got != "boom" {
		t.Fatalf("errString(err) = %q, want the message", got)
	}
}

func TestVerifyPlanRoundTrip(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	planRepo(t, dir)
	if code := io.VerifyPlanCmd([]string{"--changed", "internal/demo/demo.go", "--json"}); code != 0 {
		t.Fatalf("plan --json exit = %d stderr = %q", code, errb.String())
	}
	var plan struct {
		DiffHash string           `json:"diff_hash"`
		Gates    []map[string]any `json:"gates"`
		Tier     string           `json:"tier"`
	}
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatalf("plan --json must decode: %v\n%s", err, out.String())
	}
	if plan.DiffHash == "" || len(plan.Gates) == 0 || plan.Tier == "" {
		t.Fatalf("plan must carry diff, gates and tier, got %q", out.String())
	}
	out.Reset()
	evdir := filepath.Join(dir, "plan-evidence")
	if code := io.VerifyPlanCmd([]string{"--changed", "internal/demo/demo.go", "--out", evdir}); code != 0 {
		t.Fatalf("plan --out exit = %d stderr = %q", code, errb.String())
	}
	if st, err := os.Stat(evdir); err != nil || !st.IsDir() {
		t.Fatalf("plan evidence must persist: %v", err)
	}
	if code := io.VerifyPlanCmd([]string{"--changed", "internal/demo/demo.go", "--severity", "2"}); code != 2 {
		t.Fatalf("bad severity exit = %d, want 2", code)
	}
}

func TestVerifyRunNoToolchainBlocks(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	planRepo(t, dir)
	runOut := filepath.Join(dir, "run-evidence")
	// A host without a toolchain blocks every gate instead of
	// inventing passes: the report seals BLOCKED and exits 1.
	if code := io.VerifyRunCmd([]string{"--changed", "internal/demo/demo.go", "--no-toolchain", "--out", runOut}); code != 1 {
		t.Fatalf("no-toolchain run exit = %d, want 1 stderr = %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(runOut, "manifest.json")); err != nil {
		t.Fatalf("blocked run must still seal evidence: %v", err)
	}
	// Bare run on a healthy host: every unrecorded gate stays
	// UNKNOWN and blocks integration the same way.
	out.Reset()
	errb.Reset()
	if code := io.VerifyRunCmd([]string{"--changed", "internal/demo/demo.go"}); code != 1 {
		t.Fatalf("unevidenced run exit = %d, want 1 stderr = %q", code, errb.String())
	}
	// Injected failure and unknown gates refuse with their codes.
	if code := io.VerifyRunCmd([]string{"--changed", "internal/demo/demo.go", "--fail", "bogus-gate:x"}); code != 2 {
		t.Fatalf("unknown gate exit = %d, want 2", code)
	}
	if code := io.VerifyRunCmd([]string{"--changed", "internal/demo/demo.go", "--fail", "unit:broken by flag"}); code != 1 {
		t.Fatalf("failed gate exit = %d, want 1", code)
	}
	_ = out
}

func TestVerifyReportRerunChain(t *testing.T) {
	dir := t.TempDir()
	io, out, errb := cmdIO(dir)
	planRepo(t, dir)
	runOut := filepath.Join(dir, "run-evidence")
	if code := io.VerifyRunCmd([]string{"--changed", "internal/demo/demo.go", "--no-toolchain", "--out", runOut}); code != 1 {
		t.Fatalf("setup run exit = %d stderr = %q", code, errb.String())
	}
	// Report re-reads the sealed evidence.
	out.Reset()
	if code := io.VerifyReportCmd([]string{"--dir", runOut}); code != 0 {
		t.Fatalf("report exit = %d stderr = %q", code, errb.String())
	}
	if out.Len() == 0 {
		t.Fatal("report must render the stored result")
	}
	out.Reset()
	if code := io.VerifyReportCmd([]string{"--dir", runOut, "--json"}); code != 0 {
		t.Fatalf("report --json exit = %d stderr = %q", code, errb.String())
	}
	var both map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &both); err != nil {
		t.Fatalf("report --json must decode: %v\n%s", err, out.String())
	}
	if _, ok := both["plan"]; !ok {
		t.Fatalf("report --json must carry plan, got %q", out.String())
	}
	if _, ok := both["result"]; !ok {
		t.Fatalf("report --json must carry result, got %q", out.String())
	}
	if code := io.VerifyReportCmd([]string{"--dir", filepath.Join(dir, "missing")}); code != 1 {
		t.Fatalf("report on a missing dir exit = %d, want 1", code)
	}
	// Rerun replays the same configuration into a new dir without
	// touching the prior evidence: blocked stays blocked.
	rerunOut := filepath.Join(dir, "rerun-evidence")
	out.Reset()
	if code := io.VerifyRerunCmd([]string{"--dir", runOut, "--out", rerunOut}); code != 1 {
		t.Fatalf("rerun exit = %d, want 1 stderr = %q", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(rerunOut, "manifest.json")); err != nil {
		t.Fatalf("rerun must seal new evidence: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runOut, "manifest.json")); err != nil {
		t.Fatalf("rerun must not consume the prior evidence: %v", err)
	}
	if code := io.VerifyRerunCmd([]string{"--dir", runOut, "--out", runOut}); code != 2 {
		t.Fatalf("rerun onto the same dir exit = %d, want 2 (never overwrites)", code)
	}
}

func TestParseOutcomeList(t *testing.T) {
	m, err := parseOutcomeList("unit:broken, static:needs a look", verify.VerdictFail, "flag")
	if err != nil {
		t.Fatal(err)
	}
	if m["unit"].Verdict != verify.VerdictFail || m["unit"].Cause != "broken" {
		t.Fatalf("explicit cause must survive: %+v", m["unit"])
	}
	if m["static"].Cause == "" {
		t.Fatalf("bare-ish entries keep the flag cause: %+v", m["static"])
	}
	eq, err := parseOutcomeList("integration=green", verify.VerdictPass, "flag")
	if err != nil || eq["integration"].Verdict != verify.VerdictPass {
		t.Fatalf("= syntax must parse: %+v %v", eq, err)
	}
	if _, err := parseOutcomeList("bogus:x", verify.VerdictFail, "flag"); err == nil {
		t.Fatal("unknown gate must fail parsing")
	}
	if m, err := parseOutcomeList("", verify.VerdictFail, "flag"); err != nil || len(m) != 0 {
		t.Fatalf("empty list must parse empty: %+v %v", m, err)
	}
}

func TestLoadOutcomeResults(t *testing.T) {
	dir := t.TempDir()
	ok := `[{"kind":"unit","verdict":"PASS","cause":"green","evidence":"ci"}]`
	p := filepath.Join(dir, "ok.json")
	if err := os.WriteFile(p, []byte(ok), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := loadOutcomeResults(p)
	if err != nil {
		t.Fatal(err)
	}
	if m["unit"].Verdict != verify.VerdictPass {
		t.Fatalf("recorded PASS must load: %+v", m["unit"])
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("[{oops}]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOutcomeResults(bad); err == nil {
		t.Fatal("malformed results must fail")
	}
	unknown := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`[{"kind":"unit","verdict":"PASS","bogus":"x"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOutcomeResults(unknown); err == nil {
		t.Fatal("unknown fields must fail (strict JSON)")
	}
	if _, err := loadOutcomeResults(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing results file must fail")
	}
}

func TestVerifyExecutorBranches(t *testing.T) {
	ex := verifyExecutor(nil, true)
	if got := ex(verify.GateSpec{Kind: verify.GateUnit}); got.Verdict != verify.VerdictBlocked {
		t.Fatalf("missing toolchain must BLOCK, got %+v", got)
	}
	recorded := map[string]verify.GateResult{
		verify.GateUnit: {Kind: verify.GateUnit, Verdict: verify.VerdictPass, Cause: "ci"},
	}
	ex = verifyExecutor(recorded, false)
	if got := ex(verify.GateSpec{Kind: verify.GateUnit}); got.Verdict != verify.VerdictPass {
		t.Fatalf("recorded PASS must pass, got %+v", got)
	}
	if got := ex(verify.GateSpec{Kind: verify.GateBuild}); got.Verdict != verify.VerdictUnknown {
		t.Fatalf("unrecorded gate must stay UNKNOWN, got %+v", got)
	}
}

func TestVerifyProvenanceProbes(t *testing.T) {
	// Toolchain probe: wherever `go` resolves, the probe names it;
	// where it does not, the probe reads missing — never ok.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH (probe would read missing-go here)")
	}
	if got := probeGoToolchain(); strings.HasPrefix(got, "missing-go") || got == "" {
		t.Fatalf("go is on PATH, probe must name it, got %q", got)
	}
	if got := hostName(); got == "" {
		t.Fatal("host probe must never return empty")
	}
	dir := t.TempDir()
	io, _, _ := cmdIO(dir)
	if prov := io.verifyProvenance("tester", "abc"); prov.RunID == "" {
		t.Fatal("provenance must always mint a run id")
	}
}

func TestVerifyRegressionGuard(t *testing.T) {
	rep, err := verifyRegression("", "", "", "abc", "run-1")
	if err != nil || rep != nil {
		t.Fatalf("empty refs mean no regression input: %+v %v", rep, err)
	}
	if _, err := verifyRegression("base", "", "", "abc", "run-1"); err == nil {
		t.Fatal("baseline without window must fail (travel together)")
	}
	if _, err := verifyRegression("", "win", "", "abc", "run-1"); err == nil {
		t.Fatal("window without baseline must fail (travel together)")
	}
}
