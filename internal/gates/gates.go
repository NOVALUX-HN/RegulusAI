// Package gates runs the layered quality gauntlet (PRD R-G2): gates
// execute in ascending layer order, cheap and binary first; a failing layer
// stops the gauntlet — no gate runs after a failure.
package gates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"regulus-ai/internal/receipts"
)

// Gate is one configured quality gate. A gate with a Metric is a metric
// gate: the command must exit 0 AND the extracted metric must satisfy the
// operator against the threshold (F2, PRD R-G2 layers 5–7).
type Gate struct {
	Name       string  `json:"name"`
	Layer      int     `json:"layer"`
	Command    string  `json:"command"`
	TimeoutSec int     `json:"timeout_sec,omitempty"`
	Metric     *Metric `json:"metric,omitempty"`
}

// Metric extracts a number from the gate command's stdout and compares it
// against a threshold.
type Metric struct {
	Extract   string  `json:"extract"` // ExtractLastLineFloat | ExtractLastLinePercent
	Threshold float64 `json:"threshold"`
	Operator  string  `json:"operator"` // ">=" | "<="
}

// Metric extraction modes.
const (
	ExtractLastLineFloat   = "last-line-float"
	ExtractLastLinePercent = "last-line-percent"
)

// Metric comparison operators.
const (
	OpAtLeast = ">="
	OpAtMost  = "<="
)

// EvolutionConfig caps the evolution loop (PRD R-E4): reflect and probe
// runs per calendar day. The ledger records every run, refused ones
// included.
type EvolutionConfig struct {
	MaxPerDay int `json:"max_per_day,omitempty"`
}

// Config is the project's regulus.json.
type Config struct {
	Gates     []Gate           `json:"gates"`
	Evolution *EvolutionConfig `json:"evolution,omitempty"`
}

// DefaultTimeoutSec bounds one gate when no explicit timeout is configured.
const DefaultTimeoutSec = 600

// CmdRunner executes one shell command. The real one goes through a POSIX
// sh; tests substitute fakes.
type CmdRunner interface {
	Run(ctx context.Context, command string) (stdout, stderr string, exitCode int, err error)
}

// ShRunner executes commands via `sh -c` so configured gates may use shell
// syntax. err is nil for any completed run (including non-zero exits); it
// is non-nil only when no verdict could be produced (spawn failure, kill,
// timeout).
type ShRunner struct{}

func (ShRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err == nil {
		return out.String(), errb.String(), 0, nil
	}
	// A deadline kill surfaces as an ExitError on Windows (exit code 1)
	// and as a signal ExitError on Unix; the context check must come first
	// or a timeout masquerades as an ordinary failure.
	if ctx.Err() != nil {
		return out.String(), errb.String(), -1, fmt.Errorf("timed out: %w", ctx.Err())
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), errb.String(), ee.ExitCode(), nil
	}
	return out.String(), errb.String(), -1, err
}

// Outcome is one executed gate's summary.
type Outcome struct {
	Name       string
	Layer      int
	Verdict    string
	DurationMS int64
}

// Result is one gauntlet run.
type Result struct {
	Executed   int
	Passed     int
	Failed     bool
	FailedGate string
	Outcomes   []Outcome
	Receipts   []string
}

// Load reads and validates the gates config at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("no regulus.json found at %s — create one with a gates array", path)
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("malformed regulus.json: %v", err)
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validate enforces the config contract: at least one gate, unique names and
// layers, layers >= 1, non-empty names and commands.
func validate(cfg Config) error {
	if len(cfg.Gates) == 0 {
		return errors.New("no gates configured")
	}
	seenLayer := map[int]string{}
	seenName := map[string]bool{}
	for i, g := range cfg.Gates {
		if g.Name == "" {
			return fmt.Errorf("gates[%d]: name must not be empty", i)
		}
		if g.Command == "" {
			return fmt.Errorf("gates[%d]: command must not be empty", i)
		}
		if g.Layer < 1 {
			return fmt.Errorf("gates[%d]: layer must be 1 or greater", i)
		}
		if prev, dup := seenLayer[g.Layer]; dup {
			return fmt.Errorf("duplicate layer %d: gates %q and %q", g.Layer, prev, g.Name)
		}
		seenLayer[g.Layer] = g.Name
		if seenName[g.Name] {
			return fmt.Errorf("duplicate gate name %q", g.Name)
		}
		seenName[g.Name] = true
		if g.Metric != nil {
			switch g.Metric.Operator {
			case OpAtLeast, OpAtMost:
			default:
				return fmt.Errorf("gates[%d]: metric operator must be >= or <=", i)
			}
			switch g.Metric.Extract {
			case ExtractLastLineFloat, ExtractLastLinePercent:
			default:
				return fmt.Errorf("gates[%d]: metric extract must be last-line-float or last-line-percent", i)
			}
		}
	}
	return nil
}

// Run executes the gauntlet for one task, writing one append-only receipt
// per executed gate into evidenceDir. It stops at the first failure.
func Run(ctx context.Context, cfg Config, evidenceDir, task, gitHead string, r CmdRunner) (Result, error) {
	if err := validate(cfg); err != nil {
		return Result{}, err
	}
	ordered := append([]Gate(nil), cfg.Gates...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Layer < ordered[j].Layer })

	var res Result
	for _, g := range ordered {
		timeout := g.TimeoutSec
		if timeout <= 0 {
			timeout = DefaultTimeoutSec
		}
		gctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		start := time.Now().UTC()
		stdout, stderr, code, rerr := r.Run(gctx, g.Command)
		dur := time.Since(start)
		cancel()

		verdict := receipts.VerdictPass
		exit := code
		switch {
		case rerr == nil && code == 0:
		case rerr != nil:
			verdict = receipts.VerdictFail
			if exit == 0 {
				exit = -1
			}
			note := "gate could not run: " + rerr.Error()
			if stderr == "" {
				stderr = note
			} else {
				stderr += "\n" + note
			}
		default:
			verdict = receipts.VerdictFail
		}

		// Metric gates (F2): the command must exit 0 AND the extracted
		// metric must satisfy the operator against the threshold.
		var metricValue, metricThreshold *float64
		var metricOp string
		if verdict == receipts.VerdictPass && g.Metric != nil {
			metricOp = g.Metric.Operator
			value, err := extractMetric(g.Metric.Extract, stdout)
			if err != nil {
				verdict = receipts.VerdictFail
				note := "metric: " + err.Error()
				if stderr == "" {
					stderr = note
				} else {
					stderr += "\n" + note
				}
			} else {
				metricValue = &value
				th := g.Metric.Threshold
				metricThreshold = &th
				ok := (g.Metric.Operator == OpAtLeast && value >= th) ||
					(g.Metric.Operator == OpAtMost && value <= th)
				if !ok {
					verdict = receipts.VerdictFail
					note := fmt.Sprintf("metric %.2f does not meet %s %.2f", value, g.Metric.Operator, th)
					if stderr == "" {
						stderr = note
					} else {
						stderr += "\n" + note
					}
				}
			}
		}

		rec := receipts.Receipt{
			Task:       task,
			Gate:       g.Name,
			Layer:      g.Layer,
			Command:    g.Command,
			ExitCode:   exit,
			Verdict:    verdict,
			StartedAt:  start,
			DurationMS: dur.Milliseconds(),
			GitHead:    gitHead,

			MetricValue:     metricValue,
			MetricThreshold: metricThreshold,
			MetricOperator:  metricOp,
		}
		rec.Stdout, rec.StdoutSHA = receipts.Capture(stdout)
		rec.Stderr, rec.StderrSHA = receipts.Capture(stderr)

		path, err := receipts.Write(evidenceDir, rec)
		if err != nil {
			return res, fmt.Errorf("gate %s: writing receipt: %w", g.Name, err)
		}
		res.Executed++
		res.Receipts = append(res.Receipts, path)
		res.Outcomes = append(res.Outcomes, Outcome{Name: g.Name, Layer: g.Layer, Verdict: verdict, DurationMS: dur.Milliseconds()})
		if verdict == receipts.VerdictFail {
			res.Failed = true
			res.FailedGate = g.Name
			return res, nil
		}
		res.Passed++
	}
	return res, nil
}

// GitHead returns the current HEAD hash, or "" outside a repository.
func GitHead(ctx context.Context, r CmdRunner) string {
	out, _, _, err := r.Run(ctx, "git rev-parse --verify HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

var metricNumberRe = regexp.MustCompile(`[-+]?[0-9]*\.?[0-9]+`)

// extractMetric pulls one number out of a metric gate's stdout. Both modes
// read the last non-empty line; the percent mode strips a trailing "%"
// first. Within that line the LAST numeric token wins.
func extractMetric(extract, stdout string) (float64, error) {
	lines := strings.Split(stdout, "\n")
	var last string
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			last = s
			break
		}
	}
	if last == "" {
		return 0, errors.New("could not extract a metric: the command printed nothing")
	}
	if extract == ExtractLastLinePercent {
		last = strings.TrimSuffix(last, "%")
	}
	matches := metricNumberRe.FindAllString(last, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("could not extract a metric from %q", last)
	}
	v, err := strconv.ParseFloat(matches[len(matches)-1], 64)
	if err != nil {
		return 0, fmt.Errorf("could not extract a metric from %q", last)
	}
	return v, nil
}
