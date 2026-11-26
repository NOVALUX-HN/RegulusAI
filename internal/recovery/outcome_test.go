package recovery

import (
	"strings"
	"testing"
	"time"
)

// cp builds a checkpoint for the evidence tests.
func cp(state string, retries int, effects int, pending int) Checkpoint {
	c := Checkpoint{TaskID: "T1", AgentID: "a1", Host: "opencode", State: state, Retries: retries}
	if effects > 0 {
		c.Applied = map[string]EffectResult{}
		for i := 0; i < effects; i++ {
			c.Applied[string(rune('a'+i))] = EffectResult{Key: string(rune('a' + i))}
		}
	}
	for i := 0; i < pending; i++ {
		c.Compensate = append(c.Compensate, Compensation{Key: string(rune('a' + i)), Desc: "undo"})
	}
	return c
}

// TestEvidenceOfIsMeasured: every number comes from the checkpoint or the run
// circuit, never from a caller.
func TestEvidenceOfIsMeasured(t *testing.T) {
	c := cp(StateFailed, 2, 3, 1)
	e := EvidenceOf(c, Circuit{})
	if e.State != StateFailed || e.Retries != 2 || e.EffectsApplied != 3 || e.CompensationsPending != 1 {
		t.Fatalf("evidence = %+v", e)
	}
	if e.CircuitOpen {
		t.Fatal("a zero circuit must not read as open")
	}

	open := Circuit{OpenedAt: map[string]time.Time{"opencode": time.Unix(0, 0)}}
	if e := EvidenceOf(c, open); !e.CircuitOpen {
		t.Fatal("an opened circuit for this host must read as open")
	}
	// A circuit opened for another host must not be attributed to this one.
	other := Circuit{OpenedAt: map[string]time.Time{"codex": time.Unix(0, 0)}}
	if e := EvidenceOf(c, other); e.CircuitOpen {
		t.Fatal("another host's circuit must not leak into this checkpoint")
	}
}

// TestEffectiveIsMeasuredNotProposed pins the core of G2: an action is effective
// only when something observable advanced. A retry that changed nothing is not
// effective even after an hour.
func TestEffectiveIsMeasuredNotProposed(t *testing.T) {
	cases := []struct {
		name   string
		before Evidence
		after  Evidence
		expect bool
	}{
		{
			"reached succeeded",
			Evidence{State: StateFailed, Retries: 1, EffectsApplied: 1},
			Evidence{State: StateSucceeded, Retries: 1, EffectsApplied: 1},
			true,
		},
		{
			"applied one more effect",
			Evidence{State: StateFailed, Retries: 1, EffectsApplied: 1},
			Evidence{State: StateFailed, Retries: 2, EffectsApplied: 2},
			true,
		},
		{
			"compensated one",
			Evidence{State: StateFailed, Retries: 1, EffectsApplied: 1, CompensationsPending: 1},
			Evidence{State: StateRolledBack, Retries: 1, EffectsApplied: 1, CompensationsPending: 0},
			true,
		},
		{
			"nothing changed",
			Evidence{State: StateFailed, Retries: 1, EffectsApplied: 1},
			Evidence{State: StateFailed, Retries: 5, EffectsApplied: 1},
			false,
		},
	}
	for _, tc := range cases {
		if got := Effective(tc.before, tc.after); got != tc.expect {
			t.Fatalf("%s: Effective = %v, want %v", tc.name, got, tc.expect)
		}
	}
}

// TestNewStrategyOutcomeRecordsTheDelta: the record shows before and after and
// states the evidence behind the verdict.
func TestNewStrategyOutcomeRecordsTheDelta(t *testing.T) {
	before := cp(StateFailed, 1, 1, 0)
	after := cp(StateSucceeded, 1, 2, 0)
	o, err := NewStrategyOutcome(StrategyRetry, before, after, Circuit{}, Circuit{}, 1500*time.Millisecond, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("NewStrategyOutcome: %v", err)
	}
	if !o.Effective {
		t.Fatalf("an action that applied an effect and reached succeeded must be effective: %+v", o)
	}
	if o.CostMS != 1500 {
		t.Fatalf("cost = %d, want the measured 1500", o.CostMS)
	}
	if o.Before.State != StateFailed || o.After.State != StateSucceeded {
		t.Fatalf("before/after not recorded: %+v", o)
	}
	if len(o.Reasons) == 0 || !strings.Contains(strings.Join(o.Reasons, " "), "succeeded") {
		t.Fatalf("reasons must state the measured change: %v", o.Reasons)
	}
	if !strings.Contains(o.Delta(), "advanced") {
		t.Fatalf("delta must say it advanced: %q", o.Delta())
	}
}

// TestStrategyOutcomeEscalation: an escalation, or a circuit that opened during
// the action, is recorded as such.
func TestStrategyOutcomeEscalation(t *testing.T) {
	before := cp(StateFailed, 3, 1, 0)
	after := cp(StateFailed, 3, 1, 0)
	opened := Circuit{OpenedAt: map[string]time.Time{"opencode": time.Unix(5, 0)}}

	o, err := NewStrategyOutcome(StrategyEscalate, before, after, Circuit{}, opened, 0, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("NewStrategyOutcome: %v", err)
	}
	if !o.Escalated {
		t.Fatal("an escalate strategy must be recorded as escalated")
	}
	if o.Effective {
		t.Fatal("escalating without an advance is not effective")
	}

	// A circuit opening during a non-escalation action is escalation too.
	o, err = NewStrategyOutcome(StrategyRetry, before, after, Circuit{}, opened, 0, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("NewStrategyOutcome: %v", err)
	}
	if !o.Escalated {
		t.Fatal("a circuit that opened during the action is an escalation")
	}
}

// TestStrategyOutcomeValidate refuses records that cannot come from a
// measurement.
func TestStrategyOutcomeValidate(t *testing.T) {
	good, err := NewStrategyOutcome(StrategyRetry, cp(StateFailed, 0, 0, 0), cp(StateSucceeded, 0, 1, 0), Circuit{}, Circuit{}, 0, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("baseline must be valid: %v", err)
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("baseline must validate: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*StrategyOutcome)
	}{
		{"wrong version", func(o *StrategyOutcome) { o.Version = "recovery-v1" }},
		{"no task", func(o *StrategyOutcome) { o.TaskID = "  " }},
		{"unknown strategy", func(o *StrategyOutcome) { o.Strategy = "vibes" }},
		{"negative cost", func(o *StrategyOutcome) { o.CostMS = -1 }},
		{"unknown state", func(o *StrategyOutcome) { o.After.State = "almost" }},
		{"escalated without cause", func(o *StrategyOutcome) { o.Escalated = true }},
	}
	for _, tc := range cases {
		bad := good
		tc.mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Fatalf("%s must be refused", tc.name)
		}
	}
}

// TestNewStrategyOutcomeRefusesNegativeCost: a fabricated measurement never gets
// stored in the first place.
func TestNewStrategyOutcomeRefusesNegativeCost(t *testing.T) {
	_, err := NewStrategyOutcome(StrategyRetry, cp(StateFailed, 0, 0, 0), cp(StateFailed, 0, 0, 0), Circuit{}, Circuit{}, -time.Second, time.Unix(1, 0))
	if err == nil {
		t.Fatal("a negative cost must be refused")
	}
}
