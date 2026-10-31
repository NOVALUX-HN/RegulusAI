package runtime

import "time"

// TaskContext carries the work a session is opened for: which task, in
// which lesson domain, rooted where, with free-form metadata.
type TaskContext struct {
	Task     string
	Domain   string
	Root     string
	Metadata map[string]string
}

// AgentIdentity identifies the agent on whose behalf a session runs:
// which runtime, under which name, at which version.
type AgentIdentity struct {
	Runtime string
	Name    string
	Version string
}

// ToolCapability names a single tool a runtime offers. Negotiation
// matches on Name, so capabilities are never silently assumed.
type ToolCapability struct {
	Name string
}

// PermissionBoundary delimits what a session may do: whether
// destructive tools may run at all, plus the allowlist of permitted
// tool names.
type PermissionBoundary struct {
	AllowDestructive bool
	AllowedTools     []string
}

// CheckPermission reports whether tool may run. Unknown tools and, when
// AllowDestructive is false, destructive calls are refused with an
// *AdapterError instead of being silently allowed.
func (b PermissionBoundary) CheckPermission(tool string, destructive bool) error {
	if destructive && !b.AllowDestructive {
		return &AdapterError{
			Code:      "permission/destructive-not-allowed",
			Message:   "destructive tool " + tool + " is not allowed by this session's boundary",
			Retryable: false,
		}
	}
	for _, allowed := range b.AllowedTools {
		if allowed == tool {
			return nil
		}
	}
	return &AdapterError{
		Code:      "permission/unknown-capability",
		Message:   "tool " + tool + " is outside this session's allowlist",
		Retryable: false,
	}
}

// ExecutionMetadata carries the run-scoped facts of a session: its id,
// its timeout budget, and the isolation it runs under.
type ExecutionMetadata struct {
	RunID     string
	Timeout   time.Duration
	Isolation string
}

// ContextEntry is one unit of context exchanged into a session.
type ContextEntry struct {
	Key   string
	Value string
}

// Result is the report a session closes with: its verdict, a
// human-readable summary, and references to supporting receipts.
type Result struct {
	Verdict  string
	Summary  string
	Receipts []string
}

// AdapterError propagates failures with a stable machine-readable code,
// a human-readable message, and whether retrying may help.
type AdapterError struct {
	Code      string
	Message   string
	Retryable bool
}

// Error implements error.
func (e *AdapterError) Error() string {
	return e.Code + ": " + e.Message
}

// Adapter is the common contract every agent runtime speaks.
// Lifecycle: Open starts a session; the session itself carries
// Report/Fail/Close. Open takes the task context, the agent identity,
// the permission boundary and the execution metadata, and exposes the
// runtime's tool capabilities for negotiation.
type Adapter interface {
	Name() string
	Capabilities() []ToolCapability
	Open(task TaskContext, agent AgentIdentity, boundary PermissionBoundary, meta ExecutionMetadata) (Session, error)
}

// Session is one open run of an Adapter: permission-checked context
// exchange (Provide/Context) ending in exactly one of Report (success)
// or Fail (propagation), followed by Close.
type Session interface {
	CheckPermission(tool string, destructive bool) error
	Provide(entry ContextEntry)
	Context() []ContextEntry
	Report(result Result) error
	Fail(err *AdapterError) error
	Close() error
}
