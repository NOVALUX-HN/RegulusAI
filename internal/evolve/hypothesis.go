// Hypotheses explain an improvement opportunity (Phase 6, S3): problem,
// root cause, proposed change, expected benefit, potential regressions and
// evaluation criteria. A hypothesis is only built on generalizable support:
// Hypothesize refuses isolated-only observations (S2).
package evolve

import (
	"errors"
	"strings"
)

// Hypothesis is the HYPOTHESIZE-stage record.
type Hypothesis struct {
	Problem              string
	RootCause            string
	Change               string
	Benefit              string
	PotentialRegressions string
	EvaluationCriteria   string
}

// Validate refuses hypotheses with any unexplained point.
func (h Hypothesis) Validate() error {
	for _, field := range []struct {
		name, value string
	}{
		{"problem", h.Problem}, {"root cause", h.RootCause}, {"proposed change", h.Change},
		{"expected benefit", h.Benefit}, {"potential regressions", h.PotentialRegressions},
		{"evaluation criteria", h.EvaluationCriteria},
	} {
		if strings.TrimSpace(field.value) == "" {
			return errors.New("evolve: hypothesis must explain its " + field.name)
		}
	}
	return nil
}

// HypOption fills one hypothesis field.
type HypOption func(*Hypothesis)

// WithProblem sets the problem under test.
func WithProblem(v string) HypOption { return func(h *Hypothesis) { h.Problem = v } }

// WithRootCause sets the suspected root cause.
func WithRootCause(v string) HypOption { return func(h *Hypothesis) { h.RootCause = v } }

// WithChange sets the proposed change.
func WithChange(v string) HypOption { return func(h *Hypothesis) { h.Change = v } }

// WithBenefit sets the expected benefit.
func WithBenefit(v string) HypOption { return func(h *Hypothesis) { h.Benefit = v } }

// WithRegressions sets the potential regressions.
func WithRegressions(v string) HypOption { return func(h *Hypothesis) { h.PotentialRegressions = v } }

// WithCriteria sets the evaluation criteria.
func WithCriteria(v string) HypOption { return func(h *Hypothesis) { h.EvaluationCriteria = v } }

// Hypothesize builds a hypothesis on observation support. Isolated-only
// support is refused: one case never makes a general rule.
func Hypothesize(obs []Observation, opts ...HypOption) (Hypothesis, error) {
	if err := CheckSupport(obs); err != nil {
		return Hypothesis{}, err
	}
	var h Hypothesis
	for _, opt := range opts {
		opt(&h)
	}
	if err := h.Validate(); err != nil {
		return Hypothesis{}, err
	}
	return h, nil
}
