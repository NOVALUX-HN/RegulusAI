// Package httpapi tests the minimal read-only HTTP adapter (Phase 7, S5).
//
// RED-cover: (a) envelope error parity with the MCP error contract,
// (b) GET endpoints return the same data as the underlying CLI core path,
// (c) POST/PUT/DELETE refused, (d) versioned path present and unversioned
// refused deterministically, (e) no new policy logic in the adapter.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/mcp"
)

// stubRunner answers the gremlins probe inside statusCmd deterministically.
type stubRunner struct{ code int }

func (s stubRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	return "", "", s.code, nil
}

func testProbe() func() doctor.Result {
	return func() doctor.Result {
		return doctor.Result{Detected: "opencode 2.1.0", Major: 2, Supported: true}
	}
}

func newTestServer() Server {
	return New("test", testProbe(), stubRunner{code: 1})
}

// directCLI runs one CLI core path with private buffers (the same
// buffers-in/buffers-out pattern the MCP server uses).
func directCLI(t *testing.T, srv Server, args ...string) (string, string, int) {
	t.Helper()
	var ob, eb bytes.Buffer
	c := cli.CLI{Version: srv.Version, Probe: srv.Probe, Runner: srv.Runner, Stdout: &ob, Stderr: &eb}
	code := c.Run(args)
	return ob.String(), eb.String(), code
}

func get(t *testing.T, srv Server, path string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	return rec.Code, body
}

func doMethod(t *testing.T, srv Server, method, path string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	return rec.Code, body
}

func decodeEnvelope(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, body)
	}
	return env
}

// (a) envelope error parity with the MCP error contract.
func TestEnvelopeErrorParityWithMCP(t *testing.T) {
	// Every MCP constructor maps through Fail without changing code/message.
	cases := []mcp.Error{
		mcp.ParseError(),
		mcp.InvalidRequest(),
		mcp.UnknownMethod("nope_missing"),
		mcp.UnknownTool("nope_missing"),
		mcp.InvalidParams("bad params"),
		mcp.InternalError("boom"),
	}
	for _, mcpErr := range cases {
		env := Fail(mcpErr)
		if env.Error == nil {
			t.Fatalf("Fail(%+v) has no error", mcpErr)
		}
		if env.Error.Code != mcpErr.Code || env.Error.Message != mcpErr.Message {
			t.Fatalf("Fail(%+v) = %+v, want identical code+message", mcpErr, env.Error)
		}
		if env.OK {
			t.Fatalf("Fail(%+v) must set ok=false", mcpErr)
		}
	}

	// Pinned codes: HTTP helpers reuse the MCP constants, never literals.
	if got := NotFound("/v1/nope").Error.Code; got != mcp.CodeMethodNotFound {
		t.Fatalf("NotFound code = %d, want mcp.CodeMethodNotFound %d", got, mcp.CodeMethodNotFound)
	}
	if got := MethodNotAllowed("POST").Error.Code; got != mcp.CodeInvalidRequest {
		t.Fatalf("MethodNotAllowed code = %d, want mcp.CodeInvalidRequest %d", got, mcp.CodeInvalidRequest)
	}
	if got := InvalidParams("x").Error.Code; got != mcp.CodeInvalidParams {
		t.Fatalf("InvalidParams code = %d, want %d", got, mcp.CodeInvalidParams)
	}
	if got := Internal("x").Error.Code; got != mcp.CodeInternal {
		t.Fatalf("Internal code = %d, want %d", got, mcp.CodeInternal)
	}

	// Shape parity: the HTTP error object marshals with exactly the
	// {code,message} keys the MCP contract uses.
	raw, _ := json.Marshal(Error{Code: -32601, Message: "x"})
	var keys map[string]any
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys["code"] == nil || keys["message"] == nil {
		t.Fatalf("Error JSON shape = %s, want exactly {code,message}", raw)
	}
}

// (b) GET endpoints return the same data as the underlying CLI core path.
func TestStatusConsistencyWithCLICorePath(t *testing.T) {
	srv := newTestServer()
	stdout, stderr, exit := directCLI(t, srv, "status")

	code, body := get(t, srv, "/v1/status")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/status = %d, want 200 (%s)", code, body)
	}
	env := decodeEnvelope(t, body)
	if env["ok"] != true {
		t.Fatalf("GET /v1/status ok = %v, want true (%s)", env["ok"], body)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("GET /v1/status data = %v, want object (%s)", env["data"], body)
	}
	if data["stdout"] != stdout {
		t.Fatalf("GET /v1/status stdout = %q, want CLI core path %q", data["stdout"], stdout)
	}
	if data["stderr"] != stderr {
		t.Fatalf("GET /v1/status stderr = %q, want CLI core path %q", data["stderr"], stderr)
	}
	if int(data["exit"].(float64)) != exit {
		t.Fatalf("GET /v1/status exit = %v, want CLI core path %d", data["exit"], exit)
	}
}

func TestVersionConsistencyWithCLICorePath(t *testing.T) {
	srv := newTestServer()
	versionOut, _, _ := directCLI(t, srv, "--version")
	contractOut, _, _ := directCLI(t, srv, "contract")

	code, body := get(t, srv, "/v1/version")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/version = %d, want 200 (%s)", code, body)
	}
	env := decodeEnvelope(t, body)
	if env["ok"] != true {
		t.Fatalf("GET /v1/version ok = %v, want true (%s)", env["ok"], body)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("GET /v1/version data = %v, want object (%s)", env["data"], body)
	}
	if data["version"] != versionOut {
		t.Fatalf("GET /v1/version version = %q, want CLI --version %q", data["version"], versionOut)
	}
	if data["contract"] != contractOut {
		t.Fatalf("GET /v1/version contract = %q, want CLI contract %q", data["contract"], contractOut)
	}
}

// (c) POST/PUT/DELETE refused with 405 and the shared envelope.
func TestNonGETRefused(t *testing.T) {
	srv := newTestServer()
	for _, path := range []string{"/v1/status", "/v1/version"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			code, body := doMethod(t, srv, method, path)
			if code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s = %d, want 405 (%s)", method, path, code, body)
			}
			env := decodeEnvelope(t, body)
			if env["ok"] != false {
				t.Fatalf("%s %s ok = %v, want false (%s)", method, path, env["ok"], body)
			}
			errObj, ok := env["error"].(map[string]any)
			if !ok {
				t.Fatalf("%s %s error = %v, want object (%s)", method, path, env["error"], body)
			}
			if int(errObj["code"].(float64)) != mcp.CodeInvalidRequest {
				t.Fatalf("%s %s error code = %v, want %d", method, path, errObj["code"], mcp.CodeInvalidRequest)
			}
		}
	}
}

// (d) versioned path present; unversioned equivalent refused deterministically.
func TestVersionedPathAndUnversionedRefused(t *testing.T) {
	srv := newTestServer()
	for _, versioned := range []string{"/v1/status", "/v1/version"} {
		if code, body := get(t, srv, versioned); code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200 (%s)", versioned, code, body)
		}
	}
	for _, unversioned := range []string{"/status", "/version"} {
		code1, body1 := get(t, srv, unversioned)
		code2, body2 := get(t, srv, unversioned)
		if code1 != http.StatusNotFound || code2 != http.StatusNotFound {
			t.Fatalf("GET %s = %d,%d, want deterministic 404,404", unversioned, code1, code2)
		}
		if string(body1) != string(body2) {
			t.Fatalf("GET %s not deterministic: %q vs %q", unversioned, body1, body2)
		}
		env := decodeEnvelope(t, body1)
		errObj, ok := env["error"].(map[string]any)
		if !ok || int(errObj["code"].(float64)) != mcp.CodeMethodNotFound {
			t.Fatalf("GET %s error = %v, want code %d (%s)", unversioned, env["error"], mcp.CodeMethodNotFound, body1)
		}
	}
}

// Unknown paths 404 with the shared envelope.
func TestUnknownPath404WithSharedEnvelope(t *testing.T) {
	srv := newTestServer()
	code, body := get(t, srv, "/v1/frobnicate")
	if code != http.StatusNotFound {
		t.Fatalf("GET /v1/frobnicate = %d, want 404 (%s)", code, body)
	}
	env := decodeEnvelope(t, body)
	errObj, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("unknown path error = %v, want object (%s)", env["error"], body)
	}
	if int(errObj["code"].(float64)) != mcp.CodeMethodNotFound {
		t.Fatalf("unknown path error code = %v, want %d", errObj["code"], mcp.CodeMethodNotFound)
	}
	if !strings.Contains(errObj["message"].(string), "/v1/frobnicate") {
		t.Fatalf("unknown path message = %q, want it to name the path", errObj["message"])
	}
}

// (e) no new policy logic in the adapter: core outcomes map through the
// SAME mcp.ClassifyExit mapping, and payloads track the core verbatim.
func TestAdapterDelegatesPolicyToCore(t *testing.T) {
	// MapResult must agree with mcp.ClassifyExit on every exit:
	// only exit 2 is a protocol error (-32602).
	for _, exit := range []int{0, 1, 2, 3} {
		_, wantErr := mcp.ClassifyExit(exit)
		status, env := MapResult(map[string]any{"x": 1}, "detail", exit)
		if wantErr && env.OK {
			t.Fatalf("MapResult(exit=%d) ok=true, want protocol error (mcp says error)", exit)
		}
		if !wantErr && !env.OK {
			t.Fatalf("MapResult(exit=%d) ok=false, want success (mcp says content reply)", exit)
		}
		if wantErr && status != http.StatusBadRequest {
			t.Fatalf("MapResult(exit=%d) status = %d, want 400", exit, status)
		}
		if !wantErr && status != http.StatusOK {
			t.Fatalf("MapResult(exit=%d) status = %d, want 200", exit, status)
		}
		if wantErr && env.Error.Code != mcp.CodeInvalidParams {
			t.Fatalf("MapResult(exit=%d) code = %d, want %d", exit, env.Error.Code, mcp.CodeInvalidParams)
		}
	}

	// Payloads track the core: a different binary version flows through
	// the adapter without adapter-side policy.
	a := New("aaa", testProbe(), stubRunner{code: 1})
	b := New("bbb", testProbe(), stubRunner{code: 1})
	_, bodyA := get(t, a, "/v1/version")
	_, bodyB := get(t, b, "/v1/version")
	if string(bodyA) == string(bodyB) {
		t.Fatalf("version payload must track the core version, got identical bodies for aaa/bbb")
	}
	directA, _, _ := directCLI(t, a, "--version")
	envA := decodeEnvelope(t, bodyA)
	if envA["data"].(map[string]any)["version"] != directA {
		t.Fatalf("adapter version = %v, want core %q", envA["data"], directA)
	}
}
