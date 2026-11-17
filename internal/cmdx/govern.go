// Governance handlers (F13.6/NOV-109): `regulus govern ...` over
// the mission-governance engine (internal/govern).
//
// Handlers take the minimal IO instead of cli.CLI so cli keeps one
// edge (cli->cmdx); behavior, flags, messages and exit codes are
// defined here. The engine decides; this layer only parses flags,
// resolves the server-side context and prints.
//
// Reuse, never duplication:
//   - autonomy + approvals: internal/govern (matrix, profiles,
//     lifecycle, hash-chained audit). No second rule here.
//   - MCP permissions: internal/mcp (PrincipalFor server-derived
//     scopes, Evaluate for the optional --mcp-tool cross-check).
//     Granted scopes are never flag-invented: they come from the
//     process environment, exactly like the MCP server.
//   - gates: internal/verify (RequiredGates via
//     govern.VerificationGates). The authorize output cites the
//     same gates the mission engine enforces.
//   - trace: observability.RecordExec sharing ids (F11.1);
//     best-effort, never fails the governance decision.
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/govern"
	"regulus-ai/internal/mcp"
	"regulus-ai/internal/observability"
)

func governUsage() string {
	return "usage: regulus govern matrix|authorize|request|approve|check|pause|resume|reject|revoke|show|list|audit|policy\n" +
		"  regulus govern matrix [--json]\n" +
		"  regulus govern authorize --kind <k> --surface <s> [--irreversible] [--scope <a,b>] [--profile <p>] [--mcp-tool <t> --mcp-arg <k=v,...>] [--json]\n" +
		"  regulus govern request --kind <k> --surface <s> [--irreversible] --summary <t> --scope <a,b> --risks <a,b> --rollback <t> --cost <t> [--diff <sha>] [--plan <v>] [--tests <a,b>] [--mission <m>] [--proposer <p>] [--reviewer <r>] [--executor <e>] [--ttl-hours N] [--json]\n" +
		"  regulus govern approve --id <apr> --by <human> --artifact <sha> [--reason <t>] [--json]\n" +
		"  regulus govern check --id <apr> --artifact <sha> [--scope <a,b>] [--policy-version <v>] [--json]\n" +
		"  regulus govern pause|resume|reject|revoke --id <apr> --by <actor> --reason <t> [--json]\n" +
		"  regulus govern show --id <apr> [--json]\n" +
		"  regulus govern list [--state <s>] [--json]\n" +
		"  regulus govern audit [--id <apr>] [--json]\n" +
		"  regulus govern policy [--show] [--set-profile <p> --by <human>] [--json]"
}

// GovernCmd routes `regulus govern ...`.
func (io IO) GovernCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, governUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	switch args[0] {
	case "matrix":
		return io.GovernMatrixCmd(args[1:])
	case "authorize":
		return io.GovernAuthorizeCmd(args[1:])
	case "request":
		return io.GovernRequestCmd(args[1:])
	case "approve":
		return io.GovernApproveCmd(args[1:])
	case "check":
		return io.GovernCheckCmd(args[1:])
	case "pause":
		return io.GovernPauseCmd(args[1:])
	case "resume":
		return io.GovernResumeCmd(args[1:])
	case "reject":
		return io.GovernRejectCmd(args[1:])
	case "revoke":
		return io.GovernRevokeCmd(args[1:])
	case "show":
		return io.GovernShowCmd(args[1:])
	case "list":
		return io.GovernListCmd(args[1:])
	case "audit":
		return io.GovernAuditCmd(args[1:])
	case "policy":
		return io.GovernPolicyCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "govern: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
}

// GovernMatrixCmd implements `regulus govern matrix`: the enforced
// risk→autonomy table, generated from the engine it documents.
func (io IO) GovernMatrixCmd(args []string) int {
	fs := flag.NewFlagSet("govern matrix", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the profiles and matrix as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	if *asJSON {
		raw, err := json.MarshalIndent(map[string]any{
			"kinds": govern.AllKinds, "surfaces": govern.AllSurfaces,
			"profiles": govern.BuiltinProfiles(), "policy_version": govern.PolicyVersion,
		}, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "govern matrix: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, govern.MatrixDoc())
	return 0
}

// governOpFlags registers the shared operation flags.
func governOpFlags(fs *flag.FlagSet) (kind, surface *string, irreversible *bool, scope *string) {
	kind = fs.String("kind", "", "operation kind ("+strings.Join(govern.AllKinds, "|")+") (required)")
	surface = fs.String("surface", "", "touched surface ("+strings.Join(govern.AllSurfaces, "|")+") (required)")
	irreversible = fs.Bool("irreversible", false, "the operation cannot be rolled back")
	scope = fs.String("scope", "", "comma-separated scopes the operation needs")
	return kind, surface, irreversible, scope
}

// GovernAuthorizeCmd implements `regulus govern authorize`: the pure
// autonomy check against the stored policy (or --profile for
// what-if). Granted scopes come from the server-derived MCP
// principal — never from flags — and an optional --mcp-tool
// cross-checks the same action against the F11.8 capability
// policy, so one command shows both gates before acting.
func (io IO) GovernAuthorizeCmd(args []string) int {
	fs := flag.NewFlagSet("govern authorize", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	kind, surface, irreversible, scope := governOpFlags(fs)
	profile := fs.String("profile", "", "evaluate under this profile instead of the stored policy (what-if)")
	mcpTool := fs.String("mcp-tool", "", "cross-check this MCP tool under F11.8 (optional)")
	mcpArgs := fs.String("mcp-arg", "", "comma-separated k=v tool arguments for the cross-check")
	asJSON := fs.Bool("json", false, "print the decision as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *kind == "" || *surface == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	var pol govern.Policy
	var err error
	if *profile != "" {
		pol, err = govern.ResolvePolicy("what-if", *profile)
	} else {
		pol, err = govern.LoadPolicy(io.Dir)
	}
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern authorize: %v\n", err)
		return 1
	}
	principal := mcp.PrincipalFor(io.Dir)
	granted := principal.GrantedScopes()
	op := govern.Operation{Kind: *kind, Surface: *surface, Reversible: !*irreversible, Scopes: splitList(*scope)}
	dec := govern.Authorize(op, granted, pol)
	mcpVerdict := ""
	if *mcpTool != "" {
		margs := map[string]any{}
		for _, kv := range splitList(*mcpArgs) {
			if k, v, ok := strings.Cut(kv, "="); ok && strings.TrimSpace(k) != "" {
				margs[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		md := mcp.Evaluate(*mcpTool, margs, principal)
		if md.Allow {
			mcpVerdict = "mcp: ALLOW (" + md.Reason + ")"
		} else {
			mcpVerdict = "mcp: DENY (" + md.Reason + ")"
		}
	}
	specs, gateReasons := govern.VerificationGates(dec.Risk, nil)
	gates := make([]string, 0, len(specs))
	for _, s := range specs {
		gates = append(gates, s.Kind)
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(map[string]any{
			"allow": dec.Allow, "needs_approval": dec.NeedsApproval,
			"risk": dec.Risk, "reason": dec.Reason, "policy": pol.Name + "@" + pol.Profile,
			"policy_version": dec.PolicyVersion, "mcp": mcpVerdict,
			"gates": gates, "gate_reasons": gateReasons,
		}, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "govern authorize: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return governAuthorizeExit(dec, mcpVerdict)
	}
	if dec.Allow {
		fmt.Fprintf(io.Stdout, "govern: ALLOW risk=%s policy=%s@%s\n  %s\n", dec.Risk, pol.Name, pol.Profile, dec.Reason)
	} else if dec.NeedsApproval {
		fmt.Fprintf(io.Stdout, "govern: NEEDS_APPROVAL risk=%s policy=%s@%s\n  %s\n", dec.Risk, pol.Name, pol.Profile, dec.Reason)
	} else {
		fmt.Fprintf(io.Stdout, "govern: DENY risk=%s policy=%s@%s\n  %s\n", dec.Risk, pol.Name, pol.Profile, dec.Reason)
	}
	if mcpVerdict != "" {
		fmt.Fprintf(io.Stdout, "  %s\n", mcpVerdict)
	}
	fmt.Fprintf(io.Stdout, "  gates: %s\n", strings.Join(gates, ", "))
	for _, r := range gateReasons {
		fmt.Fprintf(io.Stdout, "  - %s\n", r)
	}
	if !dec.Allow && *mcpTool == "" {
		fmt.Fprintf(io.Stdout, "next: regulus govern request --kind %s --surface %s --summary <t> --scope <a,b> --risks <a,b> --rollback <t> --cost <t>\n", *kind, *surface)
	}
	return governAuthorizeExit(dec, mcpVerdict)
}

// governAuthorizeExit maps the decision onto exit codes: autonomous
// 0, approval-routed 1 (not an error, but not a go-ahead), denied
// 1, MCP-denied 1 even when mission autonomy allows (both gates
// must open).
func governAuthorizeExit(dec govern.Decision, mcpVerdict string) int {
	if dec.Allow && !strings.HasPrefix(mcpVerdict, "mcp: DENY") {
		return 0
	}
	return 1
}

// GovernRequestCmd implements `regulus govern request`: seal one
// approval request under the stored policy.
func (io IO) GovernRequestCmd(args []string) int {
	fs := flag.NewFlagSet("govern request", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	kind, surface, irreversible, scope := governOpFlags(fs)
	summary := fs.String("summary", "", "one-line summary of the governed change (required)")
	diff := fs.String("diff", "", "diff/artifact hash the approval will bind (required from high risk up)")
	plan := fs.String("plan", "", "plan/version reference (required)")
	risks := fs.String("risks", "", "comma-separated risks the human accepts (required)")
	tests := fs.String("tests", "", "comma-separated gates the human trusts")
	rollback := fs.String("rollback", "", "how the change rolls back (required)")
	cost := fs.String("cost", "", "time/tokens/money budget label (required)")
	mission := fs.String("mission", "", "mission id this request belongs to (optional)")
	proposer := fs.String("proposer", "", "proposer identity (required)")
	reviewer := fs.String("reviewer", "", "reviewer identity (required)")
	executor := fs.String("executor", "", "executor identity (required)")
	ttl := fs.Float64("ttl-hours", 0, "approval lease in hours (default 24, max 168)")
	asJSON := fs.Bool("json", false, "print the sealed request as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *kind == "" || *surface == "" || *summary == "" || *scope == "" || *risks == "" ||
		*rollback == "" || *cost == "" || *plan == "" || *proposer == "" || *reviewer == "" ||
		*executor == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	pol, err := govern.LoadPolicy(io.Dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern request: %v\n", err)
		return 1
	}
	now := time.Now().UTC()
	req, err := govern.RequestApproval(govern.ApprovalSpec{
		MissionID: *mission, Kind: *kind, Surface: *surface, Reversible: !*irreversible,
		Summary: *summary, DiffHash: strings.TrimSpace(*diff), PlanVersion: *plan,
		Risks: splitList(*risks), Tests: splitList(*tests),
		Rollback: *rollback, Cost: *cost, Scope: splitList(*scope),
		PolicyName: pol.Name, Proposer: *proposer, Reviewer: *reviewer, Executor: *executor,
		TTLHours: *ttl,
	}, pol, now)
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern request: %v\n", err)
		return 1
	}
	if err := govern.SaveApproval(io.Dir, req); err != nil {
		fmt.Fprintf(io.Stderr, "govern request: %v\n", err)
		return 1
	}
	io.emitGovernEvent(req.MissionID, "govern request", req.State, map[string]any{"actor": *proposer, "request": req.ID})
	_ = govern.AppendAudit(io.Dir, now.Format(time.RFC3339), strings.TrimSpace(*proposer), "requested", req.ID,
		fmt.Sprintf("%s %s under %s@%s", req.Kind, req.Surface, req.PolicyName, req.PolicyVersion))
	if *asJSON {
		raw, merr := json.MarshalIndent(req, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "govern request: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "govern: REQUESTED %s (expires %s)\n", req.ID, req.ExpiresAt)
	fmt.Fprintf(io.Stdout, "next: regulus govern approve --id %s --by <human> --artifact <sha> --reason <text>\n", req.ID)
	return 0
}

// governDecisionFlags registers the shared --id/--by/--reason flags.
func governDecisionFlags(fs *flag.FlagSet) (id, by, reason *string) {
	id = fs.String("id", "", "approval id (required)")
	by = fs.String("by", "", "actor ordering this transition (required)")
	reason = fs.String("reason", "", "audit reason (required)")
	return id, by, reason
}

// governLoadForDecision loads a request or reports it; ok=false
// means the handler must stop now.
func (io IO) governLoadForDecision(id, command string) (govern.ApprovalRequest, bool) {
	req, err := govern.LoadApproval(io.Dir, id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern %s: %v\n", command, err)
		return govern.ApprovalRequest{}, false
	}
	return req, true
}

// governSeal persists a transitioned request, audits it and prints
// the sealed state; the audit failure never fails the seal (the
// record is authoritative, the log gap is visible in audit).
func (io IO) governSeal(req govern.ApprovalRequest, actor, action, command string) int {
	if err := govern.UpdateApproval(io.Dir, req); err != nil {
		fmt.Fprintf(io.Stderr, "govern %s: %v\n", command, err)
		return 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	io.emitGovernEvent(req.MissionID, "govern "+command, req.State, map[string]any{"actor": actor, "request": req.ID})
	if err := govern.AppendAudit(io.Dir, now, actor, action, req.ID, req.DecideReason); err != nil {
		fmt.Fprintf(io.Stderr, "govern %s: audit log unwritten (%v)\n", command, err)
	}
	fmt.Fprintf(io.Stdout, "govern: %s %s state=%s\n", strings.ToUpper(command), req.ID, req.State)
	return 0
}

// GovernApproveCmd implements `regulus govern approve`: the human
// decision, bound to the exact artifact bytes.
func (io IO) GovernApproveCmd(args []string) int {
	fs := flag.NewFlagSet("govern approve", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "approval id (required)")
	by := fs.String("by", "", "human approver (required, outside proposer/reviewer/executor)")
	artifact := fs.String("artifact", "", "artifact hash the human signs (required, must equal the requested diff)")
	reason := fs.String("reason", "", "what risk the human accepts (required)")
	asJSON := fs.Bool("json", false, "print the sealed approval as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *by == "" || *artifact == "" || *reason == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	req, ok := io.governLoadForDecision(*id, "approve")
	if !ok {
		return 1
	}
	now := time.Now().UTC()
	appr, err := govern.Approve(req, *by, *artifact, *reason, now)
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern approve: %v\n", err)
		return 1
	}
	if code := io.governSeal(appr, strings.TrimSpace(*by), "approved", "approve"); code != 0 {
		return code
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(appr, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "govern approve: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
	}
	return 0
}

// GovernCheckCmd implements `regulus govern check`: validate a live
// authorization before a side effect — state, expiry, artifact,
// scope ceiling and policy binding. Anything material changed
// invalidates, never warns-and-continues.
func (io IO) GovernCheckCmd(args []string) int {
	fs := flag.NewFlagSet("govern check", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "approval id (required)")
	artifact := fs.String("artifact", "", "current artifact hash (required)")
	scope := fs.String("scope", "", "comma-separated scopes the action will use")
	policyVersion := fs.String("policy-version", "", "policy version to check against (default: stored policy)")
	asJSON := fs.Bool("json", false, "print the check as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *artifact == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	req, ok := io.governLoadForDecision(*id, "check")
	if !ok {
		return 1
	}
	pv := strings.TrimSpace(*policyVersion)
	if pv == "" {
		if pol, err := govern.LoadPolicy(io.Dir); err == nil {
			pv = pol.Version
		}
	}
	err := govern.Check(req, strings.TrimSpace(*artifact), splitList(*scope), pv, time.Now().UTC())
	if *asJSON {
		raw, merr := json.MarshalIndent(map[string]any{
			"id": req.ID, "state": req.State, "valid": err == nil,
			"reason": map[string]string{"": errString(err)}[""],
		}, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "govern check: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		if err != nil {
			return 1
		}
		return 0
	}
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern check: INVALID %s: %v\n", req.ID, err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "govern: VALID %s (bound %s, policy %s)\n", req.ID, req.BoundArtifact, req.PolicyVersion)
	return 0
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// GovernPauseCmd / GovernResumeCmd / GovernRejectCmd /
// GovernRevokeCmd implement the four non-approval transitions.
func (io IO) GovernPauseCmd(args []string) int {
	return io.governTransition(args, "pause", govern.Pause)
}

func (io IO) GovernResumeCmd(args []string) int {
	return io.governTransition(args, "resume", govern.Resume)
}

func (io IO) GovernRejectCmd(args []string) int {
	return io.governTransition(args, "reject", govern.Reject)
}

func (io IO) GovernRevokeCmd(args []string) int {
	return io.governTransition(args, "revoke", govern.Revoke)
}

func (io IO) governTransition(args []string, command string, fn func(govern.ApprovalRequest, string, string, time.Time) (govern.ApprovalRequest, error)) int {
	fs := flag.NewFlagSet("govern "+command, flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id, by, reason := governDecisionFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" || *by == "" || *reason == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	req, ok := io.governLoadForDecision(*id, command)
	if !ok {
		return 1
	}
	next, err := fn(req, *by, *reason, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern %s: %v\n", command, err)
		return 1
	}
	return io.governSeal(next, strings.TrimSpace(*by), command+"d", command)
}

// GovernShowCmd implements `regulus govern show`: quote one sealed
// request with its binding and history.
func (io IO) GovernShowCmd(args []string) int {
	fs := flag.NewFlagSet("govern show", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "approval id (required)")
	asJSON := fs.Bool("json", false, "print the request as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *id == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	req, ok := io.governLoadForDecision(*id, "show")
	if !ok {
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(req, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "govern show: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, govern.RenderShow(req))
	return 0
}

// GovernListCmd implements `regulus govern list`: every sealed
// request, optionally filtered by state.
func (io IO) GovernListCmd(args []string) int {
	fs := flag.NewFlagSet("govern list", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	state := fs.String("state", "", "only list requests in this state (optional)")
	asJSON := fs.Bool("json", false, "print the requests as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	reqs, corrupt, err := govern.ListApprovals(io.Dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern list: %v\n", err)
		return 1
	}
	if *state != "" {
		kept := reqs[:0]
		for _, r := range reqs {
			if r.State == *state {
				kept = append(kept, r)
			}
		}
		reqs = kept
	}
	if *asJSON {
		raw, err := json.MarshalIndent(reqs, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "govern list: %v\n", err)
			return 1
		}
		if reqs == nil {
			fmt.Fprintln(io.Stdout, "[]")
		} else {
			fmt.Fprintln(io.Stdout, string(raw))
		}
	} else {
		fmt.Fprint(io.Stdout, govern.RenderList(reqs))
	}
	for _, c := range corrupt {
		fmt.Fprintf(io.Stderr, "govern list: corrupt: %s\n", c)
	}
	if len(corrupt) > 0 {
		return 1
	}
	return 0
}

// GovernAuditCmd implements `regulus govern audit`: replay the
// hash-chained log (optionally one request) and verify it.
// Tampering fails loudly; the chain is tamper-evident, not just
// append-only.
func (io IO) GovernAuditCmd(args []string) int {
	fs := flag.NewFlagSet("govern audit", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "only events for this approval id (optional)")
	asJSON := fs.Bool("json", false, "print the events as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	events, err := govern.LoadAudit(io.Dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern audit: %v\n", err)
		return 1
	}
	if err := govern.VerifyAudit(events); err != nil {
		fmt.Fprintf(io.Stderr, "govern audit: CHAIN BROKEN: %v\n", err)
		return 1
	}
	if *id != "" {
		kept := events[:0]
		for _, e := range events {
			if e.RequestID == *id {
				kept = append(kept, e)
			}
		}
		events = kept
	}
	if *asJSON {
		raw, err := json.MarshalIndent(events, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "govern audit: %v\n", err)
			return 1
		}
		if events == nil {
			fmt.Fprintln(io.Stdout, "[]")
		} else {
			fmt.Fprintln(io.Stdout, string(raw))
		}
		return 0
	}
	fmt.Fprint(io.Stdout, govern.RenderAudit(events))
	return 0
}

// GovernPolicyCmd implements `regulus govern policy`: show the
// active posture, or switch it — human-only, audited, no silent
// path to a wider posture.
func (io IO) GovernPolicyCmd(args []string) int {
	fs := flag.NewFlagSet("govern policy", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	show := fs.Bool("show", false, "print the active policy (default when no flags)")
	setProfile := fs.String("set-profile", "", "switch the project profile (locked|standard|wide)")
	by := fs.String("by", "", "human owner ordering the switch (required with --set-profile)")
	asJSON := fs.Bool("json", false, "print the policy as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	if *setProfile != "" {
		if *by == "" {
			fmt.Fprintln(io.Stderr, governUsage())
			return 2
		}
		pol, err := govern.SavePolicy(io.Dir, *setProfile, *by, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(io.Stderr, "govern policy: %v\n", err)
			return 1
		}
		io.emitGovernEvent("", "govern policy", pol.Profile, map[string]any{"actor": *by})
		fmt.Fprintf(io.Stdout, "govern: policy project@%s (%s) by %s\n", pol.Profile, pol.Version, pol.UpdatedBy)
		return 0
	}
	if !*show && !*asJSON {
		fmt.Fprintln(io.Stderr, governUsage())
		return 2
	}
	pol, err := govern.LoadPolicy(io.Dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "govern policy: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(pol, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "govern policy: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "govern: policy %s@%s (%s)\n", pol.Name, pol.Profile, pol.Version)
	for _, p := range govern.BuiltinProfiles() {
		if p.Name == pol.Profile {
			fmt.Fprintf(io.Stdout, "  %s: %s (autonomy ≤ %s, scopes: %s)\n",
				p.Name, p.Description, p.MaxAutonomousRisk, strings.Join(p.AllowScopes, ", "))
		}
	}
	return 0
}

// emitGovernEvent links one govern command to the F11.1 exec log.
// TaskID carries the mission id when the request belongs to one,
// else the approval id, so every decision stays queryable.
// Best-effort (a trace failure never fails the sealed decision).
func (io IO) emitGovernEvent(missionID, command, status string, extra map[string]any) {
	runID := "govern"
	if missionID != "" {
		runID = "mission/" + missionID
	}
	taskID := missionID
	if taskID == "" {
		if r, ok := extra["request"].(string); ok {
			taskID = r
		}
	}
	eventID, err := observability.MintEventID()
	if err != nil {
		return
	}
	payload := map[string]any{"command": command, "status": status}
	for k, v := range extra {
		payload[k] = v
	}
	if _, _, rerr := observability.RecordExec(io.Dir, observability.ExecEvent{
		SchemaVersion: observability.ExecSchemaVersion,
		EventID:       eventID, RunID: runID, TaskID: taskID,
		Type: observability.ExecArtifact, Status: observability.ExecRunning,
		Payload: payload,
	}, time.Now().UTC()); rerr != nil {
		fmt.Fprintf(io.Stderr, "govern: evento de traza no registrado (%v)\n", rerr)
	}
}
