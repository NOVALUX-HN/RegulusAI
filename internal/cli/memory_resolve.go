package cli

// chart/trace/domain resolvers shared by memory commands (NOV-67 split).

import (
	"os"
	"path/filepath"
	"strings"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/library"
	"regulus-ai/internal/trace"
)

func resolveChartPin(dir string, charts []string) (string, string, bool) {
	for _, ref := range charts {
		p := ref
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, ref)
		}
		if d := readDomainLine(p); d != "" {
			return d, "chart:" + ref, true
		}
	}
	return "", "", false
}

// resolveLoneChart falls back to the single charts/*.md entry when the
// caller passed no chart refs. More (or fewer) than one candidate, a
// missing charts dir, or no Domain: line all miss — the caller moves on.
func resolveLoneChart(dir string) (string, string, bool) {
	entries, err := os.ReadDir(filepath.Join(dir, "charts"))
	if err != nil {
		return "", "", false
	}
	var mds []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			mds = append(mds, e.Name())
		}
	}
	if len(mds) != 1 {
		return "", "", false
	}
	if d := readDomainLine(filepath.Join(dir, "charts", mds[0])); d != "" {
		return d, "lone-chart:" + mds[0], true
	}
	return "", "", false
}

// traceSessionFiles collects up to 3 unique trace files for a session,
// newest-entry order. A trace miss (unknown session, unreadable log)
// yields nothing — the file-hit stage then misses and resolution falls
// through to unfiltered-multi, exactly as the inline loop did.
func traceSessionFiles(dir, session string) []string {
	entries, err := trace.List(dir, session)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var files []string
	for _, e := range entries {
		for _, f := range e.Files {
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
				if len(files) >= 3 {
					break
				}
			}
		}
		if len(files) >= 3 {
			break
		}
	}
	return files
}

// resolveTraceFileHit matches trace files against per-domain lessons:
// the first file with a lesson in a library domain wins. Pure search
// loop — file collection lives in traceSessionFiles.
func resolveTraceFileHit(dir string, files, domains []string) (string, string, bool) {
	for _, f := range files {
		for _, d := range domains {
			hits, err := almanac.Search(dir, almanac.Query{Domain: d, Path: f, Type: almanac.TypeLesson, Limit: 1})
			if err == nil && len(hits) > 0 {
				return d, "file-hit:" + f, true
			}
		}
	}
	return "", "", false
}

// resolveDomainInProcess detects the lesson domain without spawning:
// chart pins first, then the lone chart, then single library domain,
// then trace file hits, else unfiltered. Returns domain + reason.
// The stages above run in that fixed order with byte-identical
// outcomes — this body only sequences them.
func resolveDomainInProcess(dir, session string, charts []string) (string, string) {
	if d, reason, ok := resolveChartPin(dir, charts); ok {
		return d, reason
	}
	// lone chart fallback when caller passed none
	if len(charts) == 0 {
		if d, reason, ok := resolveLoneChart(dir); ok {
			return d, reason
		}
	}
	domains, err := library.Domains(dir)
	if err != nil || len(domains) == 0 {
		return "", "unfiltered"
	}
	if len(domains) == 1 {
		return domains[0], "single-domain"
	}
	if session != "" {
		if d, reason, ok := resolveTraceFileHit(dir, traceSessionFiles(dir, session), domains); ok {
			return d, reason
		}
	}
	return "", "unfiltered-multi"
}

// memoryContextCmd implements `regulus memory context [--limit N]` — the
// resume block, DATA-framed. The default block is BOUNDED (latest
// DefaultContextLimit notes, DefaultContextBudget items per long list,
// pins always surface, every cap named with its repair); explicit
// --limit 0 --budget 0 keeps the full store (the compaction escape
// hatch). --limit < 0 or --budget < 0 is a usage error.
