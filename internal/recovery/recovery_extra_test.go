// Extended pure-function tests (coverage-80/T3 stretch): retry
// budgets, backoff edges, timeout boundaries, the full error-class
// tables and the human renderers. All deterministic, no stores.
package recovery

import (
	"strings"
	"testing"
	"time"
)

func TestRetryPolicyValidateTable(t *testing.T) {
	if err := DefaultRetryPolicy().Validate(); err != nil {
		t.Fatalf("default budget must validate: %v", err)
	}
	mutate := func(f func(*RetryPolicy)) RetryPolicy {
		p := DefaultRetryPolicy()
		f(&p)
		return p
	}
	cases := []struct {
		name   string
		policy RetryPolicy
		want   string
	}{
		{"negative retries", mutate(func(p *RetryPolicy) { p.MaxRetries = -1 }), "max-retries"},
		{"retries above cap", mutate(func(p *RetryPolicy) { p.MaxRetries = 11 }), "max-retries"},
		{"zero retries allowed", mutate(func(p *RetryPolicy) { p.MaxRetries = 0 }), ""},
		{"base not positive", mutate(func(p *RetryPolicy) { p.BackoffBaseMS = 0 }), "backoff base"},
		{"factor below one", mutate(func(p *RetryPolicy) { p.BackoffFactor = 0.5 }), "backoff factor"},
		{"cap below base", mutate(func(p *RetryPolicy) { p.BackoffCapMS = 1 }), "backoff cap"},
		{"timeout not positive", mutate(func(p *RetryPolicy) { p.AttemptTimeoutMS = 0 }), "attempt timeout"},
		{"threshold not positive", mutate(func(p *RetryPolicy) { p.CircuitThreshold = 0 }), "circuit threshold"},
		{"cooldown not positive", mutate(func(p *RetryPolicy) { p.CircuitCooldownMS = -5 }), "circuit cooldown"},
	}
	for _, tc := range cases {
		err := tc.policy.Validate()
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s: must validate: %v", tc.name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: must refuse", tc.name)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: must name %q, got %v", tc.name, tc.want, err)
		}
	}
}

func TestBackoffDelayEdges(t *testing.T) {
	p := DefaultRetryPolicy()
	if got := p.BackoffDelay(0); got != p.BackoffBaseMS {
		t.Fatalf("BackoffDelay(0) = %d, want base %d", got, p.BackoffBaseMS)
	}
	flat := p
	flat.BackoffFactor = 1
	for _, n := range []int{1, 2, 10} {
		if got := flat.BackoffDelay(n); got != flat.BackoffBaseMS {
			t.Fatalf("factor 1 keeps the base, BackoffDelay(%d) = %d", n, got)
		}
	}
	if got := p.BackoffDelay(3); got != 4000 {
		t.Fatalf("BackoffDelay(3) = %d, want 4000", got)
	}
	if got := p.BackoffDelay(5); got != 16000 {
		t.Fatalf("BackoffDelay(5) = %d, want 16000", got)
	}
	if got := p.BackoffDelay(6); got != 30000 {
		t.Fatalf("BackoffDelay(6) caps at 30000, got %d", got)
	}
}

func TestTimedOutBoundary(t *testing.T) {
	p := DefaultRetryPolicy()
	since := testNow()
	if p.TimedOut(since, since.Add(59999*time.Millisecond)) {
		t.Fatal("1ms before the 60s timeout is not timed out")
	}
	if !p.TimedOut(since, since.Add(60000*time.Millisecond)) {
		t.Fatal("exactly at the timeout counts as timed out")
	}
	if !p.TimedOut(since, since.Add(time.Hour)) {
		t.Fatal("past the timeout counts as timed out")
	}
}

func TestClassifyErrorFullTable(t *testing.T) {
	cases := map[string]string{
		"": ClassCancelledE, "   ": ClassCancelledE,
		"cancelled by user": ClassCancelledE, "CANCEL requested": ClassCancelledE,
		"  Unauthorized  ": ClassAuth, "forbidden resource": ClassAuth,
		"permission denied writing store": ClassAuth, "invalid scope for tool": ClassAuth,
		"auth failed for host": ClassAuth, "not authorized": ClassAuth,
		"invalid checkpoint shape": ClassPermanent, "REFUSED: PERMANENT policy": ClassPermanent,
		"not supported on windows": ClassPermanent, "assertion failed": ClassPermanent,
		"permanent refusal":  ClassPermanent,
		"half-applied batch": ClassPartial, "incomplete apply, rerun resumes": ClassPartial,
		"no such host h9": ClassHostOff, "host unreachable": ClassHostOff,
		"host down for maintenance": ClassHostOff, "node offline": ClassHostOff,
		"timed out after 30s": ClassTimeout, "timedout waiting": ClassTimeout,
		"timeout after 1000ms":     ClassTimeout,
		"connection reset by peer": ClassNetwork, "network unreachable": ClassNetwork,
		"dns lookup failed": ClassNetwork, "broken pipe": ClassNetwork,
		"upstream 503": ClassNetwork, "gateway 502": ClassNetwork, "timeout 504 page": ClassNetwork,
		"fatal: out of memory": ClassCrash, "killed by oom": ClassCrash,
		"segfault at 0x0": ClassCrash, "received sigkill": ClassCrash,
		"sigsegv in worker": ClassCrash, "worker crash loop": ClassCrash,
		"weird new failure mode": ClassUnknown,
	}
	for signal, want := range cases {
		if got := ClassifyError(signal); got != want {
			t.Errorf("ClassifyError(%q) = %q, want %q", signal, got, want)
		}
	}
}

func TestRetryableAndSameHostTables(t *testing.T) {
	for class, want := range map[string]bool{
		ClassCrash: true, ClassNetwork: true, ClassHostOff: true,
		ClassTimeout: true, ClassUnknown: true,
		ClassPermanent: false, ClassAuth: false, ClassCancelledE: false, ClassPartial: false,
	} {
		if got := Retryable(class); got != want {
			t.Errorf("Retryable(%q) = %v, want %v", class, got, want)
		}
	}
	for class, want := range map[string]bool{
		ClassCrash: true, ClassNetwork: true, ClassTimeout: true,
		ClassUnknown: true, ClassPermanent: true, ClassAuth: true,
		ClassCancelledE: true, ClassPartial: true,
		ClassHostOff: false,
	} {
		if got := SameHostAllowed(class); got != want {
			t.Errorf("SameHostAllowed(%q) = %v, want %v", class, got, want)
		}
	}
}

func TestCircuitEmptyHostAndCooldown(t *testing.T) {
	r := testRun(t, "run-circuit-edge")
	p := DefaultRetryPolicy()
	r.RecordFailure("", testNow(), p)
	r.RecordSuccess("")
	if r.CircuitOpen("h1", testNow(), p) {
		t.Fatal("untracked host is never open")
	}
	r.RecordFailure("h1", testNow(), p)
	r.RecordFailure("h1", testNow(), p)
	if r.CircuitOpen("h1", testNow(), p) {
		t.Fatal("below threshold the circuit stays closed")
	}
	r.RecordFailure("h1", testNow(), p)
	if !r.CircuitOpen("h1", testNow(), p) {
		t.Fatal("at threshold the circuit opens")
	}
	after := testNow().Add(time.Duration(p.CircuitCooldownMS)*time.Millisecond + time.Second)
	if r.CircuitOpen("h1", after, p) {
		t.Fatal("past cooldown the circuit closes")
	}
	r.RecordSuccess("h1")
	if r.CircuitOpen("h1", testNow(), p) {
		t.Fatal("a serving host is trusted again")
	}
	if n := r.Circuit.Failures["h1"]; n != 0 {
		t.Fatalf("success resets the count, got %d", n)
	}
}

func TestRenderStatusAndPlan(t *testing.T) {
	r := testRun(t, "run-render")
	c := mustCheckpoint(t, r, "t1", "a1", StateRunning)
	c.LastClass = ClassNetwork
	c.Applied = map[string]EffectResult{"e1": {Key: "e1", Output: "ok", At: testNow()}}
	p := DefaultRetryPolicy()
	for i := 0; i < p.CircuitThreshold; i++ {
		r.RecordFailure("h1", testNow(), p)
	}
	out := RenderStatus(r)
	for _, want := range []string{
		"recovery run-render: 1 checkpoint(s)",
		"t1/a1 " + StateRunning,
		"scopes: memory:read, memory:write",
		"caps: code-review",
		"required: code-review",
		"host=h1",
		"last error: " + ClassNetwork,
		"applied effects: e1",
		"pending compensations: 2",
		"! circuit open: host h1 (3 consecutive failures)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("status must quote %q, got:\n%s", want, out)
		}
	}
	empty := RenderPlan(ResumePlan{TaskID: "t1", AgentID: "a1", Attempt: 1, Key: "k"})
	if !strings.Contains(empty, "nothing to skip") {
		t.Fatalf("empty plan must say so, got:\n%s", empty)
	}
	full := RenderPlan(ResumePlan{TaskID: "t1", AgentID: "a1", Attempt: 2, Key: "k2", Skip: []string{"e1"}, Run: []string{"e2"}})
	for _, want := range []string{"resume t1/a1 attempt=2 key=k2", "skip (applied): e1", "run: e2"} {
		if !strings.Contains(full, want) {
			t.Fatalf("plan must quote %q, got:\n%s", want, full)
		}
	}
}
