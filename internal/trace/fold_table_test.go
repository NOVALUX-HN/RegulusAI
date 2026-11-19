// Pure table-driven fold cases (no disk): zero/legacy entries,
// host-known totals, overflow aborts and RateKeys/Sessions dedup.
// Complements the on-disk CostByTask tests in cost_test.go.
package trace

import (
	"math"
	"strings"
	"testing"
)

func TestFoldTable(t *testing.T) {
	tests := []struct {
		name string
		// preset runs before the table entries (e.g. MaxInt64 totals
		// for overflow cases).
		preset  TaskCost
		session string
		// sessions optionally overrides the session per entry
		// (default: every entry folds under session).
		sessions []string
		entries  []Entry
		check    func(t *testing.T, got TaskCost)
		wantErr  string
	}{
		{
			name:    "zero legacy entry stays unknown with local proxy",
			session: "s1",
			entries: []Entry{{Status: StatusCompleted}},
			check: func(t *testing.T, got TaskCost) {
				t.Helper()
				if got.Calls != 1 || got.Entries != 1 || got.Errors != 0 {
					t.Fatalf("calls/errors/entries = %d/%d/%d, want 1/0/1", got.Calls, got.Errors, got.Entries)
				}
				if got.DurationKnown || got.BytesKnown || got.TokensKnown {
					t.Fatalf("legacy costs must read unknown, got %+v", got)
				}
				if !got.LocalTokensKnown {
					t.Fatalf("local proxy must mark measured: %+v", got)
				}
				if want := EstimateLocalTokens(Entry{Status: StatusCompleted}); got.TotalLocalTokens != want {
					t.Fatalf("local tokens = %d, want %d", got.TotalLocalTokens, want)
				}
				if len(got.Sessions) != 1 || got.Sessions[0] != "s1" {
					t.Fatalf("sessions = %v, want [s1]", got.Sessions)
				}
				if got.TokenSource != "" || len(got.RateKeys) != 0 {
					t.Fatalf("legacy entry names no source/keys: %+v", got)
				}
			},
		},
		{
			name:    "error entry counts",
			session: "s1",
			entries: []Entry{{Status: StatusError, DurationMS: 5, OutputBytes: 9}},
			check: func(t *testing.T, got TaskCost) {
				t.Helper()
				if got.Errors != 1 {
					t.Fatalf("errors = %d, want 1", got.Errors)
				}
				if !got.DurationKnown || !got.BytesKnown {
					t.Fatalf("measured proxy must read known: %+v", got)
				}
				if got.TokensKnown {
					t.Fatalf("tokenless entry must keep host unknown: %+v", got)
				}
			},
		},
		{
			name:    "host-known marks source and rate key",
			session: "s1",
			entries: []Entry{{
				Status: StatusCompleted, InputTokens: 10, OutputTokens: 5,
				Model: "m", HostVersion: "v",
			}},
			check: func(t *testing.T, got TaskCost) {
				t.Helper()
				if !got.TokensKnown || got.TokenSource != SourceMeasuredHost {
					t.Fatalf("host tokens must read measured-host: %+v", got)
				}
				if got.TotalInputTokens != 10 || got.TotalOutputTokens != 5 {
					t.Fatalf("tokens = %d+%d, want 10+5", got.TotalInputTokens, got.TotalOutputTokens)
				}
				if len(got.RateKeys) != 1 || got.RateKeys[0] != "m@v" {
					t.Fatalf("rate keys = %v, want [m@v]", got.RateKeys)
				}
			},
		},
		{
			name:    "host-known without model names no rate key",
			session: "s1",
			entries: []Entry{{Status: StatusCompleted, InputTokens: 3}},
			check: func(t *testing.T, got TaskCost) {
				t.Helper()
				if !got.TokensKnown || len(got.RateKeys) != 0 {
					t.Fatalf("model-less host entry names no key: %+v", got)
				}
			},
		},
		{
			name:    "usage source fills in only when host names none",
			session: "s1",
			entries: []Entry{{Status: StatusCompleted, UsageSource: SourceEstimated}},
			check: func(t *testing.T, got TaskCost) {
				t.Helper()
				if got.TokensKnown {
					t.Fatalf("source-only entry carries no host counts: %+v", got)
				}
				if got.TokenSource != SourceEstimated {
					t.Fatalf("source = %q, want %q", got.TokenSource, SourceEstimated)
				}
			},
		},
		{
			name:    "usage source never overrides measured host",
			session: "s1",
			entries: []Entry{
				{Status: StatusCompleted, InputTokens: 1, Model: "m", HostVersion: "v"},
				{Status: StatusCompleted, UsageSource: SourceEstimated},
			},
			check: func(t *testing.T, got TaskCost) {
				t.Helper()
				if got.TokenSource != SourceMeasuredHost {
					t.Fatalf("source = %q, want measured-host", got.TokenSource)
				}
			},
		},
		{
			name:    "rate keys and sessions dedup",
			session: "s1",
			entries: []Entry{
				{Status: StatusCompleted, InputTokens: 1, Model: "m", HostVersion: "v"},
				{Status: StatusCompleted, OutputTokens: 2, Model: "m", HostVersion: "v"},
			},
			check: func(t *testing.T, got TaskCost) {
				t.Helper()
				if len(got.RateKeys) != 1 || got.RateKeys[0] != "m@v" {
					t.Fatalf("rate keys = %v, want deduped [m@v]", got.RateKeys)
				}
				if len(got.Sessions) != 1 || got.Sessions[0] != "s1" {
					t.Fatalf("sessions = %v, want deduped [s1]", got.Sessions)
				}
				if got.Calls != 2 || got.TotalInputTokens != 1 || got.TotalOutputTokens != 2 {
					t.Fatalf("sums = %+v", got)
				}
			},
		},
		{
			name:     "distinct sessions and rate keys accumulate",
			session:  "s1",
			sessions: []string{"s1", "s2"},
			entries: []Entry{
				{Status: StatusCompleted, InputTokens: 1, Model: "a", HostVersion: "1"},
				{Status: StatusCompleted, InputTokens: 2, Model: "b", HostVersion: "2"},
			},
			check: func(t *testing.T, got TaskCost) {
				t.Helper()
				if len(got.RateKeys) != 2 || got.RateKeys[0] != "a@1" || got.RateKeys[1] != "b@2" {
					t.Fatalf("rate keys = %v, want [a@1 b@2]", got.RateKeys)
				}
			},
		},
		{
			name:    "sums accumulate across entries",
			session: "s1",
			entries: []Entry{
				{Status: StatusCompleted, DurationMS: 100, OutputBytes: 1000, InputTokens: 10, OutputTokens: 5, ReasoningTokens: 2, CacheRead: 1, CacheWrite: 3},
				{Status: StatusCompleted, DurationMS: 50, OutputBytes: 500, InputTokens: 7, OutputTokens: 3},
			},
			check: func(t *testing.T, got TaskCost) {
				t.Helper()
				if got.TotalDurationMS != 150 || got.TotalOutputBytes != 1500 {
					t.Fatalf("proxy = %dms/%dB, want 150ms/1500B", got.TotalDurationMS, got.TotalOutputBytes)
				}
				if got.TotalInputTokens != 17 || got.TotalOutputTokens != 8 || got.TotalReasoning != 2 || got.TotalCacheRead != 1 || got.TotalCacheWrite != 3 {
					t.Fatalf("tokens = %+v", got)
				}
			},
		},
		{
			name:    "overflow duration aborts",
			preset:  TaskCost{Task: "T", TotalDurationMS: math.MaxInt64},
			session: "s1",
			entries: []Entry{{Status: StatusCompleted, DurationMS: 1}},
			wantErr: "fold",
		},
		{
			name:    "overflow tokens abort",
			preset:  TaskCost{Task: "T", TotalInputTokens: math.MaxInt64},
			session: "s1",
			entries: []Entry{{Status: StatusCompleted, InputTokens: 1}},
			wantErr: "fold",
		},
		{
			name:    "overflow local proxy aborts",
			preset:  TaskCost{Task: "T", TotalLocalTokens: math.MaxInt64},
			session: "s1",
			entries: []Entry{{Status: StatusCompleted, Tool: "t", Message: "m", Call: "c"}},
			wantErr: "fold",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.preset
			if c.Task == "" {
				c.Task = "T"
			}
			var err error
			for i, e := range tc.entries {
				session := tc.session
				if i < len(tc.sessions) && tc.sessions[i] != "" {
					session = tc.sessions[i]
				}
				if err = c.fold(session, e); err != nil {
					break
				}
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("fold err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("fold: %v", err)
			}
			tc.check(t, c)
		})
	}
}
