// `regulus recovery progress` — the command that makes the F17 library real.
//
// It reads a run from the store, turns the recorded history into samples, asks
// recovery.Classify for a verdict, and shows the measured outcomes recorded
// alongside the run. Without this, both the classifier and the outcome record
// were a tested library nobody called.
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"sort"
	"time"

	"regulus-ai/internal/recovery"
)

// progressSample maps one recorded attempt to a classifier sample.
//
// The mapping is deliberately mechanical: an attempt that reached succeeded or
// checkpointed advanced the work, and any other recorded state did not. The
// failure class, when the attempt recorded one, is the signature, so two
// failures of the same class read as a repeat while failures of different
// classes read as a strategy that is not converging.
func progressSample(note recovery.AttemptNote) recovery.Sample {
	progressed := note.State == recovery.StateSucceeded || note.State == recovery.StateCheckpointed
	sig := note.Class
	if sig == "" {
		sig = note.State
	}
	if progressed {
		sig = ""
	}
	return recovery.Sample{Signature: sig, Progressed: progressed}
}

// progressSamples builds the classifier input for a run. Checkpoints are
// visited in a stable order so the same run always yields the same verdict,
// and --task narrows the scope to one task.
func progressSamples(rec *recovery.RunRecord, onlyTask string) []recovery.Sample {
	keys := make([]string, 0, len(rec.Checks))
	for key := range rec.Checks {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var samples []recovery.Sample
	for _, key := range keys {
		c := rec.Checks[key]
		if c == nil || (onlyTask != "" && c.TaskID != onlyTask) {
			continue
		}
		for _, note := range c.History {
			samples = append(samples, progressSample(note))
		}
	}
	return samples
}

// progressReport is the --json shape. It carries the verdict and the measured
// outcomes, never a plan.
type progressReport struct {
	RunID      string                     `json:"run_id"`
	Task       string                     `json:"task,omitempty"`
	Samples    int                        `json:"samples"`
	Class      string                     `json:"class"`
	Escalate   bool                       `json:"escalate"`
	Suppressed bool                       `json:"suppressed"`
	Reasons    []string                   `json:"reasons"`
	Outcomes   []recovery.StrategyOutcome `json:"outcomes,omitempty"`
}

// RecoveryProgressCmd implements `regulus recovery progress`.
func (io IO) RecoveryProgressCmd(args []string) int {
	fs := flag.NewFlagSet("recovery progress", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	run := fs.String("run", "", "run id (required)")
	task := fs.String("task", "", "only this task (optional)")
	asJSON := fs.Bool("json", false, "print the verdict as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *run == "" {
		fmt.Fprintln(io.Stderr, recoveryUsage())
		return 2
	}
	rec, code, ok := loadRecoveryRun(io.Dir, *run, io.Stderr)
	if !ok {
		return code
	}

	samples := progressSamples(rec, *task)
	verdict := recovery.Classify(recovery.ClassifyInput{
		Samples: samples,
		Policy:  recovery.DefaultProgressPolicy(),
		NowMS:   time.Now().UTC().UnixMilli(),
	})
	outcomes, err := recovery.LoadOutcomes(io.Dir, *run)
	if err != nil {
		fmt.Fprintf(io.Stderr, "recovery progress: %v\n", err)
		return 1
	}

	if *asJSON {
		data, merr := json.MarshalIndent(progressReport{
			RunID: *run, Task: *task, Samples: len(samples),
			Class: verdict.Class, Escalate: verdict.Escalate, Suppressed: verdict.Suppressed,
			Reasons: verdict.Reasons, Outcomes: outcomes,
		}, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "recovery progress: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(data))
		return 0
	}

	fmt.Fprintf(io.Stdout, "recovery progress run=%s class=%s escalate=%t suppressed=%t samples=%d\n",
		*run, verdict.Class, verdict.Escalate, verdict.Suppressed, len(samples))
	for _, reason := range verdict.Reasons {
		fmt.Fprintf(io.Stdout, "  reason: %s\n", reason)
	}
	if len(outcomes) == 0 {
		fmt.Fprintln(io.Stdout, "  outcomes: none recorded (a read-only view never invents one)")
		return 0
	}
	last := outcomes[len(outcomes)-1]
	fmt.Fprintf(io.Stdout, "  outcomes: %d recorded, last measured: %s\n", len(outcomes), last.Delta())
	return 0
}

// RecoveryOutcomesCmd implements `regulus recovery outcomes`, the full history.
func (io IO) RecoveryOutcomesCmd(args []string) int {
	fs := flag.NewFlagSet("recovery outcomes", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	run := fs.String("run", "", "run id (required)")
	asJSON := fs.Bool("json", false, "print the outcomes as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *run == "" {
		fmt.Fprintln(io.Stderr, recoveryUsage())
		return 2
	}
	outcomes, err := recovery.LoadOutcomes(io.Dir, *run)
	if err != nil {
		fmt.Fprintf(io.Stderr, "recovery outcomes: %v\n", err)
		return 1
	}
	if *asJSON {
		data, merr := json.MarshalIndent(outcomes, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "recovery outcomes: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(data))
		return 0
	}
	if len(outcomes) == 0 {
		fmt.Fprintf(io.Stdout, "recovery: no measured outcomes recorded for run %s\n", *run)
		return 0
	}
	for _, o := range outcomes {
		fmt.Fprintf(io.Stdout, "%s\n", o.Delta())
	}
	return 0
}
