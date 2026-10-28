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

Run the layered quality gauntlet for a task: gates in ascending layer order, stop at first failure, one append-only receipt per executed gate. Returns per-layer verdicts and the summary.

## spec_check

Validate the acceptance-criteria contract of a feature document (## Acceptance section: scenarios need Given/When/Then). Returns 'spec OK: N scenario(s)' or every violation with line numbers.

## doctor

Verify the OpenCode runtime and the regulus install (V2-only version gate).

## memory_note

Append one durable session note (survives restarts and compaction).

## memory_show

List the session notes, oldest first.

## memory_save

Save a typed observation to the Almanac (decision, discovery or preference) with provenance. Lessons are refused here: they enter through regulus reflect (evidence-gated).

## memory_search

Search the Almanac deterministically: optional buckets (topic key, domain, type) then exact term matching — every term must match; newest first with age annotations.

## memory_context

The deterministic Almanac resume block: charts present, latest session notes, admitted lessons. The starting point after any restart.

## evolve_propose

Assemble the evidence pack for a task (session traces, receipts, domain INDEX, decisions) plus the candidate template — the input for @regulus-proposer.

## probe_contrast

Run one gate twice with two command variants and return the receipt diff as evidence (cap-enforced; both runs executed is success).

## trace_show

Print a session's tool-execution fingerprint (sequence, tool, status) — the diffable process trace.

## skills_list

List the registered skills with their one-line triggers.

## trace_sessions

List the session ids with tool-execution traces on disk, oldest first.

## trace_files

List the distinct touched paths of one session trace, in call order.

## probe_compose

Run one gate three times (base plus two variants) and merge non-conflicting changes, flagging conflicts — behavioral contrast as evidence.
