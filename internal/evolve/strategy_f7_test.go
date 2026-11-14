package evolve

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func f7Cands(ids ...string) []ExplorationCandidate {
	out := make([]ExplorationCandidate, 0, len(ids))
	for _, id := range ids {
		out = append(out, ExplorationCandidate{ID: id, Domain: DomainINV})
	}
	return out
}

func f7PassProbe() ExploreProbe {
	return func(_ context.Context, cand ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		return ExplorationOutcome{ID: cand.ID, Domain: cand.Domain, ParentID: cand.ParentID, Status: ExplorePass, Detail: "ok", Tokens: 10}, nil
	}
}

func f7Opts(k int, mode RunMode) RunOptions {
	return RunOptions{K: k, Mode: mode, Seed: 20261114, Budget: RunBudget{MaxRetries: 1, MaxConcurrency: 4, MaxDiffBytes: 4096}}
}

func TestF7K(t *testing.T) {
	var n atomic.Int32
	pr := func(_ context.Context, _ ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		n.Add(1)
		return ExplorationOutcome{Status: ExplorePass, Tokens: 5}, nil
	}
	out, err := RunExploration(context.Background(), f7Opts(1, RunModeSequential), f7Cands("c1", "c2", "c3"), pr)
	if err != nil || len(out) != 1 || out[0].ID != "c1" || n.Load() != 1 {
		t.Fatalf("k1 %+v %v %d", out, err, n.Load())
	}
	out, err = RunExploration(context.Background(), f7Opts(3, RunModeParallel), f7Cands("c1", "c2", "c3"), f7PassProbe())
	if err != nil || len(out) != 3 {
		t.Fatalf("k3 %+v %v", out, err)
	}
	for i, id := range []string{"c1", "c2", "c3"} {
		if out[i].ID != id || out[i].Status != ExplorePass {
			t.Fatalf("order %+v", out)
		}
	}
}

func TestF7Modes(t *testing.T) {
	cands := []ExplorationCandidate{{ID: "i1", Domain: DomainINV}, {ID: "d1", Domain: DomainDOM}}
	pr := func(_ context.Context, _ ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		return ExplorationOutcome{Status: ExplorePass, Detail: "s"}, nil
	}
	out, err := RunExploration(context.Background(), f7Opts(2, RunModeParallel), cands, pr)
	if err != nil || out[0].Domain != DomainINV || out[1].Domain != DomainDOM {
		t.Fatalf("invdom %+v %v", out, err)
	}
	var order []string
	sq := func(_ context.Context, c ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		order = append(order, c.ID)
		return ExplorationOutcome{Status: ExplorePass}, nil
	}
	chain := []ExplorationCandidate{{ID: "c1", Domain: DomainINV}, {ID: "c2", Domain: DomainINV, ParentID: "c1"}}
	opts := f7Opts(2, RunModeSequential)
	opts.Workspace = t.TempDir()
	out, err = RunExploration(context.Background(), opts, chain, sq)
	if err != nil || out[1].ParentID != "c1" || len(order) != 2 || order[0] != "c1" {
		t.Fatalf("seq %+v %v %v", out, err, order)
	}
}

func TestF7CancelTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pr := func(ctx context.Context, _ ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		return ExplorationOutcome{Status: ExplorePass}, nil
	}
	if _, err := RunExploration(ctx, f7Opts(2, RunModeSequential), f7Cands("c1"), pr); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
	slow := func(ctx context.Context, _ ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		select {
		case <-ctx.Done():
			return ExplorationOutcome{Status: ExploreTimeout}, ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return ExplorationOutcome{Status: ExplorePass}, nil
		}
	}
	opts := f7Opts(2, RunModeSequential)
	opts.Budget.MaxWallTime = 20 * time.Millisecond
	if _, err := RunExploration(context.Background(), opts, f7Cands("c1"), slow); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout %v", err)
	}
}

func TestF7Resume(t *testing.T) {
	var n atomic.Int32
	pr := func(_ context.Context, _ ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		n.Add(1)
		return ExplorationOutcome{Status: ExplorePass}, nil
	}
	opts := f7Opts(3, RunModeSequential)
	opts.Workspace = t.TempDir()
	cands := f7Cands("c1", "c2", "c3")
	first, err := RunExploration(context.Background(), opts, cands[:2], pr)
	if err != nil || n.Load() != 2 {
		t.Fatalf("first %+v %v", first, err)
	}
	done := map[string]ExplorationOutcome{first[0].ID: first[0], first[1].ID: first[1]}
	second, err := RunExplorationResume(context.Background(), opts, cands, done, pr)
	if err != nil || len(second) != 3 || n.Load() != 3 {
		t.Fatalf("resume %+v %v %d", second, err, n.Load())
	}
	full := map[string]ExplorationOutcome{}
	for _, o := range second {
		full[o.ID] = o
	}
	if _, err := RunExplorationResume(context.Background(), opts, cands, full, pr); err != nil || n.Load() != 3 {
		t.Fatalf("idem %v %d", err, n.Load())
	}
}

func TestF7Partial(t *testing.T) {
	pr := func(_ context.Context, c ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		if c.ID == "bad" {
			return ExplorationOutcome{Status: ExploreFail}, nil
		}
		return ExplorationOutcome{Status: ExplorePass}, nil
	}
	out, err := RunExploration(context.Background(), f7Opts(3, RunModeParallel), f7Cands("a", "bad", "b"), pr)
	if err != nil {
		t.Fatalf("partial %v", err)
	}
	m := map[string]string{}
	for _, o := range out {
		m[o.ID] = o.Status
	}
	if m["bad"] != ExploreFail || m["a"] != ExplorePass || m["b"] != ExplorePass {
		t.Fatalf("mix %+v", out)
	}
	ep := func(_ context.Context, _ ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		return ExplorationOutcome{}, errors.New("boom")
	}
	out2, err := RunExploration(context.Background(), f7Opts(1, RunModeSequential), f7Cands("e1"), ep)
	if err != nil || out2[0].Status != ExploreFail || !strings.Contains(out2[0].Detail, "boom") {
		t.Fatalf("errfold %+v %v", out2, err)
	}
}

func TestF7Budgets(t *testing.T) {
	out, err := RunExploration(context.Background(), RunOptions{K: 5, Mode: RunModeSequential, Seed: 1, Budget: RunBudget{MaxCandidates: 2}}, f7Cands("a", "b", "c"), f7PassProbe())
	if err != nil || len(out) != 2 {
		t.Fatalf("cap %+v %v", out, err)
	}
	var tries atomic.Int32
	rp := func(_ context.Context, _ ExplorationCandidate, a int) (ExplorationOutcome, error) {
		tries.Add(1)
		if a < 3 {
			return ExplorationOutcome{Status: ExploreFail}, nil
		}
		return ExplorationOutcome{Status: ExplorePass}, nil
	}
	opts := RunOptions{K: 1, Mode: RunModeSequential, Seed: 1, Budget: RunBudget{MaxRetries: 1}}
	out, _ = RunExploration(context.Background(), opts, f7Cands("r1"), rp)
	if out[0].Attempts != 2 || out[0].Status != ExploreFail || tries.Load() != 2 {
		t.Fatalf("retry %+v", out)
	}
	dp := func(_ context.Context, _ ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		return ExplorationOutcome{Status: ExplorePass, Detail: strings.Repeat("x", 100)}, nil
	}
	opts = RunOptions{K: 1, Mode: RunModeSequential, Seed: 1, Budget: RunBudget{MaxDiffBytes: 10}}
	out, _ = RunExploration(context.Background(), opts, f7Cands("d1"), dp)
	if len(out[0].Detail) != 10 {
		t.Fatalf("diff %d", len(out[0].Detail))
	}
	tp := func(_ context.Context, _ ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		return ExplorationOutcome{Status: ExplorePass, Tokens: 100}, nil
	}
	opts = RunOptions{K: 3, Mode: RunModeSequential, Seed: 1, Budget: RunBudget{MaxTokens: 150}}
	if _, err := RunExploration(context.Background(), opts, f7Cands("t1", "t2", "t3"), tp); !errors.Is(err, ErrExplorationBudget) {
		t.Fatalf("tokens %v", err)
	}
	var live, maxLive atomic.Int32
	cp := func(_ context.Context, _ ExplorationCandidate, _ int) (ExplorationOutcome, error) {
		c := live.Add(1)
		for {
			m := maxLive.Load()
			if c <= m || maxLive.CompareAndSwap(m, c) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		live.Add(-1)
		return ExplorationOutcome{Status: ExplorePass}, nil
	}
	opts = RunOptions{K: 4, Mode: RunModeParallel, Seed: 1, Budget: RunBudget{MaxConcurrency: 1}}
	if _, err := RunExploration(context.Background(), opts, f7Cands("p1", "p2", "p3", "p4"), cp); err != nil || maxLive.Load() != 1 {
		t.Fatalf("conc %v %d", err, maxLive.Load())
	}
}

func TestF7Refusals(t *testing.T) {
	if _, err := RunExploration(context.Background(), RunOptions{K: 0, Mode: RunModeSequential, Seed: 1}, f7Cands("c1"), f7PassProbe()); err == nil {
		t.Fatal("k0")
	}
	if _, err := RunExploration(context.Background(), RunOptions{K: 1, Mode: RunMode("NOPE"), Seed: 1}, f7Cands("c1"), f7PassProbe()); err == nil {
		t.Fatal("mode")
	}
	if _, err := RunExploration(context.Background(), RunOptions{K: 1, Mode: RunModeSequential}, f7Cands("c1"), f7PassProbe()); err == nil {
		t.Fatal("seed")
	}
	if _, err := RunExploration(context.Background(), f7Opts(1, RunModeSequential), f7Cands("c1"), nil); err == nil {
		t.Fatal("nil")
	}
	bad := []ExplorationCandidate{{ID: "x", Domain: "NOPE"}}
	if _, err := RunExploration(context.Background(), f7Opts(1, RunModeSequential), bad, f7PassProbe()); err == nil {
		t.Fatal("dom")
	}
	neg := f7Opts(1, RunModeSequential)
	neg.Budget.MaxTokens = -1
	if _, err := RunExploration(context.Background(), neg, f7Cands("c1"), f7PassProbe()); err == nil {
		t.Fatal("neg")
	}
}
