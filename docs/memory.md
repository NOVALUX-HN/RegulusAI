# Regulus Almanac — Memory design

> Working name: **Almanac** — a star almanac records the precise positions of
> stars over time so navigators can return to them. Regulus is a star; its
> memory is an almanac.
>
> Status: design (feature spec S7). F1 ships the minimal session-notes slice;
> F3 ships the full system. Engram (Gentle-AI's memory) is the base model —
> studied and deliberately improved, not copied.

## 1. What Engram gets right (kept)

- **Automatic capture**: the agent saves decisions, discoveries and bugfixes
  as they happen; nobody manages memory by hand.
- **Project identity from the git remote** (normalized, lowercase) rather
  than the working directory — kills the "same project under three names"
  failure mode. Outside a repo: directory name fallback.
- **Topic keys** for evolving topics; session summaries; passive capture
  from conversation endings.
- **Team sharing** by exporting memories into the repo (`.engram/` +
  `sync --import`) — memory travels with the clone, onboarding inherits the
  team's accumulated context.
- A **review workflow** (list / mark reviewed) and **conflict relations**
  (related / supersedes / conflicts_with) for curating what accumulates.

## 2. Seven improvements — why the Almanac is not Engram

| # | Engram weakness | Almanac improvement |
|---|---|---|
| 1 | Observations are free-text claims — nothing records the conditions under which they were true | **Provenance is first-class**: every save records the task id, git HEAD, referenced receipts and affected paths at save time |
| 2 | `topic_key` upserts silently replace content — the previous version is lost | **Append-only revision chains**: a topic key groups entries; a new version appends, never overwrites; `memory history <key>` shows the whole chain |
| 3 | Recall is keyword search over one store, precision by luck | **Deterministic retrieval by construction, hybrid ranking vigente**: project (git remote) → topic key / domain bucket → exact match inside the bucket (lexical channel) fused with the vector channel via RRF (k=60) + recency decay + confidence boost (`hash-ngram-v1` encoder, derived `retrieval.bin` pinning the log sha256); stale or missing index degrades to lexical with a warning, `--lexical` forces it; `note` / `save` / `import` / `prune --yes` auto-rebuild the index (M1), so `doctor` reports `fresh` without a manual `reindex` |
| 4 | No staleness awareness — old advice misleads silently | **Staleness annotations**: hits report their age and, when cheaply computable, the git distance ("saved 214 commits ago; cited paths may have changed since") |
| 5 | Two memory systems for the same agent: session continuity (Engram) and harness lessons (Harness² Library R) | **One substrate, typed views**: every entry is an observation; `lesson` observations carry cue/lesson/evidence/confidence/domain and render into per-domain INDEX.md views — Harness²'s library structure, made persistent and unified with session memory |
| 6 | Stored text is injected into prompts as if it were instructions — an injection vector | **Evidence is data, not instructions** (Harness²'s invariant): retrieval renders entries inside a clearly delimited DATA block; nothing retrieved is executed or followed |
| 7 | Write discipline lives in prose (system prompts) and is only as good as the model's obedience | **Admission enforced in code**: the CLI refuses a lesson without evidence references (receipts); guardian validation gates what reaches an INDEX view |

## 3. Storage (per project, gitignored runtime state)

```
.regulus/
├── almanac/
      observations.jsonl    append-only log, one JSON object per line — THE
                           substrate of every observation type, including
                           session notes and lessons (almanac.go: File)
      pins.jsonl          pinned observation ids (pins survive prune)
      relations.jsonl     related / supersedes / conflicts_with edges
      retrieval.bin       DERIVED semantic index v2, header {magic, version=2,
                          embedder, dim, count, sha256-of-source} + count ×
                          ordered records (id + file offset + length +
                          topic/domain/type/dimension + 384 × int8); a pure
                          function of the JSONL, rebuilt by
                          `regulus memory reindex` (vindex.go: IndexPath) —
                          never edited by hand, never trusted on header
                          mismatch (reindex, never read); readers seek only
                          candidate lines (bounded) and verify exacto
└── library/<domain>/    (F4) rendered lesson views (library.go: Dir)
            INDEX.md            domain digest, rebuilt by RebuildIndex
            lessons/<id>/LESSON.md     admitted lessons (evidence-gated)
            candidates/<id>/LESSON.md  proposed, awaiting guardian validation
            harnesses/<task-slug>/tree/ + EXPERIENCE.md
                          adopted harness snapshot + why-note
```

Team sharing (F3): `regulus memory export` renders a shareable bundle from
the log — Engram's `sync` pattern, but as rendered views of one append-only
log instead of a second mutable copy of the data: `.regulus-export/
almanac.jsonl` (verbatim copy of the log) + `.regulus-export/pins.jsonl`,
`relations.jsonl` and `status.jsonl` (verbatim copies of the pin, relation
and governance stores; empty files when a store has nothing yet) +
`.regulus-export/INDEX.md` (a rendered digest grouped by type, with pinned
and relations sections). The default bundle is a generated artifact
(gitignored) — share it as a CI artifact or a release attachment, not as
a commit.

Portable domain bundles (M3): `regulus memory export --domain <d>`
scopes the same bundle shape to one domain's observations (pins,
relations and status follow the surviving ids); `--out <dir>` relocates
the bundle — point it at a tracked path and the bundle is committable, so
memory travels with the clone like Engram's `.engram/`. Closing a unit
also auto-saves one session observation (`task close` records id, spec
and receipts under the `task-close/<chart>/<id>` topic, skipping when the
topic already exists or the content duplicates pinned or
lesson-equivalent knowledge) — the habit is automatic, and domain bundles
carry those notes wherever the chart declares a `Domain:` line.

Cloud round-trip: on machine A run `regulus memory export`, copy the
`.regulus-export/` directory to the clean tree on machine B, and run
`regulus memory import .regulus-export` there — observations, pins,
relations and status restore idempotently by id (re-importing skips what
is already present; events pointing at unknown observations skip with a
warning), and the index is fresh without a manual reindex (the CLI
reindexes automatically after every mutating command). Domain bundles
round-trip the same way: `regulus memory export --domain go --out
docs/memory-bundle/go`, commit that directory, and `regulus memory
import docs/memory-bundle/go` on the other machine merges it without
duplicates.

## 4. Observation schema (F3)

```json
{
  "id": "obs_...",
  "type": "session-note | decision | discovery | lesson | preference",
  "title": "...",
  "content": "...",
  "topic_key": "optional group for evolving topics",
  "domain": "optional Harness² domain (lessons)",
  "lesson": {
    "cue": "when this pattern appears",
    "lesson": "what to do",
    "evidence": [".regulus/evidence/<task>/<receipt>.json"],
    "confidence": "low | medium | high",
    "dimension": "inv (how the agent works) | dom (what a practitioner knows); optional, unset for legacy lessons"
  },
  "provenance": {
    "task": "regulus-ai-v1/T2",
    "git_head": "9e17fb0...",
    "paths": ["internal/gates/gates.go"],
    "receipts": [".regulus/evidence/<task>/<receipt>.json"]
  },
  "saved_at": "RFC3339"
}
```

## 5. CLI surface

| Phase | Command | What |
|---|---|---|
| F1 | `regulus memory note <text>` | append one timestamped session note |
| F1 | `regulus memory show` | print the session notes (what the compaction hook injects) |
| F3 | `regulus memory save` | full observation with type validation and provenance |
| F3 | `regulus memory search [terms] [--topic-key] [--domain] [--dimension inv\|dom] [--path <f>] [--limit N] [--any] [--latest] [--include-superseded] [--lexical] [--embedder <id>] [--no-staleness]` | hybrid retrieval: deterministic bucket-then-match (lexical channel) fused with the vector channel via RRF (k=60) + recency decay + confidence boost; stale/missing index degrades to lexical with a warning, `--lexical` forces it; `--embedder` selects the encoder (default `hash-ngram-v1`); staleness annotations (N hits, 1 rev-list probe via process cache) skipped entirely with `--no-staleness`; the index is auto-rebuilt after `note` / `save` / `import` / `prune --yes` (M1), manual `reindex` only for log edits behind the CLI |
| F3 | `regulus memory prune [--older-than-days N] [--type ...] [--yes]` | drop stale non-lesson observations (dry run by default; lessons + pins immune) |
| F3 | `regulus memory context [--limit N] [--task T] [--path P] [--system S] [--budget N] [--no-staleness] [--resolve-domain [--session S] [--chart C] [--lessons N]]` | deterministic resume block: project identity, charts, session notes (latest 20 by default, or latest N with `--limit`, all with `--limit 0`), lessons with cue+content, decisions/discoveries/preferences (20 per list by default, all with `--budget 0`); `--task` / `--path` / `--system` filter by provenance/system, `--budget` caps long lists (pins always surface, every cap named with its repair); `--resolve-domain` detects the lesson domain in-process and scopes lessons in 1 spawn (context hook path); `--no-staleness` skips git probes |
| F4 | `regulus reflect --domain <d> --task <t> --cue <c> --lesson <l> [--dimension inv\|dom] [--confidence] <feature-doc>` | evidence-gated lesson candidate (dimension defaults to unset) |
| F3 | `regulus memory history --topic-key <key>` | revision chain of one evolving topic |
| F3 | `regulus memory export [--domain <d>] [--out <dir>]` | render the portable bundle from the log (`.regulus-export/almanac.jsonl` + `pins.jsonl` + `relations.jsonl` + `status.jsonl` + `INDEX.md`); `--domain` scopes it to one domain, `--out` relocates it — a tracked `--out` is the committable bundle (import merges it without duplicates) |
| M3 | `regulus task close <chart> <id>` (auto-save side effect) | closing a unit also saves one session observation (id, spec, receipts) under `task-close/<chart>/<id>`; skips when the topic already exists or the content duplicates pinned/lesson-equivalent knowledge (never changes the close verdict) |
| F3 | `regulus memory import <file\|dir>` | restore a bundle: a single JSONL file restores observations; a bundle directory restores observations + pins + relations + status (idempotent by id; orphan events skip with a warning; old bundles without the extra files import like the file form) |
| F3 | `regulus memory pin|unpin <id>` | pin an observation (pins survive prune, win chain collapse) |
| F3 | `regulus memory relate --id <a> --relation <r> --to <b>` / `--suggest <id>` | record a relation edge; `--suggest` prints top-5 vector candidates above the 0.5 floor (writes nothing, self-heals a stale index) |
| F3 | `regulus memory doctor` | health report: counts, age, malformed lines, orphan pins/relations, index freshness (`fresh` = hybrid active, else reindex) |
| v2 | `regulus memory reindex [--embedder <id>]` | rebuild the derived `retrieval.bin` as a pure function of the JSONL; header pins `{embedder, dim, version}`, stdout names the encoder |
| M4 | `regulus memory mine [--domain <d>] [--apply]` | deterministic envfacts miner (zero model calls): per-domain series of repeated failures (same gate failing across receipts, same tool erroring across traces); a series failing in ≥3 sessions is mined as a domain lesson (topic `envfact/<d>/<kind>/<name>`, origin `envfacts-miner`, dimension `inv`); a mined series whose last 3 executions run clean is discarded as contradicted via an `invalidated_by` note (hides from default retrieval, resurfaces with `--include-stale`; recurrence re-mines a new revision on the same topic); dry run by default, `--apply` admits and discards (auto-rebuilds the index when it writes) |

Envfacts (M4) close the loop between gauntlet evidence and session
memory: receipts and traces already record every failure, and `mine`
folds them into facts the domain injection picks up with no plugin
change (`memory search --type lesson --domain` surfaces mined lessons
like admitted ones). Receipt series group by the chart domain of the
receipt task (the `Domain:` line, same parser as the plugin hook);
trace series group by the entry task's domain — entries without a task
predate attribution and never form domain facts (the scan counts them
as unattributed). Legacy receipts without a run id count by file path
(one receipt file is one gate execution). Nothing is ever deleted: the
log stays append-only, discards are `invalidated_by` relations
(reversible by recurrence or auditable via `--include-stale`).

## 6. Plugin integration (OpenCode V2)

- The `compaction` session hook injects the bounded `memory context`
  resume block (`--limit 20 --budget 20`: latest notes and items per
  list, pins and lessons intact, every cap named with its repair)
  inside a DATA block, so summaries preserve state without ever
  treating it as instructions. Reversal: `REGULUS_COMPACTION_FULL=1`
  restores the explicit-full resume (`--limit 0 --budget 0`).
- The `context` session hook points the agent at the Almanac resume block
  when a feature document is open.
- Lesson injection is domain-scoped: the hook resolves one domain (the
  `Domain:` line of a chart cited in recent messages, else the lone
  chart's line, else the lone library domain) and injects that domain's
  lessons via `memory search --type lesson --domain`; with no signal it
  keeps the newest-first behavior. Mined envfacts ride the same hook:
  they are domain lessons (source `core`, origin `envfacts-miner`), so a
  mined fact surfaces at session start exactly when its domain resolves.
- Agents write via the CLI — single writer, like every piece of Regulus
  state. Nothing but `regulus` mutates `.regulus/`.

## 7. Phase 5 — governed memory (systems, provenance, validity, trust)

No new stores: the JSONL log stays the single source of truth, indexes
stay derived, and the only addition is `status.jsonl` (append-only
governance transitions — states mutate, the log does not).

- **Systems** (no duplication, derived from `type`): episodic
  (session-note), semantic (discovery), decision, procedural
  (preference), evolution (lesson). Filter with
  `memory search|context --system`.
- **Provenance**: `source` (agent|human|core|import), `origin`
  (project identity at save time), plus the existing task/git_head/
  paths/receipts. `memory save --source --expires` records both;
  `memory import` forces `source=import` and keeps (or marks
  `unknown-`) origin.
- **Temporal validity**: `saved_at` is created and observed; `expires_at`
  is optional. Retrieval hides expired, rejected, invalidated and
  superseded observations unless `--include-stale` is passed
  (`--include-superseded` keeps its narrower meaning).
- **New relation kinds**: `invalidated_by` (hides like superseded),
  `derived_from`, `verified_by`, `related_to` (informational). New kinds
  are validated; unknown kinds are still refused.
- **Governance**: OBSERVED (saved) → VERIFIED (`memory verify <id>
  --receipt`, needs evidence; `--accept` promotes) or REJECTED
  (`memory reject --reason`); imports enter PROPOSED until reviewed;
  lessons admitted through reflect are ACCEPTED; SUPERSEDED is computed
  from `supersedes` edges. `memory status` shows state + trail;
  `memory check` reports live contradictions (`conflicts_with` with both
  ends active) and stale-but-active knowledge.
- **Scoped context**: `memory context [--limit N] [--task T] [--path P]
  [--system S] [--budget N]` filters by provenance/system and caps long
  lists (pins always surface); the bare block is bounded by default
  (latest 20 notes, 20 items per list) and explicit `--limit 0
  --budget 0` keeps the full store; unscoped full output is
  byte-identical to the legacy block.
- **Trust boundaries**: retrieved content is DATA — `search`, `show` and
  `history` print an untrusted-content banner on stderr (stdout stays
  machine-stable); the resume block keeps its DATA framing; imported
  knowledge stays PROPOSED and is never instructions.
