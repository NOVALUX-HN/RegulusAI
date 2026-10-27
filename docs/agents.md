# The Guardians and the Crew — how to use them

This is the human-facing guide to Regulus AI's agents. You invoke them from
the OpenCode prompt with `@name` or they run automatically through the
plugin's commands.

## The Guardians (read-only, never edit code)

Guardians **verify** — they read code, run commands, and report. They have
`edit: deny` permissions and a bash allowlist; they cannot change your code.

| Guardian | What it does | When to invoke |
|---|---|---|
| `@regulus-gate-verifier` | Runs spec check + the full gauntlet + evidence audit; reports PROVEN or NOT COVERED per acceptance scenario | After every work unit, or when you want proof a feature is done |
| `@regulus-spec-keeper` | Judges acceptance criteria quality: externally observable? not mirroring the implementation? | Before implementation, when you want the spec vetted |
| `@regulus-mutation-auditor` | Runs gremlins mutation testing; reports KILLED/LIVED/NOT COVERED and what a surviving mutant means | After the gauntlet passes, to prove tests catch defects |
| `@regulus-crap-analyst` | Runs the CRAP gate; for each violation names the cure (COVER / SPLIT) | When the CRAP gate fails and you need to pay the debt |
| `@regulus-evidence-auditor` | Audits claims against receipts: every "done" must cite rec_ ids that exist and pass | When you suspect a claim is unbacked |
| `@regulus-reviewer` | The senior read: reviews the code the change touches (not just the diff); names what to cut (Necessity Ladder); four-line findings | After implementation, before the gauntlet, when you want the senior-dev eye |

**The one-command dispatch**: `/regulus-verify` runs the full gate-verifier
protocol in one step.

## The Crew (can edit, bounded by a Chart)

The Crew **works** — they take one bounded unit against a Chart and hand it
back with receipts.

| Crew | What it does | When to invoke |
|---|---|---|
| `@regulus-scout` | Read-only exploration with `file:line` evidence; reports what was NOT explored | Before writing, when you need a map |
| `@regulus-worker` | One bounded unit against a Chart; edits, runs the gauntlet, returns receipts | When a unit is parallelizable or context must stay thin |
| `@regulus-verifier` | Independent reproduction: reproduces the failure, then verifies the fix removes exactly that | High-risk changes; when a mistake would be hard to detect |

## Plugin commands

| Command | What it runs |
|---|---|
| `/regulus-doctor` | Verifies the OpenCode V2 runtime and the regulus binary |
| `/regulus-gauntlet` | Runs the gauntlet for the current task |
| `/regulus-verify` | The full guardian protocol: spec + gauntlet + audit + verdict |

## Plugin tools (visible to the model as `tools.regulus.*`)

| Tool | What it does |
|---|---|
| `gate_run` | Runs the layered gauntlet and writes receipts |
| `spec_check` | Validates a Chart's acceptance contract |
| `doctor` | Verifies the runtime |
| `memory_note` | Appends a durable session note |
| `memory_show` | Lists session notes |
| `memory_save` | Saves a typed observation with provenance |
| `memory_search` | Deterministic retrieval with age + git-distance annotations |
| `memory_context` | The deterministic resume block |
