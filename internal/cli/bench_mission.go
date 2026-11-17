// `regulus bench mission` (F13.8/NOV-111): full lifecycle journeys per
// mission class over the real engines — mission, governance, report
// and verify — in isolated stores. Exploration runs by default; the
// sealed holdout only runs with the explicit --holdout flag (access is
// audited, hypotheses stay frozen). Exit 0 only on PASS: anything else
// (FAIL, INSUFFICIENT_EVIDENCE) exits 1, usage exits 2.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"regulus-ai/internal/mbench"
)

// benchMissionSaved is the --save/--json payload: the gate verdict
// plus the per-case results it was decided from. Reproducible: a
// reviewer replays the suite and compares verdicts byte for byte.
type benchMissionSaved struct {
	Verdict mbench.Verdict      `json:"verdict"`
	Results []mbench.CaseResult `json:"results"`
}

func (c CLI) benchMissionCmd(args []string) int {
	fs := flag.NewFlagSet("bench mission", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	split := fs.String("split", "exploration", "exploration | holdout | all")
	holdout := fs.Bool("holdout", false, "allow sealed holdout cases (audited, hypotheses stay frozen)")
	save := fs.String("save", "", "write verdict+results as JSON to this path")
	asJSON := fs.Bool("json", false, "print verdict+results as JSON")
	dryRun := fs.Bool("dry-run", false, "validate corpus+splits and list selected cases without executing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus bench mission [--split exploration|holdout|all] [--holdout] [--save P] [--json] [--dry-run]")
		return 2
	}
	corpus := mbench.MissionCorpus()
	sp, code, ok := c.loadBenchMissionSplits(corpus)
	if !ok {
		return code
	}
	// Holdout discipline (same seal as mbench.SelectSplits): exploration
	// always, holdout/all only with the explicit flag. Without it the
	// refusal is usage (exit 2), never a quiet partial run.
	if (*split == "holdout" || *split == "all") && !*holdout {
		fmt.Fprintf(c.Stderr, "bench mission: --split %q includes sealed holdout: re-run with --holdout (access is audited, hypotheses stay frozen)\n", *split)
		return 2
	}
	cases, err := mbench.SelectSplits(sp, corpus, *split, *holdout)
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench mission: %v\n", err)
		return 2
	}
	// Fast-loop: --dry-run validates corpus+splits and lists the
	// selected cases without executing any journey. Zero engine cost.
	if *dryRun {
		printBenchMissionDryRun(c.Stdout, cases, *split)
		return 0
	}
	results := mbench.RunSuite(cases)
	v := mbench.Aggregate(results, corpus)
	saved := benchMissionSaved{Verdict: v, Results: results}
	if *save != "" {
		raw, err := json.MarshalIndent(saved, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "bench mission: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*save, raw, 0o644); err != nil {
			fmt.Fprintf(c.Stderr, "bench mission: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "evidence: verdict+results saved to %s\n", *save)
	}
	if *asJSON {
		raw, err := json.MarshalIndent(saved, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "bench mission: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
	} else {
		for _, r := range results {
			fmt.Fprintf(c.Stdout, "  - %s [%s]: assisted=%t baseline=%t violations=%d evidence=%t %s\n",
				r.CaseID, r.Class, r.AssistedOK, r.BaselineOK, r.Violations, r.EvidenceValid || r.SafeRefusal, shortMissionDetail(r.Detail))
		}
		fmt.Fprint(c.Stdout, v.Render())
	}
	// Gate verdict: exit 0 only on PASS — FAIL and
	// INSUFFICIENT_EVIDENCE both stop the close (exit 1).
	if v.Decision != mbench.DecisionPass {
		return 1
	}
	return 0
}

// benchMissionSplitsPath resolves the splits file: the tracked path,
// otherwise the project-root join for callers running from a
// subdirectory.
func (c CLI) benchMissionSplitsPath() string {
	if _, err := os.Stat(mbench.SplitsPath); err != nil {
		return c.dir() + "/" + mbench.SplitsPath
	}
	return mbench.SplitsPath
}

// loadBenchMissionSplits reads and validates the splits against the
// corpus. On failure it prints the refusal and reports exit 1.
func (c CLI) loadBenchMissionSplits(corpus []mbench.MissionCase) (mbench.MissionSplits, int, bool) {
	splits, err := mbench.LoadMissionSplits(c.benchMissionSplitsPath(), corpus)
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench mission: %v\n", err)
		return mbench.MissionSplits{}, 1, false
	}
	return splits, 0, true
}

// printBenchMissionDryRun lists the selected cases without executing
// any journey. Zero engine cost.
func printBenchMissionDryRun(stdout io.Writer, cases []mbench.MissionCase, split string) {
	fmt.Fprintf(stdout, "mission %s dry-run: %d case(s) [split=%s], no execution\n", mbench.GateVersion, len(cases), split)
	for _, cs := range cases {
		fmt.Fprintf(stdout, "  - %s [%s] %s\n", cs.ID, cs.Class, cs.Title)
	}
}

// shortMissionDetail keeps the per-case line to one short line.
func shortMissionDetail(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len(s) > 160 {
		return s[:160] + "…"
	}
	if s == "" {
		return "-"
	}
	return s
}
