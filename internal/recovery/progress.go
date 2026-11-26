// Progress classification (F17.2 / NOV-139).
//
// The F17.1 audit (docs/progress-signals.md) found no unified verdict
// vocabulary: LoopGuard nudges after three identical failures but lives only in
// the TypeScript plugin and never blocks, the bench diagnostics flag loops
// after a run has already finished, and recovery/failure only count retries per
// checkpoint. None of them answers "is this task making progress".
//
// This classifier answers exactly that, at the two scopes that survive across
// attempts: one checkpoint (task+agent) and one recovery run. It deliberately
// does not replace three neighbours it must not duplicate:
//
//   - evolve's repeated-failures aggregation (internal/evolve/observe.go,
//     MinSupport 2) is an observation-domain evidence bar across executions,
//     not a per-attempt verdict;
//   - the bench diagnostics (internal/bench/diag_detect.go) analyse a finished
//     run for hot loops, duplication and retry budget;
//   - LoopGuard's advisory nudge stays advisory. Classifying is not
//     authorising: a verdict here never stops a process, never kills anything,
//     and never widens a scope. S9 forbids turning detection into authority.
package recovery

import (
	"fmt"
	"strings"
)

// Verdict classes. The vocabulary is closed: a new class is a contract change.
const (
	// ClassProgress means the latest observable changed: the task moved.
	ClassProgress = "progress"
	// ClassRepeatedFailure means the same failure signature repeated up to
	// the budget. The failure is the signal, not the elapsed work.
	ClassRepeatedFailure = "repeated_failure"
	// ClassStagnation means samples accumulated without any observable
	// progress while the failures differed — the strategy is not working,
	// even though it is not literally repeating.
	ClassStagnation = "stagnation"
)

// ProgressPolicy holds the budgets that decide a verdict. Every value is a
// bound, never a heuristic tuned at call time.
type ProgressPolicy struct {
	// RepeatedFailureStreak is how many consecutive identical failure
	// signatures classify as a repeated failure. Default 3, the same bar
	// LoopGuard uses, so the two agree instead of contradicting each other.
	RepeatedFailureStreak int
	// StagnationSamples is how many consecutive samples without observable
	// progress classify as stagnation. Default 4, strictly higher than the
	// repeated-failure bar, so a repeat is always named as a repeat first.
	StagnationSamples int
	// CooldownMS suppresses a new escalation right after the previous one.
	// Zero means no cooldown. Default 300000, matching the host circuit.
	CooldownMS int
}

// DefaultProgressPolicy mirrors the thresholds already in use elsewhere so the
// system does not hold two contradictory bars.
func DefaultProgressPolicy() ProgressPolicy {
	return ProgressPolicy{
		RepeatedFailureStreak: 3,
		StagnationSamples:     4,
		CooldownMS:            300000,
	}
}

// Validate refuses a policy that could misfire. The bounds are deliberately
// narrow: a bar below two cannot distinguish a repeat from a single event, and
// an unbounded cooldown would silence escalation forever.
func (p ProgressPolicy) Validate() error {
	if p.RepeatedFailureStreak < 2 || p.RepeatedFailureStreak > 10 {
		return fmt.Errorf("recovery: repeated-failure streak %d out of 2..10", p.RepeatedFailureStreak)
	}
	if p.StagnationSamples < 2 || p.StagnationSamples > 20 {
		return fmt.Errorf("recovery: stagnation samples %d out of 2..20", p.StagnationSamples)
	}
	if p.StagnationSamples <= p.RepeatedFailureStreak {
		return fmt.Errorf("recovery: stagnation bar %d must exceed the repeated-failure bar %d (a repeat must be named as a repeat first)", p.StagnationSamples, p.RepeatedFailureStreak)
	}
	if p.CooldownMS < 0 || p.CooldownMS > 3_600_000 {
		return fmt.Errorf("recovery: cooldown %dms out of 0..3600000", p.CooldownMS)
	}
	return nil
}

// Sample is one observed attempt, reduced to what a verdict may rely on.
type Sample struct {
	// Signature is the normalized failure signature (tool plus error text).
	// Empty when the attempt succeeded or when no failure was observed.
	Signature string
	// Progressed reports an observable change: a test that ran, a file that
	// changed, a gate that moved, an artifact that appeared. Elapsed time
	// alone is never progress.
	Progressed bool
}

// Verdict is the classification plus the evidence behind it. Asks and
// Rejections are always populated: an unexplained verdict cannot be reviewed.
type Verdict struct {
	Class   string
	Reasons []string
	// Escalate is true only for a non-progress class and only when the
	// cooldown has elapsed. False never means "all good"; it means "not now".
	Escalate bool
	// Suppressed is true when a non-progress class was reached but the
	// cooldown swallowed the escalation. The class stays honest.
	Suppressed bool
}

// ClassifyInput carries everything the verdict may use. There is no hidden
// clock and no global state: the same input always yields the same verdict.
type ClassifyInput struct {
	Samples          []Sample
	Policy           ProgressPolicy
	LastEscalationMS int64 // 0 means never
	NowMS            int64
}

// Classify returns the verdict for one scope.
//
// Rules, in order:
//
//  1. No samples, or the latest sample progressed → Progress. A task that just
//     moved is never stagnating, no matter how long the failures before it.
//  2. Identical failure signatures repeated up to the bar → RepeatedFailure.
//  3. Samples without progress up to the bar, with differing signatures →
//     Stagnation.
//  4. Otherwise Progress: the bar is a budget, and spending less than a budget
//     is not evidence of stagnation.
//
// The order is what keeps false positives out: rule 1 outranks every detector,
// so a long but productive run is never flagged, and rule 4 means an unfinished
// budget never escalates.
func Classify(in ClassifyInput) Verdict {
	p := in.Policy
	if p == (ProgressPolicy{}) {
		p = DefaultProgressPolicy()
	}
	if len(in.Samples) == 0 {
		return Verdict{Class: ClassProgress, Reasons: []string{"no samples observed yet"}}
	}
	last := in.Samples[len(in.Samples)-1]
	if last.Progressed {
		return Verdict{Class: ClassProgress, Reasons: []string{"the latest sample made observable progress"}}
	}

	// Trailing run without observable progress.
	noProgress := 0
	for i := len(in.Samples) - 1; i >= 0 && !in.Samples[i].Progressed; i-- {
		noProgress++
	}
	// Trailing identical failure signature.
	streak := 0
	if sig := strings.TrimSpace(last.Signature); sig != "" {
		for i := len(in.Samples) - 1; i >= 0; i-- {
			s := in.Samples[i]
			if s.Progressed || strings.TrimSpace(s.Signature) != sig {
				break
			}
			streak++
		}
	}

	var verdict Verdict
	switch {
	case streak >= p.RepeatedFailureStreak:
		verdict = Verdict{
			Class:   ClassRepeatedFailure,
			Reasons: []string{fmt.Sprintf("the same failure signature repeated %d times (bar %d)", streak, p.RepeatedFailureStreak)},
		}
	case noProgress >= p.StagnationSamples:
		reasons := []string{fmt.Sprintf("%d samples without observable progress (bar %d)", noProgress, p.StagnationSamples)}
		if streak > 1 {
			reasons = append(reasons, fmt.Sprintf("the last failure signature repeated %d times", streak))
		}
		verdict = Verdict{Class: ClassStagnation, Reasons: reasons}
	default:
		return Verdict{
			Class:   ClassProgress,
			Reasons: []string{fmt.Sprintf("no budget reached yet: %d/%d without progress, signature streak %d/%d", noProgress, p.StagnationSamples, streak, p.RepeatedFailureStreak)},
		}
	}

	if p.CooldownMS > 0 && in.LastEscalationMS > 0 && in.NowMS-in.LastEscalationMS < int64(p.CooldownMS) {
		verdict.Suppressed = true
		verdict.Reasons = append(verdict.Reasons, fmt.Sprintf("escalation suppressed by cooldown: %dms elapsed of %dms", in.NowMS-in.LastEscalationMS, p.CooldownMS))
		return verdict
	}
	verdict.Escalate = true
	return verdict
}
