// Package httpapi is the minimal read-only HTTP adapter (Phase 7, S5).
//
// The shared interface envelope carries the SAME error shape and codes as
// the MCP error contract (internal/mcp): HTTP maps core outcomes through
// the same mcp.ClassifyExit mapping, never per-interface policies. Every
// error constructor below delegates to internal/mcp so codes stay in sync
// by construction.
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"regulus-ai/internal/mcp"
)

// ContractVersion pins the HTTP interface contract. Like
// internal/contract and internal/mcp, it evolves only through explicit
// versions: negotiation accepts the exact pinned value and refuses
// anything else.
const ContractVersion = "regulus.http/v1"

// NegotiateVersion accepts the exact pinned contract version and refuses
// anything else. Compatibility is explicit, never silent drift.
func NegotiateVersion(client string) (string, error) {
	if client == ContractVersion {
		return ContractVersion, nil
	}
	return "", fmt.Errorf("http contract mismatch: client speaks %q but server speaks %q", client, ContractVersion)
}

// Error is one interface error: the same {code,message} shape and the
// same codes as mcp.Error. Codes are never invented here — every
// constructor delegates to internal/mcp.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Envelope is the shared JSON envelope for every HTTP reply: ok plus
// data on success, ok=false plus error on failure, and the contract
// version pinned on both so clients can detect drift.
type Envelope struct {
	OK       bool   `json:"ok"`
	Data     any    `json:"data,omitempty"`
	Error    *Error `json:"error,omitempty"`
	Contract string `json:"contract"`
}

// OK wraps a success payload with the pinned contract.
func OK(data any) Envelope {
	return Envelope{OK: true, Data: data, Contract: ContractVersion}
}

// Fail maps one MCP error into the HTTP envelope without changing its
// code or message: the same contract, a different transport.
func Fail(err mcp.Error) Envelope {
	return Envelope{OK: false, Error: &Error{Code: err.Code, Message: err.Message}, Contract: ContractVersion}
}

// NotFound reports an unknown path with the same code and message shape
// as the MCP server's unknown-method error (-32601).
func NotFound(path string) Envelope {
	return Fail(mcp.UnknownMethod(path))
}

// MethodNotAllowed reports a non-GET verb on a known read-only path. The
// code reuses mcp.CodeInvalidRequest (-32600): the request envelope is
// invalid, exactly as an MCP invalid request is.
func MethodNotAllowed(method string) Envelope {
	return Envelope{
		OK:       false,
		Error:    &Error{Code: mcp.CodeInvalidRequest, Message: "invalid request: method not allowed, want GET (got " + method + ")"},
		Contract: ContractVersion,
	}
}

// InvalidParams carries a usage-level core failure (the CLI's exit-2
// signal) with the MCP invalid-params code (-32602).
func InvalidParams(detail string) Envelope {
	return Fail(mcp.InvalidParams(detail))
}

// Internal carries an unexpected failure with the MCP internal code.
func Internal(detail string) Envelope {
	return Fail(mcp.InternalError(detail))
}

// MapResult maps one CLI core outcome (buffers plus exit code) onto an
// HTTP status and envelope through the SAME mapping the MCP server uses:
// mcp.ClassifyExit. Only the CLI's usage signal (exit 2) becomes a
// protocol error (-32602, HTTP 400). Success and regulus refusals (exit
// 0, 1, and any other non-2 code) are success envelopes (HTTP 200)
// carrying the caller's data verbatim — never a protocol error, exactly
// as MCP answers them with content replies. Policy lives in
// internal/mcp; this function only applies it.
func MapResult(data any, stderr string, exit int) (int, Envelope) {
	if _, isErr := mcp.ClassifyExit(exit); isErr {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = "invalid params"
		}
		return http.StatusBadRequest, Fail(mcp.InvalidParams(detail))
	}
	return http.StatusOK, OK(data)
}

// writeEnvelope renders one envelope as JSON.
func writeEnvelope(w http.ResponseWriter, status int, env Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(env)
}
