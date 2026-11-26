// `regulus knowledge ...` — the write surface for F18.2's corroboration and
// revalidation.
//
// Both functions mutate a record, and both need the mutation to survive a
// restart. Amend records it as a new revision that supersedes the old one, so the
// log stays append-only and the history stays readable. Without this command the
// four lifecycle functions had no caller outside their tests.
package cmdx

import (
	"flag"
	"fmt"
	"time"

	"regulus-ai/internal/almanac"
)

func knowledgeUsage() string {
	return "usage: regulus knowledge corroborate --id <obs_id> --source <actor> [--by <actor>] | regulus knowledge revalidate --id <obs_id> --by <actor> --ttl <duration>"
}

// KnowledgeCmd routes `regulus knowledge ...`.
func (io IO) KnowledgeCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, knowledgeUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, knowledgeUsage())
		return 2
	}
	switch args[0] {
	case "corroborate":
		return io.KnowledgeCorroborateCmd(args[1:])
	case "revalidate":
		return io.KnowledgeRevalidateCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "knowledge: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, knowledgeUsage())
		return 2
	}
}

// KnowledgeCorroborateCmd implements `regulus knowledge corroborate`.
func (io IO) KnowledgeCorroborateCmd(args []string) int {
	fs := flag.NewFlagSet("knowledge corroborate", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "observation id to corroborate (required)")
	source := fs.String("source", "", "independent source: actor or host that saw the same claim (required)")
	by := fs.String("by", "operator", "actor recording the amendment")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || *source == "" {
		fmt.Fprintln(io.Stderr, knowledgeUsage())
		return 2
	}
	now := time.Now().UTC()
	saved, err := almanac.Amend(io.Dir, *id, func(o *almanac.Observation) error {
		return almanac.Corroborate(o, *source)
	}, *by, now)
	if err != nil {
		fmt.Fprintf(io.Stderr, "knowledge corroborate: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "knowledge: %s amended as %s (corroborations=%d confidence=%s)\n",
		*id, saved.ID, len(saved.Corroborations), almanac.DerivedConfidence(saved))
	return 0
}

// KnowledgeRevalidateCmd implements `regulus knowledge revalidate`.
func (io IO) KnowledgeRevalidateCmd(args []string) int {
	fs := flag.NewFlagSet("knowledge revalidate", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "observation id to revalidate (required)")
	by := fs.String("by", "", "actor that checked the claim again (required)")
	ttl := fs.Duration("ttl", 0, "freshness window from now, e.g. 168h (required, positive)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || *by == "" || *ttl <= 0 {
		fmt.Fprintln(io.Stderr, knowledgeUsage())
		return 2
	}
	now := time.Now().UTC()
	saved, err := almanac.Amend(io.Dir, *id, func(o *almanac.Observation) error {
		return almanac.Revalidate(o, *by, now, *ttl)
	}, *by, now)
	if err != nil {
		fmt.Fprintf(io.Stderr, "knowledge revalidate: %v\n", err)
		return 1
	}
	expires := "none"
	if saved.ExpiresAt != nil {
		expires = saved.ExpiresAt.UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(io.Stdout, "knowledge: %s revalidated by %s as %s (expires %s)\n",
		*id, *by, saved.ID, expires)
	return 0
}
