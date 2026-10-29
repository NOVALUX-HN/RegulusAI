package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/gates"
)

// memoryCmd implements the Almanac surface (docs/memory.md):
// note|show (session notes), save|search|history (the full substrate),
// context (the deterministic resume block), prune (staleness pruning
// with human confirmation) — dispatched per subcommand.
// Lessons have one door: regulus reflect.
func (c CLI) memoryCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory note|show|save|search|history|context|prune|doctor|reindex")
		return 2
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
	default:
		fmt.Fprintf(c.Stderr, "memory: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus memory note|show|save|search|history|pin|unpin|relate|export|context|prune|doctor|reindex")
		return 2
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
	h, err := almanac.Doctor(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory doctor: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "observations: %d\n", h.Total)
	types := make([]string, 0, len(h.ByType))
	for t := range h.ByType {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		fmt.Fprintf(c.Stdout, "  %s: %d\n", t, h.ByType[t])
	}
	now := time.Now().UTC()
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
	return 0
}

// memoryReindexCmd implements `regulus memory reindex`: rebuild the
// derived retrieval.bin as a pure function of observations.jsonl.
func (c CLI) memoryReindexCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory reindex")
		return 2
	}
	count, sha, err := almanac.Reindex(".")
	if errors.Is(err, almanac.ErrNoLog) {
		fmt.Fprintln(c.Stderr, "memory reindex: no observations log yet — save an observation first")
		return 1
	}
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory reindex: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "reindexed: %d observation(s)\nsha256: %s\n", count, sha)
	return 0
}

// memoryPruneCmd implements `regulus memory prune [--older-than-days N]
// [--type t1,t2] [--yes]`: drop stale non-lesson observations.
// Lessons and pinned observations are never pruned (safety
// invariants in almanac.Prune). Without --yes it is a dry run: lists
// what WOULD be removed and removes nothing — the human confirmation
// is the explicit flag.
func (c CLI) memoryPruneCmd(args []string) int {
	fs := flag.NewFlagSet("memory prune", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	olderThan := fs.Int("older-than-days", 90, "prune observations older than N days")
	types := fs.String("type", "", "only these observation types, comma-separated (lessons are never pruned)")
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
	removed, err := almanac.Prune(".", func(o almanac.Observation) bool {
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
	if err := almanac.ApplyPrune(".", removed); err != nil {
		fmt.Fprintf(c.Stderr, "memory prune: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "pruned %d observation(s)\n", len(removed))
	return 0
}

// memoryNoteCmd implements `regulus memory note <text>`.
func (c CLI) memoryNoteCmd(args []string) int {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus memory note <text>")
		return 2
	}
	title := text
	if runes := []rune(title); len(runes) > 60 {
		title = string(runes[:60])
	}
	head := gates.GitHead(context.Background(), c.runner())
	obs := &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: title, Content: text,
		Provenance: &almanac.Provenance{GitHead: head},
	}
	if err := almanac.CheckDuplicate(".", obs); err != nil {
		fmt.Fprintf(c.Stderr, "memory note: %v\n", err)
		return 1
	}
	if _, err := almanac.Save(".", obs, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory note: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stdout, "memory: note saved")
	return 0
}

// memoryShowCmd implements `regulus memory show`.
func (c CLI) memoryShowCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory show")
		return 2
	}
	notes, err := almanac.Notes(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory show: %v\n", err)
		return 1
	}
	if len(notes) == 0 {
		fmt.Fprintln(c.Stdout, "(no session notes)")
		return 0
	}
	for _, n := range notes {
		fmt.Fprintf(c.Stdout, "- %s %s\n", n.SavedAt.Format(time.RFC3339), n.Content)
	}
	return 0
}

// memorySaveCmd implements `regulus memory save --type --title [...]`.
// Lesson-type saves are refused: lessons enter through reflect.
func (c CLI) memorySaveCmd(args []string) int {
	fs := flag.NewFlagSet("memory save", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	typ := fs.String("type", "", "decision|discovery|preference|session-note (required)")
	title := fs.String("title", "", "short, searchable title (required)")
	topicKey := fs.String("topic-key", "", "group for evolving topics (revision chain)")
	domain := fs.String("domain", "", "domain bucket")
	task := fs.String("task", "", "task id for provenance")
	paths := fs.String("path", "", "affected paths, comma-separated")
	receiptsFlag := fs.String("receipt", "", "receipt paths for provenance, comma-separated")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	content := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if *typ == "" || *title == "" || content == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus memory save --type <t> --title <t> [--topic-key --domain --task --path <a,b> --receipt <r1,r2>] <content>")
		return 2
	}
	if *typ == almanac.TypeLesson {
		fmt.Fprintln(c.Stderr, "memory save: lessons enter through regulus reflect (evidence-gated)")
		return 1
	}
	provenancePaths := splitList(*paths)
	provenanceReceipts := splitList(*receiptsFlag)
	if err := checkProvenancePaths(provenancePaths, "--path"); err != nil {
		fmt.Fprintf(c.Stderr, "memory save: %v\n", err)
		return 2
	}
	if err := checkProvenancePaths(provenanceReceipts, "--receipt"); err != nil {
		fmt.Fprintf(c.Stderr, "memory save: %v\n", err)
		return 2
	}
	head := gates.GitHead(context.Background(), c.runner())
	obs := &almanac.Observation{
		Type: *typ, Title: *title, Content: content,
		TopicKey: *topicKey, Domain: *domain,
		Provenance: &almanac.Provenance{Task: *task, GitHead: head, Paths: provenancePaths, Receipts: provenanceReceipts},
	}
	if err := almanac.CheckDuplicate(".", obs); err != nil {
		fmt.Fprintf(c.Stderr, "memory save: %v\n", err)
		return 1
	}
	if _, err := almanac.Save(".", obs, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory save: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stdout, "memory: observation saved")
	return 0
}

// memorySearchCmd implements `regulus memory search [terms]` with
// bucket flags. Hybrid semantic ranking runs when retrieval.bin is fresh;
// --lexical forces the old path.
func (c CLI) memorySearchCmd(args []string) int {
	fs := flag.NewFlagSet("memory search", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	topicKey := fs.String("topic-key", "", "bucket: topic key")
	domain := fs.String("domain", "", "bucket: domain")
	typ := fs.String("type", "", "bucket: observation type")
	dimension := fs.String("dimension", "", "bucket: lesson dimension (inv | dom)")
	path := fs.String("path", "", "bucket: provenance path (exact match)")
	limit := fs.Int("limit", 20, "keep the first N results (0: all)")
	any := fs.Bool("any", false, "any term may match (default: all must)")
	latest := fs.Bool("latest", false, "collapse each topic chain to its best hit (default: all coexist)")
	includeSuperseded := fs.Bool("include-superseded", false, "show observations a supersedes relation hides")
	lexical := fs.Bool("lexical", false, "force lexical-only search (skip hybrid semantic ranking)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	q := almanac.Query{Terms: fs.Args(), TopicKey: *topicKey, Domain: *domain, Type: *typ, Dimension: *dimension, Path: *path, Limit: *limit, Any: *any, Latest: *latest, IncludeSuperseded: *includeSuperseded}
	var hits []almanac.Observation
	var err error
	if *lexical {
		hits, err = almanac.Search(".", q)
	} else {
		var degraded bool
		hits, degraded, err = almanac.HybridSearch(".", q)
		if err == nil && degraded {
			fmt.Fprintln(c.Stderr, "warning: semantic index stale or missing — lexical only")
		}
	}
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory search: %v\n", err)
		return 1
	}
	if len(hits) == 0 {
		fmt.Fprintln(c.Stdout, "no matching observations")
		return 0
	}
	// Relation semantics in the output: conflicts mark both sides;
	// superseded hits (only visible with --include-superseded) say
	// who superseded them.
	rels, _ := almanac.Relations(".")
	conflicts := map[string][]string{}
	supersededBy := map[string]string{}
	for _, r := range rels {
		switch r.Relation {
		case almanac.RelationConflictsWith:
			conflicts[r.From] = append(conflicts[r.From], r.To)
			conflicts[r.To] = append(conflicts[r.To], r.From)
		case almanac.RelationSupersedes:
			supersededBy[r.To] = r.From
		}
	}
	fmt.Fprintf(c.Stdout, "%d hit(s)\n", len(hits))
	now := time.Now().UTC()
	for _, h := range hits {
		suffix := c.commitsBehindSuffix(h)
		if by := supersededBy[h.ID]; by != "" {
			suffix += fmt.Sprintf(" [superseded by %s]", by)
		}
		if partners := conflicts[h.ID]; len(partners) > 0 {
			suffix += fmt.Sprintf(" [conflicts: %s]", strings.Join(partners, ", "))
		}
		fmt.Fprintf(c.Stdout, "%s %s %s (saved %s%s)\n", h.ID, h.Type, h.Title, humanAge(now, h.SavedAt), suffix)
	}
	return 0
}

// memoryHistoryCmd implements `regulus memory history --topic-key`.
func (c CLI) memoryHistoryCmd(args []string) int {
	fs := flag.NewFlagSet("memory history", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	topicKey := fs.String("topic-key", "", "the revision chain to list (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *topicKey == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus memory history --topic-key <key>")
		return 2
	}
	chain, err := almanac.History(".", *topicKey)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory history: %v\n", err)
		return 1
	}
	if len(chain) == 0 {
		fmt.Fprintln(c.Stdout, "(no observations with that topic key)")
		return 0
	}
	for _, o := range chain {
		fmt.Fprintf(c.Stdout, "%s %s %s %s\n", o.SavedAt.Format(time.RFC3339), o.ID, o.Type, o.Title)
	}
	return 0
}

// memoryPinCmd implements `regulus memory pin|unpin <id>`.
func (c CLI) memoryPinCmd(pin bool, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory pin|unpin <id>")
		return 2
	}
	if err := almanac.Pin(".", args[0], pin, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory: %v\n", err)
		return 1
	}
	if pin {
		fmt.Fprintf(c.Stdout, "pinned: %s\n", args[0])
	} else {
		fmt.Fprintf(c.Stdout, "unpinned: %s\n", args[0])
	}
	return 0
}

// memoryRelateCmd implements `regulus memory relate --id --relation --to`
// plus candidate suggestion via `regulus memory relate --suggest <id>`: the
// vector-channel top-k above the floor, printed one per line
// `id cosine title`; suggestion writes nothing.
func (c CLI) memoryRelateCmd(args []string) int {
	fs := flag.NewFlagSet("memory relate", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	from := fs.String("id", "", "the observation (required)")
	relation := fs.String("relation", "", "supersedes | conflicts_with | related (required)")
	to := fs.String("to", "", "the other observation (required)")
	suggest := fs.String("suggest", "", "suggest relation candidates for this observation id (top-5 above similarity 0.5, writes nothing)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *suggest != "" {
		if *from != "" || *relation != "" || *to != "" || fs.NArg() != 0 {
			fmt.Fprintln(c.Stderr, "usage: regulus memory relate --suggest <id>")
			return 2
		}
		return c.memoryRelateSuggest(*suggest)
	}
	if *from == "" || *relation == "" || *to == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory relate --id <a> --relation <r> --to <b> | --suggest <id>")
		return 2
	}
	if err := almanac.Relate(".", *from, *relation, *to, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory relate: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "recorded: %s %s %s\n", *from, *relation, *to)
	return 0
}

// memoryRelateSuggest prints the vector-channel candidates for id, one per
// line `id cosine title`. Unlike search there is no lexical fallback here:
// suggestions are defined as vector similarities, so a missing or stale
// index refuses with a reindex pointer instead of presenting keyword hits
// as similarities.
func (c CLI) memoryRelateSuggest(id string) int {
	sugs, err := almanac.SuggestRelated(".", id)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory relate: %v\n", err)
		return 1
	}
	if len(sugs) == 0 {
		fmt.Fprintln(c.Stdout, "no suggestions above threshold")
		return 0
	}
	for _, s := range sugs {
		fmt.Fprintf(c.Stdout, "%s %.4f %s\n", s.ID, s.Cosine, s.Title)
	}
	return 0
}

// memoryExportCmd implements `regulus memory export` — the team bundle.
func (c CLI) memoryExportCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory export")
		return 2
	}
	paths, err := almanac.Export(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory export: %v\n", err)
		return 1
	}
	for _, p := range paths {
		fmt.Fprintf(c.Stdout, "exported: %s\n", p)
	}
	fmt.Fprintln(c.Stdout, "commit .regulus-export/ to share the Almanac with your team")
	return 0
}

// memoryImportCmd implements `regulus memory import <file>`: restores
// observations from a `memory export` bundle. Idempotent by id —
// re-importing skips what is already present; the first invalid line
// aborts with its line number and imports nothing.
func (c CLI) memoryImportCmd(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory import <file>")
		return 2
	}
	imported, skipped, err := almanac.Import(".", args[0])
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory import: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "imported %d, skipped %d (already present)\n", imported, skipped)
	return 0
}

// memoryContextCmd implements `regulus memory context` — the resume
// block, DATA-framed.
func (c CLI) memoryContextCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory context")
		return 2
	}
	ctx, err := almanac.Context(".", time.Now().UTC())
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory context: %v\n", err)
		return 1
	}
	fmt.Fprint(c.Stdout, ctx)
	return 0
}

// commitsBehindSuffix renders the git staleness annotation for an
// observation with a provenance git head — empty when unknown.
func (c CLI) commitsBehindSuffix(o almanac.Observation) string {
	if o.Provenance == nil || o.Provenance.GitHead == "" {
		return ""
	}
	head := o.Provenance.GitHead
	out, _, _, err := c.runner().Run(context.Background(), "git rev-list --count "+head+"..HEAD")
	if err != nil || strings.TrimSpace(out) == "" {
		return ""
	}
	n := strings.TrimSpace(out)
	if n == "0" {
		return ""
	}
	return ", " + n + " commits behind"
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
