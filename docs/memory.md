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
| 3 | Recall is keyword search over one store, precision by luck | **Deterministic retrieval by construction**: project (git remote) → topic key / domain bucket → exact match inside the bucket. No embedding dependency in v1 — precision before cleverness; a reranker can plug in later without touching the contract |
| 4 | No staleness awareness — old advice misleads silently | **Staleness annotations**: hits report their age and, when cheaply computable, the git distance ("saved 214 commits ago; cited paths may have changed since") |
| 5 | Two memory systems for the same agent: session continuity (Engram) and harness lessons (Harness² Library R) | **One substrate, typed views**: every entry is an observation; `lesson` observations carry cue/lesson/evidence/confidence/domain and render into per-domain INDEX.md views — Harness²'s library structure, made persistent and unified with session memory |
| 6 | Stored text is injected into prompts as if it were instructions — an injection vector | **Evidence is data, not instructions** (Harness²'s invariant): retrieval renders entries inside a clearly delimited DATA block; nothing retrieved is executed or followed |
| 7 | Write discipline lives in prose (system prompts) and is only as good as the model's obedience | **Admission enforced in code**: the CLI refuses a lesson without evidence references (receipts); guardian validation gates what reaches an INDEX view |

## 3. Storage (per project, gitignored runtime state)

```
.regulus/
└── almanac/
      observations.jsonl    append-only log, one JSON object per line — THE
                           substrate of every observation type, including
                           session notes and lessons
      views/<domain>/            (F4) rendered lesson views
            INDEX.md
            lessons/<id>.md
```

Team sharing (F3): `regulus memory export` renders a committed bundle from
the log — Engram's `sync` pattern, but as rendered views of one append-only
log instead of a second mutable copy of the data.

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
| F3 | `regulus memory search [terms] [--topic-key] [--domain] [--dimension inv\|dom] [--path <f>]` | deterministic bucket-then-match retrieval with staleness annotations |
| F3 | `regulus memory prune [--older-than-days N] [--type ...] [--yes]` | drop stale non-lesson observations (dry run by default; lessons + pins immune) |
| F3 | `regulus memory context` | deterministic resume block: open feature document, task list, last notes |
| F4 | `regulus reflect --domain <d> --task <t> --cue <c> --lesson <l> [--dimension inv\|dom] [--confidence] <feature-doc>` | evidence-gated lesson candidate (dimension defaults to unset) |
| F3 | `regulus memory history <topic-key>` | revision chain of one evolving topic |
| F3 | `regulus memory export` | render the committed team bundle from the log |

## 6. Plugin integration (OpenCode V2)

- The `compaction` session hook injects `regulus memory show` output inside a
  DATA block, so summaries preserve state without ever treating it as
  instructions.
- The `context` session hook points the agent at the Almanac resume block
  when a feature document is open.
- Lesson injection is domain-scoped: the hook resolves one domain (the
  `Domain:` line of a chart cited in recent messages, else the lone
  chart's line, else the lone library domain) and injects that domain's
  lessons via `memory search --type lesson --domain`; with no signal it
  keeps the newest-first behavior.
- Agents write via the CLI — single writer, like every piece of Regulus
  state. Nothing but `regulus` mutates `.regulus/`.
