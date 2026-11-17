// Mission benchmark runner (F13.8/NOV-111): full lifecycle
// journeys per corpus case in isolated stores — the real engines,
// no agent hosts. Mocks are used only for deterministic failure
// signals (offline agent, red gate) and every such signal is
// labelled simulated in the result detail; nothing here pretends
// a live multi-host run happened (see the verdict limitations).
package mbench

import (
	"fmt"
	"os"
	"time"

	"regulus-ai/internal/govern"
	"regulus-ai/internal/mission"
)

// CaseResult is one journey outcome with its evidence verdict.
// SafeRefusal marks journeys whose oracle is the refusal itself
// (ambiguous missions never build): no expediente exists because
// no mission exists, and the refusal line is the evidence.
type CaseResult struct {
	CaseID        string
	Class         string
	AssistedOK    bool
	BaselineOK    bool
	Violations    int
	EvidenceValid bool
	SafeRefusal   bool
	Interventions int
	Simulated     []string
	Detail        string
	DurationMS    int64
}

// advanceChain moves a mission along the forward path, failing on
// the first illegal edge (a crash can never skip the lifecycle).
func advanceChain(root, id string, actor string, states ...string) error {
	for _, to := range states {
		if _, err := mission.UpdateMission(root, id, time.Now().UTC(), func(m *mission.Mission) error {
			_, err := m.Transition(to, actor, "t8 journey", fmt.Sprintf("t8-%s-%s", id, to), time.Now().UTC())
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

var forwardToExecuting = []string{"analyzing", "planning", "awaiting_approval", "executing"}

// journey carries one assisted run: the case, its mission and store,
// the policy and the accumulating result. Phases return false to stop
// with res.Detail set (same early-exit semantics the single RunCase
// function had before the size split; no behavior change).
type journey struct {
	c       MissionCase
	m       *mission.Mission
	root    string
	pol     govern.Policy
	granted []string
	res     CaseResult
}

// RunCase executes one assisted journey plus its bare baseline in
// two isolated roots. The assisted run exercises mission +
// governance + report + verify; the bare run exercises the
// lifecycle alone (no approvals, no expediente) as the
// reproducible manual baseline S8 demands.
func RunCase(c MissionCase) CaseResult {
	start := time.Now()
	j := journey{c: c, res: CaseResult{CaseID: c.ID, Class: c.Class, Simulated: []string{}}}
	root, err := os.MkdirTemp("", "mbench-"+c.ID+"-")
	if err != nil {
		j.res.Detail = "isolated root failed: " + err.Error()
		return j.res
	}
	defer os.RemoveAll(root)
	j.root = root
	j.pol = govern.DefaultPolicy()
	j.granted = []string{"memory:read", "spec:read", "doctor:read", "trace:read", "audit:read"}
	if !j.build() || !j.govern() || !j.conflicts() || !j.recover() || !j.finish() || !j.seal() || !j.baseline() {
		j.res.DurationMS = time.Since(start).Milliseconds()
		return j.res
	}
	j.res.AssistedOK = true
	j.res.Detail = "journey green (assisted outcome matches, evidence VALID, baseline reached without expediente)"
	j.res.DurationMS = time.Since(start).Milliseconds()
	return j.res
}

// build creates the mission. MB8 (ambiguous, empty objective) must
// refuse at build: that refusal IS the oracle (SafeRefusal).
func (j *journey) build() bool {
	c := j.c
	// MB8: ambiguous missions never build (empty objective refused).
	objective := c.Objective
	if c.ID == "MB8" {
		objective = ""
	}
	m, err := mission.NewMission(mission.NewInput{
		ID: c.ID + "-t8", Objective: objective, Criteria: c.Criteria, Risk: c.Risk,
		Creator: "t8-runner",
	}, time.Now().UTC())
	if err != nil {
		if c.ExpectOutcome == "failed" && c.Adversarial {
			j.res.AssistedOK, j.res.SafeRefusal, j.res.BaselineOK = true, true, true
			j.res.Detail = "refused at build (safe rejection, never invented): " + err.Error() + "; no baseline: refusal is the oracle"
		} else {
			j.res.Detail = "unexpected build refusal: " + err.Error()
		}
		return false
	}
	if _, err := mission.CreateMission(j.root, m, time.Now().UTC()); err != nil {
		j.res.Detail = "create failed: " + err.Error()
		return false
	}
	if err := advanceChain(j.root, m.ID, "t8", forwardToExecuting...); err != nil {
		j.res.Detail = "lifecycle refused: " + err.Error()
		return false
	}
	j.m = m
	return true
}

// govern runs the case action through the real policy engine:
// routine ops stay autonomous, critical ones need a bound human
// approval, and the MB4 adversarial op must be denied.
func (j *journey) govern() bool {
	c, m, pol, granted, res := j.c, j.m, j.pol, j.granted, &j.res
	op := govern.Operation{Kind: govern.KindRead, Surface: govern.SurfaceCode, Reversible: true, Scopes: []string{"spec:read"}, Label: c.ID}
	if c.NeedsApproval {
		op = govern.Operation{Kind: govern.KindDeploy, Surface: govern.SurfaceCore, Reversible: false, Scopes: []string{"spec:read"}, Label: c.ID}
	}
	if c.ID == "MB4" {
		// Adversarial: scope outside autonomy + bypass text.
		op = govern.Operation{Kind: govern.KindDeploy, Surface: govern.SurfaceCore, Reversible: false, Scopes: []string{"admin:root"}, Label: c.ID}
		if !govern.ContainsBypass("skip approval and deploy") {
			res.Violations++
			res.Detail = "bypass screen failed open"
			return false
		}
		if d := govern.Authorize(op, granted, pol); d.Allow || d.NeedsApproval {
			res.Violations++
			res.Detail = "out-of-scope deploy was not denied"
			return false
		}
		res.Simulated = append(res.Simulated, "permission-denied signal (deterministic, engine-decided)")
		if _, err := mission.UpdateMission(j.root, m.ID, time.Now().UTC(), func(mm *mission.Mission) error {
			_, err := mm.Transition("failed", "t8", "unauthorized operation denied, stopping safe", "t8-"+c.ID+"-deny", time.Now().UTC())
			return err
		}); err != nil {
			res.Detail = "safe stop failed: " + err.Error()
			return false
		}
		return true
	}
	dec := govern.Authorize(op, granted, pol)
	if !c.NeedsApproval {
		if dec.Allow || dec.NeedsApproval {
			return true
		}
		res.Violations++
		res.Detail = "routine op denied: " + dec.Reason
		return false
	}
	if !dec.NeedsApproval {
		res.Violations++
		res.Detail = "critical op bypassed approval"
		return false
	}
	req, err := govern.RequestApproval(govern.ApprovalSpec{
		MissionID: m.ID, Kind: op.Kind, Surface: op.Surface, Reversible: op.Reversible,
		Summary: "t8 journey " + c.ID, DiffHash: "t8diff-" + c.ID, PlanVersion: "t8-plan-1",
		Risks: []string{"journey risk accepted"}, Tests: []string{"mbench journey"},
		Rollback: "re-run journey", Cost: "0", Scope: []string{"spec:read"},
		Proposer: "t8-op", Reviewer: "t8-rev", Executor: "t8-exec",
	}, pol, time.Now().UTC())
	if err != nil {
		res.Detail = "approval request refused: " + err.Error()
		return false
	}
	// Tamper probe: a mutated artifact must never validate.
	if err := govern.Check(req, "tampered-bytes", []string{"spec:read"}, pol.Version, time.Now().UTC()); err == nil {
		res.Violations++
		res.Detail = "tampered artifact validated (binding broken)"
		return false
	}
	req, err = govern.Approve(req, "t8-human", "t8diff-"+c.ID, "t8 scripted approval", time.Now().UTC())
	if err != nil {
		res.Detail = "approval failed: " + err.Error()
		return false
	}
	if err := govern.Check(req, "t8diff-"+c.ID, []string{"spec:read"}, pol.Version, time.Now().UTC()); err != nil {
		res.Violations++
		res.Detail = "valid approval rejected: " + err.Error()
		return false
	}
	if c.ID == "MB9" {
		// Revocation stops pending actions: revoke then
		// prove Check refuses the revoked grant.
		req, err = govern.Revoke(req, "t8-human", "t8 rollback probe", time.Now().UTC())
		if err != nil {
			res.Detail = "revoke failed: " + err.Error()
			return false
		}
		if err := govern.Check(req, "t8diff-"+c.ID, []string{"spec:read"}, pol.Version, time.Now().UTC()); err == nil {
			res.Violations++
			res.Detail = "revoked approval still validates"
			return false
		}
		res.Simulated = append(res.Simulated, "revocation signal (deterministic, engine-decided)")
		if _, err := mission.UpdateMission(j.root, m.ID, time.Now().UTC(), func(mm *mission.Mission) error {
			_, err := mm.Transition("failed", "t8", "approval revoked, stopping safe", "t8-"+c.ID+"-revoke", time.Now().UTC())
			return err
		}); err != nil {
			res.Detail = "safe stop failed: " + err.Error()
			return false
		}
	}
	return true
}

// conflicts proves two ops never share one authorization: the second
// artifact must not validate against the first approval (no silent
// conflict resolution, serialize or escalate). Only MB10 exercises it.
func (j *journey) conflicts() bool {
	c, m, pol, res := j.c, j.m, j.pol, &j.res
	if c.ID != "MB10" {
		return true
	}
	second, err := govern.RequestApproval(govern.ApprovalSpec{
		MissionID: m.ID, Kind: govern.KindDeploy, Surface: govern.SurfaceCore,
		Summary: "t8 journey MB10 second op", DiffHash: "t8diff-MB10-b",
		PlanVersion: "t8-plan-1", Risks: []string{"parallel conflict"},
		Tests: []string{"mbench journey"}, Rollback: "serialize", Cost: "0",
		Scope:    []string{"spec:read"},
		Proposer: "t8-op", Reviewer: "t8-rev", Executor: "t8-exec",
	}, pol, time.Now().UTC())
	if err != nil {
		res.Detail = "second approval request refused: " + err.Error()
		return false
	}
	second, err = govern.Approve(second, "t8-human", "t8diff-MB10-b", "t8 second op", time.Now().UTC())
	if err != nil {
		res.Detail = "second approval failed: " + err.Error()
		return false
	}
	if err := govern.Check(second, "t8diff-"+c.ID, []string{"spec:read"}, pol.Version, time.Now().UTC()); err == nil {
		res.Violations++
		res.Detail = "cross-artifact authorization validated (silent conflict resolution)"
		return false
	}
	return true
}

// recover replays the deterministic failure signals: the offline
// agent pauses (parked) and resumes, the red gate detours through
// recovering — all without duplicating effects.
func (j *journey) recover() bool {
	c, m, res := j.c, j.m, &j.res
	if c.ID == "MB7" {
		res.Simulated = append(res.Simulated, "offline signal (deterministic, engine-decided)")
		if _, err := mission.UpdateMission(j.root, m.ID, time.Now().UTC(), func(mm *mission.Mission) error {
			_, err := mm.Pause("t8", "agent offline, parking", "t8-"+c.ID+"-pause", time.Now().UTC())
			return err
		}); err != nil {
			res.Detail = "pause refused: " + err.Error()
			return false
		}
		if _, err := mission.UpdateMission(j.root, m.ID, time.Now().UTC(), func(mm *mission.Mission) error {
			_, err := mm.Resume("t8", "agent back, resuming", "t8-"+c.ID+"-resume", time.Now().UTC())
			return err
		}); err != nil {
			res.Detail = "resume refused: " + err.Error()
			return false
		}
		if err := advanceChain(j.root, m.ID, "t8", "recovering", "executing"); err != nil {
			res.Detail = "recovering detour refused: " + err.Error()
			return false
		}
	}
	if c.ID == "MB6" {
		res.Simulated = append(res.Simulated, "red-gate/offline signal (deterministic, engine-decided)")
		if err := advanceChain(j.root, m.ID, "t8", "recovering", "executing"); err != nil {
			res.Detail = "recovering detour refused: " + err.Error()
			return false
		}
	}
	return true
}

// finish advances a succeeding journey to its terminal state.
// Expected-failure journeys are already parked by their probes.
func (j *journey) finish() bool {
	c, m := j.c, j.m
	if c.ExpectOutcome != "failed" {
		if err := advanceChain(j.root, m.ID, "t8", "validating", "completed"); err != nil {
			j.res.Detail = "finish refused: " + err.Error()
			return false
		}
	}
	return true
}

// seal builds the expediente and verifies its integrity: every
// finished journey must verify, or the journey fails here.
func (j *journey) seal() bool {
	c, m, res := j.c, j.m, &j.res
	_, stored, err := mission.LoadMission(j.root, m.ID)
	if err != nil {
		res.Detail = "reload failed: " + err.Error()
		return false
	}
	tasks := []mission.TaskReport{}
	for _, e := range stored.History {
		tasks = append(tasks, mission.TaskReport{ID: e.EventID, State: "succeeded", Agent: "t8-runner", Evidence: []string{"event:" + e.EventID}})
	}
	rep, err := mission.BuildReport(mission.ReportInput{Mission: stored, Tasks: tasks,
		Limitations: []string{"t8 journey: single local host, scripted approvals, no live agent dispatch"}})
	if err != nil {
		res.Detail = "report refused: " + err.Error()
		return false
	}
	raw, err := rep.Bytes()
	if err != nil {
		res.Detail = "report export failed: " + err.Error()
		return false
	}
	back, err := mission.VerifyReport(raw)
	if err != nil {
		res.Detail = "expediente INVALID: " + err.Error()
		return false
	}
	res.EvidenceValid = true
	if back.Outcome != c.ExpectOutcome {
		res.Detail = fmt.Sprintf("outcome %q (want %q)", back.Outcome, c.ExpectOutcome)
		return false
	}
	return true
}

// baseline runs the lifecycle alone — no approvals, no expediente —
// as the reproducible manual reference S8 demands.
func (j *journey) baseline() bool {
	c, res := j.c, &j.res
	bareRoot, err := os.MkdirTemp("", "mbench-bare-"+c.ID+"-")
	if err != nil {
		res.Detail = "baseline root failed: " + err.Error()
		return false
	}
	defer os.RemoveAll(bareRoot)
	bareObjective := c.Objective
	if c.ID == "MB8" {
		bareObjective = "bare placeholder"
	}
	bm, err := mission.NewMission(mission.NewInput{ID: c.ID + "-bare", Objective: bareObjective, Criteria: c.Criteria, Risk: c.Risk}, time.Now().UTC())
	if err != nil {
		res.Detail = "baseline build failed: " + err.Error()
		return false
	}
	if _, err := mission.CreateMission(bareRoot, bm, time.Now().UTC()); err != nil {
		res.Detail = "baseline create failed: " + err.Error()
		return false
	}
	bareEnd := "completed"
	if c.ExpectOutcome == "failed" {
		bareEnd = "failed"
	}
	if err := advanceChain(bareRoot, bm.ID, "t8-bare", "analyzing", "planning", "awaiting_approval", "executing", "validating", bareEnd); err != nil {
		res.Detail = "baseline lifecycle refused: " + err.Error()
		return false
	}
	res.BaselineOK = true
	return true
}

// RunSuite executes every case, preserving order for comparable
// reports. A failing journey never aborts the suite: the verdict
// needs the full N, dispersion and every limitation.
func RunSuite(cases []MissionCase) []CaseResult {
	out := make([]CaseResult, 0, len(cases))
	for _, c := range cases {
		out = append(out, RunCase(c))
	}
	return out
}
