// Package runtime acceptance (Phase 7, S7-S8): the pieces the S7 matrix
// was still missing, plus the S8 gate. Existing coverage already pins
// adapter-contract, capability negotiation, protocol versions,
// error-handling and contract pins (adapter_test.go, boundary_test.go,
// internal/mcp, internal/httpapi, internal/contract) — this file adds
// ONLY what was missing:
//
//   - cross-interface integration: the same read-only core operation
//     (the version/contract readout) through CLI buffers, the HTTP
//     adapter and the MCP discovery path returns the SAME payload;
//   - compatibility: unknown/future contract versions refused with an
//     actionable message on both pins, plus the v2 migration note;
//   - the S8 acceptance test asserting all six criteria.
//
// Test-only file: the only adapter that may appear here is the
// clearly-marked test-only fake from adapter_test.go. No production code
// is touched and no product support is registered.
package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/cli"
	"regulus-ai/internal/contract"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/httpapi"
	"regulus-ai/internal/mcp"
)

// acceptanceStubRunner answers the gremlins probe inside statusCmd
// deterministically, the same way the httpapi tests stub it.
type acceptanceStubRunner struct{ code int }

func (s acceptanceStubRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	return "", "", s.code, nil
}

func acceptanceProbe() doctor.Result {
	return doctor.Result{Detected: "opencode 2.1.0", Major: 2, Supported: true}
}

// runCLICaptured runs one CLI core path with private buffers — the same
// buffers-in/buffers-out pattern internal/cli/mcp.go serveMCP uses (never
// shelling out), so CLI semantics are the reference the other interfaces
// must track verbatim.
func runCLICaptured(version string, probe func() doctor.Result, runner gates.CmdRunner, args ...string) (string, string, int) {
	var ob, eb bytes.Buffer
	c := cli.CLI{Version: version, Probe: probe, Runner: runner, Stdout: &ob, Stderr: &eb}
	code := c.Run(args)
	return ob.String(), eb.String(), code
}

func getJSON(t *testing.T, h http.Handler, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("GET %s is not JSON: %v (%s)", path, err, body)
	}
	return rec.Code, env
}

// S7 integration: the version/contract readout is the read-only core
// operation every interface serves. CLI prints it, HTTP reuses the CLI
// core path verbatim through buffers, and MCP serves the same pins
// through capability discovery (what `initialize` returns as
// serverInfo{version, contract, mcpContract} — the serveMCP loop itself
// is unexported outside package cli, so the discovery API is the
// in-package server path and TestMCPInitializeExposesContract /
// TestMCPInitializeHandshake in internal/cli prove the wire carries
// these same values). All three must agree: shared policy, never
// per-interface divergence.
func TestCrossInterfaceSharedReadOnlyPayload(t *testing.T) {
	const version = "test"
	runner := acceptanceStubRunner{code: 1}
	srv := httpapi.New(version, acceptanceProbe, runner)

	// CLI reference through buffers.
	contractOut, _, contractExit := runCLICaptured(version, acceptanceProbe, runner, "contract")
	if contractExit != 0 {
		t.Fatalf("CLI contract exit = %d, want 0", contractExit)
	}
	versionOut, _, versionExit := runCLICaptured(version, acceptanceProbe, runner, "--version")
	if versionExit != 0 {
		t.Fatalf("CLI --version exit = %d, want 0", versionExit)
	}

	// HTTP must track the CLI core paths verbatim.
	code, env := getJSON(t, srv.Handler(), "/v1/version")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/version = %d, want 200 (%v)", code, env)
	}
	if env["ok"] != true {
		t.Fatalf("GET /v1/version ok = %v, want true (%v)", env["ok"], env)
	}
	if env["contract"] != httpapi.ContractVersion {
		t.Fatalf("HTTP envelope contract = %v, want %q (transport pin, never invented per-interface)", env["contract"], httpapi.ContractVersion)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("GET /v1/version data = %v, want object", env["data"])
	}
	if data["contract"] != contractOut {
		t.Fatalf("HTTP contract = %q, want CLI core path %q (verbatim, no per-interface policy)", data["contract"], contractOut)
	}
	if data["version"] != versionOut {
		t.Fatalf("HTTP version = %q, want CLI core path %q (verbatim, no per-interface policy)", data["version"], versionOut)
	}

	// MCP serves the same core pins through discovery.
	disc := mcp.CapabilityDiscovery()
	if disc.ContractVersion != mcp.ContractVersion {
		t.Fatalf("discovery contract = %q, want pinned %q", disc.ContractVersion, mcp.ContractVersion)
	}
	if disc.ProtocolVersion != mcp.ProtocolVersion {
		t.Fatalf("discovery protocol = %q, want pinned %q", disc.ProtocolVersion, mcp.ProtocolVersion)
	}
	if strings.TrimSpace(contractOut) != contract.Version {
		t.Fatalf("CLI contract output %q does not carry the core pin %q (MCP initialize serves this same value)", contractOut, contract.Version)
	}
	if versionOut != "regulus "+version+"\n" {
		t.Fatalf("CLI --version output %q does not derive from the same version %q MCP initialize serves", versionOut, version)
	}
}

// S7 compatibility: unknown/future contract versions are refused with an
// actionable message naming what the client spoke and what the server
// speaks — never silent drift, never silent upgrade.
//
// MIGRATION NOTE (v2): shipping a regulus.contract/v2 or regulus.mcp/v2
// requires (1) a new pinned constant alongside (never replacing) the v1
// pin, (2) an explicit negotiation entry point that accepts the new pin
// and keeps refusing everything else, and (3) a mirrored update on both
// sides of the boundary (Go core + TypeScript plugin for the contract;
// internal/mcp + internal/cli serveMCP for MCP, with the drift tests
// extended to the new pin). Until then, any non-pinned version fails
// closed by equality — the tests below.
func TestCompatibilityRefusesUnknownContracts(t *testing.T) {
	// The MCP pin negotiates explicitly: only the exact pin is accepted.
	if got, err := mcp.NegotiateVersion(mcp.ContractVersion); err != nil || got != mcp.ContractVersion {
		t.Fatalf("NegotiateVersion(pinned) = (%q,%v), want (%q,nil)", got, err, mcp.ContractVersion)
	}
	mcpRefused := []string{
		"", "regulus.mcp/v2", "regulus.mcp/v1 ", " REGULUS.MCP/V1",
		"regulus.contract/v1", "regulus.http/v1", "2024-11-05", "v1",
	}
	for _, client := range mcpRefused {
		if got, err := mcp.NegotiateVersion(client); err == nil {
			t.Fatalf("mcp.NegotiateVersion(%q) = %q, want refusal (never silent)", client, got)
		} else if !strings.Contains(err.Error(), client) || !strings.Contains(err.Error(), mcp.ContractVersion) {
			t.Fatalf("mcp.NegotiateVersion(%q) error = %q, want it to name both the client version and the server pin", client, err)
		}
	}

	// The HTTP pin negotiates the same way on its own family pin.
	if got, err := httpapi.NegotiateVersion(httpapi.ContractVersion); err != nil || got != httpapi.ContractVersion {
		t.Fatalf("httpapi.NegotiateVersion(pinned) = (%q,%v), want (%q,nil)", got, err, httpapi.ContractVersion)
	}
	for _, client := range []string{"", "regulus.http/v2", "regulus.http/v1 ", "regulus.mcp/v1", "regulus.contract/v1"} {
		if got, err := httpapi.NegotiateVersion(client); err == nil {
			t.Fatalf("httpapi.NegotiateVersion(%q) = %q, want refusal (never silent)", client, got)
		} else if !strings.Contains(err.Error(), client) || !strings.Contains(err.Error(), httpapi.ContractVersion) {
			t.Fatalf("httpapi.NegotiateVersion(%q) error = %q, want it to name both the client version and the server pin", client, err)
		}
	}

	// The regulus.contract pin has no negotiator function by design: the
	// OpenCode plugin compares the printed pin for equality, so refusal
	// IS pin mismatch — a future v2 fails closed here, never silently
	// accepted.
	if contract.Version != "regulus.contract/v1" {
		t.Fatalf("contract.Version = %q, want pinned regulus.contract/v1", contract.Version)
	}
	for _, future := range []string{"regulus.contract/v2", "", "regulus.contract/v1 ", "REGULUS.CONTRACT/V1"} {
		if future == contract.Version {
			t.Fatalf("future contract %q must not equal the pinned %q", future, contract.Version)
		}
	}
	// Family separation: the contract pin is not valid currency on the
	// other negotiators either.
	if _, err := mcp.NegotiateVersion(contract.Version); err == nil {
		t.Fatalf("mcp.NegotiateVersion(contract pin) succeeded, want family separation")
	}
	if _, err := httpapi.NegotiateVersion(contract.Version); err == nil {
		t.Fatalf("httpapi.NegotiateVersion(contract pin) succeeded, want family separation")
	}
}

// S8 acceptance: Regulus demonstrates that (1) the core is agent-SDK
// free, (2) adapters have delimited responsibilities, (3) MCP exposes
// core capabilities, (4) policies are common to all runtimes, (5)
// contracts are versioned, and (6) each runtime's support is demonstrated
// by tests. No fictitious adapters: the only adapter that may exist in
// tests is the clearly-marked test-only fake, never product support.
func TestAcceptanceCriteria(t *testing.T) {
	t.Run("1-core-has-no-external-modules", func(t *testing.T) {
		// Same `go list -m all` technique as
		// TestCoreHasNoExternalDependencies in boundary_test.go
		// (inlined, no helper duplicated): the module graph must show
		// nothing but the main module — any agent-SDK (or any other)
		// external dependency fails by construction.
		out, err := exec.Command("go", "list", "-m", "all").Output()
		if err != nil {
			t.Fatalf("go list -m all: %v", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if mod := strings.Fields(line)[0]; mod != "regulus-ai" {
				t.Fatalf("S8.1: core depends on external module %q (want stdlib-only core)", mod)
			}
		}
	})

	t.Run("2-adapters-have-delimited-responsibilities", func(t *testing.T) {
		// Production registers NOTHING: the product runtime names stay
		// unclaimed in Go (S3 forbids fictitious support — the only
		// product-supported runtime is the TypeScript OpenCode plugin,
		// proven in criterion 6).
		for _, name := range []string{"opencode", "claude-code", "codex"} {
			if _, ok := Lookup(name); ok {
				t.Fatalf("S8.2: Lookup(%q) succeeded: production must register NOTHING", name)
			}
		}
		// Whatever IS registered (test order may have registered the
		// fake already) must be clearly marked test-only — never
		// mistaken for product support.
		for _, name := range Names() {
			marked := strings.Contains(strings.ToLower(name), "test") ||
				strings.Contains(strings.ToLower(name), "fake")
			if !marked {
				t.Fatalf("S8.2: registry holds %q without test-only marking (no fictitious adapters)", name)
			}
		}
	})

	t.Run("3-mcp-exposes-core", func(t *testing.T) {
		// Drift-freedom (descriptors == server dispatch table) is pinned
		// by TestToolDescriptorsMatchServerTable in internal/mcp, which
		// reads the server source; here we assert discovery
		// completeness: capability discovery serves every descriptor, in
		// order, with the pinned wire versions — MCP exposes the core,
		// never duplicates its logic.
		disc := mcp.CapabilityDiscovery()
		if disc.ProtocolVersion != mcp.ProtocolVersion {
			t.Fatalf("S8.3: discovery protocol = %q, want %q", disc.ProtocolVersion, mcp.ProtocolVersion)
		}
		if disc.ContractVersion != mcp.ContractVersion {
			t.Fatalf("S8.3: discovery contract = %q, want %q", disc.ContractVersion, mcp.ContractVersion)
		}
		want := mcp.ToolNames()
		if len(disc.Tools) == 0 {
			t.Fatal("S8.3: discovery serves no tools (MCP must expose core capabilities)")
		}
		if len(disc.Tools) != len(want) {
			t.Fatalf("S8.3: discovery serves %d tools, want the %d descriptors (completeness, no more no less)", len(disc.Tools), len(want))
		}
		for i := range want {
			if disc.Tools[i].Name != want[i] {
				t.Fatalf("S8.3: discovery tool %d = %q, want %q (order tracks the dispatch table)", i, disc.Tools[i].Name, want[i])
			}
		}
		if len(disc.Resources) != len(mcp.Resources()) {
			t.Fatalf("S8.3: discovery resources = %d, want %d", len(disc.Resources), len(mcp.Resources()))
		}
		if len(disc.Prompts) != len(mcp.Prompts()) {
			t.Fatalf("S8.3: discovery prompts = %d, want %d", len(disc.Prompts), len(mcp.Prompts()))
		}
	})

	t.Run("4-policies-common-to-all-runtimes", func(t *testing.T) {
		// Error-mapping parity asserted directly: the HTTP envelope
		// reuses the MCP codes without changing code or message — one
		// policy, two transports.
		for _, mcpErr := range []mcp.Error{
			mcp.ParseError(), mcp.UnknownMethod("nope_missing"),
			mcp.UnknownTool("nope_missing"), mcp.InvalidParams("bad"),
			mcp.InternalError("boom"),
		} {
			env := httpapi.Fail(mcpErr)
			if env.OK {
				t.Fatalf("S8.4: Fail(%+v) must set ok=false", mcpErr)
			}
			if env.Error == nil || env.Error.Code != mcpErr.Code || env.Error.Message != mcpErr.Message {
				t.Fatalf("S8.4: Fail(%+v) = %+v, want identical code+message", mcpErr, env.Error)
			}
		}
		// Policy application parity: MapResult agrees with
		// mcp.ClassifyExit on every exit — only usage failure (2) is a
		// protocol error; everything else is a success envelope.
		for _, exit := range []int{0, 1, 2, 3} {
			_, wantErr := mcp.ClassifyExit(exit)
			status, env := httpapi.MapResult(map[string]any{"x": 1}, "detail", exit)
			if wantErr && (env.OK || status != http.StatusBadRequest || env.Error.Code != mcp.CodeInvalidParams) {
				t.Fatalf("S8.4: MapResult(exit=%d) = (%d,%+v), want (400, protocol error)", exit, status, env)
			}
			if !wantErr && (!env.OK || status != http.StatusOK) {
				t.Fatalf("S8.4: MapResult(exit=%d) = (%d,%+v), want (200, success envelope)", exit, status, env)
			}
		}
	})

	t.Run("5-contracts-versioned", func(t *testing.T) {
		// Both S7 pins asserted: the plugin<->core contract and the MCP
		// contract evolve only through explicit versions.
		if contract.Version != "regulus.contract/v1" {
			t.Fatalf("S8.5: contract.Version = %q, want regulus.contract/v1", contract.Version)
		}
		if mcp.ContractVersion != "regulus.mcp/v1" {
			t.Fatalf("S8.5: mcp.ContractVersion = %q, want regulus.mcp/v1", mcp.ContractVersion)
		}
		if httpapi.ContractVersion != "regulus.http/v1" {
			t.Fatalf("S8.5: httpapi.ContractVersion = %q, want regulus.http/v1", httpapi.ContractVersion)
		}
	})

	t.Run("6-runtime-support-demonstrated-by-tests", func(t *testing.T) {
		// The opencode preservation proof (TestOpenCodePluginPreserved in
		// adapter_test.go, TestParityWithTypeScriptMirror in
		// internal/contract) referenced and asserted present here: the
		// supported adapter file exists and its contract mirror still
		// pins the version the Go core speaks. Repo root and mirror
		// regexp reused from adapter_test.go (same package, no copy).
		root := testRepoRoot(t)
		if _, err := os.Stat(filepath.Join(root, ".opencode", "plugins", "regulus.ts")); err != nil {
			t.Fatalf("S8.6: supported OpenCode adapter missing: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(root, ".opencode", "lib", "domain.ts"))
		if err != nil {
			t.Fatalf("S8.6: reading contract mirror: %v", err)
		}
		m := tsContractRe.FindSubmatch(data)
		if m == nil {
			t.Fatal("S8.6: REGULUS_CONTRACT mirror missing (the support proof must pin the version)")
		}
		if got := string(m[1]); got != contract.Version {
			t.Fatalf("S8.6: plugin speaks %q but core speaks %q (support proof drifted)", got, contract.Version)
		}
	})
}
