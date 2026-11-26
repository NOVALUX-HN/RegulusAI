# Knowledge model and lifecycle audit — F18.1 (NOV-142)

Baseline: `develop`, audit performed at `HEAD 136cc28`, clean tree. Audit-only
artifact: every claim cites code read in this checkout.

Scope reminder from the parent: no second memory store, no promoting a single run
to a universal rule, no executing stored commands, no granting permissions.

## 1. What the record can express today

`internal/almanac/almanac.go:77` `Observation` carries: `ID`, `Type`, `Title`,
`Content`, `TopicKey`, `Domain`, `Lesson`, `Provenance`, `Source`, `Origin`,
`Author`, `Version`, `Scope`, `Confidence`, `Evidence`, `ExpiresAt`, `SavedAt`.

Everything a record can say about *where it applies* is `Scope`, `Domain`,
`Provenance` (task, git head, paths, receipts) and `Lesson.Dimension`
(`inv`/`dom`, `internal/almanac/almanac.go:39-47`). **There is no host and no
environment field** — verified against the struct, not inferred. Retrieval is
broader than the scope struct suggests: `Query`
(`internal/almanac/almanac.go:431-438`) already buckets on topic, domain, type,
dimension, path and system, and filters Dimension at `:511`.

## 2. What already works (do not rebuild)

| Concern | Location | Behaviour |
|---|---|---|
| Secret redaction | `internal/almanac/redact.go`, choke at `almanac.go:239` | every `Save` and `Import` passes through `RedactObservation` |
| Idempotency | `internal/almanac/dedup.go:99` | `contentHash` sha256 over type/title/content/topic |
| Near-duplicate | `internal/almanac/dedup.go:108` | vector bar `DedupThreshold 0.92` inside `withStoreLock` |
| Concurrency | `internal/almanac/store_lock.go` | `O_EXCL` lockfile with stale-steal; second writer tested |
| Backup integrity | `internal/almanac/backup.go` | manifest sha256+size, `unsupported bundle vN`, `MANIFEST mismatch` |
| Ranking | `internal/almanac/almanac.go:699` | pinned > confidence (high/med/low) > recency > ID |
| Retirement | `internal/almanac/almanac.go:450` | `SupersededIDs` + `staleIDs` (invalidated, REJECTED, expired) hidden by default |
| Contradictions | `internal/almanac/learn.go:441` | `ResolveContradiction` needs an actor and a live `conflicts_with` |
| Import distrust | `internal/almanac/import.go` | forces `Source=import` / `PROPOSED` |
| Cross-agent learning | `internal/almanac/learn.go:48` | kinds + scopes + `CanRead` ACL + `memory:read` |

## 3. Gap G1 — evidence is counted once, never corroborated

Promotion today rests on three fixed rules:

* `internal/envfacts/envfacts.go:31` `MinSessions = 3` — a failure repeating in
  three distinct sessions may become a lesson;
* `internal/reflect/reflect.go:43` requires at least one passing receipt;
* `internal/almanac/learn.go:100-124` requires a verified-solution to carry
  evidence, an author, a scope and a confidence.

None of them can express: how many independent sources corroborate a claim, how
many distinct actors or hosts saw it, or a confidence derived from that count.
`Confidence` is assigned by the caller (`envfacts/admit.go:249` fixes
`medium`), so a claim seen once and a claim seen twenty times look the same
unless the author wrote the difference into the record.

F18.2 must add corroboration as a measured property, not a free-text field.

## 4. Gap G2 — freshness is binary, and scope has no host or environment

`IsExpired` (`almanac.go:145`) plus the prune-GC can remove an expired record,
and `Search` hides stale ones by default. What does not exist is
**revalidation**: nothing can mark a claim as "still true as of this run", so an
expired claim is discarded rather than refreshed, and a claim that is still
correct must be re-saved as a new observation.

Scope-wise, the context builder accepts `Task/Path/System/Budget`
(`internal/almanac/context.go:72-77`, matched at `:83-95`) and records carry
`Scope`/`Domain`/`Lesson.Dimension`. A host or environment dimension — the one
that matters when the same lesson is true on Linux and false on Windows — has
nowhere to live. This is the same class of defect F16 found in the capability
matrix, in a different subsystem.

## 5. Gap G3 — measurement and isolation are unproven

Three distinct absences, all verifiable:

1. **Contextual coexistence has no state.** `ResolveContradiction` forces a
   winner and a loser (`learn.go:441-463`), so two claims that are both true in
   different contexts can only be recorded by writing the context into the text.
   There is no field or status that says "A holds here, B holds there". A
   contradiction *window* does exist, but in other domains: `IsContradicted` and
   `ContradictionWindow` (`internal/envfacts/envfacts.go:129-140`) decide when
   mined facts disagree, and `internal/evolve/tick.go:238-244` treats rot as a
   contradiction. Neither is a coexistence state for stored knowledge.
2. **Usefulness is not measured.** `ComputeLearnMetrics`
   (`learn.go:654`) reports precision, useful-recall, obsolescence and noise, and
   `misses.go` records retrieval misses. There is **no** measurement of
   retrieval → reuse → success, and no error/retry reduction against a baseline.
3. **Isolation is by construction and untested.** `CanRead` gates project/task
   knowledge behind `memory:read`, and the store is rooted per project, so a
   cross-project read has no primitive. But there is **no adversarial test** that
   tries to contaminate one project from another, or to smuggle a secret through
   an import and read it back. Isolation that is only argued is isolation that
   can regress.

## 6. What each follow-up may do

| Task | May do | Evidence it must produce |
|---|---|---|
| F18.2 (NOV-143) | Corroboration count and derived confidence; revalidation instead of discard; host/environment as a scope dimension; explicit coexistence state | Tests proving one observation cannot reach a threshold alone, and that a revalidated claim stays retrieval-eligible |
| F18.3 (NOV-144) | Scope-aware retrieval including host/environment; measure reuse and success/error-retry effect against a baseline | A measured comparison, with the baseline named, not a claim of improvement |
| F18.4 (NOV-145) | Adversarial fixtures for secrets, injection, cross-project contamination, external data, stored commands; validate reject/deactivate/rollback | Tests that fail if the guard is removed |

## 7. Explicit non-gaps (do not spend effort here)

* Redaction at the save and import choke — present and tested.
* `contentHash` idempotency and the dedup bar — correct. Note the 0.92 default is
  overridable through `REGULUS_DEDUP_THRESHOLD` (`dedup.go:39-50`), so it is a
  default, not a constant.
* The store lock and second-writer refusal — tested.
* Backup manifest verification — correct.
* Ranking order and stale-by-default hiding — correct.
* `Source=import`/`PROPOSED` distrust on import — correct, and the right default.

## 8. Machinery that exists elsewhere — F18.2 must not rebuild it

The audit's first pass missed these; the independent review found them. None of
them satisfies F18.2, but each is adjacent enough that duplicating it would be a
defect:

| Mechanism | Location | Why it is not the F18 answer |
|---|---|---|
| `SupportOf` | `internal/almanac/support.go:22` | counts how many receipts cite an observation — evidence linkage, not corroboration across independent sources |
| `CitedRot` | `internal/evidence/verify.go:329` | flags evidence that rotted, at verification time, not knowledge that needs revalidation |
| `lessonEvidence` | `internal/evolve/report.go:66-76` | reports the evidence behind an evolution lesson |
| keep/drop validation | `internal/evolve/validate.go:7-11` | experiment verdicts |
| rot-as-contradiction | `internal/evolve/tick.go:238-244` | evolution domain |
| `IsContradicted`, `ContradictionWindow` | `internal/envfacts/envfacts.go:129-140` | mined environment facts, window of 3 sessions |
| `MinSupport`, `Generalizable` | `internal/evolve/observe.go:31-49` | evolution observation bar |
| governed promotion | `internal/evolve/governed_eval.go:461-505` | promotion of harness changes, with canary and rollback |
| `RobustMinDelta`, `SignificanceFloor` | `internal/evolve/robust.go:30-39` | experiment significance, not knowledge confidence |

F18.2's corroboration and revalidation must be expressed on the Almanac record,
which is what makes a stored claim trustworthy at retrieval time.

## 9. Verification of this audit

```bash
bash scripts/verify-wsl.sh ./internal/almanac/ ./internal/envfacts/ ./internal/reflect/
```

This document changes no code: it is the deliverable F18.1 asks for ("auditar
Almanac, miners, cross-agent learning, provenance/admission/retrieval; definir
gap-only cambios de record/schema/lifecycle").
