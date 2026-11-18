// Coverage tests for the mission leftovers (F13.1/NOV-104): human
// rendering plus the model/store edges the behavior and store
// tests do not pin (validation, idempotent replay, links, effects
// and failure-shaped stores). All in-process on t.TempDir(): no
// shells, no network, portable Linux+Windows.
package mission

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func covLinks() LinkInput {
	return LinkInput{
		RunIDs: []string{"run_2", "run_1"}, TaskIDs: []string{"t1"},
		AgentIDs: []string{"a1"}, Worktrees: []string{"wt/m1"},
		Commits: []string{"abc123"}, Traces: []string{"run_1"},
		Receipts: []string{"receipts/m1.json"},
	}
}

func TestCovRenderShowFull(t *testing.T) {
	m := mustMission(t, "m-full")
	m.Description = "durable missions"
	m.Repo = "regulus-ai"
	m.Workspace = "/repo"
	m.Constraints = []string{"no network"}
	m.Criteria = []string{"green CI"}
	m.Budget = Budget{TimeMS: 1000, Tokens: 200, Cost: 3.5}
	m.Creator = "tester"
	if err := m.AttachLinks(covLinks()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ApplyEffect("send", "batch-1", testNow(), func() (string, error) { return "batch-1", nil }); err != nil {
		t.Fatal(err)
	}
	m.PausedFrom = StateExecuting
	m.CancelReason = "holding"
	m.ReopenCount = 2
	out := RenderShow(m)
	for _, want := range []string{
		"mission m-full: created", "objective: ship the mission engine",
		"description: durable missions", "repo=regulus-ai workspace=/repo",
		"constraints: no network", "acceptance: green CI",
		"risk=medium", "budget(time=1000ms tokens=200 cost=3.5)",
		"creator: tester", "generation=0",
		"paused from: executing", "cancel reason: holding", "reopened: 2 time(s)",
		"run: run_1, run_2", "task: t1", "agent: a1", "worktree: wt/m1",
		"commit: abc123", "trace: run_1", "receipt: receipts/m1.json",
		"applied effects: send", "history: 1 event(s)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show must quote %q:\n%s", want, out)
		}
	}
}

func TestCovRenderShowMinimal(t *testing.T) {
	m := mustMission(t, "m-min")
	// mustMission carries optional detail; the minimal shape is the
	// same record with every optional section cleared (RenderShow
	// guards each block on non-empty).
	m.Description, m.Repo, m.Workspace, m.Constraints, m.Criteria = "", "", "", nil, nil
	out := RenderShow(m)
	if !strings.Contains(out, "mission m-min: created") {
		t.Fatalf("show must name the mission:\n%s", out)
	}
	for _, absent := range []string{"description:", "repo=", "constraints:", "acceptance:", "budget(", "paused from:", "cancel reason:", "reopened:", "  run:", "applied effects:"} {
		if strings.Contains(out, absent) {
			t.Fatalf("minimal show must omit %q:\n%s", absent, out)
		}
	}
}

func TestCovRenderStatusTrail(t *testing.T) {
	m := mustMission(t, "m-st")
	mustTransition(t, m, StateAnalyzing)
	out := RenderStatus(m)
	if !strings.Contains(out, "mission m-st: analyzing generation=0") {
		t.Fatalf("status headlines the mission:\n%s", out)
	}
	if !strings.Contains(out, "- m-st analyzing -> analyzing by tester") {
		t.Fatalf("status audits every event:\n%s", out)
	}
	m.PausedFrom = StateAnalyzing
	paused := RenderStatus(m)
	if !strings.Contains(paused, "(paused from analyzing)") {
		t.Fatalf("status quotes the pause bookmark:\n%s", paused)
	}
}

func TestCovRenderListMixed(t *testing.T) {
	entries := []ListEntry{
		{ID: "m-good", State: StateExecuting, Objective: "go", Generation: 3},
		{ID: "m-bad", Problem: "sha mismatch (edited store is not evidence)"},
	}
	out := RenderList(entries)
	for _, want := range []string{
		"2 mission(s)", "- m-good executing gen=3", "objective: go",
		"- m-bad CORRUPT: sha mismatch",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("list must quote %q:\n%s", want, out)
		}
	}
	if got := RenderList(nil); !strings.Contains(got, "0 mission(s)") {
		t.Fatalf("empty list still reports: %q", got)
	}
}

func TestCovValidateEdges(t *testing.T) {
	m := mustMission(t, "m-val")
	m.PausedFrom = "teleport"
	if err := m.Validate(); err == nil {
		t.Fatal("unknown pause bookmarks must refuse")
	}
	m.PausedFrom = ""
	m.Generation = -1
	if err := m.Validate(); err == nil {
		t.Fatal("negative generations must refuse")
	}
	m.Generation = 0
	m.Risk = "extreme"
	if err := m.Validate(); err == nil {
		t.Fatal("unknown risk must refuse")
	}
	m.Risk = RiskLow
	m.State = "teleport"
	if err := m.Validate(); err == nil {
		t.Fatal("unknown states must refuse")
	}
	m.State = StateCreated
	m.Objective = ""
	if err := m.Validate(); err == nil {
		t.Fatal("empty objectives must refuse")
	}
	m.Objective = "ship it"
	if err := ValidateID("../escape"); err == nil {
		t.Fatal("traversal ids must refuse")
	}
	for _, id := range []string{"", "a b", "a\nb", strings.Repeat("a", 129)} {
		if err := ValidateID(id); err == nil {
			t.Fatalf("hostile id %q must refuse", id)
		}
	}
	if err := ValidateID("ok-id_1./x"); err != nil {
		t.Fatalf("conservative ids must pass: %v", err)
	}
}

func TestCovSeenIdemAndReplay(t *testing.T) {
	m := mustMission(t, "m-idem")
	if m.SeenIdem("key-1") {
		t.Fatal("unseen keys must not replay")
	}
	if _, err := m.Transition(StateAnalyzing, "op", "go", "key-1", testNow()); err != nil {
		t.Fatal(err)
	}
	if !m.SeenIdem("key-1") {
		t.Fatal("stored keys must replay")
	}
	n := len(m.History)
	if _, err := m.Transition(StatePlanning, "op", "go again", "key-1", testNow()); err != nil {
		t.Fatal(err)
	}
	if m.State != StateAnalyzing {
		t.Fatalf("idem replay on another edge must not move the mission: %q", m.State)
	}
	if len(m.History) != n {
		t.Fatal("idem replay must not append history")
	}
	same, err := m.Transition(StateAnalyzing, "op", "noop", "", testNow())
	if err != nil {
		t.Fatal(err)
	}
	if same.From != StateAnalyzing || same.To != StateAnalyzing {
		t.Fatalf("same-state transitions are idempotent no-ops: %+v", same)
	}
	if len(m.History) != n {
		t.Fatal("same-state moves append nothing")
	}
}

func TestCovPauseClearsBookmarkOnAdvance(t *testing.T) {
	m := mustMission(t, "m-pause")
	mustTransition(t, m, StateAnalyzing)
	if _, err := m.Pause("op", "hold", "", testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Transition(StatePlanning, "op", "skip resume, advance explicitly", "", testNow()); err != nil {
		t.Fatalf("blocked advances explicitly: %v", err)
	}
	if m.PausedFrom != "" {
		t.Fatalf("leaving blocked clears the bookmark, got %q", m.PausedFrom)
	}
}

func TestCovApplyEffectBranches(t *testing.T) {
	m := mustMission(t, "m-fx")
	calls := 0
	if _, err := m.ApplyEffect("", "x", testNow(), func() (string, error) { return "x", nil }); err == nil {
		t.Fatal("empty effect keys must refuse")
	}
	if _, err := m.ApplyEffect(strings.Repeat("k", 300), "x", testNow(), func() (string, error) { return "x", nil }); err == nil {
		t.Fatal("overlong effect keys must refuse")
	}
	if _, err := m.ApplyEffect("a\nb", "x", testNow(), func() (string, error) { return "x", nil }); err == nil {
		t.Fatal("multiline effect keys must refuse")
	}
	if _, err := m.ApplyEffect("flaky", "", testNow(), func() (string, error) {
		calls++
		return "", errors.New("boom")
	}); err == nil {
		t.Fatal("failing effects must surface")
	}
	res, err := m.ApplyEffect("flaky", "", testNow(), func() (string, error) {
		calls++
		return "recovered", nil
	})
	if err != nil || res.Output != "recovered" || calls != 2 {
		t.Fatalf("failures leave no record: retry must rerun (calls=%d): %+v %v", calls, res, err)
	}
}

func TestCovAttachLinksRefusals(t *testing.T) {
	m := mustMission(t, "m-lr")
	before := len(m.Links.RunIDs)
	for _, in := range []LinkInput{
		{TaskIDs: []string{"../x"}},
		{AgentIDs: []string{"a\nb"}},
		{Traces: []string{""}},
		{Worktrees: []string{""}},
		{Commits: []string{strings.Repeat("c", 300)}},
		{Receipts: []string{"a/../../x"}},
		{Worktrees: []string{"a\nb"}},
	} {
		if err := m.AttachLinks(in); err == nil {
			t.Fatalf("hostile link must refuse: %+v", in)
		}
	}
	if len(m.Links.RunIDs) != before {
		t.Fatal("refused attaches mutate nothing")
	}
}

func TestCovStoreRefusals(t *testing.T) {
	dir := t.TempDir()
	if _, err := CreateMission(dir, nil, testNow()); err == nil {
		t.Fatal("nil missions never store")
	}
	bad := mustMission(t, "m-badstore")
	bad.State = "teleport"
	if _, err := CreateMission(dir, bad, testNow()); err == nil {
		t.Fatal("invalid missions never store")
	}
	if _, err := StoreDir(dir, "../escape"); err == nil {
		t.Fatal("traversal ids never resolve")
	}
	if _, _, err := LoadMission(dir, "../escape"); err == nil {
		t.Fatal("traversal ids never load")
	}
	if _, _, err := LoadMission(dir, "msn_missing"); err == nil {
		t.Fatal("missing stores refuse")
	}
	if _, err := UpdateMission(dir, "msn_missing", testNow(), func(m *Mission) error { return nil }); err == nil {
		t.Fatal("missing stores never update")
	}
	if _, err := SaveMission(dir, nil, 0, testNow()); err == nil {
		t.Fatal("nil saves never store")
	}
	mustCreate(t, dir, "m-sv")
	ghost := mustMission(t, "m-ghostsave")
	if _, err := SaveMission(dir, ghost, 99, testNow()); err == nil {
		t.Fatal("unknown generations never save")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("must name the missing store, got %v", err)
	}
	entries, err := ListMissions(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Refused and missing stores still created their store dirs
	// (LockDir mkdirs before the generation check refuses; UpdateMission
	// likewise materializes the dir before failing), so the list reports
	// them as corrupt entries with their problems instead of skipping
	// them silently: m-sv clean, m-ghostsave + msn_missing corrupt.
	if len(entries) != 3 {
		t.Fatalf("healthy plus two refused stores list: %v", entries)
	}
	byID := map[string]ListEntry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	good, ok := byID["m-sv"]
	if !ok || good.Problem != "" || good.State != StateCreated {
		t.Fatalf("healthy store must list clean: %v", entries)
	}
	for _, bad := range []string{"m-ghostsave", "msn_missing"} {
		corrupt, ok := byID[bad]
		if !ok || corrupt.Problem == "" {
			t.Fatalf("refused store %q must surface as a corrupt entry with its problem: %v", bad, entries)
		}
	}
	empty, err := ListMissions(t.TempDir())
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing base lists empty: %v %v", empty, err)
	}
}

func TestCovLoadDriftFailsClosed(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-drift")
	store := filepath.Join(dir, ".regulus", "missions", "m-drift")
	raw, err := os.ReadFile(filepath.Join(store, MissionFile))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), `"mission_id": "m-drift"`, `"mission_id": "m-other"`, 1)
	if edited == string(raw) {
		t.Fatal("fixture must carry the mission id")
	}
	if err := os.WriteFile(filepath.Join(store, MissionFile), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	fixManifestSHA(t, store)
	if _, _, err := LoadMission(dir, "m-drift"); err == nil {
		t.Fatal("id drift must refuse")
	} else if !strings.Contains(err.Error(), "id drift") {
		t.Fatalf("must name the drift, got %v", err)
	}
	mustCreate(t, dir, "m-gendrift")
	gstore := filepath.Join(dir, ".regulus", "missions", "m-gendrift")
	graw, err := os.ReadFile(filepath.Join(gstore, MissionFile))
	if err != nil {
		t.Fatal(err)
	}
	bumped := strings.Replace(string(graw), `"generation": 1`, `"generation": 2`, 1)
	if bumped == string(graw) {
		t.Fatal("fixture must carry a generation")
	}
	if err := os.WriteFile(filepath.Join(gstore, MissionFile), []byte(bumped), 0o644); err != nil {
		t.Fatal(err)
	}
	fixManifestSHA(t, gstore)
	if _, _, err := LoadMission(dir, "m-gendrift"); err == nil {
		t.Fatal("generation drift must refuse")
	} else if !strings.Contains(err.Error(), "generation drift") {
		t.Fatalf("must name the drift, got %v", err)
	}
}

func TestCovMigrateLegacyUnknownState(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-leg")
	store := filepath.Join(dir, ".regulus", "missions", "m-leg")
	legacy := legacyV0{
		MissionID: "m-leg", Objective: "legacy", State: "teleport",
		CreatedAt: testNow(), UpdatedAt: testNow(),
	}
	raw, _ := json.MarshalIndent(legacy, "", "  ")
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(store, MissionFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	manRaw, err := os.ReadFile(filepath.Join(store, MissionMeta))
	if err != nil {
		t.Fatal(err)
	}
	var man MissionManifest
	if err := json.Unmarshal(manRaw, &man); err != nil {
		t.Fatal(err)
	}
	man.Version = LegacyVersion
	out, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(filepath.Join(store, MissionMeta), append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	fixManifestSHA(t, store)
	if _, _, err := LoadMission(dir, "m-leg"); err == nil {
		t.Fatal("legacy unknown states never migrate")
	} else if !strings.Contains(err.Error(), "migrate explicitly") {
		t.Fatalf("must refuse to guess, got %v", err)
	}
}
