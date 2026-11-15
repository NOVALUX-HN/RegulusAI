package cli

// memory context resume block, hints, staleness cache and display helpers (NOV-67 split).

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/library"
)

func (c CLI) memoryContextCmd(args []string) int {
	fs := flag.NewFlagSet("memory context", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	limit := fs.Int("limit", almanac.DefaultContextLimit, "show only the latest N session notes (0: all)")
	task := fs.String("task", "", "only observations with this provenance task")
	path := fs.String("path", "", "only observations citing this provenance path")
	system := fs.String("system", "", "only this memory system (episodic|semantic|decision|procedural|evolution)")
	budget := fs.Int("budget", almanac.DefaultContextBudget, "cap listed notes/decisions items each (0: all)")
	resolve := fs.Bool("resolve-domain", false, "resolve lesson domain in-process and scope lessons (single-spawn)")
	session := fs.String("session", "", "session id for trace file-hit domain detection (with --resolve-domain)")
	var charts chartList
	fs.Var(&charts, "chart", "chart ref for Domain: pin (repeatable, with --resolve-domain)")
	lessonsN := fs.Int("lessons", 3, "scoped lessons to include with --resolve-domain (default 3; unpinned multi-domain fallback trims to 1 unless --lessons is explicit)")
	noStaleness := fs.Bool("no-staleness", false, "skip git staleness probes (no rev-list spawns)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	// R3-001 (R1 review, reliability BLOCKER): scoping flags without
	// --resolve-domain refuse (exit 2) instead of silently discarding
	// scoping and exiting 0 with unscoped context.
	lessonsFlagSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "lessons" {
			lessonsFlagSet = true
		}
	})
	if !*resolve && (*session != "" || len(charts) > 0 || lessonsFlagSet) {
		fmt.Fprintln(c.Stderr, "memory context: --session/--chart/--lessons need --resolve-domain")
		fmt.Fprintln(c.Stderr, "usage: regulus memory context [--limit N] [--task T] [--path P] [--system S] [--budget N] [--resolve-domain [--session S] [--chart C] [--lessons N]]")
		return 2
	}
	sys, err := almanac.ParseSystem(*system)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory context: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 || *limit < 0 || *budget < 0 || *lessonsN < 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory context [--limit N] [--task T] [--path P] [--system S] [--budget N] [--resolve-domain [--session S] [--chart C] [--lessons N]]")
		return 2
	}
	if *noStaleness {
		fmt.Fprintln(c.Stderr, "memory context: --no-staleness is a search-only flag (context performs no git probes)")
		fmt.Fprintln(c.Stderr, "usage: regulus memory context [--limit N] [--task T] [--path P] [--system S] [--budget N] [--resolve-domain [--session S] [--chart C] [--lessons N]]")
		return 2
	}
	scope := almanac.ContextScope{Task: *task, Path: *path, System: sys, Budget: *budget}
	ctx, err := almanac.ContextScoped(c.dir(), time.Now().UTC(), *limit, scope)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory context: %v\n", err)
		return 1
	}
	if !*resolve {
		fmt.Fprint(c.Stdout, ctx)
		return 0
	}
	domain, reason := resolveDomainInProcess(c.dir(), *session, []string(charts))
	domainLabel := domain
	if domainLabel == "" {
		domainLabel = "(unfiltered)"
	}
	fmt.Fprintf(c.Stdout, "domain: %s (reason: %s)\n\n", domainLabel, reason)
	fmt.Fprint(c.Stdout, ctx)
	// T8 (S2,S3) cheap fallback: unpinned-multi (no pin, >1 library
	// domain) is the most expensive scope — unfiltered newest-first up to
	// --lessons. Trim the DEFAULT 3 to 1 newest lesson and hint per-domain
	// `memory search --domain <d>` so the agent pulls more on demand.
	// Explicit --lessons N wins; pinned resolutions
	// (chart/lone-chart/single-domain/file-hit) keep full behavior.
	lessonsExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "lessons" {
			lessonsExplicit = true
		}
	})
	libDomains, _ := library.Domains(c.dir())
	unpinnedMulti := reason == "unfiltered-multi" || (reason == "unfiltered" && len(libDomains) > 1)
	effectiveLessons := *lessonsN
	if unpinnedMulti && !lessonsExplicit {
		effectiveLessons = 1
	}
	q := almanac.Query{Type: almanac.TypeLesson, Limit: effectiveLessons}
	if domain != "" {
		q.Domain = domain
	}
	hits, err := almanac.Search(c.dir(), q)
	if err != nil {
		fmt.Fprintf(c.Stdout, "\nLessons scoped: (search error: %v)\n", err)
		return 0
	}
	if len(hits) == 0 {
		fmt.Fprint(c.Stdout, "\nLessons scoped: (none)\n")
		printDomainHints(c, unpinnedMulti, libDomains)
		return 0
	}
	fmt.Fprint(c.Stdout, "\nLessons scoped:\n")
	for _, h := range hits {
		fmt.Fprintf(c.Stdout, "  - [%s] %s: %s\n", h.Domain, h.Title, h.Content)
	}
	printDomainHints(c, unpinnedMulti, libDomains)
	return 0
}

// printDomainHints renders one on-demand pull line per library domain for
// the T8 cheap fallback (unpinned-multi only). Pinned resolutions and
// single/zero-domain stores print nothing.
func printDomainHints(c CLI, unpinnedMulti bool, libDomains []string) {
	if !unpinnedMulti || len(libDomains) <= 1 {
		return
	}
	for _, d := range libDomains {
		fmt.Fprintf(c.Stdout, "  hint: more %q lessons on demand: regulus memory search --domain %s\n", d, d)
	}
}

// Staleness probe cache (T3 S2-S3): HEAD (per directory — one process
// can touch many repos across invocations, and tests retarget cwd per
// case) and per-head rev-list counts live for the process. Search fans
// out to one `git rev-list --count` per hit (up to --limit 20); N hits
// sharing a head cost 1 probe because HEAD cannot move under a single
// CLI invocation. Negative results (at HEAD / unknown ref) stick too —
// one probe per head per process, then silence. resetStalenessCache
// clears it for tests.
var (
	headByDir     = map[string]string{}
	headByDirSeen = map[string]bool{}
	behindCache   = map[string]string{}
	behindSeen    = map[string]bool{}
)

// resetStalenessCache clears the process-life staleness cache (tests).
func resetStalenessCache() {
	headByDir = map[string]string{}
	headByDirSeen = map[string]bool{}
	behindCache = map[string]string{}
	behindSeen = map[string]bool{}
}

// cachedGitHead resolves HEAD once per process and directory with the
// S3 git bound (30s, same as gates.changedFiles): note/save stamp
// provenance without re-probing, and a wedged git still degrades
// to "". The directory key matters: cwd moves between invocations
// (and between tests), so a single process-wide HEAD would stamp one
// repo's sha onto another's observations.
func cachedGitHead(r gates.CmdRunner, dir string) string {
	key := dir
	if abs, err := filepath.Abs(dir); err == nil {
		key = abs
	}
	if headByDirSeen[key] {
		return headByDir[key]
	}
	headByDirSeen[key] = true
	headByDir[key] = gitHeadBounded(r)
	return headByDir[key]
}

// commitsBehindSuffix renders the git staleness annotation for an
// observation with a provenance git head — empty when unknown.
func (c CLI) commitsBehindSuffix(o almanac.Observation) string {
	if o.Provenance == nil || o.Provenance.GitHead == "" {
		return ""
	}
	head := o.Provenance.GitHead
	if behindSeen[head] {
		return behindCache[head]
	}
	// Mark before probing so negatives stick: one probe per head per
	// process, even for unknown refs.
	behindSeen[head] = true
	// S3 hygiene: the rev-list probe shares the git bound so a wedged
	// repository degrades to no annotation instead of hanging search.
	ctx, cancel := gitTimeout()
	defer cancel()
	out, _, _, err := c.runner().Run(ctx, "git rev-list --count "+head+"..HEAD")
	if err != nil || strings.TrimSpace(out) == "" {
		return ""
	}
	n := strings.TrimSpace(out)
	if n == "0" {
		return ""
	}
	behindCache[head] = ", " + n + " commits behind"
	return behindCache[head]
}

// humanAge renders a coarse age annotation for staleness awareness.
func humanAge(now, saved time.Time) string {
	d := now.Sub(saved)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
