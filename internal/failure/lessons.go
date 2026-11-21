// F12.6 feeding (F13.5/NOV-108): turn sealed decisions into
// cross-agent learning without ever elevating a hypothesis to a
// fact. Every lesson this package builds is kind hypothesis with
// a low/medium self-assessed confidence, full provenance
// (mission, evidence refs, receipts) and a title that names the
// class and the decision. Promotion to verified-solution stays
// exclusively with `learn verify` against independent evidence —
// the coordinator has no path there, by construction (hypotheses
// can never verify: the Almanac refuses and the status layer
// pins it).
package failure

import (
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
)

// LessonAuthor names the harness author on every fed lesson, so
// retrieval always shows machine-proposed provenance.
const LessonAuthor = "failure-coordinator"

// BuildLesson renders one sealed decision as an Almanac
// observation ready for almanac.LearnSave. The caller saves (or
// not — feeding is explicit via --feed-learn, never ambient);
// this function only shapes the hypothesis with its confidence
// and provenance labels.
func BuildLesson(rec DecisionRecord, receipts []string, now time.Time) almanac.Observation {
	_ = now
	evidence := append([]string(nil), rec.Evidence...)
	for _, r := range receipts {
		if strings.TrimSpace(r) != "" {
			evidence = append(evidence, "receipt:"+strings.TrimSpace(r))
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Mission %s classified %s: %s\n", rec.MissionID, rec.Class, rec.Cause)
	fmt.Fprintf(&b, "Decision: %s — %s\n", rec.Decision, rec.Reason)
	if len(rec.Discarded) > 0 {
		fmt.Fprintf(&b, "Discarded: %s\n", strings.Join(rec.Discarded, " | "))
	}
	fmt.Fprintf(&b, "Result: %s\n", rec.Result)
	if rec.SuggestedMissionTo != "" {
		fmt.Fprintf(&b, "Suggested mission edge: --to %s\n", rec.SuggestedMissionTo)
	}
	return almanac.Observation{
		Type:       almanac.KindHypothesis,
		Title:      fmt.Sprintf("failure %s on %s: %s", rec.Class, rec.MissionID, rec.Decision),
		Content:    b.String(),
		TopicKey:   "failure/" + rec.Class,
		Domain:     "mission-recovery",
		Provenance: &almanac.Provenance{Task: rec.MissionID, Receipts: receipts},
		Source:     almanac.SourceCore,
		Author:     LessonAuthor,
		Scope:      almanac.ScopeTask,
		Confidence: ConfidenceFor(rec.Class),
		Evidence:   evidence,
	}
}
