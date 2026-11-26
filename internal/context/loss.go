// Context loss and recovery measurement (F19.3 / NOV-148).
//
// The F19.1 baseline found that "quality preserved" was a boolean: the fidelity
// test checks containment, and Report carries token counts, but nothing measures
// how much of the visible corpus was left out or whether what was left out can
// still be reached. Those are the two numbers the phase asks for, and both derive
// from the same Selection that produced the context, so the measurement cannot
// disagree with the decision.
package context

// Loss is what the selection left behind.
type Loss struct {
	Visible          int   `json:"visible"`
	Delivered        int   `json:"delivered"`
	Omitted          int   `json:"omitted"`
	OmittedTokens    int64 `json:"omitted_tokens"`
	LossPct          int   `json:"loss_pct"`
	EssentialOmitted int   `json:"essential_omitted"`
}

// MeasureLoss counts fragments, not guesses: essential and on-demand layers are
// delivered, optional is not. EssentialOmitted must be zero — a critical fragment
// that ends up in the optional layer is a selector bug, and this reports it instead
// of hiding it behind a percentage.
func MeasureLoss(sel Selection) Loss {
	delivered := len(sel.Essential) + len(sel.OnDemand)
	omitted := len(sel.Optional)
	l := Loss{
		Visible:   delivered + omitted,
		Delivered: delivered,
		Omitted:   omitted,
	}
	for _, f := range sel.Optional {
		l.OmittedTokens += f.Tokens
		if f.Critical {
			l.EssentialOmitted++
		}
	}
	if l.Visible > 0 {
		l.LossPct = percentOf(l.Omitted, l.Visible)
	}
	return l
}

// Recovery is what remains reachable once context was cut.
type Recovery struct {
	Omitted     int `json:"omitted"`
	Recoverable int `json:"recoverable"`
	Unreachable int `json:"unreachable"`
	RatePct     int `json:"recovery_rate_pct"`
}

// MeasureRecovery counts omitted fragments that still carry a source pointer, so a
// caller can fetch them on demand. An expired fragment is not recoverable: the
// content may no longer be true, so pointing at it would recover a lie.
func MeasureRecovery(sel Selection) Recovery {
	r := Recovery{Omitted: len(sel.Optional)}
	for _, f := range sel.Optional {
		if f.Source != "" && !f.Expired {
			r.Recoverable++
			continue
		}
		r.Unreachable++
	}
	if r.Omitted > 0 {
		r.RatePct = percentOf(r.Recoverable, r.Omitted)
	}
	return r
}

// percentOf rounds n/d to the nearest integer percentage without claiming a rate
// it cannot compute: a zero denominator yields zero.
func percentOf(n, d int) int {
	if d <= 0 {
		return 0
	}
	return (n*200 + d) / (2 * d)
}
