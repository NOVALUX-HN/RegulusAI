# memory-hybrid — índice fresco sin fricción y round-trip cloud

User authorization: "continua hasta mejorar todo lo pendiente" 2026-10-31, siguiente = memoria híbrida (tercero de la lista). Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: eliminar la fricción de memoria sin romper garantías — índice fresco tras mutar, dedup ajustable, export/import round-trip para Cloud.
- Scope: `internal/almanac/` (save/note/import/prune reindex, dedup threshold env), `internal/cli/memory.go` (mensajes), `docs/memory.md` + `docs/troubleshooting.md`. Sin tocar gates, budget/caps (funcionan por diseño), embedder (hash-ngram-v1 intacto), retrieval format.
- Constraints: reindex sigue función pura byte-idéntica; dedup default 0.92 intacto; import sigue atómico e idempotente; `suggest` self-heal intacto; tests de sims existentes verdes.
- Acceptance: S1, S2, S3
- Artifacts: receipts `memory-hybrid/M*`.
- Dependencies: M1 primero (frescura base); M2–M3 en paralelo tras M1.
- Risk: STANDARD-equivalente por unidad (aditivo, sin borrado nuevo; reindex es derivado puro). Class COMPLEX por multi-unidad.
- Verification: `spec check charts/memory-hybrid.md` + `gate run memory-hybrid/M*` + prueba round-trip en tmp.
- Completion: 3 tasks CLOSED (+ docs), `spec OK: 3`, capa 7 pass.

## Acceptance

### Scenario: S1 índice fresco tras mutar

- Given log con índice fresh
- When `memory note` / `save` / `import` / `prune --yes` muta el log
- Then `memory doctor` reporta `index: fresh` sin `reindex` manual y `memory search` no avisa stale en stderr

### Scenario: S2 dedup ajustable

- Given dos pensamientos distintos que el hash confunde (>0.92)
- When `REGULUS_DEDUP_THRESHOLD=0.99 memory save` corre (o el default sin env)
- Then con env alto guarda (respeta override validado 0..1, rechaza fuera de rango); sin env el 0.92 y el mensaje refusal intactos

### Scenario: S3 round-trip export/import

- Given Almanac con observaciones + pins + relations + status
- When `memory export` corre y en árbol limpio `memory import <bundle>` + doctor
- Then observaciones + pins + relations + status restaurados (idempotente por id) e índice fresh sin reindex manual (vía S1)

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| M1 | S1 | auto-reindex tras `Save`/`Import`/`ApplyPrune` (o en CLI tras cada mutación) + tests (stale→fresh, search sin warning) | @regulus-worker |
| M2 | S2 | env `REGULUS_DEDUP_THRESHOLD` (default 0.92, valida 0..1) en `CheckDuplicate` + tests | @regulus-worker |
| M3 | S3 | export/import de `pins`+`relations`+`status` (+ tests round-trip en tmp, idempotencia) + docs `memory.md`/troubleshooting | @regulus-worker |

## Log

- L1: Chart opened 2026-10-31. Scout `ses_e6c6f15c0ffeQW6J1zsGDQNzhI`: `Save` (`almanac.go:214-255`), `Import` (`import.go:68-87`), `ApplyPrune` (`prune.go:38-83`) nunca llaman `Reindex` (solo CLI `memory.go:165` + self-heal `relate.go:138`); `search` degrada (`hybrid.go:54-80`), `suggest` auto-reconstruye (`relate.go:133-149`); dedup 0.92 (`dedup.go:34`) solo en save/note, exime import/topic-key/superseded; caps default 10/mitad sin bloque `evolution` en `regulus.json:1-20` (por diseño, no se toca); export solo `almanac.jsonl`+digest (`export.go:49-113`), import solo observations (`import.go:18-89`).
- L2: M1–M3 CLOSED 2026-10-31 — M1 auto-reindex tras mutar (`ses_e6c6e99eeffevt1bIDGdxRYxcw`, CLI best-effort, 12/12), M2 `REGULUS_DEDUP_THRESHOLD` (`ses_e6c69c21cffenDpzvJBpiXqFr4`, default 0.92, 12/12), M3 round-trip pins+relations+status + docs (`ses_e6c63939dffe3x4rRqZwvzRDmO`, importBundle SPLIT a max 29.46, 12/12). Verificación propia `gate run memory-hybrid/M3` 12/12 (`manifest-run_0c474b75846a847d`). `spec OK: 3`. Nota: `task close` inexistente en 0.9.0, cierre manual.
