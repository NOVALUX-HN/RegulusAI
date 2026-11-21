// Package quality is Continuous Quality Intelligence (F12.4/NOV-99):
// detect degradation of quality, performance, cost and compatibility
// from real executions.
//
// The module consumes data, never duplicates stores: observations are
// folded from the F11.1 execution log (observability), F11.3 tier
// manifests, F11.4-style quantile discipline and F11.5 token
// provenance — or from an explicit window file carrying the same
// shape. Baselines are versioned and tamper-evident (manifest +
// sha256, fail closed like tier evidence). Detection uses
// N-adequate methods (median needs MinNMedian, p90 MinNP90, p95
// MinNP95 — the F11.4 minima); below the minimum the diagnosis
// describes and claims nothing. Comparison stays inside one segment
// (agent/model/host/task/version); mixed windows are reported, never
// silently folded, so segmentation controls false positives.
//
// The module proposes investigation, never acts: a Diagnosis carries
// findings, alerts, limitations, a reproduction command and
// investigation next steps. There is no apply/fix path — no function
// here mutates production, and the CLI offers no flag that does.
package quality

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// QualityVersion pins the baseline/diagnosis document shape.
const QualityVersion = "quality-v1"

// Sample minima for quantile claims (F11.4 methodology, restated
// here so the rule travels with the detector): median needs 5
// points, p90 needs 10, p95 needs 20. MinN is the floor for any
// regression claim; below it the diagnosis describes only.
const (
	MinN       = 5
	MinNMedian = 5
	MinNP90    = 10
	MinNP95    = 20
)

// Provenance sources for one observation. window replays a stored
// window file; the rest name the live store the observation folds.
const (
	SourceExec   = "exec"
	SourceLab    = "lab"
	SourceGate   = "gate"
	SourceEval   = "eval"
	SourceWindow = "window"
)

var sources = map[string]bool{
	SourceExec: true, SourceLab: true, SourceGate: true,
	SourceEval: true, SourceWindow: true,
}

// Segment scopes one baseline: agent/model/host/task/version. Empty
// fields are wildcards (rendered as "(any)"); at least the zero
// segment (everything) always matches. Comparison never crosses
// segments: window observations outside the baseline segment are
// excluded and counted, never folded in.
type Segment struct {
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model,omitempty"`
	Host    string `json:"host,omitempty"`
	Task    string `json:"task,omitempty"`
	Version string `json:"version,omitempty"`
}

// dimOrAny renders one dimension: the value or "(any)".
func dimOrAny(v string) string {
	if strings.TrimSpace(v) == "" {
		return "(any)"
	}
	return v
}

// Key renders the segment in canonical field order.
func (s Segment) Key() string {
	return fmt.Sprintf("agent=%s|model=%s|host=%s|task=%s|version=%s",
		dimOrAny(s.Agent), dimOrAny(s.Model), dimOrAny(s.Host),
		dimOrAny(s.Task), dimOrAny(s.Version))
}

// Matches reports whether o falls inside s: every set dimension must
// equal the observation's.
func (s Segment) Matches(o Observation) bool {
	if strings.TrimSpace(s.Agent) != "" && s.Agent != o.Agent {
		return false
	}
	if strings.TrimSpace(s.Model) != "" && s.Model != o.Model {
		return false
	}
	if strings.TrimSpace(s.Host) != "" && s.Host != o.Host {
		return false
	}
	if strings.TrimSpace(s.Task) != "" && s.Task != o.Task {
		return false
	}
	if strings.TrimSpace(s.Version) != "" && s.Version != o.Version {
		return false
	}
	return true
}

// singleLine rejects empty-required and multiline values.
func singleLine(name, v string, required bool) error {
	if strings.TrimSpace(v) == "" {
		if required {
			return fmt.Errorf("quality: %s must not be empty", name)
		}
		return nil
	}
	if strings.ContainsAny(v, "\n\r") {
		return fmt.Errorf("quality: %s must be a single line", name)
	}
	return nil
}

// Observation is one execution folded for detection: success,
// retries, latency, tokens with F11.5 provenance, error signals and
// optional coverage — plus the run/evidence link that lets an alert
// point at cases instead of aggregates.
type Observation struct {
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model,omitempty"`
	Host    string `json:"host,omitempty"`
	Task    string `json:"task,omitempty"`
	Version string `json:"version,omitempty"`

	Success       bool    `json:"success"`
	Retries       int     `json:"retries"`
	LatencyMS     int64   `json:"latency_ms"`
	LatencyKnown  bool    `json:"latency_known"`
	TokensIn      int64   `json:"tokens_in"`
	TokensOut     int64   `json:"tokens_out"`
	TokensKnown   bool    `json:"tokens_known"`
	Errors        int     `json:"errors"`
	CoveragePct   float64 `json:"coverage_pct,omitempty"`
	CoverageKnown bool    `json:"coverage_known"`

	Source   string    `json:"source"`
	RunID    string    `json:"run_id"`
	Evidence string    `json:"evidence,omitempty"`
	At       time.Time `json:"at,omitempty"`
}

func (o Observation) validate() error {
	for name, v := range map[string]string{
		"agent": o.Agent, "model": o.Model, "host": o.Host,
		"task": o.Task, "version": o.Version,
		"evidence": o.Evidence,
	} {
		if err := singleLine(name, v, false); err != nil {
			return err
		}
	}
	if err := singleLine("source", o.Source, true); err != nil {
		return err
	}
	if !sources[o.Source] {
		return fmt.Errorf("quality: unknown source %q", o.Source)
	}
	if err := singleLine("run_id", o.RunID, true); err != nil {
		return err
	}
	if o.Retries < 0 {
		return fmt.Errorf("quality: retries must not be negative, got %d", o.Retries)
	}
	if o.LatencyMS < 0 {
		return fmt.Errorf("quality: latency_ms must not be negative, got %d", o.LatencyMS)
	}
	if o.TokensIn < 0 || o.TokensOut < 0 {
		return fmt.Errorf("quality: tokens must not be negative, got %d/%d", o.TokensIn, o.TokensOut)
	}
	if o.Errors < 0 {
		return fmt.Errorf("quality: errors must not be negative, got %d", o.Errors)
	}
	if o.CoverageKnown && (o.CoveragePct < 0 || o.CoveragePct > 100) {
		return fmt.Errorf("quality: coverage_pct must be within [0,100], got %v", o.CoveragePct)
	}
	return nil
}

// tokensTotal folds one observation's cost when measured.
func (o Observation) tokensTotal() (int64, bool) {
	if !o.TokensKnown {
		return 0, false
	}
	return o.TokensIn + o.TokensOut, true
}

// windowCaps bound one window file: detection reads evidence, never
// transcripts.
const (
	maxWindowBytes   = 2 * 1024 * 1024
	maxWindowEntries = 50000
)

// LoadWindow reads one strict-JSON window file: an array of
// observations. Unknown fields fail; every observation is validated.
// It returns the observations plus the file sha256 so the baseline
// and the diagnosis can cite the exact bytes they consumed.
func LoadWindow(path string) ([]Observation, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(raw) > maxWindowBytes {
		return nil, "", fmt.Errorf("quality: window file too large (%d bytes, max %d)", len(raw), maxWindowBytes)
	}
	var obs []Observation
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&obs); err != nil {
		return nil, "", fmt.Errorf("quality: malformed window: %v", err)
	}
	if len(obs) > maxWindowEntries {
		return nil, "", fmt.Errorf("quality: window has %d entries (max %d)", len(obs), maxWindowEntries)
	}
	for i := range obs {
		if obs[i].Source == "" {
			obs[i].Source = SourceWindow
		}
		if err := obs[i].validate(); err != nil {
			return nil, "", fmt.Errorf("quality: window entry %d: %v", i, err)
		}
	}
	sum := sha256.Sum256(raw)
	return obs, hex.EncodeToString(sum[:]), nil
}

// Select partitions a window by segment: matched observations
// drive the comparison, the rest are counted as excluded (mixed
// window) so segmentation stays visible to callers.
func Select(seg Segment, obs []Observation) (matched, excluded []Observation) {
	return splitWindow(seg, obs)
}

// splitWindow partitions a window by segment: matched observations
// drive the comparison, the rest are counted as excluded (mixed
// window) so segmentation stays visible.
func splitWindow(seg Segment, obs []Observation) (matched, excluded []Observation) {
	for _, o := range obs {
		if seg.Matches(o) {
			matched = append(matched, o)
		} else {
			excluded = append(excluded, o)
		}
	}
	return matched, excluded
}

// windowSpan bounds the matched observations in time; zero times
// (lab replays without a clock) leave the span unknown.
func windowSpan(obs []Observation) (from, to time.Time, known bool) {
	for _, o := range obs {
		if o.At.IsZero() {
			continue
		}
		if !known || o.At.Before(from) {
			from = o.At
		}
		if !known || o.At.After(to) {
			to = o.At
		}
		known = true
	}
	return from, to, known
}

// breakdown counts matched+excluded observations by one dimension so
// the diagnosis can expose confounds (a bad host hiding inside a
// mixed window).
func breakdown(obs []Observation, dim func(Observation) string) []string {
	counts := map[string]int{}
	for _, o := range obs {
		counts[dimOrAny(dim(o))]++
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	return out
}
