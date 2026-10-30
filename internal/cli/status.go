package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/budget"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/library"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/reflect"
)

func (c CLI) doctor() int {
	res := c.Probe()
	fmt.Fprintln(c.Stdout, doctor.VersionLine(".", c.Version))
	if res.Supported {
		fmt.Fprintf(c.Stdout, "%s — supported (V2)\n", res.Detected)
		fmt.Fprintln(c.Stdout, "plugin: cannot be verified outside OpenCode — open this repo in OpenCode to load .opencode/plugins/regulus.ts")
		return 0
	}
	fmt.Fprintf(c.Stderr, "doctor: %s\n", res.Reason)
	fmt.Fprintln(c.Stdout, "plugin: cannot be verified outside OpenCode — open this repo in OpenCode to load .opencode/plugins/regulus.ts")
	return 1
}

// statusCmd implements `regulus status` — one look at the project: the
// runtime gate, the gauntlet configuration, today's evolution against
// the cap, pending candidates, admitted lessons and receipts on disk.
func (c CLI) statusCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus status")
		return 2
	}
	res := c.Probe()
	if res.Supported {
		fmt.Fprintf(c.Stdout, "runtime:    %s — supported (V2)\n", res.Detected)
	} else {
		fmt.Fprintf(c.Stdout, "runtime:    NOT SUPPORTED — %s\n", res.Reason)
	}
	cfg, err := gates.Load("regulus.json")
	if err != nil {
		fmt.Fprintf(c.Stdout, "gauntlet:   not configured — %v\n", err)
	} else {
		fmt.Fprintf(c.Stdout, "gauntlet:   %d gate(s) configured\n", len(cfg.Gates))
	}
	now := time.Now().UTC()
	today, err := budget.TodayEvolutionCount(".", now)
	if err != nil {
		fmt.Fprintf(c.Stderr, "status: budget: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "evolution:  %d/%d run(s) today\n", today, reflect.EvolutionCap("."))
	domains, err := library.Domains(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "status: library: %v\n", err)
		return 1
	}
	pending, lessons := 0, 0
	for _, d := range domains {
		cands, err := library.Candidates(".", d)
		if err != nil {
			fmt.Fprintf(c.Stderr, "status: candidates: %v\n", err)
			return 1
		}
		ls, err := library.List(".", d)
		if err != nil {
			fmt.Fprintf(c.Stderr, "status: lessons: %v\n", err)
			return 1
		}
		pending += len(cands)
		lessons += len(ls)
	}
	fmt.Fprintf(c.Stdout, "lessons:    %d admitted, %d candidate(s) pending\n", lessons, pending)
	found, err := filepath.Glob(filepath.Join(".regulus", "evidence", "*", "*.json"))
	if err != nil {
		fmt.Fprintf(c.Stderr, "status: evidence: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "receipts:   %d on disk\n", len(found))
	// H3 (S5): the semantic index goes stale on any save after the last
	// reindex — status says so with the repairing command instead of
	// letting search surprise with a lexical-only warning.
	switch memState, memErr := almanac.IndexState("."); {
	case memErr != nil:
		fmt.Fprintf(c.Stdout, "memory:     index unreadable — %v\n", memErr)
	case memState == "fresh":
		fmt.Fprintln(c.Stdout, "memory:     semantic index fresh")
	case memState == "empty":
		fmt.Fprintln(c.Stdout, "memory:     no observations yet — nothing to index")
	default: // stale | missing
		fmt.Fprintf(c.Stdout, "memory:     semantic index %s — run `regulus memory reindex`\n", memState)
	}
	// S8: wall-time per task from receipt durations — the evolution
	// cost that matters (no model spend in the core). Unreadable
	// receipts are skipped so status stays resilient.
	type taskTime struct {
		ms int64
		n  int
	}
	byTask := map[string]*taskTime{}
	for _, p := range found {
		rec, err := receipts.ReadFile(p)
		if err != nil {
			continue
		}
		tt := byTask[rec.Task]
		if tt == nil {
			tt = &taskTime{}
			byTask[rec.Task] = tt
		}
		tt.ms += rec.DurationMS
		tt.n++
	}
	tasks := make([]string, 0, len(byTask))
	for t := range byTask {
		tasks = append(tasks, t)
	}
	sort.Strings(tasks)
	for _, t := range tasks {
		fmt.Fprintf(c.Stdout, "time:       %s %s (%d receipt(s))\n", t, formatDurationMS(byTask[t].ms), byTask[t].n)
	}
	// gremlins availability for the mutation gate
	if _, _, code, _ := c.runner().Run(context.Background(), "command -v gremlins || command -v \"$HOME/go/bin/gremlins\""); code == 0 {
		fmt.Fprintln(c.Stdout, "mutation:    gremlins available")
	} else {
		fmt.Fprintln(c.Stdout, "mutation:    gremlins NOT found — the mutation gate will fail; go install github.com/go-gremlins/gremlins/cmd/gremlins@latest")
	}
	return 0
}

// formatDurationMS renders a wall-time span compactly for status:
// ms under a second, seconds under a minute, else m/h.
func formatDurationMS(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	s := ms / 1000
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	if m < 60 {
		return fmt.Sprintf("%dm%ds", m, s%60)
	}
	return fmt.Sprintf("%dh%dm", m/60, m%60)
}
