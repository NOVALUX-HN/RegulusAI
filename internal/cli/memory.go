package cli

// memoryCmd dispatch plus doctor/reindex/prune maintenance (NOV-67 split; behavior unchanged).

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/fsx"
)

// memoryCmd implements the Almanac surface (docs/memory.md):
// note|show (session notes), save|search|history (the full substrate),
// context (the deterministic resume block), prune (staleness pruning
// with human confirmation) — dispatched per subcommand.
// Lessons have one door: regulus reflect.
func (c CLI) memoryCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory note|show|save|search|history|pin|unpin|relate|export|import|context|prune|doctor|reindex|verify|reject|check|status|mine")
		return 2
	}
	if args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(c.Stdout, "usage: regulus memory note|show|save|search|history|pin|unpin|relate|export|import|context|prune|doctor|reindex|verify|reject|check|status|mine")
		return 0
	}
	switch args[0] {
	case "note":
		return c.memoryNoteCmd(args[1:])
	case "show":
		return c.memoryShowCmd(args[1:])
	case "save":
		return c.memorySaveCmd(args[1:])
	case "search":
		return c.memorySearchCmd(args[1:])
	case "history":
		return c.memoryHistoryCmd(args[1:])
	case "pin":
		return c.memoryPinCmd(true, args[1:])
	case "unpin":
		return c.memoryPinCmd(false, args[1:])
	case "relate":
		return c.memoryRelateCmd(args[1:])
	case "export":
		return c.memoryExportCmd(args[1:])
	case "import":
		return c.memoryImportCmd(args[1:])
	case "context":
		return c.memoryContextCmd(args[1:])
	case "prune":
		return c.memoryPruneCmd(args[1:])
	case "doctor":
		return c.memoryDoctorCmd(args[1:])
	case "reindex":
		return c.memoryReindexCmd(args[1:])
	case "verify":
		return c.memoryVerifyCmd(args[1:])
	case "reject":
		return c.memoryRejectCmd(args[1:])
	case "check":
		return c.memoryCheckCmd(args[1:])
	case "status":
		return c.memoryStatusCmd(args[1:])
	case "mine":
		return c.memoryMineCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "memory: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus memory note|show|save|search|history|pin|unpin|relate|export|import|context|prune|doctor|reindex|verify|reject|check|status|mine")
		return 2
	}
}

// reportDoctorBasics prints the log shape: counts by type plus the
// oldest/newest annotations. Pure renderer split from memoryDoctorCmd.
func (c CLI) reportDoctorBasics(h almanac.Health, now time.Time) {
	fmt.Fprintf(c.Stdout, "observations: %d\n", h.Total)
	types := make([]string, 0, len(h.ByType))
	for t := range h.ByType {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		fmt.Fprintf(c.Stdout, "  %s: %d\n", t, h.ByType[t])
	}
	if h.Oldest.IsZero() {
		fmt.Fprintln(c.Stdout, "oldest:     (empty log)")
	} else {
		fmt.Fprintf(c.Stdout, "oldest:     %s (%s)\n", h.Oldest.Format(time.RFC3339), humanAge(now, h.Oldest))
	}
	if h.Newest.IsZero() {
		fmt.Fprintln(c.Stdout, "newest:     (empty log)")
	} else {
		fmt.Fprintf(c.Stdout, "newest:     %s (%s)\n", h.Newest.Format(time.RFC3339), humanAge(now, h.Newest))
	}
}

// reportDoctorIntegrity prints the damage report: malformed lines and
// provenance events (pins, relations) pointing outside the log.
func (c CLI) reportDoctorIntegrity(h almanac.Health) {
	if len(h.MalformedLines) == 0 {
		fmt.Fprintln(c.Stdout, "malformed:  none")
	} else {
		lines := make([]string, 0, len(h.MalformedLines))
		for _, ln := range h.MalformedLines {
			lines = append(lines, strconv.Itoa(ln))
		}
		fmt.Fprintf(c.Stdout, "malformed:  line(s) %s\n", strings.Join(lines, ", "))
	}
	if len(h.OrphanPins) == 0 {
		fmt.Fprintln(c.Stdout, "orphan pins: none")
	} else {
		fmt.Fprintf(c.Stdout, "orphan pins: %s\n", strings.Join(h.OrphanPins, ", "))
	}
	if len(h.OrphanRelations) == 0 {
		fmt.Fprintln(c.Stdout, "orphan relations: none")
	} else {
		for _, r := range h.OrphanRelations {
			fmt.Fprintf(c.Stdout, "orphan relations: %s %s %s\n", r.From, r.Relation, r.To)
		}
	}
}

// reportDoctorRetrieval prints miss-rate honesty plus index freshness:
// the hybrid-search fallback must never surprise. Pure renderer split.
func (c CLI) reportDoctorRetrieval() {
	// Miss rate (A3): zero-hit searches log their terms; the doctor
	// reports the count plus the most-missed sets — save the fact,
	// reword the query, or reindex. A missing log reads as none.
	if count, top, merr := almanac.MissStats(c.dir()); merr != nil {
		fmt.Fprintf(c.Stdout, "misses:      unreadable — %v\n", merr)
	} else if count == 0 {
		fmt.Fprintln(c.Stdout, "misses:      none — no zero-hit searches logged")
	} else {
		quoted := make([]string, 0, len(top))
		for _, t := range top {
			quoted = append(quoted, fmt.Sprintf("%q", t))
		}
		fmt.Fprintf(c.Stdout, "misses:      %d (top: %s)\n", count, strings.Join(quoted, ", "))
	}
	// H3 (S5): doctor also surfaces index freshness with the repairing
	// command — a save after the last reindex makes hybrid search fall
	// back to lexical, and that must never surprise.
	switch state, err := almanac.IndexState(c.dir()); {
	case err != nil:
		fmt.Fprintf(c.Stdout, "index:      unreadable — %v\n", err)
	case state == "fresh":
		// M2 tripwire: the fresh line names the index format version and
		// entry count, so a format regression (or an un-reindexed v1)
		// is visible in the canonical health report.
		if ver, count, _, serr := almanac.IndexStats(c.dir()); serr == nil {
			fmt.Fprintf(c.Stdout, "index:      fresh v%d (%d entries, hybrid ranking active)\n", ver, count)
		} else {
			fmt.Fprintln(c.Stdout, "index:      fresh (hybrid ranking active)")
		}
	case state == "empty":
		fmt.Fprintln(c.Stdout, "index:      no observations yet — nothing to index")
	default: // stale | missing
		fmt.Fprintf(c.Stdout, "index:      %s — run `regulus memory reindex`\n", state)
	}
}

// reportDoctorPermissions applies the NOV-85 private-perms fix with
// notice. Never fails the report: advisory only.
func (c CLI) reportDoctorPermissions() {
	fixed, ferr := fsx.FixPermissions(c.dir())
	switch {
	case ferr == fsx.ErrWindowsACL:
		fmt.Fprintln(c.Stdout, "permissions: Windows ACLs rule here; 0700/0600 unenforceable — use a per-user profile and encrypted disk")
	case ferr != nil:
		fmt.Fprintf(c.Stderr, "warning: permissions fix failed: %v\n", ferr)
	case len(fixed) > 0:
		fmt.Fprintf(c.Stdout, "permissions: fixed %d path(s) to 0700/0600\n", len(fixed))
	default:
		fmt.Fprintln(c.Stdout, "permissions: .regulus already private")
	}
}

// memoryDoctorCmd implements `regulus memory doctor`: the Almanac
// health report — counts by type, oldest/newest, malformed lines, and
// provenance events (pins, relations) pointing outside the log.
func (c CLI) memoryDoctorCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory doctor")
		return 2
	}
	h, err := almanac.Doctor(c.dir())
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory doctor: %v\n", err)
		return 1
	}
	c.reportDoctorBasics(h, time.Now().UTC())
	c.reportDoctorIntegrity(h)
	c.reportDoctorRetrieval()
	// NOV-85: existing installs get private perms fixed on doctor runs,
	// with notice. Never fails the report: advisory only.
	c.reportDoctorPermissions()
	return 0
}

// memoryReindexCmd implements `regulus memory reindex [--embedder <id>]`:
// rebuild the derived retrieval.bin as a pure function of
// observations.jsonl. The header pins {embedder, dim, version} and stdout
// names the encoder, so evidence receipts pin which encoder produced
// the retrieval.
func (c CLI) memoryReindexCmd(args []string) int {
	fs := flag.NewFlagSet("memory reindex", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	embedder := fs.String("embedder", "", "encoder id (default "+almanac.EmbedderID+"; only it ships — ONNX deferred)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory reindex [--embedder <id>]")
		return 2
	}
	if _, err := almanac.LookupEmbedder(*embedder); err != nil {
		fmt.Fprintf(c.Stderr, "memory reindex: %v\nusage: regulus memory reindex [--embedder <id>]\n", err)
		return 2
	}
	id := *embedder
	if id == "" {
		id = almanac.EmbedderID
	}
	count, sha, err := almanac.ReindexWith(c.dir(), *embedder)
	if errors.Is(err, almanac.ErrNoLog) {
		fmt.Fprintln(c.Stderr, "memory reindex: no observations log yet — save an observation first")
		return 1
	}
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory reindex: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "reindexed: %d observation(s)\nembedder: %s\nsha256: %s\n", count, id, sha)
	return 0
}

// memoryPruneCmd implements `regulus memory prune [--older-than-days N]
// [--type t1,t2] [--yes]`: drop stale observations. Active lessons and
// pinned observations are never pruned (safety invariants in
// almanac.Prune); retired lessons (superseded, invalidated, rejected
// or expired) prune like any other content when unpinned. Without --yes it is a dry run: lists
// what WOULD be removed and removes nothing — the human confirmation
// is the explicit flag. A successful --yes apply refreshes the derived
// retrieval.bin best-effort (M1 S1: doctor stays fresh without a manual
// reindex); a rebuild failure warns on stderr and never fails the prune.
func (c CLI) memoryPruneCmd(args []string) int {
	fs := flag.NewFlagSet("memory prune", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	olderThan := fs.Int("older-than-days", 90, "prune observations older than N days")
	types := fs.String("type", "", "only these observation types, comma-separated (active lessons are never pruned)")
	yes := fs.Bool("yes", false, "execute the prune (default is a dry run)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *olderThan < 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory prune [--older-than-days N] [--type t1,t2] [--yes]")
		return 2
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -*olderThan)
	var filter map[string]bool
	if *types != "" {
		filter = map[string]bool{}
		for _, t := range splitList(*types) {
			filter[t] = true
		}
	}
	removed, err := almanac.Prune(c.dir(), func(o almanac.Observation) bool {
		if filter != nil && !filter[o.Type] {
			return true
		}
		return !o.SavedAt.Before(cutoff)
	})
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory prune: %v\n", err)
		return 1
	}
	if len(removed) == 0 {
		fmt.Fprintln(c.Stdout, "nothing to prune")
		return 0
	}
	for _, o := range removed {
		fmt.Fprintf(c.Stdout, "%s %s %s\n", o.ID, o.Type, o.Title)
	}
	if !*yes {
		fmt.Fprintln(c.Stdout, "dry run: re-run with --yes to remove the above")
		return 0
	}
	if err := almanac.ApplyPrune(c.dir(), removed); err != nil {
		fmt.Fprintf(c.Stderr, "memory prune: %v\n", err)
		return 1
	}
	c.refreshIndexBestEffort("prune")
	fmt.Fprintf(c.Stdout, "pruned %d observation(s)\n", len(removed))
	return 0
}

// refreshIndexBestEffort rebuilds retrieval.bin after a mutating CLI
// command (M1 S1: note/save/import/prune --yes leave the index fresh).
// The index is a pure derived function of observations.jsonl — cheap
// and byte-deterministic — so the refresh is best-effort: success is
// silent (stdout stays machine-stable), ErrNoLog (empty log, nothing
// to index) is silent, and any other failure warns on stderr without
// failing the mutation that already succeeded.
//
// T4 (S2-S3) deferred bursts: REGULUS_DEFER_REINDEX=1 skips the
// synchronous rebuild. The log append already stales the index (sha
// mismatch), so the next `memory search` honestly degrades to lexical
// with the stale-index warning; the caller reindexes once with an
// explicit `memory reindex` (or the next non-deferred write). Unset
// (default) keeps the synchronous rebuild — backwards compatible.
func (c CLI) refreshIndexBestEffort(op string) {
	if os.Getenv("REGULUS_DEFER_REINDEX") == "1" {
		return
	}
	if _, _, err := almanac.Reindex(c.dir()); err != nil {
		if errors.Is(err, almanac.ErrNoLog) {
			return
		}
		fmt.Fprintf(c.Stderr, "warning: auto-reindex after %s failed (%v) — run `regulus memory reindex`\n", op, err)
	}
}

// memoryNoteCmd implements `regulus memory note <text>`.
