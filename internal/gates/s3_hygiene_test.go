// S3 hygiene: capWriter is race-safe under concurrent writes with a
// single global kill, and GitHead respects deadlines without hanging.
package gates

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Concurrent writes to one capWriter must stay consistent under -race:
// totals add up, the buffer stays bounded, overflow sticks, kill fires
// exactly once.
func TestCapWriterConcurrentRace(t *testing.T) {
	var kills atomic.Int64
	w := &capWriter{limit: 64, kill: func() { kills.Add(1) }}
	const writers = 8
	const perWrite = 16
	const writes = 50
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			chunk := make([]byte, perWrite)
			for j := 0; j < writes; j++ {
				if n, err := w.Write(chunk); n != perWrite || err != nil {
					t.Errorf("Write = %d, %v; want %d, nil", n, err, perWrite)
					return
				}
			}
		}()
	}
	wg.Wait()
	out, total, overflow := w.snapshot()
	wantTotal := int64(writers * perWrite * writes)
	if total != wantTotal {
		t.Fatalf("total = %d, want %d", total, wantTotal)
	}
	if !overflow {
		t.Fatal("large concurrent write volume must overflow a 64-byte cap")
	}
	if len(out) > 64 {
		t.Fatalf("buffered output must respect the cap, got %d bytes", len(out))
	}
	if got := kills.Load(); got != 1 {
		t.Fatalf("kill must fire exactly once, got %d", got)
	}
}

// Both streams share one process kill: concurrent overflows on out+err
// must still kill exactly once.
func TestCapWriterSharedKillOnce(t *testing.T) {
	var kills atomic.Int64
	outW := &capWriter{limit: 8}
	errW := &capWriter{limit: 8}
	var killOnce sync.Once
	kill := func() {
		killOnce.Do(func() { kills.Add(1) })
	}
	outW.kill, errW.kill = kill, kill
	var wg sync.WaitGroup
	for _, w := range []*capWriter{outW, errW} {
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func(w *capWriter) {
				defer wg.Done()
				_, _ = w.Write([]byte("0123456789ABCDEF"))
			}(w)
		}
	}
	wg.Wait()
	if got := kills.Load(); got != 1 {
		t.Fatalf("shared kill must fire exactly once, got %d", got)
	}
}

// An already-expired deadline must return "" without invoking a
// hanging runner (no hang, fast return).
func TestGitHeadRespectsDeadline(t *testing.T) {
	hanging := &hangRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	// Ensure the deadline passes before the call.
	time.Sleep(5 * time.Millisecond)
	start := time.Now()
	if got := GitHead(ctx, hanging); got != "" {
		t.Fatalf("expired ctx must yield empty head, got %q", got)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("GitHead with expired deadline hung for %v", elapsed)
	}
	if hanging.called.Load() {
		t.Fatal("GitHead must not invoke the runner once the ctx is expired")
	}
}

// A cancelled ctx mid-flight surfaces as "" via the runner's ctx error.
func TestGitHeadCancelledRunner(t *testing.T) {
	r := &fakeRunner{results: map[string]fakeResult{
		"git rev-parse --verify HEAD": {delay: 5 * time.Second, stdout: "abc\n"},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if got := GitHead(ctx, r); got != "" {
		t.Fatalf("cancelled run must yield empty head, got %q", got)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("GitHead did not respect cancellation: %v", elapsed)
	}
}

// hangRunner ignores nothing: it blocks forever unless the test fails.
// GitHead must never reach it with an expired ctx.
type hangRunner struct {
	called atomic.Bool
}

func (h *hangRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	h.called.Store(true)
	select {
	case <-ctx.Done():
		return "", "", -1, ctx.Err()
	case <-time.After(30 * time.Second):
		return "hung", "", 0, nil
	}
}
