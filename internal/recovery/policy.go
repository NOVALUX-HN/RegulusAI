// Retry budgets, backoff, timeouts and circuit breakers
// (F12.5/NOV-100): every retry is bounded, delayed deterministically
// and refused when the host is melting.
//
// Deterministic by design: backoff is pure exponential without
// jitter, so two operators replaying the same history compute the
// same delay and the audit can quote it. Cooldowns use the caller's
// clock (tests pin it; the CLI passes UTC now).
package recovery

import (
	"fmt"
	"time"
)

// RetryPolicy bounds one checkpoint's retries: MaxRetries caps the
// count (0 = no retry, the attempt stands); BackoffBaseMS with
// BackoffFactor/BackoffCapMS spaces attempts; AttemptTimeoutMS
// marks a running attempt timed out; CircuitThreshold opens a host
// after that many consecutive failures until CircuitCooldownMS.
type RetryPolicy struct {
	MaxRetries        int     `json:"max_retries"`
	BackoffBaseMS     int64   `json:"backoff_base_ms"`
	BackoffFactor     float64 `json:"backoff_factor"`
	BackoffCapMS      int64   `json:"backoff_cap_ms"`
	AttemptTimeoutMS  int64   `json:"attempt_timeout_ms"`
	CircuitThreshold  int     `json:"circuit_threshold"`
	CircuitCooldownMS int64   `json:"circuit_cooldown_ms"`
}

// DefaultRetryPolicy is the documented budget: 3 retries, 1s base
// doubling to a 30s cap, 60s per-attempt timeout, host opens after
// 3 consecutive failures for 5 minutes.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 3, BackoffBaseMS: 1000, BackoffFactor: 2.0,
		BackoffCapMS: 30000, AttemptTimeoutMS: 60000,
		CircuitThreshold: 3, CircuitCooldownMS: 300000,
	}
}

// Validate refuses unbounded or hostile budgets before anything
// runs: negative retries, non-positive backoff, caps below base,
// non-positive timeouts or circuit parameters.
func (p RetryPolicy) Validate() error {
	if p.MaxRetries < 0 || p.MaxRetries > 10 {
		return fmt.Errorf("recovery: --max-retries %d outside 0..10", p.MaxRetries)
	}
	if p.BackoffBaseMS <= 0 {
		return fmt.Errorf("recovery: backoff base must be positive, got %d", p.BackoffBaseMS)
	}
	if p.BackoffFactor < 1 {
		return fmt.Errorf("recovery: backoff factor must be >= 1, got %v", p.BackoffFactor)
	}
	if p.BackoffCapMS < p.BackoffBaseMS {
		return fmt.Errorf("recovery: backoff cap %d below base %d", p.BackoffCapMS, p.BackoffBaseMS)
	}
	if p.AttemptTimeoutMS <= 0 {
		return fmt.Errorf("recovery: attempt timeout must be positive, got %d", p.AttemptTimeoutMS)
	}
	if p.CircuitThreshold <= 0 {
		return fmt.Errorf("recovery: circuit threshold must be positive, got %d", p.CircuitThreshold)
	}
	if p.CircuitCooldownMS <= 0 {
		return fmt.Errorf("recovery: circuit cooldown must be positive, got %d", p.CircuitCooldownMS)
	}
	return nil
}

// BackoffDelay returns the wait before retry number n (1-based):
// base * factor^(n-1), capped. Pure and deterministic.
func (p RetryPolicy) BackoffDelay(n int) int64 {
	d := float64(p.BackoffBaseMS)
	for i := 1; i < n; i++ {
		d *= p.BackoffFactor
		if d >= float64(p.BackoffCapMS) {
			return p.BackoffCapMS
		}
	}
	if d >= float64(p.BackoffCapMS) {
		return p.BackoffCapMS
	}
	return int64(d)
}

// TimedOut reports whether an attempt started at since has exceeded
// the per-attempt timeout at now.
func (p RetryPolicy) TimedOut(since, now time.Time) bool {
	return now.Sub(since) >= time.Duration(p.AttemptTimeoutMS)*time.Millisecond
}

// CircuitOpen reports whether host is currently refused: opened
// after Threshold consecutive failures and still inside cooldown.
func (r *RunRecord) CircuitOpen(host string, now time.Time, p RetryPolicy) bool {
	opened, ok := r.Circuit.OpenedAt[host]
	if !ok {
		return false
	}
	return now.Sub(opened) < time.Duration(p.CircuitCooldownMS)*time.Millisecond
}

// RecordFailure counts one consecutive host failure, opening the
// circuit at Threshold. Callers pass the checkpoint host ("" skips
// tracking: no host, no circuit).
func (r *RunRecord) RecordFailure(host string, now time.Time, p RetryPolicy) {
	if host == "" {
		return
	}
	if r.Circuit.Failures == nil {
		r.Circuit.Failures = map[string]int{}
	}
	r.Circuit.Failures[host]++
	if r.Circuit.Failures[host] < p.CircuitThreshold {
		return
	}
	if r.Circuit.OpenedAt == nil {
		r.Circuit.OpenedAt = map[string]time.Time{}
	}
	if _, ok := r.Circuit.OpenedAt[host]; !ok {
		r.Circuit.OpenedAt[host] = now.UTC()
	}
}

// RecordSuccess resets one host's consecutive count and closes its
// circuit: a host that serves again is trusted again.
func (r *RunRecord) RecordSuccess(host string) {
	if host == "" {
		return
	}
	delete(r.Circuit.Failures, host)
	delete(r.Circuit.OpenedAt, host)
}
