// Rendering and versioned alert evidence (F12.4/NOV-99).
//
// RenderDiagnosis writes the human report strictly from a Diagnosis;
// WriteDiagnosisEvidence stores that same diagnosis with a manifest
// carrying its sha256 (redacción previa al store: the prose is
// rendered first, then stored verbatim). ReadDiagnosisEvidence
// verifies before use, failing closed like every evidence dir.
package quality

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

// Evidence filenames inside a diagnosis dir: one check, one pair of
// files. A new check uses a new directory.
const (
	DiagnosisFile = "diagnosis.json"
	DiagnosisMeta = "manifest.json"
)

// DiagnosisManifest describes one stored diagnosis: the check inputs
// and the diagnosis checksum.
type DiagnosisManifest struct {
	Version       string `json:"version"`
	SegmentKey    string `json:"segment"`
	DiagnosisFile string `json:"diagnosis_file"`
	DiagnosisSHA  string `json:"diagnosis_sha256"`
	BaselineSHA   string `json:"baseline_sha256,omitempty"`
	WindowSHA     string `json:"window_sha256,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// WriteDiagnosisEvidence stores one diagnosis plus a manifest with
// its sha256 and the input shas, so reproduction can pin the exact
// bytes on both sides.
func WriteDiagnosisEvidence(dir string, d Diagnosis, baselineSHA, windowSHA string) (string, error) {
	if d.Version != QualityVersion {
		return "", fmt.Errorf("quality: refusing to store diagnosis version %q", d.Version)
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, DiagnosisFile), append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	m := DiagnosisManifest{
		Version: QualityVersion, SegmentKey: d.SegmentKey,
		DiagnosisFile: DiagnosisFile,
		DiagnosisSHA:  hex.EncodeToString(sum[:]),
		BaselineSHA:   baselineSHA, WindowSHA: windowSHA,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	mraw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	manPath := filepath.Join(dir, DiagnosisMeta)
	if err := os.WriteFile(manPath, append(mraw, '\n'), 0o644); err != nil {
		return "", err
	}
	return manPath, nil
}

// ReadDiagnosisEvidence loads one stored diagnosis, verifying its
// sha256 first: edited diagnoses fail instead of rendering alerts
// from a lie.
func ReadDiagnosisEvidence(dir string) (DiagnosisManifest, Diagnosis, error) {
	var m DiagnosisManifest
	var d Diagnosis
	raw, err := os.ReadFile(filepath.Join(dir, DiagnosisMeta))
	if err != nil {
		return m, d, fmt.Errorf("reading quality manifest: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, d, fmt.Errorf("malformed quality manifest: %v", err)
	}
	if m.DiagnosisFile == "" {
		m.DiagnosisFile = DiagnosisFile
	}
	praw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(m.DiagnosisFile)))
	if err != nil {
		return m, d, fmt.Errorf("reading quality diagnosis: %v", err)
	}
	sum := sha256.Sum256(bytesTrimNewline(praw))
	if hex.EncodeToString(sum[:]) != strings.ToLower(strings.TrimSpace(m.DiagnosisSHA)) {
		return m, d, fmt.Errorf("quality diagnosis sha256 mismatch: evidence was modified after recording")
	}
	pdec := json.NewDecoder(strings.NewReader(string(praw)))
	pdec.DisallowUnknownFields()
	if err := pdec.Decode(&d); err != nil {
		return m, d, fmt.Errorf("malformed quality diagnosis: %v", err)
	}
	if d.Version != QualityVersion {
		return m, d, fmt.Errorf("quality: diagnosis version %q is not %q", d.Version, QualityVersion)
	}
	return m, d, nil
}

// RenderDiagnosis builds the human check report strictly from a
// diagnosis: segment, baseline, window, N, findings with methods,
// alerts with magnitude and cases, limitations, investigation steps
// and the reproduction command.
func RenderDiagnosis(d Diagnosis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "quality %s segment %s\n", d.Version, d.SegmentKey)
	fmt.Fprintf(&b, "baseline n=%d, window n=%d matched + %d excluded\n", d.BaselineN, d.WindowN, d.ExcludedN)
	if len(d.Findings) == 0 {
		fmt.Fprintf(&b, "no metric comparisons ran (see limitations)\n")
	}
	for _, f := range d.Findings {
		mark := "ok"
		if f.Alerted {
			mark = "ALERT"
		}
		fmt.Fprintf(&b, "[%s] %s: baseline=%.3f observed=%.3f (Δ=%+.3f) | %s | rule %s\n",
			mark, f.Metric, f.Baseline, f.Observed, f.DeltaAbs, f.Method, f.Threshold)
	}
	if len(d.Alerts) == 0 {
		fmt.Fprintf(&b, "no regressions: all rules within threshold\n")
	}
	for _, a := range d.Alerts {
		fmt.Fprintf(&b, "ALERT %s\n  magnitude: %s\n", a.Metric, a.Magnitude)
		fmt.Fprintf(&b, "  cases: %s\n", joinOrNone(a.Cases))
		for _, ev := range a.Evidence {
			fmt.Fprintf(&b, "  evidence: %s\n", ev)
		}
		fmt.Fprintf(&b, "  limit: %s\n", a.Limitation)
		fmt.Fprintf(&b, "  investigate: %s\n", a.NextStep)
	}
	for _, l := range d.Breakdown {
		fmt.Fprintf(&b, "mix: %s\n", l)
	}
	for _, l := range d.Limitations {
		fmt.Fprintf(&b, "limit: %s\n", l)
	}
	for _, s := range d.NextSteps {
		fmt.Fprintf(&b, "next: %s\n", s)
	}
	fmt.Fprintf(&b, "reproduce: %s\n", d.Reproduce)
	return b.String()
}

// RenderAlerts builds the alerts-only view from a stored diagnosis:
// one line per alert plus limitations and the reproduction command.
func RenderAlerts(d Diagnosis) string {
	var b strings.Builder
	if len(d.Alerts) == 0 {
		fmt.Fprintf(&b, "quality %s: no stored alerts for %s\n", d.Version, d.SegmentKey)
	}
	for _, a := range d.Alerts {
		fmt.Fprintf(&b, "ALERT %s: %s\n", a.Metric, a.Magnitude)
		fmt.Fprintf(&b, "  cases: %s\n", joinOrNone(a.Cases))
		fmt.Fprintf(&b, "  limit: %s\n", a.Limitation)
	}
	for _, l := range d.Limitations {
		fmt.Fprintf(&b, "limit: %s\n", l)
	}
	fmt.Fprintf(&b, "reproduce: %s\n", d.Reproduce)
	return b.String()
}
