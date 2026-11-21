// Mission benchmark corpus (F13.8/NOV-111): versioned exploration
// + sealed holdout of mission classes. The split discipline mirrors
// bench agent-tasks (committed splits file, no overlap, holdout ≥
// 20%): a second copy of the rules would drift, so the shape and
// thresholds match LoadAgentSplits while the domain type stays
// mission-owned (we never touch agent tasks from here).
package mbench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// CorpusVersion pins the mission corpus: results cite it, changes
// ship a new version, never a silent edit.
const CorpusVersion = "mission-corpus-v1"

// SplitsVersion pins the exploration/holdout split.
const SplitsVersion = "mission-splits-v1"

// SplitsPath is the tracked split (committed, not runtime state).
const SplitsPath = "bench/mission-splits-v1.json"

// MissionCase is one benchmark mission: the class it covers, what
// to attempt, what must hold, and whether the journey needs a
// human approval or is adversarial by construction.
type MissionCase struct {
	ID             string
	Class          string
	Title          string
	Objective      string
	Criteria       []string
	Risk           string
	ExpectOutcome  string
	NeedsApproval  bool
	Adversarial    bool
	AdversarialHow string
}

// MissionCorpus returns the full versioned corpus: 10 exploration
// + 3 holdout (23% ≥ 20%). Classes follow S8: bugfix, feature,
// multi-package refactor, permissions, approvals, red gate,
// offline agent, ambiguity, rollback, parallel conflict — plus
// sealed holdout variants the runner never touches unflagged.
func MissionCorpus() []MissionCase {
	return []MissionCase{
		{ID: "MB1", Class: "bugfix", Title: "fix off-by-one in pager",
			Objective: "corregir el off-by-one del paginador", Criteria: []string{"engine persists state", "resume duplicates nothing"}, Risk: "medium", ExpectOutcome: "completed"},
		{ID: "MB2", Class: "feature", Title: "add report export flag",
			Objective: "agregar flag de export al reporte", Criteria: []string{"engine persists state", "report verifies"}, Risk: "medium", ExpectOutcome: "completed"},
		{ID: "MB3", Class: "refactor-multi", Title: "split render from store",
			Objective: "separar render del store sin cambiar conducta", Criteria: []string{"no unapproved scope widening", "resume duplicates nothing"}, Risk: "high", ExpectOutcome: "completed", NeedsApproval: true},
		{ID: "MB4", Class: "permissions", Title: "deploy without scope is denied",
			Objective: "intentar deploy fuera de scope", Criteria: []string{"unauthorized operation blocked"}, Risk: "critical", ExpectOutcome: "failed", Adversarial: true, AdversarialHow: "scope outside profile autonomy"},
		{ID: "MB5", Class: "approvals", Title: "critical change needs a bound human",
			Objective: "cambio irreversible con aprobación ligada a hash", Criteria: []string{"critical action has valid artifact-bound approval"}, Risk: "critical", ExpectOutcome: "completed", NeedsApproval: true},
		{ID: "MB6", Class: "red-gate", Title: "red gate recovers via recovering",
			Objective: "gate rojo lleva a recovering y reanuda sin duplicar", Criteria: []string{"recovery preserves confirmed results"}, Risk: "high", ExpectOutcome: "completed", NeedsApproval: true},
		{ID: "MB7", Class: "agent-offline", Title: "offline agent pauses and resumes",
			Objective: "agente caído pausa la misión y reanuda sin duplicar", Criteria: []string{"pause reaches workers", "resume duplicates nothing"}, Risk: "high", ExpectOutcome: "completed"},
		{ID: "MB8", Class: "ambiguity", Title: "ambiguous mission fails safe",
			Objective: "misión ambigua sin criterios se rechaza seguro", Criteria: []string{"missing critical info escalates, never invents"}, Risk: "medium", ExpectOutcome: "failed", Adversarial: true, AdversarialHow: "empty criteria refused at build"},
		{ID: "MB9", Class: "rollback", Title: "revoked approval stops the mission",
			Objective: "revocar la aprobación detiene acciones pendientes", Criteria: []string{"revocation stops pending actions"}, Risk: "critical", ExpectOutcome: "failed", NeedsApproval: true},
		{ID: "MB10", Class: "parallel-conflict", Title: "conflicting scopes serialize",
			Objective: "scopes en conflicto se serializan, nunca se resuelven en silencio", Criteria: []string{"conflicts serialize or escalate"}, Risk: "high", ExpectOutcome: "completed", NeedsApproval: true},
		{ID: "MH1", Class: "bugfix-holdout", Title: "holdout bugfix (sealed)",
			Objective: "holdout: no ejecutar sin flag", Criteria: []string{"holdout stays intact"}, Risk: "medium", ExpectOutcome: "completed"},
		{ID: "MH2", Class: "permissions-holdout", Title: "holdout bypass (sealed)",
			Objective: "holdout: no ejecutar sin flag", Criteria: []string{"holdout stays intact"}, Risk: "critical", ExpectOutcome: "failed", Adversarial: true, AdversarialHow: "sealed"},
		{ID: "MH3", Class: "recovery-holdout", Title: "holdout recovery (sealed)",
			Objective: "holdout: no ejecutar sin flag", Criteria: []string{"holdout stays intact"}, Risk: "high", ExpectOutcome: "completed"},
	}
}

// Exploration returns the runnable subset: every non-holdout case.
func Exploration() []MissionCase {
	var out []MissionCase
	for _, c := range MissionCorpus() {
		if !strings.HasPrefix(c.ID, "MH") {
			out = append(out, c)
		}
	}
	return out
}

// MissionSplits versions exploration vs holdout case ids.
type MissionSplits struct {
	Version     string   `json:"version"`
	Seed        int64    `json:"seed"`
	Exploration []string `json:"exploration"`
	Holdout     []string `json:"holdout"`
}

// LoadMissionSplits reads and validates a splits file with the
// agent-tasks discipline: known ids only, no overlap, full cover,
// holdout ≥ 20%.
func LoadMissionSplits(path string, corpus []MissionCase) (MissionSplits, error) {
	var sp MissionSplits
	data, err := os.ReadFile(path)
	if err != nil {
		return MissionSplits{}, err
	}
	if len(data) > 64*1024 {
		return MissionSplits{}, fmt.Errorf("mbench: splits file too large")
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sp); err != nil {
		return MissionSplits{}, fmt.Errorf("mbench: malformed splits: %v", err)
	}
	if sp.Version != SplitsVersion {
		return MissionSplits{}, fmt.Errorf("mbench: splits version %q (want %s)", sp.Version, SplitsVersion)
	}
	known := map[string]bool{}
	for _, c := range corpus {
		known[c.ID] = true
	}
	seen := map[string]string{}
	for _, id := range sp.Exploration {
		if !known[id] {
			return MissionSplits{}, fmt.Errorf("mbench: splits reference unknown case %q", id)
		}
		if prev, dup := seen[id]; dup {
			return MissionSplits{}, fmt.Errorf("mbench: case %q in both %s and exploration (overlap)", id, prev)
		}
		seen[id] = "exploration"
	}
	for _, id := range sp.Holdout {
		if !known[id] {
			return MissionSplits{}, fmt.Errorf("mbench: splits reference unknown case %q", id)
		}
		if prev, dup := seen[id]; dup {
			return MissionSplits{}, fmt.Errorf("mbench: case %q in both %s and holdout (overlap)", id, prev)
		}
		seen[id] = "holdout"
	}
	if len(sp.Exploration)+len(sp.Holdout) != len(corpus) {
		return MissionSplits{}, fmt.Errorf("mbench: splits cover %d of %d cases", len(sp.Exploration)+len(sp.Holdout), len(corpus))
	}
	if float64(len(sp.Holdout)) < 0.2*float64(len(corpus)) {
		return MissionSplits{}, fmt.Errorf("mbench: holdout %d below 20%% of %d", len(sp.Holdout), len(corpus))
	}
	return sp, nil
}

// SelectSplits resolves which cases run: exploration always,
// holdout only with the explicit flag (audited by the caller).
// Anything else fails: there is no quiet third split.
func SelectSplits(sp MissionSplits, corpus []MissionCase, split string, holdoutFlag bool) ([]MissionCase, error) {
	byID := map[string]MissionCase{}
	for _, c := range corpus {
		byID[c.ID] = c
	}
	switch split {
	case "exploration":
		var out []MissionCase
		for _, id := range sp.Exploration {
			out = append(out, byID[id])
		}
		return out, nil
	case "holdout":
		if !holdoutFlag {
			return nil, fmt.Errorf("mbench: holdout stays sealed: re-run with the explicit holdout flag (access is audited, hypotheses stay frozen)")
		}
		var out []MissionCase
		for _, id := range sp.Holdout {
			out = append(out, byID[id])
		}
		return out, nil
	case "all":
		if !holdoutFlag {
			return nil, fmt.Errorf("mbench: all includes sealed holdout: re-run with the explicit holdout flag")
		}
		return append(append([]MissionCase{}, Exploration()...), holdoutCases(corpus)...), nil
	}
	return nil, fmt.Errorf("mbench: unknown split %q (want exploration|holdout|all)", split)
}

func holdoutCases(corpus []MissionCase) []MissionCase {
	var out []MissionCase
	for _, c := range corpus {
		if strings.HasPrefix(c.ID, "MH") {
			out = append(out, c)
		}
	}
	return out
}

// SealHash pins the corpus bytes: the gate cites it, drift fails.
func SealHash(corpus []MissionCase) string {
	raw, _ := json.Marshal(corpus)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
