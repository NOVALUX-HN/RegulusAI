# Regulus tool guidance — editable tool descriptions (harness surface)

One `## <name>` section per tool plus `## regulus` for the namespace.
The plugin feeds these descriptions to OpenCode instead of the compiled
defaults, so Evolution can refine tool guidance through
`regulus surface propose/apply` with guardian validation, under the
evolution budget cap. A missing section — or a missing file — falls back
to the compiled default. Keep each description to one line.

## regulus

Regulus AI harness: gauntlet gates, spec contract, doctor, session memory

## gate_run

Run the gauntlet for a task: layered gates, stop at first failure, one receipt per gate. Returns per-layer verdicts.

## spec_check

Validate a feature document's acceptance contract (Given/When/Then scenarios). Returns 'spec OK: N' or violations with line numbers.

## doctor

Verify the OpenCode V2 runtime and the regulus install.

## memory_note

Append one durable session note (survives restarts).

## memory_show

List the session notes, oldest first.

## memory_save

Save a typed observation (decision, discovery, preference) with provenance. Lessons are refused: they enter via regulus reflect.

## memory_search

Search the Almanac: optional buckets (topic, domain, type), exact term matching, newest first with age annotations.

## memory_context

Resume block: charts present, latest notes, admitted lessons. Start here after any restart.

## memory_history

List one topic key's observations in saved order — its revision chain.

## evolve_propose

Assemble a task's evidence pack (traces, receipts, INDEX, decisions) plus candidate template for @regulus-proposer.

## probe_contrast

Run one gate twice with two variants; return the receipt diff (cap-enforced).

## trace_show

Print a session's tool-execution fingerprint: sequence, tool, status.

## skills_list

List registered skills with one-line triggers.

## trace_sessions

List session ids with tool traces, oldest first.

## trace_files

List one session trace's distinct touched paths, in order.

## probe_compose

Run one gate three times (base + two variants); merge cleanly or flag conflicts.
