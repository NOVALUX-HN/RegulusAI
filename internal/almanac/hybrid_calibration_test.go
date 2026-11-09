package almanac

// T5 (almanac-elite): retrieval calibration is pinned, not magic.
// RRFK, vectorTopN and the recency half-lives are the hybrid ranking
// contract: this test fails on any silent drift so the drift becomes a
// deliberate decision with a re-measured perf budget (docs/memory-v2.md
// §5: reindex < 2s, hybrid p95 < 50ms design on 10k obs).
import (
	"math"
	"testing"
	"time"
)

func TestHybridCalibrationPinned(t *testing.T) {
	if RRFK != 60 {
		t.Fatalf("RRFK = %d, want 60 (memory-v2 §5 fusion constant)", RRFK)
	}
	if vectorTopN != 50 {
		t.Fatalf("vectorTopN = %d, want 50 (flat-scan ceiling for personal scale)", vectorTopN)
	}
	if got := decayFactor(TypeLesson, 365*24*time.Hour); got != 1 {
		t.Fatalf("lessons must never decay, got %v", got)
	}
	if got := decayFactor(TypeSessionNote, 14*24*time.Hour); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("notes half-life: decay(14d) = %v, want 0.5", got)
	}
	if got := decayFactor(TypeDecision, 60*24*time.Hour); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("decisions half-life: decay(60d) = %v, want 0.5", got)
	}
	// Fresh notes outrank month-old notes at equal RRF: recency is real.
	if !(decayFactor(TypeSessionNote, time.Hour) > decayFactor(TypeSessionNote, 30*24*time.Hour)) {
		t.Fatal("recency must strictly decrease with age for notes")
	}
}
