// Experiment manifests (F7, NOV-57, f7-protocol-v1): compact evidence
// pointers for exploration. A manifest links — never copies — the
// candidate manifest (NOV-25), baseline, dataset/split (bench NOV-35 +
// bench/f7-splits.json), seeds, config, budgets, evaluator, results and
// decisions through file/ID references plus SHA-256 pins. Verification
// recomputes the pins from disk: drift or a broken hash fails instead of
// silently comparing against moved content. Free text (notes, baseline
// label) is secret-redacted before persistence; retention delegates to
// Almanac/receipts (the manifest is a pointer, it owns no GC).
package evolve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ExpManifestSchema versions the experiment-manifest document.
const ExpManifestSchema = 1

// F7ProtocolVersion pins the normative protocol (docs/f7-protocol.md).
const F7ProtocolVersion = "f7-protocol-v1"

// Manifest result statuses (f7-protocol-v1 §5): quality (pass/fail),
// cost lives elsewhere (NOV-39), infra stays distinguishable.
const (
	ManifestPass         = "pass"
	ManifestFail         = "fail"
	ManifestTimeout      = "timeout"
	ManifestInfra        = "infra"
	ManifestNonEvaluable = "non-evaluable"
	ManifestMissing      = "missing"
)

// ManifestCandidateRef links the evaluated candidate manifest without
// copying it: path + run id + content hash.
type ManifestCandidateRef struct {
	ManifestPath string `json:"manifest_path"`
	RunID        string `json:"run_id"`
	SHA256       string `json:"sha256"`
}

// ManifestSplitRef links the versioned exploration/holdout split
// (bench/f7-splits.json) by file hash. Holdout IDs are never stored in
// exploration manifests: the split file holds them, the manifest only
// pins the file.
type ManifestSplitRef struct {
	File    string `json:"file"`
	Name    string `json:"name"`
	Version int    `json:"version"`
	SHA256  string `json:"sha256"`
}

// ManifestBudget mirrors the hard exploration budgets (NOV-58).
type ManifestBudget struct {
	MaxWallTimeMS  int64 `json:"max_wall_time_ms"`
	MaxTokens      int64 `json:"max_tokens"`
	MaxCandidates  int   `json:"max_candidates"`
	MaxConcurrency int   `json:"max_concurrency"`
	MaxRetries     int   `json:"max_retries"`
	MaxDiffBytes   int64 `json:"max_diff_bytes"`
}

// ManifestResult is one task/candidate outcome pointer: status plus
// receipt/detail hashes, never bodies.
type ManifestResult struct {
	TaskID       string `json:"task_id"`
	Status       string `json:"status"`
	ReceiptID    string `json:"receipt_id,omitempty"`
	DetailSHA256 string `json:"detail_sha256,omitempty"`
}

// ManifestDecision is one verdict pointer: verdict plus reason hash.
type ManifestDecision struct {
	Verdict      string `json:"verdict"`
	By           string `json:"by"`
	At           string `json:"at"`
	ReasonSHA256 string `json:"reason_sha256,omitempty"`
	ReceiptID    string `json:"receipt_id,omitempty"`
}

// ExperimentManifest is the compact F7 evidence record.
type ExperimentManifest struct {
	Schema          int                  `json:"schema"`
	Protocol        string               `json:"protocol"`
	ExperimentID    string               `json:"experiment_id"`
	Candidate       ManifestCandidateRef `json:"candidate"`
	Baseline        string               `json:"baseline"`
	BaselineSHA256  string               `json:"baseline_sha256,omitempty"`
	Suite           string               `json:"suite"`
	SuiteVersion    string               `json:"suite_version"`
	Split           ManifestSplitRef     `json:"split"`
	Seed            int64                `json:"seed"`
	ConfigRef       string               `json:"config_ref,omitempty"`
	ConfigSHA256    string               `json:"config_sha256,omitempty"`
	Budget          ManifestBudget       `json:"budget"`
	Evaluator       string               `json:"evaluator"`
	EvaluatorVer    string               `json:"evaluator_version"`
	EvaluatorSHA256 string               `json:"evaluator_sha256,omitempty"`
	Results         []ManifestResult     `json:"results"`
	Decisions       []ManifestDecision   `json:"decisions"`
	Notes           string               `json:"notes,omitempty"`
}

// ManifestSummary is the compact index row: identity plus counts, no
// bodies. ListManifests serves the index; LoadExperimentManifest
// recovers the full record on demand.
type ManifestSummary struct {
	ExperimentID string `json:"experiment_id"`
	Protocol     string `json:"protocol"`
	Suite        string `json:"suite"`
	Results      int    `json:"results"`
	Pass         int    `json:"pass"`
	Infra        int    `json:"infra"`
	Verdict      string `json:"verdict,omitempty"`
}

// ExpManifestDir returns the manifest store under a project root.
func ExpManifestDir(root string) string {
	return filepath.Join(root, ".regulus", "evolution", "manifests")
}

// validManifestStatus reports whether s is a protocol status.
func validManifestStatus(s string) bool {
	switch s {
	case ManifestPass, ManifestFail, ManifestTimeout, ManifestInfra, ManifestNonEvaluable, ManifestMissing:
		return true
	}
	return false
}

// sha256HexBytes hashes bytes.
func sha256HexBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// sha256File hashes a file exactly as stored.
func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256HexBytes(data), nil
}

var (
	manifestBearerRe = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9\-._~+/=]+`)
	manifestKVRe     = regexp.MustCompile(`(?i)\b(api_key|apikey|secret|token|password|credential|authorization|private_key)\b\s*([:=])\s*("[^"]*"|'[^']*'|[^\s,}]+)`)
	manifestTokenRe  = regexp.MustCompile(`\b(ghp_[A-Za-z0-9]+|gho_[A-Za-z0-9]+|sk-live_[A-Za-z0-9_-]+|sk-proj_[A-Za-z0-9_-]+|AKIA[0-9A-Z]{16})\b`)
)

// redactManifestString scrubs one free-text value, mirroring
// almanac.RedactObservation without importing it (evolve stays lean).
// Idempotent: scrubbing twice equals once.
func redactManifestString(s string) string {
	s = manifestTokenRe.ReplaceAllString(s, RedactedMarker)
	s = manifestBearerRe.ReplaceAllString(s, "bearer "+RedactedMarker)
	s = manifestKVRe.ReplaceAllString(s, `$1$2`+RedactedMarker)
	return s
}

// RedactedMarker replaces secret-bearing values.
const RedactedMarker = "[redacted]"

// RedactFreeText scrubs secret-bearing free text before persistence.
// Alias over the manifest redactor so validation plans reuse the exact
// same scrubbing instead of re-implementing it with a different shape.
func RedactFreeText(s string) string { return redactManifestString(s) }

// RedactManifest returns a copy with secret-bearing free text scrubbed
// (baseline label, evaluator label, notes). References, hashes, ids and
// statuses pass through: they are pointers, never secret carriers.
func RedactManifest(m ExperimentManifest) ExperimentManifest {
	m.Baseline = redactManifestString(m.Baseline)
	m.Evaluator = redactManifestString(m.Evaluator)
	m.Notes = redactManifestString(m.Notes)
	return m
}

// ValidateExperimentManifest checks required fields and vocabularies
// without touching disk. Hash-vs-content agreement is Verify's job.
func ValidateExperimentManifest(m ExperimentManifest) error {
	if m.Schema != ExpManifestSchema {
		return fmt.Errorf("evolve: manifest schema %d, want %d", m.Schema, ExpManifestSchema)
	}
	if m.Protocol != F7ProtocolVersion {
		return fmt.Errorf("evolve: manifest protocol %q, want %q", m.Protocol, F7ProtocolVersion)
	}
	if strings.TrimSpace(m.ExperimentID) == "" {
		return errors.New("evolve: manifest needs an experiment id")
	}
	if strings.TrimSpace(m.Candidate.ManifestPath) == "" || strings.TrimSpace(m.Candidate.SHA256) == "" {
		return errors.New("evolve: manifest needs a candidate reference with path and sha256")
	}
	if strings.TrimSpace(m.Baseline) == "" {
		return errors.New("evolve: manifest needs a baseline")
	}
	if strings.TrimSpace(m.Suite) == "" || strings.TrimSpace(m.SuiteVersion) == "" {
		return errors.New("evolve: manifest needs a suite and suite version (bench NOV-35)")
	}
	if strings.TrimSpace(m.Split.File) == "" || strings.TrimSpace(m.Split.Name) == "" || m.Split.Version < 1 || strings.TrimSpace(m.Split.SHA256) == "" {
		return errors.New("evolve: manifest needs a versioned split reference with file, name, version and sha256")
	}
	if m.Seed == 0 {
		return errors.New("evolve: manifest needs a non-zero seed")
	}
	if strings.TrimSpace(m.Evaluator) == "" || strings.TrimSpace(m.EvaluatorVer) == "" {
		return errors.New("evolve: manifest needs an evaluator name and version")
	}
	seen := map[string]bool{}
	for _, r := range m.Results {
		if strings.TrimSpace(r.TaskID) == "" {
			return errors.New("evolve: manifest result needs a task id")
		}
		if !validManifestStatus(r.Status) {
			return fmt.Errorf("evolve: manifest result %s has unknown status %q", r.TaskID, r.Status)
		}
		if seen[r.TaskID] {
			return fmt.Errorf("evolve: manifest duplicates result for %s", r.TaskID)
		}
		seen[r.TaskID] = true
	}
	for _, d := range m.Decisions {
		if strings.TrimSpace(d.Verdict) == "" {
			return errors.New("evolve: manifest decision needs a verdict")
		}
	}
	return nil
}

// WriteExperimentManifest redacts, validates, checks declared hashes
// against current file bytes (when the referenced files exist) and
// stores the manifest atomically. A wrong declared hash is refused
// before anything lands on disk; later drift is caught by Verify.
func WriteExperimentManifest(root string, m ExperimentManifest) (string, error) {
	m = RedactManifest(m)
	if err := ValidateExperimentManifest(m); err != nil {
		return "", err
	}
	resolve := func(ref, declared, kind string) error {
		if ref == "" || declared == "" {
			return nil
		}
		path := ref
		if !filepath.IsAbs(path) && strings.TrimSpace(root) != "" {
			path = filepath.Join(root, ref)
		}
		got, err := sha256File(path)
		if err != nil {
			// Missing reference at write time is allowed only when the
			// caller pins a hash for a file materialized later (e.g. a
			// split generated by the runner); Verify will fail until it
			// exists. Anything else must match now.
			return fmt.Errorf("evolve: manifest %s reference %q unreadable: %v", kind, ref, err)
		}
		if got != declared {
			return fmt.Errorf("evolve: manifest %s hash mismatch for %q: declared %s, disk %s", kind, ref, shortHash(declared), shortHash(got))
		}
		return nil
	}
	if err := resolve(m.Candidate.ManifestPath, m.Candidate.SHA256, "candidate"); err != nil {
		return "", err
	}
	if err := resolve(m.Split.File, m.Split.SHA256, "split"); err != nil {
		return "", err
	}
	if err := resolve(m.ConfigRef, m.ConfigSHA256, "config"); err != nil {
		return "", err
	}
	dir := ExpManifestDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, m.ExperimentID+".json")
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "manifest-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	return path, nil
}

// LoadExperimentManifest recovers one manifest on demand (index stays
// compact; bodies load only here).
func LoadExperimentManifest(root, experimentID string) (ExperimentManifest, error) {
	if strings.TrimSpace(experimentID) == "" {
		return ExperimentManifest{}, errors.New("evolve: manifest load needs an experiment id")
	}
	data, err := os.ReadFile(filepath.Join(ExpManifestDir(root), experimentID+".json"))
	if err != nil {
		return ExperimentManifest{}, fmt.Errorf("evolve: %v", err)
	}
	var m ExperimentManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return ExperimentManifest{}, fmt.Errorf("evolve: malformed manifest %s: %v", experimentID, err)
	}
	return m, nil
}

// VerifyExperimentManifest revalidates structure plus hash agreement
// with current disk bytes. Any drift — moved content, edited split,
// rotated config, candidate rewrite — fails with the offending
// reference named. Missing files fail as missing, never as pass.
func VerifyExperimentManifest(root string, m ExperimentManifest) error {
	if err := ValidateExperimentManifest(m); err != nil {
		return err
	}
	check := func(ref, declared, kind string) error {
		if ref == "" || declared == "" {
			return nil
		}
		path := ref
		if !filepath.IsAbs(path) && strings.TrimSpace(root) != "" {
			path = filepath.Join(root, ref)
		}
		got, err := sha256File(path)
		if err != nil {
			return fmt.Errorf("evolve: manifest %s reference %q missing: %v", kind, ref, err)
		}
		if got != declared {
			return fmt.Errorf("evolve: manifest %s drift for %q: declared %s, disk %s", kind, ref, shortHash(declared), shortHash(got))
		}
		return nil
	}
	if err := check(m.Candidate.ManifestPath, m.Candidate.SHA256, "candidate"); err != nil {
		return err
	}
	if err := check(m.Split.File, m.Split.SHA256, "split"); err != nil {
		return err
	}
	if err := check(m.ConfigRef, m.ConfigSHA256, "config"); err != nil {
		return err
	}
	return nil
}

// SummarizeManifest separates quality from infra (f7-protocol-v1 §5):
// pass/fail drive quality, timeout/infra/non-evaluable/missing stay in
// their own bucket and never dilute the pass rate.
func SummarizeManifest(m ExperimentManifest) (pass, fail, infra int) {
	for _, r := range m.Results {
		switch r.Status {
		case ManifestPass:
			pass++
		case ManifestFail:
			fail++
		case ManifestTimeout, ManifestInfra, ManifestNonEvaluable, ManifestMissing:
			infra++
		}
	}
	return pass, fail, infra
}

// SummarizeManifestIndex folds one summary the same way for the compact
// index.
func SummarizeManifestIndex(s ManifestSummary) (pass, fail, infra int) {
	return s.Pass, 0, s.Infra
}

// ListManifests returns the compact index: identity plus counts, newest
// filenames first, bodies never loaded beyond the summary fold.
func ListManifests(root string) ([]ManifestSummary, error) {
	paths, err := filepath.Glob(filepath.Join(ExpManifestDir(root), "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []ManifestSummary
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var m ExperimentManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		pass, _, infra := SummarizeManifest(m)
		verdict := ""
		if len(m.Decisions) > 0 {
			verdict = m.Decisions[len(m.Decisions)-1].Verdict
		}
		out = append(out, ManifestSummary{
			ExperimentID: m.ExperimentID, Protocol: m.Protocol, Suite: m.SuiteVersion,
			Results: len(m.Results), Pass: pass, Infra: infra, Verdict: verdict,
		})
	}
	return out, nil
}

// RetentionRefs lists what the manifest points at: receipt ids plus
// referenced files. Retention itself lives in Almanac/receipts — the
// manifest owns no GC and stores no bodies.
func RetentionRefs(m ExperimentManifest) []string {
	var out []string
	out = append(out, "candidate:"+m.Candidate.ManifestPath)
	out = append(out, "split:"+m.Split.File)
	if m.ConfigRef != "" {
		out = append(out, "config:"+m.ConfigRef)
	}
	for _, r := range m.Results {
		if r.ReceiptID != "" {
			out = append(out, "receipt:"+r.ReceiptID)
		}
	}
	for _, d := range m.Decisions {
		if d.ReceiptID != "" {
			out = append(out, "receipt:"+d.ReceiptID)
		}
	}
	return out
}

// ResolveReceipt recovers one receipt body on demand by id, scanning
// the evidence store. The manifest keeps the id; this call fetches the
// bytes only when the reader asks.
func ResolveReceipt(root, receiptID string) (string, error) {
	if strings.TrimSpace(receiptID) == "" {
		return "", errors.New("evolve: resolve needs a receipt id")
	}
	paths, err := filepath.Glob(filepath.Join(root, ".regulus", "evidence", "*", "*.json"))
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		if id, _ := raw["id"].(string); id == receiptID {
			return p, nil
		}
	}
	return "", fmt.Errorf("evolve: receipt %s not found under evidence", receiptID)
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
