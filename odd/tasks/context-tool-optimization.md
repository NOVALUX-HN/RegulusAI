# context-tool-optimization — F19 · Context & Tool Optimization with Quality Safeguards

Linear: NOV-131 (padre), NOV-146..NOV-149 (hijas). Baseline: `develop`, nunca `main`.

Estado de arranque verificado: rama `develop`, `HEAD fb8b7ee`, árbol limpio, sincronizado con `origin/develop`.

## Specs

S1 — No reconstruir lo existente. Texto verbatim del padre: "No inventar token savings; no dynamic SDK changes unsupported; no eliminar tools por frecuencia sola; no crear parallel selector/compactor." Reutilizar NOV-39 (presupuesto de tokens y eficiencia), NOV-98 (predictive context engine y selección de contexto), toolset y context engine.

S2 — Auditar los perfiles Full/Minimal existentes y documentar los huecos de evidencia: qué cambian, qué miden y qué no.

S3 — Medir uso de tools y de contexto, redundancia, dependencia, criticidad y overhead de contexto **por tarea y por host**.

S4 — Selección profile/task-aware. Perfiles Specialized o ContextAdaptive **sólo si** un gap o benchmark lo justifica; no se crean por simetría.

S5 — Relevancia de contexto, precisión de retrieval, pérdida por compaction, recuperación de información omitida y efecto sobre el resultado de la tarea.

S6 — Preservar tools críticos de seguridad, gobernanza, validación y recovery. Regla explícita del padre: **low-frequency != expendable**.

S7 — Flujo baseline → compare → validate → promote → revert, con configuración de arranque para hosts que no permiten cambios dinámicos.

S8 — Telemetría etiquetada por procedencia: `measured` / `estimated` / `proxy` / `unknown`. Nunca presentar un estimado como medido ni inventar un ahorro.

S9 — Texto verbatim del DoD del padre: "Reproducible Full vs Minimal/profile comparison; task holdout; selection and critical-tool tests; context-quality/loss tests; capability-degraded host validation; measured savings labeled by provenance; no quality regressions; Gauntlet and independent review."

S10 — Si no hay brecha demostrada, no se inventa trabajo: se reporta y se propone cierre como audit-only.

S11 — Baseline `develop`. Distinguir host real de fixture y medición de estimación/proxy. Nunca debilitar tests o gates para aparentar éxito.

## Tasks

| ID | Linear | Specs | Ruta | Commit |
|----|--------|-------|------|--------|
| T1 | NOV-146 | S1 S2 S3 S8 S10 | inline + explore | `docs: context and tool optimization baseline (F19.1)` |
| T2 | NOV-147 | S4 S6 S11 | inline o audit-only | `feat(context): profile and task-aware selection` |
| T3 | NOV-148 | S5 S9 | inline | `test(context): quality, loss and recovery fixtures` |
| T4 | NOV-149 | S7 S8 | inline | `feat(context): measured promotion with reversibility` |

T1 — auditoría de toolsets, compaction, predictive context, token budget, telemetría y perfiles Full/Minimal; baseline por host y tarea. Read-only.

T2 — evaluación de selección Full/Minimal y de perfiles nuevos, **sólo con gap demostrado**. Si el baseline no muestra un gap, se cierra audit-only y se documenta por qué.

T3 — tests de calidad, pérdida y recuperación de contexto.

T4 — flujo de promoción medido y reversible, con etiquetas de procedencia en los tokens.

## Agent execution protocol (obligatorio, del padre)

* Sólo `develop`; nunca `main` como baseline.
* Confirmar rama, HEAD, árbol y estado remoto antes de empezar.
* Confirmar que cada ruta/API referenciada existe antes de proponer cambios.
* Demostrar la brecha con evidencia; si no existe, no inventar trabajo.
* Distinguir medido de estimado y host real de fixture en todo reporte.
* Reportar archivos, evidencia, comandos y resultados exactos, límites y riesgos.

## Log

L1: "procede papi"

L2: Las cuatro hijas comparten el mismo bloque genérico de Acceptance criteria y las mismas Instrucciones de 6 pasos; el requisito real de cada una vive en su `## Objetivo`. Cuarta fase consecutiva con este patrón (F16–F19). Criterio: cada tarea se juzga contra su Objetivo + las specs de este documento.

L3: F19.2 dice explícitamente "solo si gap demostrado". Eso habilita un cierre audit-only legítimo si el baseline de F19.1 no muestra un hueco real; se documentará en vez de inventar un perfil nuevo.

L4: F19.1 entregó `docs/context-tool-baseline.md`. La revisión independiente refutó la conclusión central y con razón: no es que nada marque un tool como crítico — es que **los tools ya se eliminan por frecuencia**. `PRUNE_MIN_SESSIONS = 3` y `applyUsagePruning` recortan el set full por uso medido, aplicado automáticamente en `regulus.ts:242-249` salvo `TE_TOOLSET` explícito. La regla del padre estaba siendo violada, no ignorada.

L5: F19.2 (`9a45696`). `NON_EXPENDABLE_TOOLS` con seis superficies y su categoría; el filtro de pruning las conserva antes de mirar el uso. El test existente **pineaba la violación** (`expect(...).not.toContain("gate_run")`) y fue flipeado con la razón al lado. Comentario obsoleto de "all 16" corregido.

La mitad task-aware se cierra **audit-only**: no existe comparación Full-vs-Minimal en `internal/bench`, así que un tercer perfil sería inventado, no justificado.

L6: El primer Gauntlet de F19.2 falló por **deriva documental**: la frase "(3 tools: …)" en el baseline fue leída por `docdrift` como una afirmación sobre la superficie total. Corregido en `0a657b9` nombrando los tres sin número. Tercera vez en el programa que un guard del repo frena un cambio: `TestNoRawProcessLaunch`, el pin del dispatch y ahora `docdrift`.
