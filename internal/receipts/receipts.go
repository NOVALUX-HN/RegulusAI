// Package receipts writes append-only evidence receipts: the Regulus
// Gauntlet's proof-of-work. A receipt is written once, never modified, and
// never overwritten (PRD R-G4: a claim without a receipt is not evidence).
package receipts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MaxCapture bounds the stdout/stderr text stored verbatim in a receipt;
// the full content is always pinned by its SHA-256.
const MaxCapture = 8192

// truncMarker notes that the stored text is a prefix of the full output,
// whose integrity is pinned by the *_sha256 field.
const truncMarker = "\n…[truncated; full output pinned by sha256]"

// Verdicts.
const (
	VerdictPass = "pass"
	VerdictFail = "fail"
)

// Receipt records one gate execution.
type Receipt struct {
	ID         string    `json:"id"`
	Task       string    `json:"task"`
	Gate       string    `json:"gate"`
	Layer      int       `json:"layer"`
	Command    string    `json:"command"`
	ExitCode   int       `json:"exit_code"`
	Verdict    string    `json:"verdict"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int64     `json:"duration_ms"`
	Stdout     string    `json:"stdout"`
	StdoutSHA  string    `json:"stdout_sha256"`
	Stderr     string    `json:"stderr"`
	StderrSHA  string    `json:"stderr_sha256"`
	GitHead    string    `json:"git_head"`

	// Metric fields are set by metric gates (F2) whenever the metric was
	// extracted, pass or fail. nil MetricValue means: not a metric gate,
	// or extraction refused (see the receipt stderr).
	MetricValue     *float64 `json:"metric_value,omitempty"`
	MetricThreshold *float64 `json:"metric_threshold,omitempty"`
	MetricOperator  string   `json:"metric_operator,omitempty"`
}

// Capture bounds s, marking truncation, and returns the SHA-256 of the
// FULL content.
func Capture(s string) (text, sha string) {
	sum := sha256.Sum256([]byte(s))
	sha = hex.EncodeToString(sum[:])
	if len(s) > MaxCapture {
		return s[:MaxCapture] + truncMarker, sha
	}
	return s, sha
}

// Slug turns a task id into a safe directory name ("/" becomes "__").
func Slug(task string) string {
	return strings.ReplaceAll(task, "/", "__")
}

// Write appends one receipt under dir as <gate>-<nanos>.json. It never
// overwrites: a name collision advances to the next free nanosecond slot.
// Write assigns the receipt ID (tied to the slot it landed on) when empty
// and returns the file path.
func Write(dir string, r Receipt) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ts := r.StartedAt.UnixNano()
	base := fileNameSafe(r.Gate)
	for {
		name := fmt.Sprintf("%s-%d.json", base, ts)
		path := filepath.Join(dir, name)
		if r.ID == "" {
			r.ID = "rec_" + strconv.FormatInt(ts, 16)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if os.IsExist(err) {
			ts++
			r.ID = ""
			continue
		}
		if err != nil {
			return "", err
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		return path, nil
	}
}

// ReadFile parses one receipt written by Write.
func ReadFile(path string) (Receipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Receipt{}, err
	}
	var r Receipt
	if err := json.Unmarshal(data, &r); err != nil {
		return Receipt{}, fmt.Errorf("%s: malformed receipt: %v", path, err)
	}
	return r, nil
}

// fileNameSafe maps a gate name to a conservative file-name fragment.
func fileNameSafe(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "gate"
	}
	return b.String()
}
