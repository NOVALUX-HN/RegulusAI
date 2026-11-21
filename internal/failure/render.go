// Human-readable rendering (F13.5/NOV-108): one-line decision
// confirmations, mission decision lists and full explanations.
// Machine-stable output stays on --json at the cmdx layer; these
// printers are the operator-facing surface.
package failure

import (
	"fmt"
	"strings"
)

// RenderDecision prints the one-line seal confirmation the decide
// command emits: what was chosen, for which class, with the
// backoff, the suggested edge and whether the record replayed an
// idempotent repeat.
func RenderDecision(rec DecisionRecord, replayed bool) string {
	var b strings.Builder
	verb := "decided"
	if replayed {
		verb = "replayed (idempotent, already recorded)"
	}
	fmt.Fprintf(&b, "failure: %s %s on %s class=%s decision=%s\n",
		verb, rec.ID, rec.MissionID, rec.Class, rec.Decision)
	if rec.BackoffMS > 0 {
		fmt.Fprintf(&b, "backoff: %dms before the next attempt\n", rec.BackoffMS)
	}
	if rec.SuggestedMissionTo != "" {
		fmt.Fprintf(&b, "suggested mission edge: --to %s\n", rec.SuggestedMissionTo)
	}
	fmt.Fprintf(&b, "result: %s\n", rec.Result)
	return b.String()
}

// RenderList prints every sealed decision for one mission in
// append order: index, id, class, decision and cause.
func RenderList(missionID string, recs []DecisionRecord) string {
	var b strings.Builder
	if len(recs) == 0 {
		fmt.Fprintf(&b, "(no failure decisions recorded for %s)\n", missionID)
		return b.String()
	}
	for i, r := range recs {
		fmt.Fprintf(&b, "%d. %s class=%s decision=%s cause=%s\n",
			i+1, r.ID, r.Class, r.Decision, oneLine(r.Cause))
	}
	return b.String()
}

func oneLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}
