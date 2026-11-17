// Phase gate verdict (F13.8/NOV-111): the explicit PASS / FAIL /
// INSUFFICIENT_EVIDENCE decision for the F13 close. The rules are
// the acceptance criteria, not judgment calls: zero violations,
// every outcome matching its oracle, every expediente verifying,
// every class journeyed, baseline contrasted, holdout sealed. What
// the run cannot prove (live multi-host, live human review, real
// agent dispatch) stays a limitation on the verdict — never a
// silent pass.
package mbench

import (
	"fmt"
	"sort"
	"strings"
)

// GateVersion pins the verdict rules.
const GateVersion = "mission-gate-v1"

// Decisions: the only three closings S8 allows.
const (
	DecisionPass         = "PASS"
	DecisionFail         = "FAIL"
	DecisionInsufficient = "INSUFFICIENT_EVIDENCE"
)

// Verdict is the aggregated gate decision with its N, rates and
// limitations. Reproducible from the results alone: a reviewer
// replays the suite and compares verdicts byte for byte.
type Verdict struct {
	Version        string
	Decision       string
	N              int
	Classes        []string
	SuccessRate    float64
	EvidenceRate   float64
	Violations     int
	BaselineRate   float64
	Limitations    []string
	CorpusVersion  string
	CorpusSeal     string
	SplitsVersion  string
	SimulatedNotes []string
}

// Aggregate decides over one full suite run. FAIL is sticky on any
// violation, outcome mismatch or unverifiable expediente.
// INSUFFICIENT covers thin evidence (empty suite, uncovered
// classes, missing baseline). PASS needs the full bar — and still
// carries the environmental limitations.
func Aggregate(results []CaseResult, corpus []MissionCase) Verdict {
	v := Verdict{
		Version:       GateVersion,
		CorpusVersion: CorpusVersion,
		CorpusSeal:    SealHash(corpus),
		SplitsVersion: SplitsVersion,
		N:             len(results),
		Limitations: []string{
			"single local host: no live multi-host run (recorded not-executed, never mocked-as-real)",
			"scripted approvals by fixed test identities: no live human review in the automated run (independent review = replay + expediente files)",
			"no live agent dispatch: journeys drive the real engines (mission, governance, report, verify) in isolated stores; agent/host behaviour is engine-decided signals, labelled simulated",
			"costs/tokens measured unknown: no metered host; wall-clock only",
		},
	}
	if len(results) == 0 {
		v.Decision = DecisionInsufficient
		v.Limitations = append(v.Limitations, "empty suite: N=0 proves nothing")
		return v
	}
	covered := map[string]bool{}
	ok, evidenced, base := 0, 0, 0
	simSeen := map[string]bool{}
	for _, r := range results {
		covered[r.Class] = true
		v.Violations += r.Violations
		if r.AssistedOK {
			ok++
		}
		if r.EvidenceValid || r.SafeRefusal {
			evidenced++
		}
		if r.BaselineOK {
			base++
		}
		for _, s := range r.Simulated {
			if !simSeen[s] {
				simSeen[s] = true
				v.SimulatedNotes = append(v.SimulatedNotes, s)
			}
		}
	}
	for c := range covered {
		v.Classes = append(v.Classes, c)
	}
	sort.Strings(v.Classes)
	n := float64(len(results))
	v.SuccessRate = float64(ok) / n
	v.EvidenceRate = float64(evidenced) / n
	v.BaselineRate = float64(base) / n

	want := map[string]bool{}
	for _, c := range Exploration() {
		want[c.Class] = true
	}
	var missing []string
	for class := range want {
		if !covered[class] {
			missing = append(missing, class)
		}
	}
	switch {
	case v.Violations > 0:
		v.Decision = DecisionFail
		v.Limitations = append(v.Limitations, fmt.Sprintf("%d permission/approval violations: objective zero missed", v.Violations))
	case ok != len(results):
		v.Decision = DecisionFail
		v.Limitations = append(v.Limitations, "outcome mismatch: at least one journey missed its oracle")
	case evidenced != len(results):
		v.Decision = DecisionFail
		v.Limitations = append(v.Limitations, "unverifiable expediente: at least one report failed integrity")
	case len(missing) > 0:
		sort.Strings(missing)
		v.Decision = DecisionInsufficient
		v.Limitations = append(v.Limitations, "uncovered classes: "+strings.Join(missing, ", "))
	case base != len(results):
		v.Decision = DecisionInsufficient
		v.Limitations = append(v.Limitations, "baseline contrast incomplete: assisted-vs-bare comparison missing")
	default:
		v.Decision = DecisionPass
	}
	return v
}

// Render quotes the verdict as readable text: the results stay
// authoritative, these lines are the quote.
func (v Verdict) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mission gate %s N=%d success=%.2f evidence=%.2f baseline=%.2f violations=%d\n",
		v.Decision, v.N, v.SuccessRate, v.EvidenceRate, v.BaselineRate, v.Violations)
	fmt.Fprintf(&b, "  classes: %s\n", strings.Join(v.Classes, ", "))
	fmt.Fprintf(&b, "  corpus %s seal %s splits %s\n", v.CorpusVersion, shortSeal(v.CorpusSeal), v.SplitsVersion)
	for _, l := range v.Limitations {
		fmt.Fprintf(&b, "  limitation: %s\n", l)
	}
	return b.String()
}

func shortSeal(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
