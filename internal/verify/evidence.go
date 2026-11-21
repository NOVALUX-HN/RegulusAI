// Tamper-evident verification evidence (F13.4/NOV-107).
//
// Layout (like F11.3 tier dirs and F12.4 baseline dirs, one document
// per dir, new run → new dir):
//   - plan dir: plan.json + manifest.json (plan sha256).
//   - result dir: plan.json + result.json + manifest.json (both
//     sha256 + the plan sha the run executed, so a rerun proves
//     "mismo plan" from bytes, not from filenames).
//
// Reads verify both shas first: edited evidence fails instead of
// gating from a lie. Reruns never touch the prior dir — compare
// happens across dirs (CompareResults).
package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Evidence filenames: fixed names so checks always know where to look.
const (
	PlanFile       = "plan.json"
	PlanMeta       = "manifest.json"
	ResultFile     = "result.json"
	ResultMeta     = "manifest.json"
	StoredPlanFile = "plan.json"
)

// PlanManifest describes one stored plan: the config it derives from
// plus the plan checksum.
type PlanManifest struct {
	Version   string `json:"version"`
	Scope     string `json:"scope"`
	ConfigID  string `json:"config_id"`
	PlanFile  string `json:"plan_file"`
	PlanSHA   string `json:"plan_sha256"`
	GitHead   string `json:"git_head"`
	RunID     string `json:"run_id"`
	CreatedAt string `json:"created_at"`
}

// ResultManifest describes one stored result: both checksums plus the
// decision, so listing dirs answers allow/block without parsing.
type ResultManifest struct {
	Version   string `json:"version"`
	ConfigID  string `json:"config_id"`
	PlanSHA   string `json:"plan_sha256"`
	ResultSHA string `json:"result_sha256"`
	GitHead   string `json:"git_head"`
	RunID     string `json:"run_id"`
	Decision  string `json:"decision"`
	Overall   string `json:"overall"`
	CreatedAt string `json:"created_at"`
}

// WritePlanEvidence stores one plan plus a manifest carrying its
// sha256.
func WritePlanEvidence(dir string, p Plan, gitHead string, now time.Time) (string, error) {
	if p.Version != VerifyVersion {
		return "", fmt.Errorf("verify: refusing to store plan version %q", p.Version)
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, PlanFile), append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(trimNewline(raw))
	m := PlanManifest{
		Version: VerifyVersion, Scope: p.Scope, ConfigID: p.ConfigID,
		PlanFile: PlanFile, PlanSHA: hex.EncodeToString(sum[:]),
		GitHead: gitHead, RunID: p.Provenance.RunID,
		CreatedAt: now.UTC().Format(time.RFC3339),
	}
	mraw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	manPath := filepath.Join(dir, PlanMeta)
	if err := os.WriteFile(manPath, append(mraw, '\n'), 0o644); err != nil {
		return "", err
	}
	return manPath, nil
}

// ReadPlanEvidence loads one plan dir, verifying the plan sha256
// first: edited plans fail instead of verifying from a lie.
func ReadPlanEvidence(dir string) (PlanManifest, Plan, error) {
	var m PlanManifest
	var p Plan
	raw, err := os.ReadFile(filepath.Join(dir, PlanMeta))
	if err != nil {
		return m, p, fmt.Errorf("reading verify manifest: %v", err)
	}
	if err := strictUnmarshal(raw, &m); err != nil {
		return m, p, fmt.Errorf("malformed verify manifest: %v", err)
	}
	if m.PlanFile == "" {
		m.PlanFile = PlanFile
	}
	praw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(m.PlanFile)))
	if err != nil {
		return m, p, fmt.Errorf("reading verify plan: %v", err)
	}
	sum := sha256.Sum256(trimNewline(praw))
	if hex.EncodeToString(sum[:]) != strings.ToLower(strings.TrimSpace(m.PlanSHA)) {
		return m, p, fmt.Errorf("verify plan sha256 mismatch: evidence was modified after recording")
	}
	if err := strictUnmarshal(praw, &p); err != nil {
		return m, p, fmt.Errorf("malformed verify plan: %v", err)
	}
	if p.Version != VerifyVersion {
		return m, p, fmt.Errorf("verify: plan version %q is not %q", p.Version, VerifyVersion)
	}
	return m, p, nil
}

// WriteResultEvidence stores one result with a copy of the plan it
// ran plus a manifest carrying both shas. The result dir is
// self-contained: rerun and report reproduce from these bytes alone.
func WriteResultEvidence(dir string, p Plan, r Result, gitHead string, now time.Time) (string, error) {
	if p.Version != VerifyVersion {
		return "", fmt.Errorf("verify: refusing to store plan version %q", p.Version)
	}
	if r.Version != VerifyVersion {
		return "", fmt.Errorf("verify: refusing to store result version %q", r.Version)
	}
	if p.ConfigID != r.ConfigID {
		return "", fmt.Errorf("verify: result config %s does not match plan config %s (refusing to bind)", shortSHA(r.ConfigID), shortSHA(p.ConfigID))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	praw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, StoredPlanFile), append(praw, '\n'), 0o644); err != nil {
		return "", err
	}
	rraw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ResultFile), append(rraw, '\n'), 0o644); err != nil {
		return "", err
	}
	psum := sha256.Sum256(trimNewline(praw))
	rsum := sha256.Sum256(trimNewline(rraw))
	m := ResultManifest{
		Version: VerifyVersion, ConfigID: p.ConfigID,
		PlanSHA: hex.EncodeToString(psum[:]), ResultSHA: hex.EncodeToString(rsum[:]),
		GitHead: gitHead, RunID: r.Provenance.RunID,
		Decision: r.Decision, Overall: r.Overall,
		CreatedAt: now.UTC().Format(time.RFC3339),
	}
	mraw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	manPath := filepath.Join(dir, ResultMeta)
	if err := os.WriteFile(manPath, append(mraw, '\n'), 0o644); err != nil {
		return "", err
	}
	return manPath, nil
}

// ReadResultEvidence loads one result dir, verifying both shas first.
func ReadResultEvidence(dir string) (ResultManifest, Plan, Result, error) {
	var m ResultManifest
	var p Plan
	var r Result
	raw, err := os.ReadFile(filepath.Join(dir, ResultMeta))
	if err != nil {
		return m, p, r, fmt.Errorf("reading verify manifest: %v", err)
	}
	if err := strictUnmarshal(raw, &m); err != nil {
		return m, p, r, fmt.Errorf("malformed verify manifest: %v", err)
	}
	praw, err := os.ReadFile(filepath.Join(dir, StoredPlanFile))
	if err != nil {
		return m, p, r, fmt.Errorf("reading verify plan: %v", err)
	}
	psum := sha256.Sum256(trimNewline(praw))
	if hex.EncodeToString(psum[:]) != strings.ToLower(strings.TrimSpace(m.PlanSHA)) {
		return m, p, r, fmt.Errorf("verify plan sha256 mismatch: evidence was modified after recording")
	}
	rraw, err := os.ReadFile(filepath.Join(dir, ResultFile))
	if err != nil {
		return m, p, r, fmt.Errorf("reading verify result: %v", err)
	}
	rsum := sha256.Sum256(trimNewline(rraw))
	if hex.EncodeToString(rsum[:]) != strings.ToLower(strings.TrimSpace(m.ResultSHA)) {
		return m, p, r, fmt.Errorf("verify result sha256 mismatch: evidence was modified after recording")
	}
	if err := strictUnmarshal(praw, &p); err != nil {
		return m, p, r, fmt.Errorf("malformed verify plan: %v", err)
	}
	if err := strictUnmarshal(rraw, &r); err != nil {
		return m, p, r, fmt.Errorf("malformed verify result: %v", err)
	}
	if p.Version != VerifyVersion || r.Version != VerifyVersion {
		return m, p, r, fmt.Errorf("verify: evidence version is not %q", VerifyVersion)
	}
	if p.ConfigID != r.ConfigID {
		return m, p, r, fmt.Errorf("verify: stored plan config %s does not match result config %s", shortSHA(p.ConfigID), shortSHA(r.ConfigID))
	}
	return m, p, r, nil
}

// Comparison is the rerun diff: same plan or not, per-gate verdict
// moves, and whether the integration answer changed. Comparing never
// writes: both dirs stay byte-identical.
func Comparison(oldM ResultManifest, oldR Result, newM ResultManifest, newR Result) string {
	var b strings.Builder
	if oldR.ConfigID == newR.ConfigID {
		b.WriteString(fmt.Sprintf("rerun: mismo plan (config %s)\n", shortSHA(oldR.ConfigID)))
	} else {
		b.WriteString(fmt.Sprintf("rerun: plan distinto (config %s -> %s): no es un rerun, es otra verificación\n",
			shortSHA(oldR.ConfigID), shortSHA(newR.ConfigID)))
	}
	oldBy, newBy := map[string]string{}, map[string]string{}
	for _, g := range oldR.Gates {
		oldBy[g.Kind] = g.Verdict
	}
	for _, g := range newR.Gates {
		newBy[g.Kind] = g.Verdict
	}
	kinds := map[string]bool{}
	for k := range oldBy {
		kinds[k] = true
	}
	for k := range newBy {
		kinds[k] = true
	}
	var sorted []string
	for k := range kinds {
		sorted = append(sorted, k)
	}
	sortStrings(sorted)
	moved := 0
	for _, k := range sorted {
		o, n := oldBy[k], newBy[k]
		if o == "" {
			o = "—"
		}
		if n == "" {
			n = "—"
		}
		mark := "="
		if o != n {
			mark = "≠"
			moved++
		}
		fmt.Fprintf(&b, "  %s %s: %s -> %s\n", mark, k, o, n)
	}
	fmt.Fprintf(&b, "overall: %s -> %s, decision: %s -> %s (%d gate(s) cambiaron)\n",
		oldR.Overall, newR.Overall, oldR.Decision, newR.Decision, moved)
	_ = oldM
	_ = newM
	return b.String()
}

// strictUnmarshal decodes strict JSON: unknown fields fail, so shape
// drift is a refusal, never a silent reinterpretation.
func strictUnmarshal(raw []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// trimNewline drops one trailing newline before hashing: the writer
// appends it, the hash covers the document (same convention as
// quality and bench evidence).
func trimNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}

// sortStrings sorts in place (stdlib sort without importing it into
// every caller: kept local so evidence.go stays self-contained).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
