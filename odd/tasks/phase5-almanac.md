# Phase 5 — Almanac Engineering Memory

Header: Evolve Almanac without breaking its invariants: single source of truth
(.regulus/almanac/observations.jsonl), append-only log, derived indexes are
regenerable. No new stores except one append-only status log; no vector infra.
No Evolution changes, no plugin/MCP changes. No commits.

## Specs

S1: "Dominios conceptuales sin duplicar storage: system derivado del type
(episodic=session-note, semantic=discovery, decision=decision,
procedural=preference, evolution=lesson) + filtros `--system` en search/context."
S2: "Provenance: Source (agent|human|core|import), Origin (identidad del repo),
más lo existente (task, git_head, paths, receipts). Import fija source=import
y conserva origin."
S3: "Validez temporal: SavedAt existente (=observed_at); ExpiresAt optativo
(--expires 30d); verified_at/superseded_at derivados (status log / relations).
Retrieval excluye expirado/rechazado/superseded/invalidado salvo --include-stale."
S4: "Relaciones: supersedes, conflicts_with e related existen; se agregan
invalidated_by, derived_from, verified_by, related_to (validación aditiva).
`memory check` detecta contradicciones activas y conocimiento obsoleto vigente."
S5: "Governance: OBSERVED (save), PROPOSED (import), VERIFIED (`memory verify`
con receipt), ACCEPTED (lessons vía reflect; estado por defecto de lesson),
REJECTED (`memory reject` con razón), SUPERSEDED (computado de relations).
Transiciones en status.jsonl append-only; agentes nunca promueven directo."
S6: "Retrieval: filtros governance+temporal+system sobre el híbrido existente;
sin tocar pesos de ranking; documentar precisión/coste; sin infra vectorial nueva."
S7: "Context reconstruction: `memory context --task --path --system --budget`
con anotaciones de edad/staleness; acotado por defecto."
S8: "Trust: banner DATA/no-ejecutar en search/context/show; import entra PROPOSED;
tests fijan banner y no-passthrough."
S9: "Tests §9: persistencia (nuevos campos), retrieval (filtros), provenance,
temporalidad, supersession, contradicciones, invalidaciones, corrupción
(comportamiento definido), context reconstruction."
S10: "Aceptación 1-7: persistencia, procedencia, relaciones, obsolescencia,
contexto, gobernanza, protección anti-instrucciones."

## Tasks

- T1 (S1,S2,S3): modelo (Source/Origin/ExpiresAt) + system derivado + save/import + tests — route: inline — commit: —
- T2 (S4,S5): kinds + status log + verify/reject/check + import PROPOSED + tests — route: inline — commit: —
- T3 (S6,S7): filtros retrieval + context scoped + tests — route: inline — commit: —
- T4 (S8,S10 docs): banners + docs/memory* + ADRs + CHANGELOG + tests — route: inline — commit: —
- T5: verificación total y reporte — route: inline — commit: —

## Log

L1 (user original verbatim): "PHASE 5 -- ALMANAC ENGINEERING MEMORY MISSION [full spec S1-S10 as above] Actualiza documentación y contratos."
L2 (decision): Cero stores nuevos salvo status.jsonl (los estados mutan; el log no). SUPERSEDED computado de relations (ya excluido en retrieval). Ranking intacto: filtros, no pesos. Sin vectores nuevos: flat quantized existente basta.
L3 (2026-10-31, Phase 5 completed): T1-T5 done. Full `go test ./...` green, bun 56/56, vet+gofmt clean. Live: search hide/resurface expired, reject con trail, check exit 1 en stale-active, scoped context. Save collision fixed (Windows timer). Fixture quedo en /tmp (rm bloqueado por policy, fuera del repo, inofensivo).
