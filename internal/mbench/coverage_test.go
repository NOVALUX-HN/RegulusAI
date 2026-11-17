// Coverage tests (odd/tasks/coverage-80 T2): corpus validation errors,
// splits selection, per-class journeys, empty suite, aggregate
// branches and verdict rendering. All in-process; temp dirs only.
package mbench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSplitsFile(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "splits.json")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func validSplitsJSON() string {
	return `{"version":"mission-splits-v1","seed":20261117,` +
		`"exploration":["MB1","MB2","MB3","MB4","MB5","MB6","MB7","MB8","MB9","MB10"],` +
		`"holdout":["MH1","MH2","MH3"]}`
}

func TestCoverageCorpusShape(t *testing.T) {
	corpus := MissionCorpus()
	if len(corpus) != 13 {
		t.Fatalf("corpus = %d (want 13)", len(corpus))
	}
	seen := map[string]bool{}
	for _, c := range corpus {
		if c.ID == "" || c.Class == "" || c.Objective == "" || len(c.Criteria) == 0 {
			t.Errorf("case %+v misses identity fields", c)
		}
		if seen[c.ID] {
			t.Errorf("duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
		if c.ExpectOutcome != "completed" && c.ExpectOutcome != "failed" {
			t.Errorf("case %s has open outcome %q", c.ID, c.ExpectOutcome)
		}
		if c.Adversarial && c.AdversarialHow == "" {
			t.Errorf("adversarial case %s must name how", c.ID)
		}
	}
	exp := Exploration()
	if len(exp) != 10 {
		t.Fatalf("exploration = %d (want 10)", len(exp))
	}
	for i, c := range exp {
		if c.ID != corpus[i].ID {
			t.Errorf("exploration order drift at %d: %q vs %q", i, c.ID, corpus[i].ID)
		}
		if strings.HasPrefix(c.ID, "MH") {
			t.Errorf("exploration leaks holdout %q", c.ID)
		}
	}
	if got := SealHash(corpus); got == "" {
		t.Fatal("seal empty")
	} else if again := SealHash(MissionCorpus()); got != again {
		t.Fatal("seal unstable across identical corpora")
	}
	mutated := append([]MissionCase(nil), corpus...)
	mutated[0].Title = "tampered title"
	if SealHash(mutated) == SealHash(corpus) {
		t.Fatal("seal did not move on corpus drift")
	}
	if SealHash(nil) == "" {
		t.Fatal("seal of empty corpus must still pin bytes")
	}
}

func TestCoverageLoadSplitsErrors(t *testing.T) {
	corpus := MissionCorpus()
	cases := []struct {
		name    string
		data    string
		missing bool
		big     bool
		want    string
	}{
		{"missing file", "", true, false, "no such file"},
		{"too large", "", false, true, "too large"},
		{"malformed", `{"version":`, false, false, "malformed"},
		{"unknown field", `{"version":"mission-splits-v1","seed":1,"exploration":[],"holdout":[],"extra":1}`, false, false, "unknown field"},
		{"version mismatch", `{"version":"mission-splits-v9","seed":1,"exploration":[],"holdout":[]}`, false, false, "splits version"},
		{"unknown exploration", `{"version":"mission-splits-v1","seed":1,"exploration":["MB1","NOPE"],"holdout":[]}`, false, false, "unknown case"},
		{"unknown holdout", `{"version":"mission-splits-v1","seed":1,"exploration":[],"holdout":["NOPE"]}`, false, false, "unknown case"},
		{"dup within exploration", `{"version":"mission-splits-v1","seed":1,"exploration":["MB1","MB1"],"holdout":[]}`, false, false, "overlap"},
		{"dup within holdout", `{"version":"mission-splits-v1","seed":1,"exploration":[],"holdout":["MH1","MH1"]}`, false, false, "overlap"},
		{"holdout overlaps exploration", `{"version":"mission-splits-v1","seed":1,"exploration":["MB1"],"holdout":["MB1"]}`, false, false, "overlap"},
		{"incomplete cover", `{"version":"mission-splits-v1","seed":1,"exploration":["MB1","MB2"],"holdout":["MH1"]}`, false, false, "cover"},
		{"thin holdout", `{"version":"mission-splits-v1","seed":1,` +
			`"exploration":["MB1","MB2","MB3","MB4","MB5","MB6","MB7","MB8","MB9","MB10","MH1","MH2"],` +
			`"holdout":["MH3"]}`, false, false, "below 20%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			switch {
			case tc.missing:
				path = filepath.Join(t.TempDir(), "absent.json")
			case tc.big:
				path = writeSplitsFile(t, strings.Repeat("x", 65*1024))
			default:
				path = writeSplitsFile(t, tc.data)
			}
			if _, err := LoadMissionSplits(path, corpus); err == nil {
				t.Fatalf("LoadMissionSplits(%s) must fail", tc.name)
			} else if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) &&
				!(tc.missing && err != nil) {
				t.Fatalf("LoadMissionSplits(%s) = %v (want %q)", tc.name, err, tc.want)
			}
		})
	}
}

func TestCoverageSelectSplitsAll(t *testing.T) {
	corpus := MissionCorpus()
	sp, err := LoadMissionSplits(writeSplitsFile(t, validSplitsJSON()), corpus)
	if err != nil {
		t.Fatal(err)
	}
	all, err := SelectSplits(sp, corpus, "all", true)
	if err != nil {
		t.Fatalf("SelectSplits(all, flagged): %v", err)
	}
	if len(all) != 13 {
		t.Fatalf("all = %d (want 13)", len(all))
	}
	for i, id := range []string{"MB1", "MB2", "MB3", "MB4", "MB5", "MB6", "MB7", "MB8", "MB9", "MB10"} {
		if all[i].ID != id {
			t.Fatalf("all[%d] = %q (want %q, exploration first)", i, all[i].ID, id)
		}
	}
	hold, err := SelectSplits(sp, corpus, "holdout", true)
	if err != nil || len(hold) != 3 {
		t.Fatalf("holdout flagged = %d, %v", len(hold), err)
	}
	for _, c := range hold {
		if !strings.HasPrefix(c.ID, "MH") {
			t.Fatalf("holdout leaks exploration %q", c.ID)
		}
	}
	exp, err := SelectSplits(sp, corpus, "exploration", false)
	if err != nil || len(exp) != 10 {
		t.Fatalf("exploration = %d, %v", len(exp), err)
	}
}

func TestCoverageRunCasePerClass(t *testing.T) {
	wantSimulated := map[string]bool{
		"MB4": true, "MB6": true, "MB7": true, "MB9": true,
	}
	for _, c := range Exploration() {
		c := c
		t.Run(c.ID, func(t *testing.T) {
			r := RunCase(c)
			if r.CaseID != c.ID || r.Class != c.Class {
				t.Fatalf("identity drift: %+v (want %s/%s)", r, c.ID, c.Class)
			}
			if r.DurationMS < 0 {
				t.Fatalf("negative duration: %+v", r)
			}
			if r.Simulated == nil {
				t.Fatal("Simulated must stay non-nil (append-only discipline)")
			}
			if c.ID == "MB8" {
				if !r.AssistedOK || !r.SafeRefusal || !r.BaselineOK {
					t.Fatalf("MB8 must refuse safe at build: %+v", r)
				}
				if r.EvidenceValid {
					t.Fatalf("safe refusal carries no expediente: %+v", r)
				}
				if !strings.Contains(r.Detail, "refused at build") {
					t.Fatalf("MB8 detail must quote the refusal: %q", r.Detail)
				}
				return
			}
			if !r.AssistedOK || !r.EvidenceValid || !r.BaselineOK || r.Violations != 0 {
				t.Fatalf("case %s not green: %+v", c.ID, r)
			}
			if wantSimulated[c.ID] && len(r.Simulated) == 0 {
				t.Fatalf("case %s must label its simulated signal: %+v", c.ID, r)
			}
			if !wantSimulated[c.ID] && len(r.Simulated) != 0 {
				t.Fatalf("case %s must run without simulated signals: %+v", c.ID, r)
			}
			if !strings.Contains(r.Detail, "journey green") {
				t.Fatalf("detail must quote green: %q", r.Detail)
			}
		})
	}
}

func TestCoverageRunSuiteEmptyAndOrder(t *testing.T) {
	if got := RunSuite(nil); len(got) != 0 {
		t.Fatalf("empty suite = %d (want 0)", len(got))
	}
	exp := Exploration()
	got := RunSuite(exp[:2])
	if len(got) != 2 || got[0].CaseID != exp[0].ID || got[1].CaseID != exp[1].ID {
		t.Fatalf("suite must preserve order: %+v", got)
	}
}

func greenResultsFor(classes []MissionCase, mutate func(*CaseResult)) []CaseResult {
	out := make([]CaseResult, 0, len(classes))
	for _, c := range classes {
		out = append(out, CaseResult{
			CaseID: c.ID, Class: c.Class,
			AssistedOK: true, BaselineOK: true, EvidenceValid: true,
			Simulated: []string{"sim-a", "sim-a"},
		})
	}
	for i := range out {
		mutate(&out[i])
	}
	return out
}

func TestCoverageAggregateBranches(t *testing.T) {
	corpus := MissionCorpus()
	exp := Exploration()

	t.Run("pass synthetic", func(t *testing.T) {
		v := Aggregate(greenResultsFor(exp, func(*CaseResult) {}), corpus)
		if v.Decision != DecisionPass {
			t.Fatalf("gate = %s (want PASS):\n%s", v.Decision, v.Render())
		}
		if v.N != 10 || v.SuccessRate != 1 || v.EvidenceRate != 1 || v.BaselineRate != 1 || v.Violations != 0 {
			t.Fatalf("weak verdict: %+v", v)
		}
		if len(v.Classes) != 10 || v.Classes[0] != "agent-offline" {
			t.Fatalf("classes must sort: %v", v.Classes)
		}
		if len(v.SimulatedNotes) != 1 || v.SimulatedNotes[0] != "sim-a" {
			t.Fatalf("simulated notes must dedupe: %v", v.SimulatedNotes)
		}
		if v.CorpusVersion != CorpusVersion || v.SplitsVersion != SplitsVersion || v.CorpusSeal != SealHash(corpus) {
			t.Fatalf("verdict must pin versions+seal: %+v", v)
		}
	})

	t.Run("fail on outcome mismatch", func(t *testing.T) {
		res := greenResultsFor(exp, func(*CaseResult) {})
		res[3].AssistedOK = false
		v := Aggregate(res, corpus)
		if v.Decision != DecisionFail {
			t.Fatalf("gate = %s (want FAIL)", v.Decision)
		}
		if !strings.Contains(strings.Join(v.Limitations, "\n"), "outcome mismatch") {
			t.Fatalf("limitations must name the mismatch:\n%s", v.Render())
		}
		if v.SuccessRate != 0.9 {
			t.Fatalf("success rate = %.2f (want 0.90)", v.SuccessRate)
		}
	})

	t.Run("fail on unverifiable expediente", func(t *testing.T) {
		res := greenResultsFor(exp, func(*CaseResult) {})
		res[0].EvidenceValid = false
		v := Aggregate(res, corpus)
		if v.Decision != DecisionFail {
			t.Fatalf("gate = %s (want FAIL)", v.Decision)
		}
		if !strings.Contains(strings.Join(v.Limitations, "\n"), "unverifiable expediente") {
			t.Fatalf("limitations must name integrity:\n%s", v.Render())
		}
	})

	t.Run("insufficient on thin baseline", func(t *testing.T) {
		res := greenResultsFor(exp, func(*CaseResult) {})
		res[0].BaselineOK = false
		v := Aggregate(res, corpus)
		if v.Decision != DecisionInsufficient {
			t.Fatalf("gate = %s (want INSUFFICIENT)", v.Decision)
		}
		if !strings.Contains(strings.Join(v.Limitations, "\n"), "baseline contrast") {
			t.Fatalf("limitations must name baseline:\n%s", v.Render())
		}
	})

	t.Run("safe refusal counts as evidence", func(t *testing.T) {
		res := []CaseResult{{CaseID: "MB8", Class: "ambiguity",
			AssistedOK: true, SafeRefusal: true, BaselineOK: true}}
		v := Aggregate(res, corpus)
		if v.EvidenceRate != 1 {
			t.Fatalf("safe refusal must evidence: %+v", v)
		}
		if v.Decision != DecisionInsufficient {
			t.Fatalf("gate = %s (want INSUFFICIENT on uncovered classes)", v.Decision)
		}
		if !strings.Contains(strings.Join(v.Limitations, "\n"), "uncovered classes") {
			t.Fatalf("limitations must name missing classes:\n%s", v.Render())
		}
	})
}

func TestCoverageVerdictRender(t *testing.T) {
	corpus := MissionCorpus()
	v := Aggregate(greenResultsFor(Exploration(), func(*CaseResult) {}), corpus)
	text := v.Render()
	for _, want := range []string{
		"mission gate PASS N=10", "success=1.00", "evidence=1.00", "baseline=1.00",
		"violations=0", "classes: ", "corpus mission-corpus-v1",
		"splits mission-splits-v1", "limitation: ",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("render misses %q:\n%s", want, text)
		}
	}
	if seal := v.CorpusSeal[:12]; !strings.Contains(text, seal) {
		t.Errorf("render must quote the short seal %q:\n%s", seal, text)
	}
	if got := shortSeal("abc"); got != "abc" {
		t.Errorf("shortSeal(short) = %q (want identity)", got)
	}
	if got := shortSeal(""); got != "" {
		t.Errorf("shortSeal(empty) = %q", got)
	}
}

func TestCoverageAdvanceChainRefused(t *testing.T) {
	if err := advanceChain(t.TempDir(), "absent", "t8", forwardToExecuting...); err == nil {
		t.Fatal("advanceChain on a missing mission must fail, never skip the lifecycle")
	}
}
