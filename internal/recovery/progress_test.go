package recovery

import (
	"strings"
	"testing"
)

// failed builds a non-progressing sample with a signature.
func failed(sig string) Sample { return Sample{Signature: sig} }

// moved builds a progressing sample.
func moved() Sample { return Sample{Progressed: true} }

// TestClassifyDefaultsMatchLoopGuard pins the repeated-failure bar to the one
// LoopGuard already uses, so the two detectors cannot contradict each other.
func TestClassifyDefaultsMatchLoopGuard(t *testing.T) {
	p := DefaultProgressPolicy()
	if p.RepeatedFailureStreak != 3 {
		t.Fatalf("repeated-failure bar = %d, want 3 to agree with LoopGuard", p.RepeatedFailureStreak)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("default policy must validate: %v", err)
	}
}

// TestClassifyRepeatedFailure: the same signature up to the bar escalates.
func TestClassifyRepeatedFailure(t *testing.T) {
	in := ClassifyInput{
		Samples: []Sample{failed("tool=go test err=exit 1"), failed("tool=go test err=exit 1"), failed("tool=go test err=exit 1")},
		Policy:  DefaultProgressPolicy(),
		NowMS:   1000,
	}
	v := Classify(in)
	if v.Class != ClassRepeatedFailure {
		t.Fatalf("class = %q, want %q (%v)", v.Class, ClassRepeatedFailure, v.Reasons)
	}
	if !v.Escalate {
		t.Fatalf("a repeated failure must escalate: %+v", v)
	}
}

// TestClassifyStagnationWithoutRepeating: differing failures with no observable
// progress are stagnation, not a repeat. This is the case the retry counters
// cannot see, because each class is retryable on its own.
func TestClassifyStagnationWithoutRepeating(t *testing.T) {
	in := ClassifyInput{
		Samples: []Sample{failed("a"), failed("b"), failed("c"), failed("d")},
		Policy:  DefaultProgressPolicy(),
		NowMS:   1000,
	}
	v := Classify(in)
	if v.Class != ClassStagnation {
		t.Fatalf("class = %q, want %q (%v)", v.Class, ClassStagnation, v.Reasons)
	}
	if !v.Escalate {
		t.Fatalf("stagnation past the bar must escalate: %+v", v)
	}
}

// TestClassifyBelowBarIsNotStagnation is the false-positive guard for an
// unfinished budget: spending part of a budget is not evidence of anything.
func TestClassifyBelowBarIsNotStagnation(t *testing.T) {
	cases := []struct {
		name    string
		samples []Sample
	}{
		{"two identical failures", []Sample{failed("x"), failed("x")}},
		{"three differing failures", []Sample{failed("a"), failed("b"), failed("c")}},
		{"one failure", []Sample{failed("a")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := Classify(ClassifyInput{Samples: tc.samples, Policy: DefaultProgressPolicy(), NowMS: 1000})
			if v.Class != ClassProgress {
				t.Fatalf("class = %q, want progress below the bar (%v)", v.Class, v.Reasons)
			}
			if v.Escalate {
				t.Fatalf("nothing may escalate below the bar: %+v", v)
			}
		})
	}
}

// TestClassifyProgressOutranksEveryDetector is the central false-positive case:
// a long run with many prior failures must not be flagged once it moves. This
// is what the audit says the existing detectors cannot express.
func TestClassifyProgressOutranksEveryDetector(t *testing.T) {
	samples := make([]Sample, 0, 12)
	for i := 0; i < 11; i++ {
		samples = append(samples, failed("tool=go test err=exit 1"))
	}
	samples = append(samples, moved())

	v := Classify(ClassifyInput{Samples: samples, Policy: DefaultProgressPolicy(), NowMS: 5000})
	if v.Class != ClassProgress {
		t.Fatalf("a moving task must be progress, got %q (%v)", v.Class, v.Reasons)
	}
	if v.Escalate {
		t.Fatalf("progress never escalates: %+v", v)
	}
}

// TestClassifyLongIsNotProgress: elapsed time alone must never count as
// progress, and a single long attempt must not be flagged either. The second
// half is the false-positive fixture the audit said was missing.
func TestClassifyLongIsNotProgress(t *testing.T) {
	long := []Sample{
		{Signature: "s"},
		{Signature: "s"},
		{Signature: "s"},
	}
	v := Classify(ClassifyInput{Samples: long, Policy: DefaultProgressPolicy(), NowMS: 7_200_001})
	if v.Class != ClassRepeatedFailure {
		t.Fatalf("long identical failures are still a repeat, got %q", v.Class)
	}
	single := Classify(ClassifyInput{Samples: []Sample{{Signature: "s"}}, Policy: DefaultProgressPolicy(), NowMS: 9_999_999})
	if single.Class != ClassProgress || single.Escalate {
		t.Fatalf("one long attempt must not be flagged: %+v", single)
	}
}

// TestClassifyCooldownSuppressesEscalationButKeepsClass: the verdict stays
// honest while the escalation waits. Suppression is not a downgrade.
func TestClassifyCooldownSuppressesEscalationButKeepsClass(t *testing.T) {
	in := ClassifyInput{
		Samples:          []Sample{failed("x"), failed("x"), failed("x")},
		Policy:           DefaultProgressPolicy(),
		LastEscalationMS: 1000,
		NowMS:            1000 + 60_000, // inside the 300000ms cooldown
	}
	v := Classify(in)
	if v.Class != ClassRepeatedFailure {
		t.Fatalf("class must stay repeated_failure, got %q", v.Class)
	}
	if v.Escalate {
		t.Fatal("a cooldown must suppress the escalation")
	}
	if !v.Suppressed {
		t.Fatal("suppression must be reported, not silent")
	}
	if !strings.Contains(strings.Join(v.Reasons, " "), "cooldown") {
		t.Fatalf("reasons must name the cooldown: %v", v.Reasons)
	}

	// Past the cooldown the same input escalates again.
	in.NowMS = 1000 + 300_001
	if v := Classify(in); !v.Escalate || v.Suppressed {
		t.Fatalf("after the cooldown it must escalate: %+v", v)
	}
}

// TestClassifyProgressResetsTheStreak: a move in the middle breaks the repeat,
// so a single later failure is not a repeat.
func TestClassifyProgressResetsTheStreak(t *testing.T) {
	in := ClassifyInput{
		Samples: []Sample{failed("x"), failed("x"), moved(), failed("x")},
		Policy:  DefaultProgressPolicy(),
		NowMS:   1000,
	}
	if v := Classify(in); v.Class != ClassProgress || v.Escalate {
		t.Fatalf("one failure after progress is below the bar: %+v", v)
	}
}

// TestClassifyIsDeterministic: no hidden clock and no global state, so the same
// input always yields the same verdict.
func TestClassifyIsDeterministic(t *testing.T) {
	in := ClassifyInput{Samples: []Sample{failed("a"), failed("b")}, Policy: DefaultProgressPolicy(), NowMS: 10}
	first, second := Classify(in), Classify(in)
	if first.Class != second.Class || first.Escalate != second.Escalate || len(first.Reasons) != len(second.Reasons) {
		t.Fatalf("same input must yield the same verdict: %+v vs %+v", first, second)
	}
}

// TestProgressPolicyValidate refuses policies that could misfire.
func TestProgressPolicyValidate(t *testing.T) {
	cases := []struct {
		name string
		p    ProgressPolicy
		ok   bool
	}{
		{"default", DefaultProgressPolicy(), true},
		{"streak below two", ProgressPolicy{RepeatedFailureStreak: 1, StagnationSamples: 4}, false},
		{"streak above ten", ProgressPolicy{RepeatedFailureStreak: 11, StagnationSamples: 12}, false},
		{"samples below two", ProgressPolicy{RepeatedFailureStreak: 2, StagnationSamples: 1}, false},
		{"stagnation not above repeat", ProgressPolicy{RepeatedFailureStreak: 3, StagnationSamples: 3}, false},
		{"negative cooldown", ProgressPolicy{RepeatedFailureStreak: 3, StagnationSamples: 4, CooldownMS: -1}, false},
		{"cooldown above an hour", ProgressPolicy{RepeatedFailureStreak: 3, StagnationSamples: 4, CooldownMS: 3_600_001}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if tc.ok && err != nil {
				t.Fatalf("want valid, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("want refused, got nil")
			}
		})
	}
}

// TestClassifyZeroPolicyFallsBackToDefault keeps a zero policy usable without
// silently disabling every detector.
func TestClassifyZeroPolicyFallsBackToDefault(t *testing.T) {
	in := ClassifyInput{
		Samples: []Sample{failed("x"), failed("x"), failed("x")},
		NowMS:   1000,
	}
	if v := Classify(in); v.Class != ClassRepeatedFailure {
		t.Fatalf("zero policy must fall back to defaults, got %q", v.Class)
	}
}
