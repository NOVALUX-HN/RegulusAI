// Store tests for F13.1/NOV-104: durable atomic persistence,
// checksum integrity, versioned migration, generation-conflict
// concurrency, crash recovery and cancel/resume durability.
package mission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func shaOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func mustCreate(t *testing.T, dir, id string) *Mission {
	t.Helper()
	m, err := NewMission(testInput(id), testNow())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateMission(dir, m, testNow()); err != nil {
		t.Fatalf("CreateMission: %v", err)
	}
	return m
}

func TestCreateLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m1")
	man, loaded, err := LoadMission(dir, "m1")
	if err != nil {
		t.Fatalf("LoadMission: %v", err)
	}
	if man.MissionID != "m1" || man.Generation != 1 {
		t.Fatalf("manifest = %+v, want id m1 generation 1", man)
	}
	if loaded.State != StateCreated || loaded.Generation != 1 {
		t.Fatalf("loaded = %+v", loaded)
	}
	if _, err := CreateMission(dir, mustMission(t, "m1"), testNow()); err == nil {
		t.Fatal("double create must refuse")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("must name the existing mission, got %q", err)
	}
}

func TestUpdateAdvancesGeneration(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m2")
	updated, err := UpdateMission(dir, "m2", testNow(), func(m *Mission) error {
		_, err := m.Transition(StateAnalyzing, "op", "go", "", testNow())
		return err
	})
	if err != nil {
		t.Fatalf("UpdateMission: %v", err)
	}
	if updated.Generation != 2 || updated.State != StateAnalyzing {
		t.Fatalf("updated = gen %d state %q", updated.Generation, updated.State)
	}
	_, loaded, err := LoadMission(dir, "m2")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Generation != 2 || loaded.State != StateAnalyzing {
		t.Fatal("stored state must reflect the confirmed update")
	}
}

func TestStaleSaveRefusesConflict(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m3")
	_, first, err := LoadMission(dir, "m3")
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := LoadMission(dir, "m3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Transition(StateAnalyzing, "a", "first wins", "", testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveMission(dir, first, first.Generation, testNow()); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if _, err := second.Transition(StateAnalyzing, "b", "stale", "", testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveMission(dir, second, 1, testNow()); err == nil {
		t.Fatal("stale save must refuse with a generation conflict")
	} else if !strings.Contains(err.Error(), "generation conflict") {
		t.Fatalf("must name the conflict, got %q", err)
	}
	_, loaded, err := LoadMission(dir, "m3")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StateAnalyzing || loaded.Generation != 2 {
		t.Fatalf("loser must not corrupt the store: %+v", loaded)
	}
}

// Concurrent coordinators racing the same edge must never corrupt
// the store: serialized saves seal valid checksums (a conflicting
// stale write still refuses, see TestStaleSaveRefusesConflict) and
// the single real move appends exactly one audit event.
func TestConcurrentCoordinatorsStayConsistent(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-race")
	const racers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < 8; attempt++ {
				_, m, err := LoadMission(dir, "m-race")
				if err != nil {
					return
				}
				if _, err := m.Transition(StateAnalyzing, "racer", "go", "", testNow()); err != nil {
					return
				}
				if _, err := SaveMission(dir, m, m.Generation, testNow()); err == nil {
					mu.Lock()
					wins++
					mu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()
	if wins != racers {
		t.Fatalf("serialized saves must all land with retry, got %d of %d", wins, racers)
	}
	_, loaded, err := LoadMission(dir, "m-race")
	if err != nil {
		t.Fatalf("store must stay loadable after the race: %v", err)
	}
	if loaded.State != StateAnalyzing {
		t.Fatalf("state = %q, want analyzing", loaded.State)
	}
	if loaded.Generation != 1+int64(wins) {
		t.Fatalf("generation = %d, want %d (one per sealed write)", loaded.Generation, 1+wins)
	}
	if len(loaded.History) != 2 {
		t.Fatalf("history = %d events, want 2 (create + the single real move)", len(loaded.History))
	}
}

func TestEditedStoreIsNotEvidence(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-edit")
	path := filepath.Join(dir, ".regulus", "missions", "m-edit", MissionFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), StateCreated, StateCompleted, 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadMission(dir, "m-edit"); err == nil {
		t.Fatal("edited store must refuse")
	} else if !strings.Contains(err.Error(), "sha mismatch") {
		t.Fatalf("must name the sha mismatch, got %q", err)
	}
}

func TestMalformedStoreFailsClosed(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-mal")
	meta := filepath.Join(dir, ".regulus", "missions", "m-mal", MissionMeta)
	if err := os.WriteFile(meta, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadMission(dir, "m-mal"); err == nil {
		t.Fatal("malformed manifest must refuse")
	} else if !strings.Contains(err.Error(), "malformed manifest") {
		t.Fatalf("must name the malformed manifest, got %q", err)
	}
}

func TestMigrateV0Explicitly(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-v0")
	store := filepath.Join(dir, ".regulus", "missions", "m-v0")
	// Rewrite the record as the legacy shape with a matching
	// legacy manifest: same bytes discipline, older version.
	legacy := legacyV0{
		MissionID: "m-v0", Objective: "legacy objective",
		Description: "old", Repo: "r", State: StateExecuting,
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
	manOut, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(filepath.Join(store, MissionMeta), append(manOut, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	// Recompute the sha so the legacy bytes verify: the
	// migration path, not the tamper path, is under test.
	fixManifestSHA(t, store)
	_, loaded, err := LoadMission(dir, "m-v0")
	if err != nil {
		t.Fatalf("LoadMission of legacy shape: %v", err)
	}
	if loaded.Version != MissionVersion || loaded.State != StateExecuting {
		t.Fatalf("migrated = %+v", loaded)
	}
	if loaded.Risk != RiskMedium || loaded.Generation != 0 {
		t.Fatalf("migration defaults wrong: %+v", loaded)
	}
	if len(loaded.History) != 1 || !strings.Contains(loaded.History[0].Reason, "mission-v0") {
		t.Fatalf("migration must enter the audit trail: %+v", loaded.History)
	}
	// Sealing the migration persists v1 durably.
	if _, err := SaveMission(dir, loaded, loaded.Generation, testNow()); err != nil {
		t.Fatalf("sealing the migration: %v", err)
	}
	man2, sealed, err := LoadMission(dir, "m-v0")
	if err != nil {
		t.Fatal(err)
	}
	if man2.Version != MissionVersion || sealed.Generation != 1 {
		t.Fatalf("sealed = %+v gen %d", man2, sealed.Generation)
	}
}

func fixManifestSHA(t *testing.T, store string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(store, MissionFile))
	if err != nil {
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
	man.MissionSHA = shaOf(raw)
	out, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(filepath.Join(store, MissionMeta), append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownVersionRefused(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-future")
	store := filepath.Join(dir, ".regulus", "missions", "m-future")
	manRaw, err := os.ReadFile(filepath.Join(store, MissionMeta))
	if err != nil {
		t.Fatal(err)
	}
	var man MissionManifest
	if err := json.Unmarshal(manRaw, &man); err != nil {
		t.Fatal(err)
	}
	man.Version = "mission-v9"
	out, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(filepath.Join(store, MissionMeta), append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadMission(dir, "m-future"); err == nil {
		t.Fatal("unknown version must refuse")
	} else if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("must name the unsupported version, got %q", err)
	}
}

func TestCrashBetweenCriticalTransitionsRecoversConfirmed(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-crash")
	path := []string{
		StateAnalyzing, StatePlanning, StateAwaitingApproval,
		StateExecuting, StateIntegrating, StateValidating, StateCompleted,
	}
	for i, to := range path {
		_, m, err := LoadMission(dir, "m-crash")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Transition(to, "op", "step", "", testNow()); err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			// Simulate the crash: mutate, never persist. The
			// next load must see the previous confirmed state.
			_, again, err := LoadMission(dir, "m-crash")
			if err != nil {
				t.Fatal(err)
			}
			want := StateCreated
			if i > 0 {
				want = path[i-1]
			}
			if again.State != want {
				t.Fatalf("after dropped transition %d: state = %q, want confirmed %q", i, again.State, want)
			}
			if _, err := SaveMission(dir, m, m.Generation, testNow()); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := SaveMission(dir, m, m.Generation, testNow()); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, loaded, err := LoadMission(dir, "m-crash")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StateCompleted || loaded.Generation != 8 {
		t.Fatalf("final = %q gen %d, want completed gen 8", loaded.State, loaded.Generation)
	}
}

func TestRestartRecoversLastConfirmed(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-restart")
	for _, to := range []string{StateAnalyzing, StatePlanning} {
		if _, err := UpdateMission(dir, "m-restart", testNow(), func(m *Mission) error {
			_, err := m.Transition(to, "op", "step", "", testNow())
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Restart: a fresh handle loads the confirmed state.
	_, loaded, err := LoadMission(dir, "m-restart")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StatePlanning || loaded.Generation != 3 {
		t.Fatalf("after restart: %q gen %d", loaded.State, loaded.Generation)
	}
}

func TestCrashResumeKeepsEffectLedger(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-ledger")
	_, m, err := LoadMission(dir, "m-ledger")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	if _, err := m.ApplyEffect("notify", "", testNow(), func() (string, error) {
		calls++
		return "sent", nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveMission(dir, m, m.Generation, testNow()); err != nil {
		t.Fatal(err)
	}
	// Crash + restart: the ledger survives, the key replays.
	_, restart, err := LoadMission(dir, "m-ledger")
	if err != nil {
		t.Fatal(err)
	}
	res, err := restart.ApplyEffect("notify", "", testNow(), func() (string, error) {
		calls++
		return "DUPLICATE", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "sent" || calls != 1 {
		t.Fatalf("resume must not duplicate the effect: %q calls %d", res.Output, calls)
	}
}

func TestCancelResumeDurable(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-cancel")
	if _, err := UpdateMission(dir, "m-cancel", testNow(), func(m *Mission) error {
		if _, err := m.Transition(StateAnalyzing, "op", "go", "", testNow()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMission(dir, "m-cancel", testNow(), func(m *Mission) error {
		if _, err := m.Pause("op", "hold", "", testNow()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMission(dir, "m-cancel", testNow(), func(m *Mission) error {
		if _, err := m.Resume("op", "go on", "", testNow()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateMission(dir, "m-cancel", testNow(), func(m *Mission) error {
		_, err := m.Cancel("op", "no longer needed", "", testNow())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, loaded, err := LoadMission(dir, "m-cancel")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != StateCancelled || loaded.CancelReason != "no longer needed" {
		t.Fatalf("cancelled = %+v", loaded)
	}
	if _, err := UpdateMission(dir, "m-cancel", testNow(), func(m *Mission) error {
		_, err := m.Resume("op", "x", "", testNow())
		return err
	}); err == nil {
		t.Fatal("resume of a cancelled mission must refuse durably")
	}
}

func TestListReportsCorruptExplicitly(t *testing.T) {
	dir := t.TempDir()
	mustCreate(t, dir, "m-good")
	mustCreate(t, dir, "m-bad")
	path := filepath.Join(dir, ".regulus", "missions", "m-bad", MissionFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, 'X'), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ListMissions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("list = %d entries, want 2 (corrupt reported, not dropped)", len(entries))
	}
	var bad *ListEntry
	for i := range entries {
		if entries[i].ID == "m-bad" {
			bad = &entries[i]
		}
	}
	if bad == nil || bad.Problem == "" {
		t.Fatalf("corrupt entry must carry its problem: %+v", entries)
	}
	if !strings.Contains(bad.Problem, "sha mismatch") {
		t.Fatalf("problem must name the cause, got %q", bad.Problem)
	}
}
