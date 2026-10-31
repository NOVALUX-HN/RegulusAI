package mcp

import "fmt"

// JSON-RPC 2.0 error codes served.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
)

// Error is one JSON-RPC error: a machine-readable code plus a stable
// human-readable message.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements error.
func (e Error) Error() string {
	return fmt.Sprintf("mcp error %d: %s", e.Code, e.Message)
}

// ParseError reports malformed input, matching the server's -32700 shape.
func ParseError() Error {
	return Error{Code: CodeParseError, Message: "parse error"}
}

// InvalidRequest reports a malformed envelope, matching the server shape.
func InvalidRequest() Error {
	return Error{Code: CodeInvalidRequest, Message: "invalid request"}
}

// UnknownMethod reports an unknown method with the server's stable shape:
// "method not found: <method>".
func UnknownMethod(method string) Error {
	return Error{Code: CodeMethodNotFound, Message: "method not found: " + method}
}

// UnknownTool reports an unknown tool name with the server's stable
// shape: "unknown tool: <name>".
func UnknownTool(name string) Error {
	return Error{Code: CodeMethodNotFound, Message: "unknown tool: " + name}
}

// InvalidParams carries a usage-level failure (the CLI's exit-2 signal).
func InvalidParams(detail string) Error {
	if detail == "" {
		detail = "invalid params"
	}
	return Error{Code: CodeInvalidParams, Message: detail}
}

// InternalError carries an unexpected failure.
func InternalError(detail string) Error {
	if detail == "" {
		detail = "internal error"
	}
	return Error{Code: CodeInternal, Message: detail}
}

// ClassifyExit maps a regulus CLI exit code to the JSON-RPC contract.
// Only usage failures (exit 2) become protocol errors (-32602). Success
// (exit 0) and regulus refusals/failures (exit 1 and other non-zero
// codes) are NOT protocol errors: the server answers them as content
// replies carrying the combined output and the exit code.
func ClassifyExit(exit int) (code int, protocolError bool) {
	if exit == 2 {
		return CodeInvalidParams, true
	}
	return 0, false
}
