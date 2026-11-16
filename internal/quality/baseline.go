// Baseline construction and versioned evidence (F12.4/NOV-99).
//
// A Baseline freezes one segment's reference statistics plus the
// thresholds it ran with (echoed verbatim, never auto-tuned) and the
// evidence refs (source, path, sha256, git head) that reproduce it.
// WriteBaselineEvidence stores baseline.json with a manifest carrying
// its sha256; ReadBaselineEvidence verifies before use, failing
// closed like F11.3 tier evidence: edited baselines are not evidence.
package quality

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Thresholds carry the detection rules. Every report echoes the exact
// values it ran with, so a reader always knows which rule fired.
type Thresholds struct {
	SuccessDropAbs float64 `json:"success_drop_abs"`
	ErrorRiseAbs   float64 `json:"error_rise_abs"`
	RetryRiseAbs   float64 `json:"retry_rise_abs"`
	LatencyRisePct float64 `json:"latency_rise_pct"`
	TokensRisePct  float64 `json:"tokens_rise_pct"`
	CoverageDrop   float64 `json:"coverage_drop_abs"`
}

// DefaultThresholds returns the documented rule set: success falling
// 10 points, errors rising 10 points, mean retries up by 1, median
// latency up 20%, measured tokens up 25%, coverage down 5 points.
func DefaultThresholds() Thresholds {
	return Thresholds{
		SuccessDropAbs: 0.10,
		ErrorRiseAbs:   0.10,
		RetryRiseAbs:   1.0,
		LatencyRisePct: 0.20,
		TokensRisePct:  0.25,
		CoverageDrop:   0.05,
	}
}

func (t Thresholds) validate() error {
	for name, v := range map[string]float64{
		"success_drop_abs": t.SuccessDropAbs, "error_rise_abs": t.ErrorRiseAbs,
		"latency_rise_pct": t.LatencyRisePct, "tokens_rise_pct": t.TokensRisePct,
		"coverage_drop_abs": t.CoverageDrop,
	} {
		if v <= 0 || v > 1 {
			return fmt.Errorf("quality: %s must be within (0,1], got %v", name, v)
		}
	}
	if t.RetryRiseAbs <= 0 || t.RetryRiseAbs > 100 {
		return fmt.Errorf("quality: retry_rise_abs must be within (0,100], got %v", t.RetryRiseAbs)
	}
	return nil
}

// LoadThresholds reads one strict-JSON thresholds file: unknown
// fields fail, omitted keys keep documented defaults.
func LoadThresholds(path string) (Thresholds, error) {
	th := DefaultThresholds()
	raw, err := os.ReadFile(path)
	if err != nil {
		return Thresholds{}, err
	}
	if len(raw) > 64*1024 {
		return Thresholds{}, fmt.Errorf("quality: thresholds file too large")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&th); err != nil {
		return Thresholds{}, fmt.Errorf("quality: malformed thresholds: %v", err)
	}
	if err := th.validate(); err != nil {
		return Thresholds{}, err
	}
	return th, nil
}

// Stats freezes one window's reference figures. Quantile validity
// follows the N minima: median needs MinNMedian, p90 MinNP90, p95
// MinNP95; anything below reads as invalid, never as zero.
type Stats struct {
	N           int     `json:"n"`
	SuccessRate float64 `json:"success_rate"`
	ErrorRate   float64 `json:"error_rate"`
	MeanRetries float64 `json:"mean_retries"`
	MedianMS    int64   `json:"median_ms"`
	MedianValid bool    `json:"median_valid"`
	P90MS       int64   `json:"p90_ms"`
	P90Valid    bool    `json:"p90_valid"`
	P95MS       int64   `json:"p95_ms"`
	P95Valid    bool    `json:"p95_valid"`
	MeanTokens  float64 `json:"mean_tokens"`
	TokensKnown bool    `json:"tokens_known"`
	CoverageAvg float64 `json:"coverage_avg,omitempty"`
	CoverAvgOK  bool    `json:"coverage_known"`
}

// summarize folds matched observations into reference statistics.
// Pure: no thresholds, no claims, only arithmetic.
func summarize(obs []Observation) Stats {
	s := Stats{N: len(obs)}
	if len(obs) == 0 {
		return s
	}
	var succ, errs float64
	var retries float64
	var lats []int64
	var toks float64
	var toksN int
	var cov float64
	var covN int
	for _, o := range obs {
		if o.Success {
			succ++
		}
		if o.Errors > 0 {
			errs++
		}
		retries += float64(o.Retries)
		if o.LatencyKnown {
			lats = append(lats, o.LatencyMS)
		}
		if t, ok := o.tokensTotal(); ok {
			toks += float64(t)
			toksN++
		}
		if o.CoverageKnown {
			cov += o.CoveragePct
			covN++
		}
	}
	n := float64(len(obs))
	s.SuccessRate = succ / n
	s.ErrorRate = errs / n
	s.MeanRetries = retries / n
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	if len(lats) >= MinNMedian {
		s.MedianMS, s.MedianValid = quantile(lats, 0.5), true
	}
	if len(lats) >= MinNP90 {
		s.P90MS, s.P90Valid = quantile(lats, 0.90), true
	}
	if len(lats) >= MinNP95 {
		s.P95MS, s.P95Valid = quantile(lats, 0.95), true
	}
	if toksN > 0 {
		s.MeanTokens, s.TokensKnown = toks/float64(toksN), true
	}
	if covN > 0 {
		s.CoverageAvg, s.CoverAvgOK = cov/float64(covN), true
	}
	return s
}

// quantile renders the nearest-rank quantile over ascending values.
func quantile(ascending []int64, q float64) int64 {
	if len(ascending) == 0 {
		return 0
	}
	rank := int(q*float64(len(ascending)) + 0.5)
	if rank < 1 {
		rank = 1
	}
	if rank > len(ascending) {
		rank = len(ascending)
	}
	return ascending[rank-1]
}

// EvidenceRef cites one consumed source: kind, path, content sha256
// and git head, so any baseline or diagnosis reproduces from the
// exact bytes it read.
type EvidenceRef struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	GitHead string `json:"git_head,omitempty"`
	Note    string `json:"note,omitempty"`
}

// Baseline is one segment's versioned reference: statistics,
// thresholds echoed verbatim, window span and evidence refs.
type Baseline struct {
	Version    string        `json:"version"`
	Segment    Segment       `json:"segment"`
	From       time.Time     `json:"from,omitempty"`
	To         time.Time     `json:"to,omitempty"`
	N          int           `json:"n"`
	Stats      Stats         `json:"stats"`
	Thresholds Thresholds    `json:"thresholds"`
	Evidence   []EvidenceRef `json:"evidence"`
	CreatedAt  time.Time     `json:"created_at"`
	GitHead    string        `json:"git_head,omitempty"`
}

// BuildBaseline folds matched observations into a baseline over seg.
// Observations outside seg are refused (use splitWindow first and
// cite the exclusion); an empty match is an error, never a baseline.
func BuildBaseline(seg Segment, matched []Observation, th Thresholds, ev []EvidenceRef, now time.Time) (Baseline, error) {
	if err := th.validate(); err != nil {
		return Baseline{}, err
	}
	if len(matched) == 0 {
		return Baseline{}, fmt.Errorf("quality: no observations match segment %s", seg.Key())
	}
	for i, o := range matched {
		if !seg.Matches(o) {
			return Baseline{}, fmt.Errorf("quality: observation %d outside segment %s", i, seg.Key())
		}
	}
	from, to, known := windowSpan(matched)
	b := Baseline{
		Version: QualityVersion, Segment: seg,
		N: len(matched), Stats: summarize(matched),
		Thresholds: th, Evidence: ev, CreatedAt: now.UTC(),
	}
	if known {
		b.From, b.To = from.UTC(), to.UTC()
	}
	return b, nil
}

// Evidence filenames inside a baseline dir: one baseline, one pair
// of files. A new baseline uses a new directory.
const (
	BaselineFile = "baseline.json"
	BaselineMeta = "manifest.json"
)

// BaselineManifest describes one stored baseline: everything needed
// to reproduce and audit it, plus the baseline checksum.
type BaselineManifest struct {
	Version      string `json:"version"`
	SegmentKey   string `json:"segment"`
	BaselineFile string `json:"baseline_file"`
	BaselineSHA  string `json:"baseline_sha256"`
	GitHead      string `json:"git_head"`
	CreatedAt    string `json:"created_at"`
}

// WriteBaselineEvidence stores one baseline plus a manifest carrying
// its sha256. The baseline file name is fixed so checks always know
// where to look.
func WriteBaselineEvidence(dir string, b Baseline, gitHead string) (string, error) {
	if b.Version != QualityVersion {
		return "", fmt.Errorf("quality: refusing to store baseline version %q", b.Version)
	}
	b.GitHead = gitHead
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, BaselineFile), append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	m := BaselineManifest{
		Version: QualityVersion, SegmentKey: b.Segment.Key(),
		BaselineFile: BaselineFile,
		BaselineSHA:  hex.EncodeToString(sum[:]),
		GitHead:      gitHead,
		CreatedAt:    b.CreatedAt.UTC().Format(time.RFC3339),
	}
	mraw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	manPath := filepath.Join(dir, BaselineMeta)
	if err := os.WriteFile(manPath, append(mraw, '\n'), 0o644); err != nil {
		return "", err
	}
	return manPath, nil
}

// ReadBaselineEvidence loads one baseline evidence dir, verifying
// the baseline sha256 first: edited baselines fail instead of
// feeding detection from a lie.
func ReadBaselineEvidence(dir string) (BaselineManifest, Baseline, error) {
	var m BaselineManifest
	var b Baseline
	raw, err := os.ReadFile(filepath.Join(dir, BaselineMeta))
	if err != nil {
		return m, b, fmt.Errorf("reading quality manifest: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, b, fmt.Errorf("malformed quality manifest: %v", err)
	}
	if m.BaselineFile == "" {
		m.BaselineFile = BaselineFile
	}
	praw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(m.BaselineFile)))
	if err != nil {
		return m, b, fmt.Errorf("reading quality baseline: %v", err)
	}
	trimmed := bytesTrimNewline(praw)
	sum := sha256.Sum256(trimmed)
	if hex.EncodeToString(sum[:]) != strings.ToLower(strings.TrimSpace(m.BaselineSHA)) {
		return m, b, fmt.Errorf("quality baseline sha256 mismatch: evidence was modified after recording")
	}
	pdec := json.NewDecoder(strings.NewReader(string(praw)))
	pdec.DisallowUnknownFields()
	if err := pdec.Decode(&b); err != nil {
		return m, b, fmt.Errorf("malformed quality baseline: %v", err)
	}
	if b.Version != QualityVersion {
		return m, b, fmt.Errorf("quality: baseline version %q is not %q", b.Version, QualityVersion)
	}
	return m, b, nil
}

// bytesTrimNewline drops one trailing newline before hashing: the
// writer appends it, the hash covers the document.
func bytesTrimNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}
