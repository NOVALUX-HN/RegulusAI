package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/contract"
)

// testOnlyFakeAdapter is a TEST-ONLY in-memory Adapter used to prove the
// runtime contract (lifecycle, permissions, context exchange, reporting).
// It is NOT product support for any new runtime: no production code
// registers it, and the only product-supported runtime remains the
// existing TypeScript OpenCode plugin (.opencode/plugins/regulus.ts).
type testOnlyFakeAdapter struct {
	name string
	caps []ToolCapability
}

// testOnlyFakeSession is the TEST-ONLY Session opened by
// testOnlyFakeAdapter. Same test-only status: never shipped as support.
type testOnlyFakeSession struct {
	boundary PermissionBoundary
	entries  []ContextEntry
	reported *Result
	failures []*AdapterError
	closed   bool
}

func newTestOnlyFakeAdapter() *testOnlyFakeAdapter {
	return &testOnlyFakeAdapter{
		name: "test-only-fake",
		caps: []ToolCapability{{Name: "read"}, {Name: "write"}},
	}
}

func (f *testOnlyFakeAdapter) Name() string { return f.name }

func (f *testOnlyFakeAdapter) Capabilities() []ToolCapability { return f.caps }

func (f *testOnlyFakeAdapter) Open(task TaskContext, agent AgentIdentity, boundary PermissionBoundary, meta ExecutionMetadata) (Session, error) {
	offered := make([]string, 0, len(f.caps))
	for _, c := range f.caps {
		offered = append(offered, c.Name)
	}
	if _, missing := Negotiate(boundary.AllowedTools, offered); len(missing) > 0 {
		return nil, &AdapterError{
			Code:      "capability/missing",
			Message:   "adapter lacks capabilities: " + strings.Join(missing, ", "),
			Retryable: false,
		}
	}
	_ = task
	_ = agent
	_ = meta
	return &testOnlyFakeSession{boundary: boundary}, nil
}

func (s *testOnlyFakeSession) CheckPermission(tool string, destructive bool) error {
	return s.boundary.CheckPermission(tool, destructive)
}

func (s *testOnlyFakeSession) Provide(entry ContextEntry) {
	s.entries = append(s.entries, entry)
}

func (s *testOnlyFakeSession) Context() []ContextEntry {
	out := make([]ContextEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

func (s *testOnlyFakeSession) Report(result Result) error {
	if s.closed {
		return &AdapterError{Code: "lifecycle/closed", Message: "report on closed session", Retryable: false}
	}
	r := result
	s.reported = &r
	return nil
}

func (s *testOnlyFakeSession) Fail(err *AdapterError) error {
	if s.closed {
		return &AdapterError{Code: "lifecycle/closed", Message: "fail on closed session", Retryable: false}
	}
	if err == nil {
		return nil
	}
	s.failures = append(s.failures, err)
	return nil
}

func (s *testOnlyFakeSession) Close() error {
	if s.closed {
		return &AdapterError{Code: "lifecycle/closed", Message: "session already closed", Retryable: false}
	}
	s.closed = true
	return nil
}

func mustTestOnlyAdapter(t *testing.T) Adapter {
	t.Helper()
	f := newTestOnlyFakeAdapter()
	Register(f.Name(), f)
	got, ok := Lookup(f.Name())
	if !ok {
		t.Fatal("Lookup(test-only-fake) failed after Register")
	}
	return got
}

func openTestOnlySession(t *testing.T, a Adapter, tools []string) Session {
	t.Helper()
	sess, err := a.Open(
		TaskContext{Task: "phase7-t2", Domain: "runtime", Root: "/tmp", Metadata: map[string]string{"phase": "7"}},
		AgentIdentity{Runtime: "test-only", Name: "fake", Version: "0.0.0"},
		PermissionBoundary{AllowDestructive: false, AllowedTools: tools},
		ExecutionMetadata{RunID: "run-1", Timeout: time.Second, Isolation: "none"},
	)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return sess
}

func adapterErrorCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("want AdapterError, got nil")
	}
	var ae *AdapterError
	if !errors.As(err, &ae) {
		t.Fatalf("want *AdapterError, got %T (%v)", err, err)
	}
	return ae.Code
}

// Happy path: Open -> permission-check -> context-exchange -> Report -> Close.
func TestAdapterHappyPath(t *testing.T) {
	sess := openTestOnlySession(t, mustTestOnlyAdapter(t), []string{"read"})

	if err := sess.CheckPermission("read", false); err != nil {
		t.Fatalf("CheckPermission(read): %v", err)
	}

	sess.Provide(ContextEntry{Key: "chart", Value: "charts/x.md"})
	sess.Provide(ContextEntry{Key: "lesson", Value: "prove it"})
	got := sess.Context()
	if len(got) != 2 || got[0].Key != "chart" || got[1].Value != "prove it" {
		t.Fatalf("Context() = %+v, want the two provided entries in order", got)
	}

	res := Result{Verdict: "pass", Summary: "all green", Receipts: []string{"receipt-1"}}
	if err := sess.Report(res); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// Unknown capabilities are refused, never silently assumed.
func TestAdapterRefusesUnknownCapability(t *testing.T) {
	sess := openTestOnlySession(t, mustTestOnlyAdapter(t), []string{"read"})

	if code := adapterErrorCode(t, sess.CheckPermission("teleport", false)); code != "permission/unknown-capability" {
		t.Fatalf("CheckPermission(teleport) code = %q, want permission/unknown-capability", code)
	}
	if code := adapterErrorCode(t, sess.CheckPermission("read", true)); code != "permission/destructive-not-allowed" {
		t.Fatalf("CheckPermission(read, destructive) code = %q, want permission/destructive-not-allowed", code)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// Missing capabilities are explicit: Negotiate reports them and Open refuses.
func TestNegotiateReportsMissing(t *testing.T) {
	granted, missing := Negotiate([]string{"read", "teleport"}, []string{"read"})
	if len(granted) != 1 || granted[0] != "read" {
		t.Fatalf("granted = %q, want [read]", granted)
	}
	if len(missing) != 1 || missing[0] != "teleport" {
		t.Fatalf("missing = %q, want [teleport] (never silently assumed)", missing)
	}

	if granted, missing := Negotiate(nil, []string{"read"}); len(granted) != 0 || len(missing) != 0 {
		t.Fatalf("empty required: granted = %q, missing = %q, want both empty", granted, missing)
	}

	if _, err := mustTestOnlyAdapter(t).Open(
		TaskContext{Task: "t", Domain: "d", Root: "/tmp", Metadata: nil},
		AgentIdentity{Runtime: "test-only", Name: "fake", Version: "0.0.0"},
		PermissionBoundary{AllowDestructive: false, AllowedTools: []string{"read", "teleport"}},
		ExecutionMetadata{RunID: "run-x", Timeout: time.Second, Isolation: "none"},
	); err == nil {
		t.Fatal("Open with missing capability succeeded, want capability/missing refusal")
	} else if code := adapterErrorCode(t, err); code != "capability/missing" {
		t.Fatalf("Open code = %q, want capability/missing", code)
	}
}

// Unknown adapters are refused.
func TestLookupUnknownAdapterRefused(t *testing.T) {
	if _, ok := Lookup("no-such-adapter"); ok {
		t.Fatal("Lookup(no-such-adapter) succeeded, want refusal")
	}
}

// S3 forbids fictitious support: production registers NOTHING, so the
// product runtime names stay unclaimed in Go. Only the test-only fake
// (registered above) may appear.
func TestNoProductAdaptersRegistered(t *testing.T) {
	for _, name := range []string{"opencode", "claude-code", "codex"} {
		if _, ok := Lookup(name); ok {
			t.Fatalf("Lookup(%q) succeeded: production must register NOTHING (S3 forbids fictitious support)", name)
		}
	}
}

// Error propagation carries code/message/retryable through Fail.
func TestFailPropagatesAdapterError(t *testing.T) {
	sess := openTestOnlySession(t, mustTestOnlyAdapter(t), []string{"read"})
	boom := &AdapterError{Code: "tool/boom", Message: "boom", Retryable: true}
	if err := sess.Fail(boom); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if !strings.Contains(boom.Error(), "tool/boom") || !strings.Contains(boom.Error(), "boom") {
		t.Fatalf("AdapterError.Error() = %q, want code and message", boom.Error())
	}
	if !boom.Retryable {
		t.Fatal("Retryable lost through Fail path")
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func testRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("could not locate this test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

var tsContractRe = regexp.MustCompile(`REGULUS_CONTRACT\s*=\s*"([^"]+)"`)

// Preservation proof (S3): the product-supported OpenCode adapter stays
// untouched — its plugin file exists and its contract mirror still pins
// the same version the Go core speaks.
func TestOpenCodePluginPreserved(t *testing.T) {
	root := testRepoRoot(t)
	if _, err := os.Stat(filepath.Join(root, ".opencode", "plugins", "regulus.ts")); err != nil {
		t.Fatalf(".opencode/plugins/regulus.ts missing: %v (the supported adapter must be preserved)", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".opencode", "lib", "domain.ts"))
	if err != nil {
		t.Fatalf("reading .opencode/lib/domain.ts: %v", err)
	}
	m := tsContractRe.FindSubmatch(data)
	if m == nil {
		t.Fatal("REGULUS_CONTRACT not found in .opencode/lib/domain.ts")
	}
	if got := string(m[1]); got != contract.Version {
		t.Fatalf("contract drift: plugin speaks %q but core speaks %q", got, contract.Version)
	}
}
