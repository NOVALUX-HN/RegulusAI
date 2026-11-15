// Agent evidence (NOV-77): versioned, tamper-evident run records.
// `bench agent-tasks --evidence-dir` stores one run as outcomes.json
// plus manifest.json (suite version, model/host, config, git head and
// the outcomes sha256). `bench agent-report` renders ONLY from those
// files — reports are never hand-written. A sha256 mismatch fails
// closed: edited evidence is not evidence.
package bench

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

	"regulus-ai/internal/trace"
)

// AgentRunManifest describes one stored agent-tasks run: everything
// needed to reproduce and audit it, plus the outcomes checksum.
type AgentRunManifest struct {
	SuiteVersion string  `json:"suite_version"`
	Model        string  `json:"model"`
	Host         string  `json:"host"`
	HostVersion  string  `json:"host_version"`
	Variant      string  `json:"variant"`
	Reps         int     `json:"reps"`
	Split        string  `json:"split"`
	Seeds        []int64 `json:"seeds"`
	GitHead      string  `json:"git_head"`
	StartedAt    string  `json:"started_at"`
	FinishedAt   string  `json:"finished_at"`
	OutcomesFile string  `json:"outcomes_file"`
	OutcomesSHA  string  `json:"outcomes_sha256"`
}

// EvidenceFilenames are stable inside an evidence dir: one run, one
// pair of files. A new run uses a new directory.
const (
	EvidenceOutcomesFile = "outcomes.json"
	EvidenceManifestFile = "manifest.json"
)

// WriteAgentEvidence stores one run: outcomes plus a manifest carrying
// the config and the outcomes sha256. The outcomes file name is fixed
// so reports always know where to look.
func WriteAgentEvidence(dir string, outcomes []AgentOutcome, m AgentRunManifest) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(outcomes, "", "  ")
	if err != nil {
		return "", err
	}
	outPath := filepath.Join(dir, EvidenceOutcomesFile)
	if err := os.WriteFile(outPath, append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	m.OutcomesFile = EvidenceOutcomesFile
	m.OutcomesSHA = hex.EncodeToString(sum[:])
	m.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	mraw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	manPath := filepath.Join(dir, EvidenceManifestFile)
	if err := os.WriteFile(manPath, append(mraw, '\n'), 0o644); err != nil {
		return "", err
	}
	return manPath, nil
}

// ReadAgentEvidence loads one evidence dir, verifying the outcomes
// sha256 first: tampered or truncated evidence fails instead of
// producing a pretty report from lies.
func ReadAgentEvidence(dir string) (AgentRunManifest, []AgentOutcome, error) {
	var m AgentRunManifest
	raw, err := os.ReadFile(filepath.Join(dir, EvidenceManifestFile))
	if err != nil {
		return m, nil, fmt.Errorf("reading manifest: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, nil, fmt.Errorf("malformed manifest: %v", err)
	}
	if m.OutcomesFile == "" {
		m.OutcomesFile = EvidenceOutcomesFile
	}
	oraw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(m.OutcomesFile)))
	if err != nil {
		return m, nil, fmt.Errorf("reading outcomes: %v", err)
	}
	sum := sha256.Sum256(bytesTrimNewline(oraw))
	if hex.EncodeToString(sum[:]) != strings.ToLower(strings.TrimSpace(m.OutcomesSHA)) {
		return m, nil, fmt.Errorf("outcomes sha256 mismatch: evidence was modified after recording")
	}
	var outcomes []AgentOutcome
	odec := json.NewDecoder(strings.NewReader(string(oraw)))
	odec.DisallowUnknownFields()
	if err := odec.Decode(&outcomes); err != nil {
		return m, nil, fmt.Errorf("malformed outcomes: %v", err)
	}
	return m, outcomes, nil
}

func bytesTrimNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		return b[:len(b)-1]
	}
	return b
}

// RenderAgentReport builds the human report strictly from a verified
// evidence dir: per-task rep grid, buckets, pass-rate over evaluable
// only, rep spread (variability), and tokens split by measurement
// source (measured-host never mixed with unmeasured).
func RenderAgentReport(m AgentRunManifest, outcomes []AgentOutcome) string {
	var b strings.Builder
	fmt.Fprintf(&b, "agent-tasks %s variant %q model %s host %s",
		m.SuiteVersion, m.Variant, m.Model, m.Host)
	if m.HostVersion != "" {
		fmt.Fprintf(&b, " (%s)", m.HostVersion)
	}
	fmt.Fprintf(&b, " split %s reps %d\n", m.Split, m.Reps)
	fmt.Fprintf(&b, "evidence: %s sha256:%s git %s %s..%s\n",
		m.OutcomesFile, shortSHA(m.OutcomesSHA), m.GitHead, m.StartedAt, m.FinishedAt)
	byTask := map[string][]AgentOutcome{}
	var ids []string
	for _, o := range outcomes {
		if _, ok := byTask[o.TaskID]; !ok {
			ids = append(ids, o.TaskID)
		}
		byTask[o.TaskID] = append(byTask[o.TaskID], o)
	}
	sort.Strings(ids)
	sum := SummarizeAgent(m.Variant, outcomes)
	for _, id := range ids {
		seq := byTask[id]
		parts := make([]string, 0, len(seq))
		tokens := int64(0)
		source := "unknown"
		for _, o := range seq {
			parts = append(parts, string(o.Status))
			tokens += o.TokensIn + o.TokensOut
			if o.TokenSource == trace.SourceMeasuredHost {
				source = trace.SourceMeasuredHost
			}
		}
		fmt.Fprintf(&b, "- %s [%s] %s tokens=%d (%s)\n", id, seq[0].Split, strings.Join(parts, ","), tokens, source)
	}
	fmt.Fprintf(&b, "total %d evaluable %d pass %d fail %d timeout %d infra %d not-run %d pass-rate %.2f rep-spread %.2f-%.2f\n",
		sum.Total, sum.Evaluable, sum.Pass, sum.Fail, sum.Timeout, sum.Infra, sum.NotRun, sum.PassRate, sum.RepMin, sum.RepMax)
	if sum.Evaluable == 0 {
		b.WriteString("verdict: INCONCLUSIVE — nothing evaluable, run again with a usable host\n")
	} else {
		fmt.Fprintf(&b, "verdict: max %.2f over %d evaluable (floor: significance needs volume, see RobustSignificanceFloor)\n", sum.PassRate, sum.Evaluable)
	}
	return b.String()
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
